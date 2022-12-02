#!/bin/bash

set -e

kubectl create ns egressd || true
helm upgrade -i grafana-egressd grafana/grafana -n egressd -f ./grafana-values.yaml
helm upgrade -i loki-egressd grafana/loki-simple-scalable -n egressd -f ./loki-values.yaml
helm upgrade -i vector-egressd vector/vector -n egressd -f ./vector-values.yaml
kubectl apply -f egressd.yaml -n egressd
