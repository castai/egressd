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

TARGET_ARCH ?= x86_64

ifeq ($(TARGET_ARCH),x86_64)
   ARCH = x86_64
   LINUX_ARCH = x86
   GO_ARCH = amd64
endif

ifeq ($(TARGET_ARCH),aarch64)
   ARCH = arm64
   LINUX_ARCH = arm64
   GO_ARCH = arm64
endif

BPF_CFLAGS='-D__TARGET_ARCH_x86'

CMD_DOCKER_BUILDER=docker run --rm -it \
	-v $$(pwd)/.cache/go-build:/home/.cache/go-build \
	-v $$(pwd)/.cache/go-mod:/home/go/pkg/mod \
	-v $$(pwd):/app --privileged \
	-e BPF2GO_STRIP=llvm-strip-14 \
	-w /app ghcr.io/castai/egressd/egressd-builder:latest

OUTPUT_DIR=./bin
$(OUTPUT_DIR):
	@mkdir -p $@

$(OUTPUT_DIR)/builder-image: $(OUTPUT_DIR) \
#
	docker build -t ghcr.io/castai/egressd/egressd-builder:latest --build-arg TARGETARCH=$(GO_ARCH) . -f Dockerfile.builder
	touch $(OUTPUT_DIR)/builder-image

.PHONY: builder-image
builder-image: $(OUTPUT_DIR)/builder-image

.PHONY: gen-bpf
gen-bpf:
	BPF_CFLAGS='-D__TARGET_ARCH_x86' go generate ./ebpf

.PHONY: gen-bpf-docker
gen-bpf-docker: builder-image
	$(CMD_DOCKER_BUILDER) make gen-bpf

.PHONY: builder-image-enter
builder-image-enter: builder-image
	$(CMD_DOCKER_BUILDER)