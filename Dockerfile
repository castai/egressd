# Cilium uses bpftool to find supported ebpf maps features. Our bpftool image is statically linked.
ARG BPFTOOL_IMAGE=ghcr.io/castai/egressd/bpftool@sha256:a4cb5507957aa97aabd77005f57818def8d61d5125445dad0014799b63241643
FROM ${BPFTOOL_IMAGE} as bpftool-dist

FROM gcr.io/distroless/static-debian11
COPY --from=bpftool-dist /bin/bpftool /bin/bpftool
COPY ./bin/egressd /usr/local/bin/egressd
CMD ["/usr/local/bin/egressd"]
