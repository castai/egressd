ARG CILIUM_BPFTOOL_IMAGE=quay.io/cilium/cilium-bpftool:d3093f6aeefef8270306011109be623a7e80ad1b@sha256:2c28c64195dee20ab596d70a59a4597a11058333c6b35a99da32c339dcd7df56
ARG UBUNTU_IMAGE=docker.io/library/ubuntu:22.04@sha256:4b1d0c4a2d2aaf63b37111f34eb9fa89fa1bf53dd6e4ca954d47caebca4005c2

FROM ${CILIUM_BPFTOOL_IMAGE} as bpftool-dist

FROM ${UBUNTU_IMAGE}
# libelf is needed for bpftool. In the feature we may remove this dep by using low level ebpf maps. Currently we import cilium packages which uses bpftool.
RUN apt update && apt install -y libelf-dev
COPY --from=bpftool-dist /usr/local /usr/local
COPY ./bin/egressd /usr/local/bin/egressd
CMD ["/usr/local/bin/egressd"]
