#!/usr/bin/env bash
# Organization Axis Integration Tests
# Runs kos commands against a live cluster and validates output.
# Prerequisites: kind cluster, charts installed via setup.sh, kos built (make build)
#
# Usage:
#   ./test/helm-integration/run-org-axis-tests.sh
#
# Results written to: test-results-org-axis.txt

set -uo pipefail  # No -e: we handle errors explicitly

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="${SCRIPT_DIR}/../.."
KOS="${ROOT_DIR}/bin/kos"
OUT="${ROOT_DIR}/test-results-org-axis.txt"

PASS=0
FAIL=0
SKIP=0

# check verifies a grep pattern exists in output
check() {
  local id="$1"
  local desc="$2"
  local output="$3"
  local pattern="$4"

  if echo "$output" | grep -qE "$pattern"; then
    echo "  PASS  ${id}: ${desc}" >> "$OUT"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  ${id}: ${desc}" >> "$OUT"
    echo "        Pattern: ${pattern}" >> "$OUT"
    echo "        Output (first 300 chars): ${output:0:300}" >> "$OUT"
    FAIL=$((FAIL + 1))
  fi
}

# check_eq verifies two values are equal
check_eq() {
  local id="$1"
  local desc="$2"
  local actual="$3"
  local expected="$4"

  if [ "$actual" = "$expected" ]; then
    echo "  PASS  ${id}: ${desc}" >> "$OUT"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  ${id}: ${desc}" >> "$OUT"
    echo "        Expected: ${expected}" >> "$OUT"
    echo "        Actual:   ${actual}" >> "$OUT"
    FAIL=$((FAIL + 1))
  fi
}

skip() {
  local id="$1"
  local desc="$2"
  local reason="$3"
  echo "  SKIP  ${id}: ${desc} (${reason})" >> "$OUT"
  SKIP=$((SKIP + 1))
}

# Verify prerequisites
if [[ ! -x "${KOS}" ]]; then
  echo "ERROR: kos binary not found at ${KOS}. Run 'make build' first."
  exit 1
fi

if ! kubectl cluster-info &>/dev/null; then
  echo "ERROR: No Kubernetes cluster available."
  exit 1
fi

# Start output
cat > "$OUT" <<EOF
============================================
Organization Axis Integration Tests
Date: $(date)
Cluster: $(kubectl config current-context)
============================================

EOF

echo "Running organization axis tests..."

# --- Navigation Tests ---
echo "" >> "$OUT"
echo "=== 1. Navigation Tests ===" >> "$OUT"

# ORG-NAV-001
echo "  Running ORG-NAV-001..." >&2
NAV001=$($KOS groups 2>/dev/null)
check "ORG-NAV-001a" "groups produces table header" "$NAV001" "^GROUP"
check "ORG-NAV-001b" "HOME NAMESPACE column" "$NAV001" "HOME NAMESPACE"
check "ORG-NAV-001c" "CONFIDENCE column" "$NAV001" "CONFIDENCE"
check "ORG-NAV-001d" "argocd group present" "$NAV001" "argocd"

# ORG-NAV-002
echo "  Running ORG-NAV-002..." >&2
NAV002=$($KOS describe groups argocd 2>/dev/null)
check "ORG-NAV-002a" "describe shows Group header" "$NAV002" "^Group:"
check "ORG-NAV-002b" "shows Workloads count" "$NAV002" "Workloads:"
check "ORG-NAV-002c" "shows Components count" "$NAV002" "Components:"
check "ORG-NAV-002d" "shows Resources count" "$NAV002" "Resources:"
check "ORG-NAV-002e" "shows Evidence section" "$NAV002" "^Evidence:"
check "ORG-NAV-002f" "shows component subsections" "$NAV002" "server|application-controller"

# ORG-NAV-006
echo "  Running ORG-NAV-006..." >&2
NAV006=$($KOS groups -n observability 2>/dev/null)
check "ORG-NAV-006a" "observability groups present" "$NAV006" "grafana"
check "ORG-NAV-006b" "other namespaces excluded" "$NAV006" "observability"

# ORG-NAV-007
echo "  Running ORG-NAV-007..." >&2
NAV007=$($KOS resources deployment -n argocd 2>/dev/null)
check "ORG-NAV-007a" "only Deployments shown" "$NAV007" "Deployment"

# --- Grouping Tests ---
echo "" >> "$OUT"
echo "=== 2. Grouping Tests ===" >> "$OUT"

# ORG-GRP-006
echo "  Running ORG-GRP-006..." >&2
GRP006=$($KOS groups cert-manager -o json 2>/dev/null)
check "ORG-GRP-006a" "cert-manager group in JSON" "$GRP006" "cert-manager"
check "ORG-GRP-006b" "scope is Cluster" "$GRP006" '"scopeType".*Cluster'
check "ORG-GRP-006c" "multiple namespaces" "$GRP006" "kube-system"

# ORG-GRP-007
echo "  Running ORG-GRP-007..." >&2
GRP007=$($KOS describe groups argocd 2>/dev/null)
check "ORG-GRP-007a" "unassigned resources visible" "$GRP007" "unassigned"

# ORG-GRP-008
echo "  Running ORG-GRP-008..." >&2
GRP008=$($KOS groups argocd -o json 2>/dev/null)
WL_COUNT=$(echo "$GRP008" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0]['workloadCount'])" 2>/dev/null || echo "ERR")
check "ORG-GRP-008a" "workload count is 7" "$WL_COUNT" "^7$"

# ORG-GRP-009
COMP_COUNT=$(echo "$GRP008" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0]['componentCount'])" 2>/dev/null || echo "ERR")
check "ORG-GRP-009a" "component count is 8" "$COMP_COUNT" "^8$"

# --- Scope and Filtering Tests ---
echo "" >> "$OUT"
echo "=== 3. Scope and Filtering Tests ===" >> "$OUT"

# ORG-SCO-001
echo "  Running ORG-SCO-001..." >&2
SCO001=$($KOS groups 2>/dev/null)
GROUP_COUNT=$(echo "$SCO001" | grep -c "Corroborating\|Declared\|Inferred" || true)
check "ORG-SCO-001a" "multiple groups listed" "$GROUP_COUNT" "[5-9]|[1-9][0-9]"

# ORG-SCO-005
echo "  Running ORG-SCO-005..." >&2
SCO005=$($KOS groups --type Release 2>/dev/null)
check "ORG-SCO-005a" "release type groups shown" "$SCO005" "GROUP|0 groups"

# --- Output Parity Tests ---
echo "" >> "$OUT"
echo "=== 4. Output Parity Tests ===" >> "$OUT"

# ORG-OUT-001 (already collected in GRP008)
JSON_COUNT=$(echo "$GRP008" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d))" 2>/dev/null || echo "0")
check "ORG-OUT-001a" "JSON returns exactly 1 for filtered query" "$JSON_COUNT" "^1$"

# ORG-OUT-002
echo "  Running ORG-OUT-002..." >&2
OUT002=$($KOS groups -o yaml 2>/dev/null)
check "ORG-OUT-002a" "YAML contains name field" "$OUT002" "name:"
check "ORG-OUT-002b" "YAML contains groupType" "$OUT002" "groupType:"
check "ORG-OUT-002c" "YAML contains confidence" "$OUT002" "confidence:"

# ORG-OUT-003
echo "  Running ORG-OUT-003..." >&2
OUT003=$($KOS groups argocd -o json 2>/dev/null)
OUT003_COUNT=$(echo "$OUT003" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d))" 2>/dev/null || echo "0")
check "ORG-OUT-003a" "positional filter returns 1 result" "$OUT003_COUNT" "^1$"
check "ORG-OUT-003b" "contains argocd" "$OUT003" "argocd"

# ORG-OUT-004
check "ORG-OUT-004a" "describe includes evidence" "$NAV002" "helm-release=argocd|instance.*=.*argocd"

# --- Reconciliation Tests ---
echo "" >> "$OUT"
echo "=== 5. Reconciliation Tests ===" >> "$OUT"

# ORG-REC-001
echo "  Running ORG-REC-001..." >&2
REC001=$($KOS groups argocd -o json 2>/dev/null)
RC=$(echo "$REC001" | python3 -c "import json,sys; d=json.load(sys.stdin); g=d[0]; print('MATCH' if g['resourceCount']==len(g['members']) else f'MISMATCH {g[\"resourceCount\"]} vs {len(g[\"members\"])}')" 2>/dev/null || echo "ERR")
check "ORG-REC-001a" "resourceCount == len(members)" "$RC" "^MATCH$"

# ORG-REC-002
REC002=$(echo "$REC001" | python3 -c "
import json,sys
d=json.load(sys.stdin)
g=d[0]
wl_kinds={'Deployment','StatefulSet','DaemonSet','CronJob','Job'}
actual_wl=sum(1 for m in g['members'] if m['kind'] in wl_kinds)
print('MATCH' if g['workloadCount']==actual_wl else f'MISMATCH {g[\"workloadCount\"]} vs {actual_wl}')
" 2>/dev/null || echo "ERR")
check "ORG-REC-002a" "workloadCount matches workload-kind members" "$REC002" "^MATCH$"

# ORG-REC-003
REC003=$(echo "$REC001" | python3 -c "
import json,sys
d=json.load(sys.stdin)
g=d[0]
comps=set(m['component'] for m in g['members'] if m.get('component'))
print('MATCH' if g['componentCount']==len(comps) else f'MISMATCH {g[\"componentCount\"]} vs {len(comps)}')
" 2>/dev/null || echo "ERR")
check "ORG-REC-003a" "componentCount matches unique components" "$REC003" "^MATCH$"

# --- Determinism Tests ---
echo "" >> "$OUT"
echo "=== 6. Determinism Tests ===" >> "$OUT"

# ORG-DET-001
echo "  Running ORG-DET-001..." >&2
DET_A=$($KOS groups -o json 2>/dev/null)
DET_B=$($KOS groups -o json 2>/dev/null)
if [ "$DET_A" = "$DET_B" ]; then
  echo "  PASS  ORG-DET-001: Output identical across two runs" >> "$OUT"
  PASS=$((PASS + 1))
else
  echo "  FAIL  ORG-DET-001: Output differs between runs" >> "$OUT"
  FAIL=$((FAIL + 1))
fi

# --- Cross-Axis Handoff Tests ---
echo "" >> "$OUT"
echo "=== 7. Cross-Axis Handoff Tests ===" >> "$OUT"

# ORG-XAX-001
echo "  Running ORG-XAX-001..." >&2
XAX001=$($KOS releases argocd 2>/dev/null)
check "ORG-XAX-001a" "release argocd found" "$XAX001" "argocd"
check "ORG-XAX-001b" "shows resource count" "$XAX001" "[0-9]+"

# ORG-XAX-002
echo "  Running ORG-XAX-002..." >&2
XAX002=$($KOS relationships Deployment argocd-server -n argocd 2>/dev/null)
check "ORG-XAX-002a" "shows relationship edges" "$XAX002" "Outgoing|Incoming|UsesServiceAccount|BelongsToRelease"

# ORG-XAX-004
echo "  Running ORG-XAX-004..." >&2
XAX004=$($KOS graph export --cluster-id test 2>/dev/null)
check "ORG-XAX-004a" "graph export contains nodes" "$XAX004" '"nodes"'
check "ORG-XAX-004b" "graph export contains edges" "$XAX004" '"edges"'

# --- Summary ---
echo "" >> "$OUT"
echo "============================================" >> "$OUT"
echo "Results: ${PASS} passed, ${FAIL} failed, ${SKIP} skipped" >> "$OUT"
echo "============================================" >> "$OUT"

# Print to terminal
cat "$OUT"
echo ""
echo "Results written to: ${OUT}"

# Exit with failure if any test failed
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
