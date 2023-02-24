# Cilium uses bpftool to find supported ebpf maps features. Our bpftool image is statically linked.
ARG BPFTOOL_IMAGE=ghcr.io/castai/egressd/bpftool@sha256:93f06b391f8e821cef06294bb70228313e11aeac181979109a4fe1a8a3379f7a
FROM ${BPFTOOL_IMAGE} as bpftool-dist

FROM gcr.io/distroless/static-debian11
COPY --from=bpftool-dist /bin/bpftool /bin/bpftool
COPY ./bin/egressd /usr/local/bin/egressd
CMD ["/usr/local/bin/egressd"]
