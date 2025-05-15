#!/bin/bash
set -e

CYAN='\033[0;36m'
NC='\033[0m'
function log() { echo -e "${CYAN}${1}${NC}"; }

script_dir="$(cd "$(dirname "$0")" >/dev/null 2>&1 && pwd -P)"
project_dir=$(dirname "$script_dir")

log "[~] Delete existed cluster"
kind delete cluster

log "[~] Create new cluster"
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF

# If within devcontainers
if [ "$REMOTE_CONTAINERS" = "true" ]; then
  log "[~] Modify kube config hosts for devcontainer"
  sed -i -e "s/localhost/host.docker.internal/g" $HOME/.kube/config
  sed -i -e "s/127.0.0.1/host.docker.internal/g" $HOME/.kube/config
fi

log "[~] Modify kube config"
yq e '(.clusters[] | select(.name == "kind-kind").cluster.insecure-skip-tls-verify) = true' -i ~/.kube/config
yq e 'del(.clusters[] | select(.name == "kind-kind").cluster."certificate-authority-data")' -i ~/.kube/config

log "[~] Check connectivity"
kubectl cluster-info --context kind-kind"

log "[.] Prepared"

log "[~] Run 'make manifests crds.install' to generate and apply CRDs"