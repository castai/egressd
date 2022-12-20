#!/bin/bash

set -e

helm upgrade -i egressd ../../charts/egressd -n egressd --create-namespace -f ./egressd-values.yaml
helm upgrade -i grafana-egressd grafana/grafana -n egressd -f ./grafana-values.yaml
helm upgrade -i loki-egressd grafana/loki -n egressd -f ./loki-values.yaml
helm upgrade -i vector-egressd vector/vector -n egressd -f ./vector-values.yaml
helm upgrade -i prom prometheus --repo https://prometheus-community.github.io/helm-charts -n egressd -f ./prometheus-values.yaml