build:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/egressd .

build-docker: build
	DOCKER_DEFAULT_PLATFORM=linux/amd64 docker build -t ghcr.io/castai/egressd/egressd:$(IMAGE_TAG) -f Dockerfile .

push-docker: build-docker
	DOCKER_DEFAULT_PLATFORM=linux/amd64 docker push ghcr.io/castai/egressd/egressd:$(IMAGE_TAG)

lint:
	golangci-lint run -v --timeout=10m
