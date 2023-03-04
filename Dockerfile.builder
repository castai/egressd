FROM ubuntu:22.04

RUN set -eux; \
   apt-get update && \
   apt-get install -y wget git build-essential clang-14 dnsutils curl && \
   rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*


RUN wget https://golang.org/dl/go1.19.1.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.19.1.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"

CMD ["/bin/bash"]
