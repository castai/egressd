build:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/egressd .

build-docker: build
	DOCKER_DEFAULT_PLATFORM=linux/amd64 docker build -t ghcr.io/castai/egressd/egressd:$(IMAGE_TAG) -f Dockerfile .

push-docker: build-docker
	DOCKER_DEFAULT_PLATFORM=linux/amd64 docker push ghcr.io/castai/egressd/egressd:$(IMAGE_TAG)

lint:
	golangci-lint run -v --timeout=10m

gen-ebpf:
	BPF_CLANG='clang-14' BPF_CFLAGS='-D__TARGET_ARCH_x86' go generate ./ebpf/...

build-builder-image:
	docker build -t egressd-builder . -f Dockerfile.builder

build-linux: build-builder-image
	docker run --rm -it --platform linux/amd64 \
		-w /app \
	 	-v $(PWD):/app \
	 	-v $(PWD)/.cache/go-build:/root/.cache/go-build \
	 	-v $(PWD)/.cache/go-mod:/root/go/pkg/mod \
	 	-e CGO_ENABLED=0 \
	 	-e GOOS=linux \
	 	-e GOARCH=amd64 \
	 	-e BPF_CLANG='clang-14' \
	 	-e BPF_CFLAGS='-D__TARGET_ARCH_x86' \
	 	egressd-builder \
	 	/bin/bash -c 'go generate ./ebpf/... && go build -p 4 -installsuffix cgo -ldflags="-s -w -X main.Version=$(IMAGE_TAG)" -o ./bin/egressd .'