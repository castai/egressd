#!/bin/bash

set -e

# Install Grafana and Prometheus and Loki.
helm upgrade -i grafana grafana --repo https://grafana.github.io/helm-charts -n egressd --create-namespace -f ./grafana-values.yaml
helm upgrade -i loki loki --repo https://grafana.github.io/helm-charts -n egressd -f ./loki-values.yaml
helm upgrade -i prom prometheus --repo https://prometheus-community.github.io/helm-charts -n egressd -f ./prometheus-values.yaml

# Install egressd and vector aggregator.
helm upgrade -i egressd ../../charts/egressd -n egressd -f ./egressd-values.yaml
helm upgrade -i vector vector --repo https://helm.vector.dev -n egressd -f ./vector-values.yaml
