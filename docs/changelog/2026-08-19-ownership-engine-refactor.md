# Ownership Engine Refactor — Fact-Based Decision Architecture

## Problem

The current ownership implementation grows a new Go file for each class of resource to recognize. This does not scale. Adding Kubernetes distributions, controller identities, operator patterns, and community-contributed knowledge packs requires modifying compiled code.

## Core Principle

**Code observes facts. Configuration interprets facts. The engine resolves competing interpretations. The explanation trace proves the result.**

## Architecture

```
Kubernetes resource
    ↓
Normalized field/fact identifiers (code: extractors)
    ↓
FactStore (materialized once per collection cycle)
    ↓
Lookup/catalog evaluation (config: catalogs)
    ↓
Ownership decision rules (config: rules)
    ↓
All applicable candidates (evaluate every rule)
    ↓
Precedence/conflict resolution (engine)
    ↓
Ownership result + explanation trace
```

---

## Layer 1: Fact Model

### Typed, Attributable Fact with Subject

```go
type Fact struct {
    Subject    string            // resource being described (target of the fact)
    Field      string            // normalized identifier
    Value      any               // observed value (string, bool, struct)
    Attributes map[string]string // bound contextual values
    Source     string            // resource key that supplied the evidence
    Evidence   EvidenceRef       // link to the Kubernetes object/field
}

type EvidenceRef struct {
    ResourceKey  string
    FieldPath    string
    DisplayValue string // safe display representation (never raw Secret data)
    ValueDigest  string // locally-keyed HMAC for non-sensitive values; omitted for sensitive
    Sensitive    bool   // if true, digest is omitted and raw value never appears in traces
}
```

Subject vs Source distinction:
- **Subject**: the resource this fact describes (e.g., `Deployment/argocd/argocd-server`)
- **Source**: the object that supplied the evidence (e.g., `Secret/argocd/sh.helm.release.v1.argocd.v1`)

A Helm membership fact is produced from a release Secret but belongs to another resource:
```
Subject: Deployment/argocd/argocd-server
Source:  Secret/argocd/sh.helm.release.v1.argocd.v1
Field:   release.manifestMember
Value:   true
Attributes:
  release.name: argocd
  release.namespace: argocd
  release.revision: "1"
```

This allows the batch extractor to unambiguously place facts into `FactStore.byResource[subject]`.

### Fact Identifiers

Standard fields:
```
resource.apiGroup
resource.kind
resource.name
resource.namespace
resource.uid
metadata.label[<key>]
metadata.annotation[<key>]
metadata.ownerReference[].kind
metadata.ownerReference[].name
metadata.ownerReference[].uid
metadata.managedField[].manager
metadata.managedField[].operation
secret.type
```

Derived fields (from specialized extractors):
```
release.manifestMember          # resource declared in Helm release manifest
release.metadataRecord          # resource IS a Helm release Secret
release.name                    # bound release name
release.namespace               # bound release namespace
release.revision                # bound revision
runtime.ownerChainRoot          # the root of the ownerReference chain
runtime.ownerChainDepth         # hops to root
argocd.trackingClaim.appName    # parsed from tracking-id annotation
argocd.trackingClaim.appNS      # parsed from tracking-id annotation
lease.resolvedController        # controller identity owning this Lease
```

### FactStore

Facts are materialized once per collection cycle, not per-rule evaluation:

```go
type FactStore struct {
    byResource map[string][]Fact // resourceKey → facts
}
```

Build sequence:
1. Collect resources → build index
2. Run all extractors → materialize FactStore
3. Evaluate decisions against FactStore (read-only)
4. Propagate lifecycle authority through owner chains

This avoids O(resources × releases) behavior and ensures determinism.

---

## Layer 2: Fact Extractors (Code)

### Interface

```go
type FactExtractor interface {
    Name() string
    Extract(index *knowledge.Index) []Fact  // produces facts for ALL applicable resources
}
```

Note: extractors operate over the entire index, not per-resource. This allows efficient batch processing (e.g., decode all Helm Secrets once, emit membership facts for all member resources).

### Built-in Extractors

| Extractor | Purpose | Phase |
|-----------|---------|-------|
| MetadataExtractor | Labels, annotations, managedFields → standard field facts | A |
| HelmManifestExtractor | Decode release Secrets → manifest membership + record facts | A |
| HelmRecordExtractor | Identify release Secret → release.metadataRecord fact | A |
| RuntimeChainExtractor | Traverse ownerRefs → root, depth, chain facts | A |
| ArgoCDTrackingExtractor | Parse tracking-id → structured claim facts | B |
| LeaseControllerExtractor | Resolve lease holder to controller identity | B |
| PVCTemplateExtractor | Associate PVCs with StatefulSet VCT | B |

### What extractors must NOT do

- Make ownership decisions
- Assign authority
- Determine confidence
- Apply naming heuristics as evidence

---

## Layer 3: Catalogs (Configuration)

```yaml
catalogs:
  kubernetesControllerServiceAccounts:
    type: exactSet
    version: "1.30"
    values:
      - attachdetach-controller
      - certificate-controller
      - cronjob-controller
      - daemon-set-controller
      - deployment-controller
      # ... (full list)

  kubernetesBootstrapManagers:
    type: exactSet
    values:
      - kube-apiserver
      - kubeadm

  clusterDistributionComponents:
    type: exactSet
    values:
      - kindnet
      - kindnetd
      - kube-proxy
      - coredns
      - local-path-provisioner

  helmReleaseSecretPattern:
    type: prefix
    value: "sh.helm.release.v1."
```

### Catalog Properties

- Versioned (Kubernetes version, distribution version)
- Loaded from embedded defaults + filesystem overrides
- Schema-validated before acceptance
- Pack identity, version, applicability, and digest tracked
- Missing catalog reference → rule rejected (fail-closed)

---

## Layer 4: Decision Rules (Configuration)

### Rule Structure

```yaml
decisions:
  - name: helm-manifest-member
    priority: 1000
    when:
      all:
        - field: release.manifestMember
          exists: true
    result:
      authority:
        type: Helm
        nameFrom: release.name
        namespaceFrom: release.namespace
      claimLayer: LifecycleAuthority
      evidenceStrength: Authoritative
      authorityState: Verified
      attribution: Direct
```

Every authority-producing rule MUST declare `claimLayer`. Without it, the resolution engine cannot implement the contention model. Rules that produce only supporting evidence and no authority candidate do not require a claim layer.

### Evidence Strength vs Authority State

Rules must declare BOTH separately:

| Field | Meaning |
|-------|---------|
| evidenceStrength | How reliable is the evidence? (Authoritative, Corroborating, Supporting) |
| authorityState | Is the authority actually present? (Verified, Detected, Missing) |
| decisionConfidence | Overall confidence (derived from both) |

### Corrected Evidence Strengths

| Rule | evidenceStrength | authorityState | Rationale |
|------|-----------------|----------------|-----------|
| Helm manifest membership (release Secret exists) | Authoritative | Verified | Release Secret proves both evidence and authority presence |
| Helm release record (the Secret itself) | Authoritative | Verified | The Secret IS the authority record |
| Helm chart label alone (no release annotation) | Corroborating | Detected | Argo-rendered resources may carry Helm labels without an actual Helm release |
| Helm meta.helm.sh/release-name annotation | Authoritative | Verified | Set by Helm itself, release Secret should exist |
| Helm release annotation WITHOUT matching release Secret | Authoritative | Detected | Annotation proves claim; release may have been deleted |
| ArgoCD tracking annotation without Application CR | Authoritative | Detected | Authority claim detected; Application presence not verified |
| Controller SA by namespace+name catalog | Corroborating | Detected | Namespace+name matching alone is not proof of lifecycle |
| Kubernetes bootstrapping label | Authoritative | Verified | Explicitly declared by the API server |
| Generic managed-field manager | Supporting | — | Mutation evidence only; must not assign lifecycle authority |
| Lease with any managed-field entry | Supporting | — | Invalid as authority rule; requires LeaseControllerExtractor |

### What managedFields means

managedFields proves who CHANGED specific fields. It does NOT prove who says the resource SHOULD EXIST.

- `kube-apiserver` mutates resources it does not lifecycle-own (defaulting, admission)
- `kube-controller-manager` mutates resources it does not lifecycle-own (status updates)
- A managedFields entry may CORROBORATE a more specific rule but must NOT independently assign lifecycle authority

### Resolution Semantics

1. Evaluate EVERY applicable rule (do not stop at first match)
2. Bind each match to its exact supporting facts
3. Group candidates by complete authority identity (type + name + namespace)
4. Merge evidence for the same authority
5. Compare evidence strength and specificity
6. Preserve weaker candidates as supporting evidence
7. Mark multiple incompatible authoritative claims as Contended
8. Use numeric priority only as a final tie-breaker when strength is equal

---

## Layer 5: Authority Identity and Claim Layers

### Authority Identity

```go
type AuthorityIdentity struct {
    Type      string // Helm, ArgoCD, KubernetesController, ClusterDistribution
    Name      string
    Namespace string // release/application namespace (scope)
}
```

Complete identity determines uniqueness. Within one edge, cluster is implicit; persisted composite key is `clusterID + AuthorityIdentity`.

### Claim Layers

Each authority candidate declares which LAYER it claims:

```go
type ClaimLayer string

const (
    ClaimRuntimeController     ClaimLayer = "RuntimeController"
    ClaimLifecycleAuthority    ClaimLayer = "LifecycleAuthority"
    ClaimHigherLevelReconciler ClaimLayer = "HigherLevelReconciler"
    ClaimAuthorityRecord       ClaimLayer = "AuthorityRecord"
)
```

### Contention Rules

Contention occurs ONLY when incompatible authorities claim the SAME layer:

| Scenario | Result |
|----------|--------|
| Helm release A and Helm release B both directly declare one resource | **Contended** (same layer: LifecycleAuthority) |
| Kubernetes controller owns ReplicaSet; Helm owns its Deployment root | **Valid chain** (different layers: RuntimeController + LifecycleAuthority) |
| ArgoCD Application manages a HelmRelease CR which owns Kubernetes resources | **Valid chain** (HigherLevelReconciler manages another authority object) |
| Two ArgoCD Applications both claim the same resource | **Contended** (same layer: LifecycleAuthority) |
| ArgoCD tracking annotation on a directly-applied resource | **Single authority** (LifecycleAuthority: ArgoCD). Helm chart labels are source provenance, not a competing Helm authority. |

### ArgoCD and Helm: Claim Layer Semantics

**Normal case — ArgoCD renders a Helm chart and applies resources directly:**
```
Resource ← ArgoCD Application (LifecycleAuthority)
  Source generator: Helm chart
  Helm chart labels: provenance metadata only
  No Helm release Secret exists
  No Helm authority claim
```
ArgoCD claims LifecycleAuthority. Helm labels describe provenance, not ownership.

**Rare case — ArgoCD manages a deployment authority object (HelmRelease CR):**
```
ArgoCD Application (HigherLevelReconciler)
└── HelmRelease CR or similar (LifecycleAuthority, managed by its controller)
    └── Kubernetes resources
```
HigherLevelReconciler applies ONLY when an authority manages another authority's defining object. This requires explicit evidence of both authorities and their relationship.

### Required Tests for ArgoCD/Helm Interaction

- ArgoCD tracking annotation + Helm chart label on same resource → single LifecycleAuthority (ArgoCD), Helm labels are provenance only
- ArgoCD tracking annotation + actual Helm release Secret membership → Contended (two LifecycleAuthority claims)
- ArgoCD Application managing a HelmRelease CR → HigherLevelReconciler chain, not contention

### Resolution with Layers

1. Group candidates by claim layer
2. Within each layer: merge same-authority evidence, detect same-layer contention
3. Across layers: build the authority chain (RuntimeController → LifecycleAuthority → HigherLevelReconciler)
4. Report contention only for same-layer conflicts

---

## Operational Safeguards

Because these decisions eventually influence janitor actions:

1. Schema validation before accepting a knowledge pack
2. Pack identity, version, applicability, and digest tracked
3. Kubernetes-version constraints on catalogs
4. Deterministic ordering (same input → same output)
5. Rejection of unresolved fact identifiers
6. Rejection of missing catalogs (fail-closed)
7. Explanation output: matched rule, bound facts, evidence source, pack version, losing candidates
8. Invalid configuration must not silently create ownership
9. Reject duplicate rule/catalog identifiers unless override behavior is explicitly declared
10. Sort emitted facts and candidates before resolution so map iteration cannot affect deterministic output
11. For sensitive evidence (decoded Secrets), omit digest or use a locally-keyed HMAC — plain SHA-256 of low-entropy values becomes a verification oracle

### Required Tests

- Weak heuristics cannot override explicit Helm membership
- Multiple authoritative claims at the same layer become Contended
- managedFields alone cannot assign lifecycle authority
- Missing catalog causes rule rejection, not silent skip
- Same facts + same rules = deterministic result across restarts
- ArgoCD tracking + Helm chart label on same resource → single LifecycleAuthority (ArgoCD)
- ArgoCD tracking + Helm release Secret membership → Contended (two LifecycleAuthority claims)
- RuntimeController + LifecycleAuthority on same resource → valid chain, not contention
- Every authority-producing rule includes claimLayer (schema validation)

---

## Implementation Phases

### Phase A: Fact model, FactStore, rule engine, explanation trace

- `Fact` type with attributes and evidence binding
- `FactStore` materialized once per cycle
- `FactExtractor` interface (batch over index)
- `MetadataExtractor` — labels, annotations, managedFields
- `HelmManifestExtractor` — decode Secrets, emit membership facts
- `HelmRecordExtractor` — identify release Secrets
- `RuntimeChainExtractor` — ownerRef traversal
- Rule engine with `equals`, `exists`, `inCatalog`, `matchesCatalog` conditions
- Built-in catalogs (embedded YAML)
- Resolution engine with explanation trace

### Phase B: Migrate existing rules in shadow mode

- Convert detector logic to decision rules
- Add ArgoCD tracking extractor
- Add LeaseController extractor
- Add distribution-specific rules
- Run new engine in parallel with old detectors
- Compare results — document intentional improvements (not parity with known-bad detectors)
- Validate shadow results cover Argo, Lease, and distribution cases before switching
- Switch new engine to primary

### Phase C: External knowledge packs

- Load catalogs from `config/ownership/catalogs/*.yaml`
- Load rules from `config/ownership/rules/*.yaml`
- Support distribution packs (kind, eks, gke, aks)
- Validate schema, version constraints, digest

### Phase D: Custom rules

- Organizational conventions as rules
- Operator-specific authority patterns
- Custom catalog entries

---

## Corrections from Current Implementation

| Current | Corrected |
|---------|-----------|
| `Platform/kubernetes` owns kube-root-ca.crt | `KubernetesController/root-ca-cert-publisher` |
| `Platform/kubernetes` owns default SA | `KubernetesController/service-account-controller` |
| managedFields manager → authoritative authority | managedFields → supporting evidence only |
| Lease + any managedField → authoritative | Lease requires resolved controller identity |
| Controller SA by name → authoritative | By name → corroborating only |
| First match wins | All rules evaluated, then resolved |
| `kos ownership catalogs` CLI command | Removed — configuration introspection designed separately |

---

## File Structure

```
internal/edge/ownership/
  engine.go           — DecisionEngine, Evaluate, Resolve
  facts.go            — Fact, FactStore, FactExtractor interface
  extractors/
    metadata.go       — MetadataExtractor
    helm_manifest.go  — HelmManifestExtractor, HelmRecordExtractor
    runtime_chain.go  — RuntimeChainExtractor
    argocd.go         — ArgoCDTrackingExtractor
    lease.go          — LeaseControllerExtractor
  catalogs.go         — Catalog type, loading, evaluation
  rules.go            — DecisionRule, condition evaluation, binding
  resolution.go       — Candidate grouping, strength comparison, contention
  explanation.go      — Trace output, losing candidates, evidence chain
  chain.go            — OwnershipRecord, ChainLink, DeriveResult (stable interface)
  types.go            — Classification, Confidence, OwnerRef (stable interface)
  defaults/
    catalogs.yaml     — embedded default catalogs
    rules.yaml        — embedded default decision rules
```

## Migration

The external interface (`ResolveChain`, `ResolveAllChains`, `DeriveResult`) remains stable. The new engine replaces the detector chain internally. Callers are unaffected.

Acceptance criterion: the new engine produces CORRECT results (per the evidence strength table), not identical results to the known-bad detectors. Intentional improvements in evidence handling are expected and documented.
