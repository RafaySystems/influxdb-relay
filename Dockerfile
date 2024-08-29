
# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.17.6 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY default.conf relay.conf

# setup github user and token
ARG BUILD_USR="nothing"
ARG BUILD_PWD="nothing"

# Create the user and group files that will be used in the running 
# container to run the process as an unprivileged user.
RUN mkdir /user && \
    echo 'nobody:x:65534:65534:nobody:/:' > /user/passwd && \
    echo 'nobody:x:65534:' > /user/group

# Create a netrc file using the credentials specified using --build-arg
RUN printf "machine github.com\n\
    login ${BUILD_USR}\n\
    password ${BUILD_PWD}\n\
    \n\
    machine api.github.com\n\
    login ${BUILD_USR}\n\
    password ${BUILD_PWD}\n"\
    >> /root/.netrc
RUN chmod 600 /root/.netrc

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN GOPRIVATE=github.com/RafaySystems/* go mod download

# Copy the go source
COPY main.go main.go
COPY relay/ relay/
COPY relayservice/ relayservice/
COPY config/ config/


# Build
ARG TARGETARCH TARGETOS
RUN GOPRIVATE=github.com/RafaySystems/* CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GO111MODULE=on go build -ldflags "-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn" -a -o influxdb-relay main.go


# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:latest
## use base:debug for shell access
FROM gcr.io/distroless/static:latest
WORKDIR /
COPY --from=builder /workspace/influxdb-relay .
#COPY --from=builder /workspace/relay.conf /etc/influxdb-relay/influxdb-relay.conf
ENTRYPOINT ["/influxdb-relay"]

CMD ["-config", "/etc/influxdb-relay/influxdb-relay.conf" ]

