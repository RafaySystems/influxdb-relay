package relay

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/influxdata/influxdb/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type status struct {
	Status map[string]stats `json:"status"`
}

func (h *HTTP) handleStatus(w http.ResponseWriter, r *http.Request, _ time.Time) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		st := status{Status: make(map[string]stats)}

		for _, b := range h.backends {
			st.Status[b.name] = b.poster.getStats()
		}

		jsonResponse(w, response{http.StatusOK, st})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusOK)).Inc()
	} else {
		jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusMethodNotAllowed)).Inc()
		return
	}
}

func (h *HTTP) handlePing(w http.ResponseWriter, r *http.Request, _ time.Time) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		for key, value := range h.pingResponseHeaders {
			w.Header().Add(key, value)
		}
		w.WriteHeader(h.pingResponseCode)
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusOK)).Inc()
	} else {
		jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusMethodNotAllowed)).Inc()
		return
	}
}

type health struct {
	name     string
	err      error
	duration time.Duration
}

type healthReport struct {
	Status  string            `json:"status"`
	Healthy map[string]string `json:"healthy,omitempty"`
	Problem map[string]string `json:"problem,omitempty"`
}

func (h *HTTP) handleHealth(w http.ResponseWriter, r *http.Request, _ time.Time) {
	var responses = make(chan health, len(h.backends))
	var wg sync.WaitGroup
	var validEndpoints = 0
	wg.Add(len(h.backends))

	for _, b := range h.backends {
		b := b

		validEndpoints++

		go func() {
			defer wg.Done()

			var healthCheck = health{name: b.name, err: nil}

			client := http.Client{
				Timeout: h.healthTimeout,
			}
			start := time.Now()
			res, err := client.Get(b.location + b.endpoints.Ping)

			if err != nil {
				if h.log {
					h.logger.Println(err)
				}
				healthCheck.err = err
				responses <- healthCheck
				return
			}
			if res.StatusCode/100 != 2 {
				//healthCheck.err = errors.New("Unexpected error code " + strconv.FormatInt(int64(res.StatusCode), 10))
				healthCheck.err = errors.New("Unexpected error code " + strconv.Itoa(res.StatusCode))
			}
			healthCheck.duration = time.Since(start)
			responses <- healthCheck
			return
		}()
	}

	go func() {
		wg.Wait()
		close(responses)
	}()

	nbDown := 0
	report := healthReport{}
	for r := range responses {
		if r.err == nil {
			if report.Healthy == nil {
				report.Healthy = make(map[string]string)
			}
			report.Healthy[r.name] = "OK. Time taken " + r.duration.String()

		} else {
			if report.Problem == nil {
				report.Problem = make(map[string]string)
			}
			report.Problem[r.name] = "KO. " + r.err.Error()
			nbDown++
		}
	}
	switch {
	case nbDown == validEndpoints:
		report.Status = "critical"
	case nbDown >= 1:
		report.Status = "problem"
	case nbDown == 0:
		report.Status = "healthy"
	}
	response := response{code: 200, body: report}
	jsonResponse(w, response)
	httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusOK)).Inc()
	return
}

func (h *HTTP) handleAdmin(w http.ResponseWriter, r *http.Request, _ time.Time) {
	// Client to perform the raw queries
	client := http.Client{}

	// Base body for all requests
	baseBody := bytes.Buffer{}
	_, err := baseBody.ReadFrom(r.Body)
	if err != nil {
		log.Printf("relay %q: could not read body: %v", h.Name(), err)
		return
	}

	if r.Method != http.MethodPost {
		// Bad method
		w.Header().Set("Allow", http.MethodPost)
		jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusMethodNotAllowed)).Inc()
		return
	}

	// Responses
	var responses = make(chan *http.Response, len(h.backends))

	// Associated waitgroup
	var wg sync.WaitGroup
	wg.Add(len(h.backends))

	// Iterate over all backends
	for _, b := range h.backends {
		b := b

		go func() {
			defer wg.Done()

			bodyBytes := baseBody

			// Create new request
			// Update location according to backend
			// Forward body
			req, err := http.NewRequest("POST", b.location+b.endpoints.Query, &bodyBytes)
			if err != nil {
				log.Printf("problem posting to relay %q backend %q: could not prepare request: %v", h.Name(), b.name, err)
				responses <- &http.Response{}
				return
			}

			// Forward headers
			req.Header = r.Header

			// Forward the request
			resp, err := client.Do(req)
			if err != nil {
				// Internal error
				log.Printf("problem posting to relay %q backend %q: %v", h.Name(), b.name, err)

				// So empty response
				responses <- &http.Response{}
			} else {
				if resp.StatusCode/100 == 5 {
					// HTTP error
					log.Printf("5xx response for relay %q backend %q: %v", h.Name(), b.name, resp.StatusCode)
				}

				// Get response
				responses <- resp
			}
		}()
	}

	// Wait for requests
	go func() {
		wg.Wait()
		close(responses)
	}()

	var errResponse *responseData
	for resp := range responses {
		switch resp.StatusCode / 100 {
		case 2:
			w.WriteHeader(http.StatusNoContent)
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusNoContent)).Inc()
			return

		case 4:
			// User error
			resp.Write(w)
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusBadRequest)).Inc()
			return

		default:
			// Hold on to one of the responses to return back to the client
			errResponse = nil
		}
	}

	// No successful writes
	if errResponse == nil {
		// Failed to make any valid request...
		jsonResponse(w, response{http.StatusServiceUnavailable, "unable to forward query"})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusServiceUnavailable)).Inc()
		return
	}
}

func (h *HTTP) handleFlush(w http.ResponseWriter, r *http.Request, start time.Time) {
	if h.log {
		h.logger.Println("Flushing buffers...")
	}

	for _, b := range h.backends {
		r := b.getRetryBuffer()

		if r != nil {
			if h.log {
				h.logger.Println("Flushing " + b.name)
			} else {
				h.logger.Println("NOT flushing " + b.name + " (is empty)")
			}

			r.empty()
		}
	}

	jsonResponse(w, response{http.StatusOK, http.StatusText(http.StatusOK)})
	httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusOK)).Inc()
}

func (h *HTTP) handleStandard(w http.ResponseWriter, r *http.Request, start time.Time) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
		} else {
			jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusMethodNotAllowed)).Inc()
			return
		}
	}

	queryParams := r.URL.Query()
	bodyBuf := getBuf()
	_, _ = bodyBuf.ReadFrom(r.Body)

	precision := queryParams.Get("precision")
	points, err := models.ParsePointsWithPrecision(bodyBuf.Bytes(), start, precision)
	if err != nil {
		putBuf(bodyBuf)
		log.Printf("parse points error: %s", err)
		jsonResponse(w, response{http.StatusBadRequest, "unable to parse points"})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusBadRequest)).Inc()
		return
	}

	outBuf := getBuf()
	for _, p := range points {
		// Those two functions never return any errors, let's just ignore the return value
		_, _ = outBuf.WriteString(p.PrecisionString(precision))
		_ = outBuf.WriteByte('\n')
	}

	// done with the input points
	putBuf(bodyBuf)

	// normalize query string
	query := queryParams.Encode()

	outBytes := outBuf.Bytes()

	// check for authorization performed via the header
	authHeader := r.Header.Get("Authorization")

	var wg sync.WaitGroup
	wg.Add(len(h.backends))

	var responses = make(chan *responseData, len(h.backends))

	for _, b := range h.backends {
		b := b

		// Don't do the request if the tags do not match the filters
		err := b.validateRegexps(points)
		if err != nil {
			if h.log {
				h.logger.Printf("request invalidated by regular expression for backend: %s", b.name)
				h.logger.Printf(err.Error())
			}

			wg.Done()
			continue
		}

		go func() {
			defer wg.Done()
			resp, err := b.post(outBytes, query, authHeader, b.endpoints.Write)
			if err != nil {
				log.Printf("Problem posting to relay %q backend %q: %v", h.Name(), b.name, err)
				if h.log {
					h.logger.Printf("Content: %s", bodyBuf.String())
				}

				responses <- &responseData{}
			} else {
				if resp.StatusCode/100 == 5 {
					log.Printf("5xx response for relay %q backend %q: %v", h.Name(), b.name, resp.StatusCode)
				}
				responses <- resp
			}
		}()
	}

	go func() {
		wg.Wait()
		close(responses)
		putBuf(outBuf)
	}()

	var errResponse *responseData

	w.Header().Set("Content-Type", "text/plain")

	for resp := range responses {

		switch resp.StatusCode / 100 {
		case 2:
			// Status accepted means buffering,
			if resp.StatusCode == http.StatusAccepted {
				if h.log {
					h.logger.Printf("could not reach relay %q, buffering...", h.Name())
				}

				w.WriteHeader(http.StatusAccepted)
				httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusAccepted)).Inc()
				return
			}

			w.WriteHeader(http.StatusNoContent)
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusNoContent)).Inc()
			return

		case 4:
			// User error
			resp.Write(w)
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusBadRequest)).Inc()
			return

		default:
			// Hold on to one of the responses to return back to the client
			errResponse = nil
		}
	}

	// No successful writes
	if errResponse == nil {
		// Failed to make any valid request...
		jsonResponse(w, response{http.StatusServiceUnavailable, "unable to write points"})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusServiceUnavailable)).Inc()
		return
	}
}

func (h *HTTP) handleProm(w http.ResponseWriter, r *http.Request, _ time.Time) {

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
		} else {
			jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusMethodNotAllowed)).Inc()
			return
		}
	}

	authHeader := r.Header.Get("Authorization")

	bodyBuf := getBuf()
	_, _ = bodyBuf.ReadFrom(r.Body)

	outBytes := bodyBuf.Bytes()

	respBuf := getBuf()

	var responses = make(chan *responseData, len(h.backends))

	params := r.URL.Query()
	urlType := "pod-node"
	if params["type"] != nil {
		urlType = params["type"][0]
	}

	if !backendTypes[urlType] {
		urlType = "pod-node"
	}

	targetBackends := h.typedBackends[urlType]

	var wg sync.WaitGroup
	wg.Add(len(targetBackends))

	for _, b := range targetBackends {
		b := b

		go func() {
			defer wg.Done()
			resp, err := b.post(outBytes, r.URL.RawQuery, authHeader, b.endpoints.PromWrite)
			if err != nil {
				log.Printf("problem posting to relay %q backend %q: %v", h.Name(), b.name, err)

				responses <- &responseData{}
			} else {
				if resp.StatusCode >= 400 {
					log.Printf("5xx/4xx response for relay %q backend %q: %v", h.Name(), b.name, resp.StatusCode)
					if resp.Body == nil {
						log.Printf("Empty body in request for influx %q write", b.name)
					} else {
						log.Printf("5xx/4xx url: %q response: %q", r.URL.String(), string(resp.Body))
					}
				}
				responses <- resp
			}
		}()
	}

	go func() {
		wg.Wait()
		close(responses)
		putBuf(bodyBuf)
		putBuf(respBuf)
	}()

	var errResponse *responseData

	w.Header().Set("Content-Type", "text/plain")

	for resp := range responses {

		switch resp.StatusCode / 100 {
		case 2:
			// Status accepted means buffering,
			if resp.StatusCode == http.StatusAccepted {
				if h.log {
					h.logger.Printf("could not reach relay %q, buffering...", h.Name())
				}
				w.WriteHeader(http.StatusAccepted)
				httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusAccepted)).Inc()
				return
			}

			w.WriteHeader(http.StatusNoContent)
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusNoContent)).Inc()
			return

		case 4:
			// User error
			resp.Write(w)
			httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusBadRequest)).Inc()
			return

		default:
			// Hold on to one of the responses to return back to the client
			errResponse = nil
		}
	}

	// No successful writes
	if errResponse == nil {
		// Failed to make any valid request...
		jsonResponse(w, response{http.StatusServiceUnavailable, "unable to write points"})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusServiceUnavailable)).Inc()
		return
	}
}

type bufferSizeRequestsCollector struct {
	currentBufferSize       int
	maxBufferSize           int
	currentBufferSizeMetric *prometheus.Desc
	maxBufferSizeMetric     *prometheus.Desc
}

func newBufferSizeRequestsCollector(name string) *bufferSizeRequestsCollector {
	collector := &bufferSizeRequestsCollector{
		currentBufferSizeMetric: prometheus.NewDesc(
			"influxrelay_current_buffer_size",
			"Current Influx relay buffer size",
			nil,
			prometheus.Labels{"dbname": name},
		),
		maxBufferSizeMetric: prometheus.NewDesc(
			"influxrelay_max_buffer_size",
			"Maximum Influx relay buffer size",
			nil,
			prometheus.Labels{"dbname": name},
		),
	}
	prometheus.MustRegister(collector)
	return collector
}

func (collector *bufferSizeRequestsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.currentBufferSizeMetric
	ch <- collector.maxBufferSizeMetric
}

func (collector *bufferSizeRequestsCollector) Collect(ch chan<- prometheus.Metric) {
	currentBufferSizeMetric := prometheus.MustNewConstMetric(collector.currentBufferSizeMetric, prometheus.GaugeValue, float64(collector.currentBufferSize))
	maxBufferSizeMetric := prometheus.MustNewConstMetric(collector.maxBufferSizeMetric, prometheus.GaugeValue, float64(collector.maxBufferSize))
	ch <- currentBufferSizeMetric
	ch <- maxBufferSizeMetric
}

func (h *HTTP) handleMetrics(w http.ResponseWriter, r *http.Request, _ time.Time) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		promhttp.Handler().ServeHTTP(w, r)
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusOK)).Inc()
	} else {
		jsonResponse(w, response{http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed)})
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(http.StatusMethodNotAllowed)).Inc()
		return
	}
}
