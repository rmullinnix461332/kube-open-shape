#!/usr/bin/env bash
set -euo pipefail

# Helm Integration Test Teardown
# Removes all releases and namespaces installed by setup.sh

echo "=== Removing Helm releases ==="
helm uninstall argocd --namespace argocd 2>/dev/null || true
helm uninstall external-secrets --namespace external-secrets 2>/dev/null || true
helm uninstall cert-manager --namespace cert-manager 2>/dev/null || true
helm uninstall ingress-nginx --namespace ingress-system 2>/dev/null || true
helm uninstall node-exporter --namespace observability 2>/dev/null || true
helm uninstall kube-state-metrics --namespace observability 2>/dev/null || true
helm uninstall grafana --namespace observability 2>/dev/null || true
helm uninstall fixture-adv-unmounted --namespace fixture-adv-unmounted 2>/dev/null || true
helm uninstall fixture-adv-disconnected --namespace fixture-adv-disco 2>/dev/null || true
helm uninstall fixture-stateful --namespace fixture-stateful 2>/dev/null || true
helm uninstall fixture-simple-c --namespace fixture-c 2>/dev/null || true
helm uninstall fixture-simple-b --namespace fixture-b 2>/dev/null || true
helm uninstall fixture-simple-a --namespace fixture-a 2>/dev/null || true

echo ""
echo "=== Removing namespaces ==="
kubectl delete namespace fixture-a fixture-b fixture-c fixture-stateful \
  fixture-adv-disco fixture-adv-unmounted \
  observability ingress-system cert-manager external-secrets argocd \
  --ignore-not-found --wait=false

echo ""
echo "=== Teardown complete ==="
