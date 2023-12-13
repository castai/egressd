# Cilium uses bpftool to find supported ebpf maps features. Our bpftool image is statically linked.
ARG BPFTOOL_IMAGE=ghcr.io/castai/egressd/bpftool@sha256:d2cf7a30c598e1b39c8b04660d6f1f9ab0925af2951c09216d87eb0d3de0f27b
FROM ${BPFTOOL_IMAGE} as bpftool-dist

FROM gcr.io/distroless/static-debian11:latest
ARG TARGETARCH
COPY --from=bpftool-dist /bin/bpftool /bin/bpftool
COPY ./bin/egressd-$TARGETARCH /usr/local/bin/egressd
ENTRYPOINT ["/usr/local/bin/egressd"]
