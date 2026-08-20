#!/usr/bin/env bash
set -euo pipefail

# Helm Integration Test Setup
# Installs all chart releases needed for shape grouping integration tests.
# Requires: kind cluster running, helm installed.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="${SCRIPT_DIR}/../charts/kos-shape-fixture"

echo "=== Adding Helm repositories ==="
helm repo add grafana-community https://grafana-community.github.io/helm-charts 2>/dev/null || true
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx 2>/dev/null || true
helm repo add external-secrets https://charts.external-secrets.io 2>/dev/null || true
helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
helm repo update

echo ""
echo "=== Stage 1: Local fixture releases ==="
helm upgrade --install fixture-simple-a "${CHART_DIR}" \
  --namespace fixture-a --create-namespace \
  --set profile=simple --wait --timeout 60s

helm upgrade --install fixture-simple-b "${CHART_DIR}" \
  --namespace fixture-b --create-namespace \
  --set profile=simple --wait --timeout 60s

helm upgrade --install fixture-simple-c "${CHART_DIR}" \
  --namespace fixture-c --create-namespace \
  --set profile=rbac --wait --timeout 60s

helm upgrade --install fixture-stateful "${CHART_DIR}" \
  --namespace fixture-stateful --create-namespace \
  --set profile=stateful --wait --timeout 60s

helm upgrade --install fixture-adv-disconnected "${CHART_DIR}" \
  --namespace fixture-adv-disco --create-namespace \
  --set profile=adversarial-disconnected-service --wait --timeout 60s

helm upgrade --install fixture-adv-unmounted "${CHART_DIR}" \
  --namespace fixture-adv-unmounted --create-namespace \
  --set profile=adversarial-unmounted-config --wait --timeout 60s

echo ""
echo "=== Stage 2: Application charts ==="
helm upgrade --install grafana grafana-community/grafana \
  --namespace observability --create-namespace \
  --set persistence.enabled=false \
  --set service.type=ClusterIP \
  --wait --timeout 120s

helm upgrade --install kube-state-metrics prometheus-community/kube-state-metrics \
  --namespace observability \
  --wait --timeout 120s

helm upgrade --install node-exporter prometheus-community/prometheus-node-exporter \
  --namespace observability \
  --wait --timeout 120s

echo ""
echo "=== Stage 3: Operator/controller charts ==="
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-system --create-namespace \
  --set controller.service.type=ClusterIP \
  --wait --timeout 120s

helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true \
  --wait --timeout 180s

helm upgrade --install external-secrets external-secrets/external-secrets \
  --namespace external-secrets --create-namespace \
  --set installCRDs=true \
  --wait --timeout 180s

helm upgrade --install argocd argo/argo-cd \
  --namespace argocd --create-namespace \
  --set server.service.type=ClusterIP \
  --set "configs.secret.argocdServerAdminPassword=\$2a\$10\$placeholder" \
  --wait --timeout 180s

echo ""
echo "=== Setup complete: 11 releases installed ==="
helm list --all-namespaces
