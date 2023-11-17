#!/bin/bash

set -ex

KIND_CONTEXT="${KIND_CONTEXT:-kind}"
GOARCH="${GOARCH:-amd64}"

if [ "$IMAGE_TAG" == "" ]
then
  echo "env variable IMAGE_TAG is required"
  exit 1
fi

name=egressd-$GOARCH
exporter_name=egressd-exporter-$GOARCH

# Build e2e docker image.
pushd ./e2e
GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -o ../bin/$name-e2e .
popd
docker build . -t $name-e2e:local --build-arg image_tag=$IMAGE_TAG -f Dockerfile.e2e

# Load e2e image into kind.
kind load docker-image $name-e2e:local --name $KIND_CONTEXT

if [ "$IMAGE_TAG" == "local" ]
then
  GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -o bin/$name ./cmd/collector
  docker build . -t $name:local -f Dockerfile
  kind load docker-image $name:local --name $KIND_CONTEXT

  GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -o bin/$exporter_name ./cmd/exporter
  docker build . -t $exporter_name:local -f Dockerfile.exporter
  kind load docker-image $exporter_name:local --name $KIND_CONTEXT
fi

# Deploy e2e resources.
function printJobLogs() {
  echo "Jobs:"
  kubectl get jobs -owide
  echo "Pods:"
  kubectl get pods -owide
  echo "E2E Job pods:"
  kubectl describe pod -l job-name=e2e
  echo "E2E Job logs:"
  kubectl logs -l job-name=e2e --tail=-1
  echo "Egressd pods:"
  kubectl describe pod -l app.kubernetes.io/name=egressd
  echo "Egressd logs:"
  kubectl logs -l app.kubernetes.io/name=egressd
  echo "Egressd aggregator logs:"
  kubectl logs -l app.kubernetes.io/name=egressd-aggregator
}
trap printJobLogs EXIT

ns="castai-egressd-e2e"
kubectl delete ns $ns --force || true
kubectl create ns $ns || true
kubectl config set-context --current --namespace=$ns
# Create job pod. It will install egressd helm chart inside the k8s.
kubectl apply -f ./e2e/e2e.yaml
# Make sure egressd is running.
for (( i=1; i<=20; i++ ))
do
    if eval kubectl get ds castai-egressd; then
        break
    fi
    sleep 1
done
# Deploy some basic http communication services. Now we should get some conntrack records.
kubectl apply -f ./e2e/conn-generator.yaml
echo "Waiting for job to finish"

i=0
sleep_seconds=5
retry_count=20
while true; do
  if [ "$i" == "$retry_count" ];
  then
    echo "Timeout waiting for job to complete"
    exit 1
  fi

  if kubectl wait --for=condition=complete --timeout=0s job/e2e 2>/dev/null; then
    job_result=0
    break
  fi

  if kubectl wait --for=condition=failed --timeout=0s job/e2e 2>/dev/null; then
    job_result=1
    break
  fi

  sleep $sleep_seconds
  echo "Job logs:"
  kubectl logs -l job-name=e2e --since=5s
  i=$((i+1))
done

if [[ $job_result -eq 1 ]]; then
    echo "Job failed!"
    exit 1
fi
echo "Job succeeded!"
