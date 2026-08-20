#!/usr/bin/env bash
set -euo pipefail

# Run KOS CLI commands against the live cluster and capture output for review.
# Prerequisites: kind cluster running, fixture charts installed (setup.sh), kos built (make build)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="${SCRIPT_DIR}/../.."
KOS="${ROOT_DIR}/bin/kos"
OUT="${ROOT_DIR}/test-results-helm.txt"

if [[ ! -x "${KOS}" ]]; then
  echo "ERROR: kos binary not found at ${KOS}. Run 'make build' first."
  exit 1
fi

echo "Writing results to ${OUT}"
echo ""

{
echo "============================================"
echo "KOS Helm Integration CLI Review"
echo "Date: $(date)"
echo "============================================"
echo ""

echo "=== Resources: fixture-a (simple) ==="
${KOS} resources --namespace fixture-a 2>&1
echo ""

echo "=== Resources: fixture-b (simple) ==="
${KOS} resources --namespace fixture-b 2>&1
echo ""

echo "=== Resources: fixture-c (rbac) ==="
${KOS} resources --namespace fixture-c 2>&1
echo ""

echo "=== Resources: fixture-stateful ==="
${KOS} resources --namespace fixture-stateful 2>&1
echo ""

echo "=== Ownership: fixture-a ==="
${KOS} ownership --namespace fixture-a 2>&1
echo ""

echo "=== Ownership: fixture-c ==="
${KOS} ownership --namespace fixture-c 2>&1
echo ""

echo "=== Ownership: fixture-stateful ==="
${KOS} ownership --namespace fixture-stateful 2>&1
echo ""

echo "=== Ownership Summary ==="
${KOS} ownership --summary 2>&1
echo ""

echo "=== Relationships: fixture-a Deployment ==="
${KOS} relationships Deployment fixture-simple-a -n fixture-a 2>&1
echo ""

echo "=== Relationships: fixture-a Service ==="
${KOS} relationships Service fixture-simple-a -n fixture-a 2>&1
echo ""

echo "=== Relationships: fixture-c Deployment (RBAC) ==="
${KOS} relationships Deployment fixture-simple-c -n fixture-c 2>&1
echo ""

echo "=== Relationships: fixture-c RoleBinding ==="
${KOS} relationships RoleBinding fixture-simple-c -n fixture-c 2>&1
echo ""

echo "=== Relationships: fixture-stateful StatefulSet ==="
${KOS} relationships StatefulSet fixture-stateful -n fixture-stateful 2>&1
echo ""

echo "=== Shapes ==="
${KOS} shapes 2>&1
echo ""

echo "=== Candidates (deterministic ordering) ==="
${KOS} candidates 2>&1
echo ""

echo "=== Candidates Explain (first) ==="
${KOS} candidates explain --first 2>&1
echo ""

echo "=== Candidates Generate (first) ==="
${KOS} candidates generate --first 2>&1
echo ""

echo "=== Candidates Test (first) ==="
${KOS} candidates test --first 2>&1
echo ""

echo "=== Relationships: adversarial-disconnected-service ==="
${KOS} relationships Deployment fixture-adv-disconnected -n fixture-adv-disco 2>&1
echo ""

echo "=== Relationships: adversarial-unmounted-config ==="
${KOS} relationships Deployment fixture-adv-unmounted -n fixture-adv-unmounted 2>&1
echo ""

echo "=== Report ==="
${KOS} report 2>&1
echo ""

echo "=== Stage 2: Grafana Relationships ==="
${KOS} relationships Deployment grafana -n observability 2>&1
echo ""

echo "=== Stage 2: kube-state-metrics Relationships ==="
${KOS} relationships Deployment kube-state-metrics -n observability 2>&1
echo ""

echo "=== Stage 3: cert-manager Relationships ==="
${KOS} relationships -n cert-manager 2>&1
echo ""

echo "=== Stage 3: argocd Relationships ==="
${KOS} relationships -n argocd 2>&1
echo ""

echo "=== Janitor Findings ==="
${KOS} findings 2>&1
echo ""

echo "=== Janitor Rules ==="
${KOS} rules 2>&1
echo ""

echo "=== Graph Export Summary ==="
${KOS} graph export --cluster-id kind-kos-shapes 2>&1 | python3 -c "
import json, sys
data = json.load(sys.stdin)
print(json.dumps(data.get('summary', {}), indent=2))
" 2>/dev/null || ${KOS} graph export --cluster-id kind-kos-shapes 2>&1 | head -20
echo ""

echo "============================================"
echo "Done"
echo "============================================"

} > "${OUT}" 2>&1

echo "Results written to: ${OUT}"
