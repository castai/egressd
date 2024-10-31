#!/bin/bash

set -e

# Default variables for components and versions
EGRESSD_REPO="https://castai.github.io/helm-charts"
GRAFANA_REPO="https://grafana.github.io/helm-charts"
VICTORIA_REPO="https://victoriametrics.github.io/helm-charts"
EGRESSD_NAME="egressd"
GRAFANA_NAME="egressd-grafana"
VICTORIA_NAME="egressd-victoria"
DEFAULT_NAMESPACE="egressd"
GRAFANA_VERSION="6.50.7"
VICTORIA_VERSION="0.8.58"

# Function to print components and versions
print_components() {
  echo "Components to be installed:"
  echo "1. $EGRESSD_NAME from $EGRESSD_REPO"
  echo "2. $GRAFANA_NAME from $GRAFANA_REPO, version $GRAFANA_VERSION"
  echo "3. $VICTORIA_NAME from $VICTORIA_REPO, version $VICTORIA_VERSION"
}

# Function to install components
install_components() {
  print_components
  # Install egressd
  helm upgrade --install --repo $EGRESSD_REPO $EGRESSD_NAME $EGRESSD_NAME -n $NAMESPACE --create-namespace -f ./hack/egressd-demo-values.yaml

  # Install grafana and victoria metrics
  helm upgrade --install --repo $GRAFANA_REPO $GRAFANA_NAME grafana --version $GRAFANA_VERSION -n $NAMESPACE -f ./hack/grafana-demo-values.yaml
  helm upgrade --install --repo $VICTORIA_REPO $VICTORIA_NAME victoria-metrics-single --version $VICTORIA_VERSION -n $NAMESPACE -f ./hack/victoria-demo-values.yaml
}

# Function to uninstall components and delete namespace
uninstall_components() {
  echo "Uninstalling components and deleting namespace $NAMESPACE..."
  helm uninstall $EGRESSD_NAME -n $NAMESPACE || true
  helm uninstall $GRAFANA_NAME -n $NAMESPACE || true
  helm uninstall $VICTORIA_NAME -n $NAMESPACE || true
  kubectl delete namespace $NAMESPACE || true
  echo "Uninstallation completed."
}

# Default namespace
NAMESPACE=$DEFAULT_NAMESPACE

# Check parameters
while [[ $# -gt 0 ]]; do
  case $1 in
    --uninstall)
      UNINSTALL=true
      shift
      ;;
    --namespace)
      NAMESPACE=$2
      shift 2
      ;;
    *)
      echo "Unknown parameter $1"
      exit 1
      ;;
  esac
done

# Execute based on parameters
if [[ $UNINSTALL == true ]]; then
  uninstall_components
else
  install_components
fi
