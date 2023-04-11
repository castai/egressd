.PHONY: build
build:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/egressd .

.PHONY: build-docker
build-docker: build
	DOCKER_DEFAULT_PLATFORM=linux/amd64 docker build -t ghcr.io/castai/egressd/egressd:$(IMAGE_TAG) -f Dockerfile .

.PHONY: push-docker
push-docker: build-docker
	DOCKER_DEFAULT_PLATFORM=linux/amd64 docker push ghcr.io/castai/egressd/egressd:$(IMAGE_TAG)

.PHONY: lint
lint:
	golangci-lint run -v --timeout=10m

.PHONY: gen-proto
gen-proto:
	protoc pb/metrics.proto --go_out=paths=source_relative:.