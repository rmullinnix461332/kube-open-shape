# KOS Ownership Refactor — Implementation Plan

## Problem Statement

The current ownership model classifies resources into flat buckets (Managed, Inherited, AdHoc, Unknown, PlatformManaged, Orphaned, Conflicted) with a single owner attribution. This does not answer the operational questions operators actually ask:

- What authority is responsible for this resource existing?
- What would recreate it if deleted?
- What is the complete ownership chain from pod to lifecycle authority?
- Where does the ownership chain become unknown?

## Design Principles

1. Ownership is a **chain**, not a flat classification.
2. **Runtime ownership** (Kubernetes controller relationships) and **lifecycle ownership** (declarative authority) are separate layers.
3. Evidence at every step must be preserved and explainable.
4. Multiple authorities claiming the same resource is a real conflict — not a display artifact.
5. Ownership does not answer organization, structure, or graph questions.

## Target Model

### Ownership Chain

Every resource has an ownership chain from itself upward to the highest identifiable authority:

```
Pod
└── ReplicaSet          (runtime: ownerReference)
    └── Deployment      (runtime: ownerReference)
        └── Helm release argocd    (lifecycle: release manifest membership)
            └── Argo CD Application argocd   (lifecycle: Application spec)
```

### Two Layers

| Layer | Question | Evidence Sources |
|-------|----------|-----------------|
| Runtime | What Kubernetes controller directly manages this resource? | ownerReferences, controller annotations |
| Lifecycle | What declarative authority declares the root resource should exist? | Helm release manifest, ArgoCD Application, Flux resources, operator CRs, platform bootstrap |

### Resource Ownership Record

```go
type OwnershipRecord struct {
    ResourceKey      string
    RuntimeChain     []ChainLink       // ownerRef traversal from resource to root
    LifecycleAuthority *LifecycleAuth  // the declarative authority at the root
    Classification   Classification    // derived from chain completeness
    Confidence       Confidence
    Evidence         []Evidence
}

type ChainLink struct {
    ResourceKey string
    Kind        string
    Name        string
    Namespace   string
    Relationship string // "ownerReference", "controller"
    Evidence    string
}

type LifecycleAuth struct {
    Type       string  // Helm, ArgoCD, Flux, Operator, Platform, Manual
    Name       string
    Namespace  string
    Evidence   []Evidence
    Present    bool    // is the authority resource still present in cluster?
}
```

### Classifications (Derived)

Classifications remain but are derived from chain analysis:

| Classification | Meaning |
|----------------|---------|
| Authoritative | Complete chain to a present lifecycle authority |
| Inherited | Resource reaches an authority through runtime chain |
| Orphaned | Runtime owner exists but lifecycle authority is missing |
| Unmanaged | No lifecycle authority identified for the root |
| PlatformManaged | System-generated resource (kube-root-ca.crt, default SA) |
| Conflicted | Multiple lifecycle authorities claim the same root |
| Direct | Lifecycle authority directly declares this resource (no runtime intermediate) |

## CLI Refactor

### Default: `kos ownership`

Summary by lifecycle authority:

```
LIFECYCLE AUTHORITY    TYPE   RESOURCES  DIRECT  INHERITED  COVERAGE
argocd                 Helm   64         55      9          12.7%
cert-manager           Helm   49         42      7          9.7%
external-secrets       Helm   20         18      2          4.0%
(platform)             K8s    32         32      0          6.3%
(unmanaged)            —      211        211     0          41.9%
```

### `kos ownership <authority-name>`

Inventory view — one row per resource attributed to that ownership lineage:

```
kos ownership argocd
```

```
RESOURCE                                      DIRECT OWNER                         LIFECYCLE AUTHORITY  CONFIDENCE
ConfigMap/argocd/argocd-cm                    —                                    Helm/argocd          Authoritative
Deployment/argocd/argocd-server               —                                    Helm/argocd          Authoritative
ReplicaSet/argocd/argocd-server-6c967757bb    Deployment/argocd/argocd-server      Helm/argocd          Authoritative
Pod/argocd/argocd-server-6c967757bb-abc12     ReplicaSet/argocd/...                Helm/argocd          Authoritative

61 resources
```

This view supports:
- Scanning everything attributed to an authority.
- Seeing direct versus inherited ownership.
- Finding ownership chains that terminate unexpectedly.
- Identifying resources with ambiguous or competing authority.

It does NOT print the full evidence chain for every resource.

### `kos ownership Unmanaged`

Classification filter (backward compatible):

```
kos ownership Unmanaged -n kube-system
```

### `kos describe ownership <authority-name>`

Synthesized explanation of an ownership authority — its hierarchy, evidence, coverage, and exceptions:

```
kos describe ownership argocd
```

```
Ownership: argocd

Lifecycle Authorities:
  Helm Release:
    Name:       argocd
    Namespace:  argocd
    Status:     Present
    Resources:  61
    Confidence: Authoritative

  Argo CD Application:
    Name:       argocd
    Namespace:  argocd
    Status:     Present
    Controls:   Helm/argocd
    Confidence: Authoritative

Ownership Chain:
  ArgoCD/argocd
  └── Helm/argocd
      ├── 55 directly declared resources
      └── 6 runtime descendants

Evidence:
  Helm stored release manifest
  Helm release metadata
  Argo CD desired manifests
  Kubernetes ownerReferences

Coverage:
  Directly declared:     55
  Runtime descendants:   6
  Unknown termination:   0
  Competing claims:      0

Exceptions:
  Shared resources:       0
  Contended resources:    0
  Broken owner chains:    0
  Inferred-only claims:   0
```

Helm/argocd and ArgoCD/argocd appear as separate layers — not competing owners. Argo CD reconciles the release-level desired state; Helm provenance identifies the release that declares the Kubernetes resources.

### `kos describe ownership <kind> <name> -n <ns>`

Full chain for one resource:

```
kos describe ownership Deployment argocd-server -n argocd
```

```
Resource:         Deployment/argocd/argocd-server
Classification:   Authoritative
Confidence:       Authoritative

Ownership Chain:
  Deployment/argocd/argocd-server
  └── Helm release argocd (lifecycle authority)
      Evidence: labels[helm.sh/chart], annotations[meta.helm.sh/release-name]

Runtime Descendants:
  ReplicaSet/argocd/argocd-server-6c967757bb (ownerReference)
  Pod/argocd/argocd-server-6c967757bb-xyz   (ownerReference via ReplicaSet)

If deleted:
  Would be recreated by: Helm release argocd (next sync/upgrade)
```

### Command/object distinction

| Command | Primary object | Purpose |
|---------|---------------|---------|
| `kos ownership argocd` | Resources | Show the inventory attributed to an ownership lineage |
| `kos describe ownership argocd` | Ownership lineage | Explain its authorities, hierarchy, evidence, coverage, and exceptions |
| `kos describe ownership Deployment argocd-server -n argocd` | Single resource | Show complete ownership chain and evidence for one resource |

### `-o wide`

```
kos ownership -o wide
```

```
AUTHORITY    TYPE   RESOURCES  DIRECT  INHERITED  UNRESOLVED  CONFLICTS  NAMESPACES
argocd       Helm   64         55      9          0           0          argocd
cert-manager Helm   49         42      7          0           0          cert-manager, kube-system
(unmanaged)  —      211        211     0          —           —          (multiple)
```

## Implementation Phases

### Phase 1: Chain Model (internal refactor)

- Replace flat `Result` with `OwnershipRecord` containing `RuntimeChain` and `LifecycleAuthority`
- Runtime chain: traverse ownerReferences upward until root (no ownerRef)
- Lifecycle authority: at the root, apply detector chain (Helm, ArgoCD, Platform)
- Preserve backward compatibility: derive Classification from chain

**Files changed:**
- `internal/edge/ownership/types.go` — new chain types
- `internal/edge/ownership/resolver.go` — refactor to build chains
- `internal/edge/ownership/chain.go` — new: chain traversal and analysis

### Phase 2: CLI Authority View

- `kos ownership` → authority-level summary (replace flat classification summary as default)
- `kos ownership <authority>` → resources for that authority
- `kos ownership Unmanaged` → classification filter (backward compat)
- Keep `-o wide` for extended columns

**Files changed:**
- `cli/ownership.go` — rewrite for authority-centric output

### Phase 3: Describe Ownership

- `kos describe ownership <kind> <name>` → full chain + evidence + "if deleted" reasoning
- Integrate with release model to show Helm/ArgoCD details
- Show runtime descendants (what would be affected if this resource changes)

**Files changed:**
- `cli/describe.go` — add ownership describe with chain rendering

### Phase 4: API + Graph

- `/api/v1/ownership` → authority-level summary
- `/api/v1/ownership/{key}` → full chain for one resource
- Graph export: ownership chain edges with layer classification
- Runtime ownership edges: `OwnedBy` (runtime layer)
- Lifecycle authority edges: `DeclaredBy` (lifecycle layer)

### Phase 5: Conflict and Gap Analysis

- Detect resources where multiple lifecycle authorities compete
- Detect chains that terminate unexpectedly (orphaned controllers)
- Detect authorities that are no longer present in cluster
- Surface systemic patterns (e.g., all resources in a namespace are unmanaged)

## Boundary

Ownership does NOT answer:
- What application/component the resource belongs to → Organization axis
- What structural shape it fulfills → Structure axis
- What depends on it or is reachable from it → Graph axis
- Whether it is healthy → outside KOS

Ownership CAN link to those facts via cross-axis navigation:
```
kos describe ownership Deployment argocd-server -n argocd
→ "Part of group: argocd"
→ "Shape: kos-default-application"
→ "Relationships: 5 outgoing, 3 incoming"
```

## Evidence Hierarchy

| Evidence Type | Confidence | Supports |
|---------------|-----------|----------|
| ownerReference | Authoritative | Runtime chain link |
| Helm release manifest | Authoritative | Lifecycle authority (Direct) |
| meta.helm.sh/release-name annotation | Authoritative | Lifecycle authority (Direct) |
| helm.sh/chart label | Authoritative | Lifecycle authority (Direct) |
| ArgoCD tracking-id annotation | Authoritative | Lifecycle authority (Direct) |
| ArgoCD Application resource reference | Authoritative | Higher-level reconciler |
| managed-by=Helm + instance label | Declared | Lifecycle authority (inferred name) |
| managedFields manager name | Corroborating | Manual vs automated |
| Naming convention | Heuristic | Fallback only |

## Migration

The refactor is additive — the existing `Classification` field and `Resolve()` method remain available during transition. New chain-based output is exposed through new CLI modes while old modes are deprecated.

Deprecation path:
1. Phase 1: Add chain internally, derive old classifications from chain.
2. Phase 2: New CLI default is authority view; old classification view available via `--legacy` or `kos ownership --by-classification`.
3. Phase 3: Remove legacy after organizational testing confirms no regressions.


## Open Questions — Resolved

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Authority name resolution | Qualify when ambiguous. Unqualified name resolves only when unique; otherwise report matching authorities with type. | Authority identity must include type and scope. Combining same-name authorities creates a synthetic owner that does not exist. |
| 2 | Shared resource attribution | Distinguish manager from consumers. One authority manages; many releases may use. Multiple authorities asserting lifecycle control = Contended. | "Shared" describes usage, not lifecycle ownership. Never select the first claimant. |
| 3 | Pod inclusion | Do not include or watch Pods. Stop runtime chains at stable workload/controller resources. | KOS is not monitoring runtime health. Pod churn destabilizes inventory without advancing the product goal. |
| 4 | "Present" authority semantics | Present = authority object exists in the current knowledge index. | Helm Secret present → verified. Argo Application CR present → verified. Tracking metadata proves a claim, not that the authority still exists. Preserve lastObserved separately. |
| 5 | Partial ArgoCD instrumentation | Show as "Detected (unverified)". Display the tracking signal without representing the Application as fully modeled. Keep outside verified chain until CR is indexed. | Honest about model completeness without hiding available evidence. |
| 6 | Namespace vs cluster scope | Authorities may manage both namespaced and cluster-scoped resources. Release namespace is identity/storage, not an ownership boundary. | A Helm release in namespace cert-manager can legitimately own ClusterRoles. |
| 7 | Unmanaged as classification or authority | Separate "No known authority" coverage section. Not a pseudo-authority. Include in totals so coverage accounts for 100%. | This is absence of knowledge, not a declared authority. |
| 8 | ArgoCD-managed Helm chain | Model the actual authority chain; do not assume Helm is an intermediary. | See correction below. |
| 9 | Framework resource chain depth | Terminate at workload root. kube-controller-manager is implicit. | Implementation machinery, not useful lifecycle authority. Already represented by Kubernetes ownership edges. |
| 10 | Ownership vs Releases overlap | Complementary axes. Ownership references releases by name without reproducing release details. | Ownership answers authority attribution; Releases answers revision, source, status, and progression. |

### Correction to Question 8: ArgoCD and Helm Authority Chains

Three distinct cases exist:

**Case 1: Helm installs resources directly**
```
Resource → Helm release
```
Standard Helm-managed resource. Helm release Secret proves authority.

**Case 2: Argo CD renders a Helm chart (no Helm release)**
```
Resource → Argo CD Application
  Source generator: Helm chart
  No Helm release authority
```
Argo CD uses Helm as a template engine. It does NOT run `helm install`. No Helm release Secret exists. The `helm.sh/chart` label identifies chart *provenance* but does not prove a Helm release authority.

**Case 3: Argo CD manages configuration that triggers an actual Helm installation**
```
Resource → Helm release → higher authority
```
This chain should appear only when KOS has evidence of BOTH authorities AND their relationship. Rare in practice.

The critical distinction: `helm.sh/chart` label = provenance metadata, NOT proof of Helm lifecycle authority. A Helm release Secret = proof of Helm authority.

## Revised Ownership Model

Based on resolved questions, the ownership model simplifies to:

### Authority States

| State | Meaning |
|-------|---------|
| Verified | Authority object exists in knowledge index |
| Detected (unverified) | Evidence of authority exists but authority object not indexed |
| Missing | Previously verified authority no longer present |
| Contended | Multiple lifecycle authorities assert control over the same resource |
| No known authority | No lifecycle authority identified |

### Attribution Types

| Type | Meaning |
|------|---------|
| Direct | Lifecycle authority directly declares this resource |
| Inherited | Resource reaches authority through Kubernetes owner chain |

### Separated Concerns

Former ownership classifications that actually belong to other axes:

| Former classification | Correct domain | Meaning |
|----------------------|----------------|---------|
| Shared | Usage/Graph | Multiple consumers reference this resource |
| PlatformManaged | Organization/Role | System-generated resource with a structural role |
| AdHoc | Provenance | Resource was created manually (no lifecycle authority) |
| Orphaned | Graph condition | Runtime owner exists but lifecycle authority is missing |
| Contended | Ownership | Multiple lifecycle authorities assert control |

The critical outcome: these no longer compete as ownership classifications. Each is a fact about a different dimension of the resource.



## Phase 2b: Reduce Unknown — Bootstrap and Substrate Attribution

### Problem

Unknown is inflated by unmodeled cluster substrate. 260 "unknown" resources are largely recognizable Kubernetes default RBAC, bootstrap configuration, distribution add-ons, and controller runtime artifacts.

### Categories to Resolve

| Category | Examples | Authority | Evidence |
|----------|----------|-----------|----------|
| Kubernetes default RBAC | system:*, admin, edit, view | KubernetesBootstrap/rbac-defaults | kubernetes.io/bootstrapping label, managedFields manager |
| Cluster bootstrap config | kubeadm-config, kubelet-config, cluster-info | KubernetesBootstrap/kubeadm | Namespace kube-system + known names + managedFields |
| Distribution add-ons | kindnet, CoreDNS, kube-proxy, local-path-provisioner | ClusterDistribution/<name> | managedFields manager, known DaemonSet patterns |
| Controller runtime artifacts | Leader-election Leases, generated Secrets | Controller that created them | managedFields manager |
| Helm authority records | sh.helm.release.v1.* Secrets | Resource function: Authority record | Excluded from release-owned count |
| Runtime descendants (ReplicaSets) | ReplicaSets | Inherited through workload root | ownerReferences |

### Implementation (Done)

- `BootstrapDetector` added to detect:
  - `kubernetes.io/bootstrapping` label (authoritative)
  - managedFields bootstrap managers (kube-apiserver, kubeadm, kube-controller-manager)
  - system: prefix RBAC (corroborating only)
- Wired into resolver chain between Platform and ArgoCD

### Remaining Work

- Controller Lease/Secret attribution via managedFields manager identity
- Distribution component detection (kindnet, coredns as cluster-distribution authorities)
- Helm release Secret classification as authority record (not owned member)
- Namespace attribution (leave genuinely unresolved unless declared by manifest)
- PVC retention semantics (created-from vs independently-retained)

### Display Fix (Done)

- Resources with no known authority show attribution as "—" instead of "Direct"
- Summary shows "No known authority" count separately
