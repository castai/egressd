FROM ubuntu:jammy

RUN export DEBIAN_FRONTEND=noninteractive && \
    apt-get update && \
    apt-get install -y sudo coreutils findutils && \
    apt-get install -y bash git curl rsync && \
    apt-get install -y build-essential clang-14 make pkg-config && \
    apt-get install -y linux-headers-generic && \
    apt-get install -y libelf-dev && \
    apt-get install -y zlib1g-dev && \
    update-alternatives --install /usr/bin/clang clang /usr/bin/clang-14 140 --slave /usr/bin/clang++ clang++ /usr/bin/clang++-14 --slave /usr/bin/llc llc /usr/bin/llc-14 --slave /usr/bin/clang-format clang-format /usr/bin/clang-format-14 --slave /usr/bin/clangd clangd /usr/bin/clangd-14

ARG TARGETARCH
RUN export DEBIAN_FRONTEND=noninteractive && \
    curl -L -o /tmp/golang.tar.xz https://go.dev/dl/go1.19.5.linux-$TARGETARCH.tar.gz && \
    tar -C /usr/local -xzf /tmp/golang.tar.xz && \
    update-alternatives --install /usr/bin/go go /usr/local/go/bin/go 1

RUN export PATH=$PATH:/usr/bin
ENV HOME /home
WORKDIR /home/app
