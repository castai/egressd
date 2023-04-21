#!/bin/bash

set -e

# Install egressd
helm upgrade --install --repo https://castai.github.io/helm-charts egressd egressd -n egressd --create-namespace -f ./hack/egressd-demo-values.yaml

# Install grafana and victoria metrics.
helm upgrade --install --repo https://grafana.github.io/helm-charts egressd-grafana grafana --version 6.50.7 -n egressd -f ./hack/grafana-demo-values.yaml
helm upgrade --install --repo https://victoriametrics.github.io/helm-charts egressd-victoria victoria-metrics-single --version 0.8.58 -n egressd -f ./hack/victoria-demo-values.yaml
