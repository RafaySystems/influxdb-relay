
IMAGE := $(shell basename `pwd`)

IMG ?= influxdb-relay:latest
TS := $(shell /bin/date "+%Y%m%d%H%M%S")
DEV_USER ?= dev
DEV_TAG := registry.dev.rafay-edge.net:5000/${DEV_USER}/influxdb-relay:$(TS)


.PHONY: docker-image image-nobase test-images docker-build test fast-test push fast-push retag fast-retag
docker-image: image-nobase
image-nobase: $(IMAGE)
test-images:
	-@:

.PHONY: $(IMAGE)


test: docker-image test-images


fast-test:


push: docker-image


fast-push:


#retag 'docker-image' with current time stamp
retag: docker-image

fast-retag:


# Not part of infra image
.PHONY: list-images
list-images:
	@:

.PHONY: clean
clean:

build:
	docker build . -t ${IMG} --build-arg BUILD_USR=${BUILD_USER} --build-arg BUILD_PWD=${BUILD_PASSWORD}

tag-dev:
	docker tag ${IMG} $(DEV_TAG)
	docker push $(DEV_TAG)

build-dev:
	rm -rf influxdb-relay.*
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -ldflags "-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn" -o influxdb-relay.dev main.go
	upx -5 -o influxdb-relay.upx influxdb-relay.dev	
	docker build -f Dockerfile.dev -t ${IMG} .
	$(MAKE) tag-dev


