# Kube Open Shape — Implementation Plan

## Overview

Kube Open Shape is a Kubernetes cluster knowledge and janitor system. An autonomous edge controller observes declared Kubernetes resources, builds a working knowledge model with ownership, relationships, and structural shapes, and evaluates deterministic janitor rules to report, mark, neutralize, restore, or delete qualifying resources. A central service distributes versioned policy, receives knowledge summaries and findings, and provides fleet-level intelligence.

## Product Strategy

The product has three delivery phases aligned to distinct market moments:

| Phase | Focus | Model | Timeline |
|-------|-------|-------|----------|
| **Phase 1** | Prove the edge | Open-source autonomous edge | Months 1–4 |
| **Phase 2** | Prove fleet value | Commercial self-hosted Central | Months 5–10 |
| **Phase 3** | Launch SaaS | Managed SaaS Central | Months 11–18 |

**Phase 1** ships as open source. The edge runs autonomously on a single cluster with a local SQLite knowledge store, a CLI, and a local API. A mock center validates the open protocol and enables offline development. Adoption requires only `kubectl apply` — no central dependency.

**Phase 2** introduces the commercial self-hosted Central. Fleet operators get cross-cluster shape comparison, policy distribution, governance reporting, and enterprise identity. The edge protocol does not change.

**Phase 3** launches a managed SaaS version of Central. Customers migrate from self-hosted to SaaS without reinstalling edges. Elastic analytics, shape benchmarks, and compliance packages differentiate the cloud tier.

The edge is the permanent open-source foundation. Central is the commercial layer built on top of the open protocol.

---

---

## 1. Architecture Overview

```
CENTRAL SERVICES
  REST API | Policy Store | Fleet Knowledge | Shape Catalog | Reporting
                      |
          Private HTTPS + mTLS (edge-initiated)
                      |
┌─────────────────────────────────────────────────────┐
│                  EDGE CONTROLLER                      │
│                                                       │
│  Communication Client ──── Policy Manager             │
│          |                       |                    │
│          v                       v                    │
│   Reporting Spool       Scheduler / Event Router      │
│                                  |                    │
│                                  v                    │
│   Resource Collector ── Ownership Resolver            │
│   ── Relationship Graph ── Shape Engine               │
│   ── Working Knowledge                                │
│                                  |                    │
│                                  v                    │
│                    Janitor Rule Engine                 │
│                                  |                    │
│                                  v                    │
│                    Finding / Action Reconciler         │
└──────────────────────────────────────────────────────┘
                      |
              Kubernetes API / RBAC
```

---

## 2. Technology Decisions

### Edge (open-source, all phases)

| Concern | Choice | Notes |
|---------|--------|-------|
| Language | Go 1.22+ | |
| Kubernetes integration | controller-runtime, client-go | |
| Working knowledge index | In-memory + SQLite | SQLite for lifecycle clocks, findings, actions, spool — one file per edge install |
| CRD management | controller-gen | |
| CLI | cobra + local HTTP API | `kubectl` users can query knowledge, shapes, findings without a browser |
| Local API | go-chi, localhost by default | Used by CLI and optional local dashboards |
| Leader election | Kubernetes Lease | |
| Metrics | Prometheus | |
| Config | YAML (kubebuilder markers) | |

**Why SQLite over BoltDB**: SQLite supports SQL queries from the CLI directly, is more broadly understood, and provides a better foundation for the local API and reporting layer. One database file contains lifecycle clocks, findings, actions, and the outbound spool.

### Phase 1 Mock Center

A lightweight mock center ships with the open-source repo. It:
- Accepts the open protocol (heartbeat, events, policy pull)
- Returns static or file-driven policy bundles
- Logs received payloads for protocol validation and development
- Is not a product — it validates the edge works correctly against the specified protocol

### Central (commercial — Phase 2+)

| Concern | Choice | Notes |
|---------|--------|-------|
| Language | Go 1.22+ | |
| Central API | go-chi | |
| Storage | PostgreSQL + JSONB | |
| Identity (Phase 2) | OIDC integration, RBAC | Enterprise SSO |
| Analytics (Phase 3) | ClickHouse or TimescaleDB | Elastic analytics at SaaS scale |
| Deployment | Helm chart (self-hosted), managed (SaaS) | |

---

## 3. Project Structure

```
kube-open-shape/                  # Open-source repository
├── cmd/
│   ├── edge/                     # Edge controller binary
│   └── mock-center/              # Mock central for protocol development/testing
├── internal/
│   ├── edge/
│   │   ├── collector/            # Resource collector and informers
│   │   ├── ownership/            # Ownership resolver and detectors
│   │   ├── graph/                # Relationship graph
│   │   ├── shape/                # Shape engine, fingerprinting, canonicalization
│   │   ├── knowledge/            # Working knowledge index
│   │   ├── store/                # SQLite persistence (clocks, findings, actions, spool)
│   │   ├── janitor/              # Rule engine, finding reconciler
│   │   ├── policy/               # Policy manager, bundle validation
│   │   ├── comm/                 # Communication client, outbound spool
│   │   ├── scheduler/            # Event router, debounce, schedule
│   │   └── api/                  # Local HTTP API (used by CLI)
│   └── mock/
│       └── center/               # Mock central server for protocol testing
├── pkg/
│   └── protocol/                 # Open protocol types (shared by edge and any center impl)
├── api/
│   └── v1alpha1/                 # CRD types
├── cli/                          # CLI commands (kubectl-style)
│   ├── knowledge.go
│   ├── shapes.go
│   ├── findings.go
│   └── report.go
├── config/
│   ├── crd/                      # Generated CRD manifests
│   ├── rbac/                     # RBAC manifests
│   └── samples/                  # Example CRs
├── docs/
│   ├── changelog/
│   └── README.md
└── go.mod

kube-open-shape-central/          # Commercial repository (Phase 2+, separate repo)
├── cmd/central/
├── internal/central/
│   ├── api/
│   ├── fleet/
│   ├── policy/
│   ├── shape/
│   └── reporting/
└── go.mod
```

The edge and mock center are in the same open-source repository. The commercial Central lives in a separate repository and depends on the open `pkg/protocol` types.

---

## 4. CRD Types

All edge CRDs are cluster-scoped unless noted.

### 4.1 ResourceOwner

Defines a recognized management owner and its detection behavior.

```go
type ResourceOwnerSpec struct {
    Type       string              `json:"type"`
    Detection  []DetectionRule     `json:"detection"`
    Resolution OwnerResolution     `json:"resolution"`
}

type DetectionRule struct {
    Type          string `json:"type"` // Annotation, Label, ManagedField
    Key           string `json:"key,omitempty"`
    ManagerPattern string `json:"managerPattern,omitempty"`
    Authoritative bool   `json:"authoritative"`
}

type OwnerResolution struct {
    Type      string `json:"type"`
    Namespace string `json:"namespace,omitempty"`
}
```

### 4.2 JanitorRule

Defines target scope, lifecycle, actions, and safety limits.

```go
type JanitorRuleSpec struct {
    Evaluator  EvaluatorSpec    `json:"evaluator"`
    Target     TargetSpec       `json:"target"`
    Match      MatchSpec        `json:"match,omitempty"`
    Family     FamilySpec       `json:"family,omitempty"`
    Lifecycle  []LifecycleStage `json:"lifecycle,omitempty"`
    Retention  *RetentionSpec   `json:"retention,omitempty"`
    Action     *ActionSpec      `json:"action,omitempty"`
    Safety     SafetySpec       `json:"safety"`
    Triggers   []TriggerSpec    `json:"triggers"`
}

type LifecycleStage struct {
    After  string `json:"after"` // Duration: "0d", "7d", "30d"
    Action string `json:"action"` // Report, Mark, Neutralize, Delete
}
```

### 4.3 JanitorFinding

Namespace-scoped (`janitor-system`). One finding per rule and resource family.

```go
type JanitorFindingSpec struct {
    Rule       RuleRef     `json:"rule"`
    Family     FamilyRef   `json:"family"`
    Condition  string      `json:"condition"`
    FirstObservedAt metav1.Time `json:"firstObservedAt"`
    Members    []ResourceRef `json:"members"`
}

type JanitorFindingStatus struct {
    Phase      string      `json:"phase"` // Active, Marked, Neutralized, Resolved
    Actions    []ActionRef `json:"actions,omitempty"`
}
```

### 4.4 JanitorAction

Namespace-scoped (`janitor-system`). One action per mutable target.

```go
type JanitorActionSpec struct {
    Rule        RuleRef      `json:"rule"`
    Target      TargetRef    `json:"target"` // includes UID + resourceVersion
    Operation   string       `json:"operation"` // Report, Mark, Neutralize, Restore, Delete
}

type JanitorActionStatus struct {
    Phase          string              `json:"phase"` // Pending, Validating, Applying, Verifying, Completed, Failed, Stale
    Implementation string              `json:"implementation,omitempty"`
    PriorState     *runtime.RawExtension `json:"priorState,omitempty"`
    ResultingState *runtime.RawExtension `json:"resultingState,omitempty"`
}
```

### 4.5 JanitorPolicyBundle

Cluster-scoped. Represents bundle revision state on the edge.

```go
type JanitorPolicyBundleStatus struct {
    ReceivedRevision   string `json:"receivedRevision,omitempty"`
    StagedRevision     string `json:"stagedRevision,omitempty"`
    ActiveRevision     string `json:"activeRevision,omitempty"`
    LastKnownGood      string `json:"lastKnownGood,omitempty"`
    ValidationState    string `json:"validationState,omitempty"`
}
```

---

## 5. Working Knowledge Model

The working knowledge index is **not** stored in CRDs. It lives in an in-memory map rebuilt from Kubernetes on restart. Only durable lifecycle clock state (first-observed timestamps) persists to SQLite.

### 5.1 ResourceRecord

```go
type ResourceRecord struct {
    Identity    ResourceIdentity
    Ownership   OwnershipResult
    Provenance  ProvenanceResult
    Relationships []Relationship
    Shape       ShapeAssignment
    Lifecycle   LifecycleState
}

type OwnershipResult struct {
    Classification string      // Managed, Inherited, AdHoc, Unknown, Orphaned, Conflicted, Excluded
    Owner          *OwnerRef
    Confidence     string      // Authoritative, Corroborating, Inferred
    Evidence       []Evidence
    TraversalPath  []ResourceRef
}

type LifecycleState struct {
    FirstObservedAt              time.Time
    FirstObservedUnmanagedAt     *time.Time
    CurrentConditionFirstSeenAt  *time.Time
    JanitorPhase                 string
}
```

### 5.2 ShapeRecord

```go
type ShapeRecord struct {
    ShapeID      string
    Role         string     // Controller, Operator, NodeSystem, Application, ScheduledWorkload, Unclassified
    Fingerprint  string     // sha256 of canonical representation
    RootRef      ResourceRef
    Members      []ResourceRef
    Traits       []string
    Instances    int
    ApprovedAt   *time.Time
}
```

---

## 6. Component Design

### 6.1 Resource Collector

- Uses controller-runtime informers for each configured GVK
- Collects: GVK, namespace, name, UID, resourceVersion, creationTimestamp, labels, annotations, managedFields, ownerReferences, configured spec fields
- Explicitly excludes: status health fields, pod readiness, restart counts
- Publishes add/update/delete events to the event router

### 6.2 Ownership Resolver

Ordered detector chain per resource:

1. Authoritative annotation/label detectors (ArgoCD tracking ID, Flux labels, Helm release secret)
2. Kubernetes ownerReference chain traversal (follows UID links)
3. Registered operator detection (custom ResourceOwner CRDs)
4. Corroborating evidence (app.kubernetes.io/managed-by, managedFields manager names)
5. Manual mutation evidence (kubectl, kubectl.kubernetes.io/last-applied-configuration)

Conflict detection: multiple authoritative claims → `Conflicted`
Dangling ownerReference → `Orphaned`
All evidence weak or absent → `Unknown`

Every result retains: detector used, source fields, confidence level, traversal path, conflicting evidence.

### 6.3 Relationship Graph

In-memory directed graph. Supported edge types:

| Type | Implementation |
|------|---------------|
| Owns | ownerReference |
| ManagedBy | recognized management owner |
| References | label selector match, field reference |
| Uses | serviceAccountName, configMapRef, secretRef |
| Selects | label selector → Pod/Deployment |
| Binds | RoleBinding → ServiceAccount |
| Mounts | VolumeMount → ConfigMap/Secret |
| BelongsToRelease | Helm release label family |
| ReachableFromRoot | graph traversal result |

### 6.4 Shape Engine

Pipeline per candidate root:

1. **Root identification**: Deployment, StatefulSet, DaemonSet, CronJob, Operator roots, registered root kinds
2. **Subgraph extraction**: BFS from root via relationship graph, bounded depth and fan-out
3. **Normalization**: remove name/UID/timestamp/hash/image-version/replica-count fields; retain kind, cardinality, relationship type, scope, traits
4. **Canonical representation**: sorted, deterministic JSON structure
5. **Fingerprint**: SHA-256 of canonical representation
6. **Classification**: exact match → approved ShapeRecord; no match → unclassified

Shape traits extracted per kind:
- `clusterScopedRBAC` — ClusterRole/ClusterRoleBinding present
- `leaderElection` — Lease in subgraph
- `exposesService` — Service present
- `ownsCustomResources` — CRD in subgraph
- `fixedScaling` — no HPA, replicas hard-coded
- `dedicatedServiceAccount` — non-default ServiceAccount
- `externalSecret` — ExternalSecret or sealed-secret present

### 6.5 Janitor Rule Engine

Evaluation sequence per rule:

1. Resolve target scope (GVK, namespace, selector, excluded namespaces)
2. Apply ownership/shape/provenance match conditions against working knowledge
3. Group into resource families if family groupBy defined
4. Apply cardinality/retention thresholds
5. Check lifecycle clock — has the condition been stable for the required duration?
6. Apply protections and administrator exceptions
7. Check policy posture ceiling
8. Create or resolve JanitorFinding
9. Request permitted lifecycle action via Action Reconciler

Evaluator types:
- `Ownership` — targets resources by ownership classification
- `ResourceRetention` — targets resource families for count/age pruning
- `ShapeConformance` — targets resources by shape role or fingerprint
- `MutationEvidence` — targets managed resources with external mutation detected

### 6.6 Action Reconciler

Before any mutation:

1. Re-fetch live resource from Kubernetes API
2. Verify UID matches (not a replacement)
3. Verify resourceVersion (precondition)
4. Verify policy posture permits the operation
5. Verify RBAC capability (attempt fails closed)
6. Re-evaluate rule match condition against live state
7. Check action rate limits (maxActionsPerRun, maxPercentagePerFamily)

Neutralization registry (extensible):

| Kind | Strategy |
|------|----------|
| CronJob | `spec.suspend: true` |
| Deployment | `spec.replicas: 0` (capture prior) |
| StatefulSet | `spec.replicas: 0` (capture prior) |
| Secret, ConfigMap | Not supported — fail closed |
| Custom resource | Registered handler required |

Delete preconditions:
- UID + resourceVersion precondition
- Posture must be `Collect`
- Rule re-evaluation must still match
- Policy revision must be valid

### 6.7 Communication Client

All connections edge-initiated. mTLS with client certificate.

Interactions:

```
POST /v1/edges/connect         — registration, returns edge token
POST /v1/edges/heartbeat       — operational state, not cluster health
GET  /v1/edges/{id}/messages   — long poll, ?after={seq}&wait=45
POST /v1/edges/acknowledgements — message ack
GET  /v1/policies/{digest}     — fetch policy bundle by content digest
POST /v1/edges/events          — batch reports (findings, shapes, ownership, actions)
```

Heartbeat payload:
```json
{
  "connectedToApi": true,
  "cacheSynchronized": true,
  "activePolicyRevision": "sha256:...",
  "lastEvaluationCompletedAt": "...",
  "pendingReports": 12
}
```

Controller-to-edge messages:
- `policy.available` — new bundle digest available
- `policy.activate` — activate staged bundle
- `rule.suspend` / `rule.resume`
- `rule.run` — immediate named-rule evaluation
- `inventory.refresh`
- `diagnostics.request`

The controller must not send resource-level mutation commands.

### 6.8 Policy Manager

Bundle lifecycle:

1. Pull bundle by digest from central
2. Verify HMAC signature and content digest
3. Validate schema and edge capability compatibility
4. Stage bundle (do not activate)
5. Activate on `policy.activate` message or auto-promote after configured delay
6. Preserve last-known-good on rollback or validation failure

### 6.9 Scheduler and Event Router

- Maintains per-GVK informer-driven indexes (not the working knowledge itself)
- Debounces knowledge-change events (default 30s) before triggering rule evaluation
- Schedules cron-triggered rules via a standard cron scheduler
- Runs periodic full reconciliation (default: every 4 hours) to repair missed events
- Leader election via Kubernetes Lease — only leader schedules evaluations and creates findings

### 6.10 Reporting Spool

Durable local spool using SQLite (same database as lifecycle clocks and findings).

Retained until central acknowledgement:
- Findings
- Shape summaries
- Ownership summaries
- Rule evaluation outcomes
- Action outcomes
- Policy acceptance events
- Edge operational state

Collapse policy: ownership summaries and operational state may be collapsed to latest; findings and actions must be delivered in full.

---

## 7. Central Service Design

### 7.1 Storage

PostgreSQL with JSONB for config fields.

Tables:
- `edges` — registration, identity, last heartbeat, capability
- `policy_bundles` — versioned bundles, signatures, targeting
- `policy_assignments` — desired vs active per edge
- `knowledge_events` — ingested finding/shape/ownership/action events
- `shape_catalog` — approved shapes, traits, fingerprints, roles
- `shape_instances` — per-cluster fingerprint counts
- `resource_history` — bounded retention of resource config snapshots
- `findings_history` — long-term finding history
- `action_history` — long-term action history

### 7.2 API Surface

```
POST /v1/edges/connect
POST /v1/edges/{id}/heartbeat
GET  /v1/edges/{id}/messages
POST /v1/edges/{id}/acknowledgements
POST /v1/edges/{id}/events
GET  /v1/policies/{digest}

POST /v1/admin/policies              — create/version policy bundle
PUT  /v1/admin/policies/{id}/sign    — sign bundle
POST /v1/admin/policies/{id}/target  — assign to edges/clusters
GET  /v1/fleet/edges                 — edge inventory
GET  /v1/fleet/shapes                — cross-cluster shape catalog
GET  /v1/fleet/findings              — aggregated findings
GET  /v1/fleet/ownership             — ownership coverage report
```

---

## 8. RBAC Design

Two ClusterRoles:

**edge-reader**: List/watch all configured GVKs. No mutation rights.

**edge-mutator**: Narrow patch/delete rights, only for kinds explicitly listed:
- Patch labels/annotations (Mark)
- Patch specific fields per registered neutralizer (Neutralize, Restore)
- Delete specific kinds with ownerReference or label selector scope (Delete)

Rule: `RBAC capability < posture ceiling < rule intent`

If RBAC denies an action, the action fails closed with a `Failed` status. It does not fall back to a lesser action.

---

## 9. Safety Invariants

These are non-negotiable in all phases:

1. A new policy bundle never replaces last-known-good until fully validated.
2. Destructive actions require a currently-valid policy revision.
3. The condition that triggered an action must remain stable through its grace period.
4. Every mutation revalidates the live resource state immediately before applying.
5. Delete requires UID + resourceVersion precondition.
6. Unsupported neutralizer fails closed — no generic field mutation.
7. Restore only applies if the resource UID matches, the neutralized state is present, and no newer actor changed the captured fields.
8. Shape similarity (non-exact) authorizes reporting only, never destructive action.
9. Rules cap absolute actions per run and optionally cap percentage per family.
10. The edge preserves last-known-good configuration indefinitely during central outages.
11. GitOps and autoscaler conflicts are surfaced as findings, not fought.

---

## 10. Delivery Phases

### Phase 1: Prove the Edge (open-source)

**Goal**: An autonomous edge that builds explainable cluster knowledge, runs observe-only rules, and is useful without any central dependency.

**License**: Open-source (Apache 2.0)

**Deliverables**:

| Area | Deliverables |
|------|-------------|
| Scaffold | Go module, CRD types, controller-runtime bootstrap, RBAC manifests, Makefile |
| Collection | Resource collector with configurable GVK list, informer indexes |
| Ownership | Kubernetes ownerReference traversal; Argo CD attribution (tracking ID + label detectors); Helm ownership (release Secret detection + family attribution) |
| Relationships | Graph with: Owns, ManagedBy, References, Uses, Selects, Binds, Mounts |
| Shapes | Exact canonical shape recognition with fingerprinting; shape trait extraction; instance counting |
| Knowledge | In-memory working knowledge index; SQLite persistence for lifecycle clocks |
| Janitor | Observe-only JanitorRule evaluation; JanitorFinding CRD; lifecycle clock tracking |
| CLI | `kos knowledge` — list resources with ownership; `kos shapes` — list shapes + instances; `kos findings` — list active findings; `kos report` — generate cluster knowledge report |
| Local API | HTTP API on localhost: GET /knowledge, GET /shapes, GET /findings, GET /report |
| Protocol | Open protocol types in `pkg/protocol`; mock center accepting heartbeat + events + policy pull |
| Testing | Mock center for protocol validation; sample clusters for shape recognition |

**Success criteria**:
- Edge reports recognized ownership percentage for a representative cluster
- Edge identifies exact shapes and instance counts by role (controller, operator, node-system, application, unclassified)
- CLI produces a human-readable cluster knowledge report
- Mock center receives and logs protocol payloads correctly
- Observe-only rules create findings for unmanaged resources

---

### Phase 2: Prove Fleet Value (commercial self-hosted)

**Goal**: A self-hosted Central that aggregates fleet knowledge, distributes policy, and provides governance visibility.

**License**: Commercial (separate repository)

**Deliverables**:

| Area | Deliverables |
|------|-------------|
| Central Service | Go-based REST API, PostgreSQL storage, policy store |
| Fleet Inventory | Edge registration, heartbeat, knowledge ingestion |
| Shape Comparison | Cross-cluster shape catalog, divergence detection |
| Policy | Policy authoring, signing, versioning, targeting, distribution |
| Governance | Findings reporting, action visibility, posture enforcement |
| Mark/Neutralize | Mark action (labels/annotations); registered neutralizers (CronJob, Deployment, StatefulSet); action preconditions |
| JanitorAction CRD | Full action lifecycle with prior-state capture |
| Reporting | Ownership coverage, shape compression, ad hoc detection, action outcomes |
| Identity | OIDC integration, role-based access control |
| Deployment | Helm chart for self-hosted installation |
| Edge upgrades | Policy manager with bundle validation, staging, activation, rollback |
| Leader election | Edge HA via Kubernetes Lease |
| Reporting spool | SQLite-backed durable spool with acknowledgement protocol |

**Success criteria**:
- Central shows fleet-level ownership coverage and shape breakdown
- Policy distributed to edges and activated without downtime
- Enforcement posture correctly limits actions per cluster
- Helm history pruned to configured retention on non-production
- Neutralized resources correctly restored
- Delete executes after all preconditions on Collect-posture clusters

---

### Phase 3: Launch SaaS

**Goal**: A managed SaaS version of Central with elastic analytics, shape benchmarks, and compliance automation.

**License**: SaaS (multi-tenant, managed)

**Deliverables**:

| Area | Deliverables |
|------|-------------|
| Core parity | All Phase 2 Central features in managed infrastructure |
| Managed ops | Automated upgrades, managed retention, backup |
| Analytics | Elastic analytics backend (ClickHouse or TimescaleDB); shape evolution tracking; historical trend queries |
| Benchmarks | Optional anonymized cross-tenant shape benchmarks |
| Compliance | Automated compliance packages (ownership coverage, drift detection, shape conformance) |
| Migration | Self-hosted-to-SaaS migration path (re-point edge certificates) |
| Multi-tenancy | Tenant isolation, per-tenant RBAC, usage-based billing |
| SLAs | Uptime SLA, data retention SLA, support tiers |

**Success criteria**:
- Self-hosted customer migrates to SaaS by rotating edge certificate and endpoint
- Analytics queries return sub-second for fleets of 50+ clusters
- Compliance reports auto-generate on schedule
- Shape benchmarks show percentile comparison across opted-in tenants

---

## 11. Open Questions Requiring Early Decisions

The following open questions from the spec affect Phase 1 and must be resolved before implementation begins:

| Question | Impact |
|----------|--------|
| Which GVKs are collected by default? | Collector configuration |
| Are Pods excluded or retained for ownership traversal only? | Collector + graph |
| Which relationship types are supported initially beyond ownerReference? | Graph design |
| Where are first-observed lifecycle clocks stored? (SQLite proposed above) | Persistence design |
| Can centrally managed and locally authored ResourceOwner/JanitorRule objects coexist? | Policy model |
| Which Argo CD tracking modes must be supported? | Ownership detector |
| Which payload format: JSON or Protobuf? | Communication protocol |
| How is stable cluster identity established and bootstrap enrollment performed? | Edge identity |

---

## 12. Key Dependencies

### Edge (open-source)

```go
// go.mod (partial)
require (
    sigs.k8s.io/controller-runtime v0.19.0
    k8s.io/client-go v0.31.0
    k8s.io/apimachinery v0.31.0
    k8s.io/api v0.31.0
    sigs.k8s.io/controller-tools v0.16.0  // controller-gen
    modernc.org/sqlite v1.30.0            // Pure-Go SQLite (no CGO)
    github.com/go-chi/chi/v5 v5.1.0      // Local API
    github.com/spf13/cobra v1.8.0         // CLI
    github.com/sirupsen/logrus v1.9.3
)
```

**Why modernc.org/sqlite**: Pure-Go SQLite — cross-compiles without CGO, works in scratch/distroless containers, single-binary deployment.

### Central (commercial, Phase 2+)

```go
require (
    github.com/go-chi/chi/v5 v5.1.0
    github.com/lib/pq v1.10.9             // PostgreSQL
    github.com/sirupsen/logrus v1.9.3
    github.com/coreos/go-oidc/v3 v3.9.0   // OIDC identity
)
```

---

## 13. What Does Not Belong in This System

Per the spec, explicitly excluded from implementation scope:
- Pod health, readiness, restart counts, liveness
- Job success/failure detection
- Application traffic monitoring
- Logs, metrics, traces
- Runtime threat detection
- Vulnerability or malware scanning
- Network intrusion detection
- Determining workload maliciousness
- Central execution of resource-level Kubernetes mutations
- Pre-delete snapshot storage (deferred, not in MVP)
