# Kube Open Shape Edge — Implementation Plan

## Overview

This plan breaks the open-source edge controller into manageable implementation phases. Each phase produces a testable, demonstrable increment. The edge is useful at every phase boundary — not only after all phases complete.

## Testing Requirements

**Every phase must include integration tests** in `test/integration/` that:

1. Create a dedicated namespace (`kos-integration`)
2. Apply test fixture resources
3. Run `kos` CLI commands against the live cluster
4. Verify output matches expectations
5. Tear down resources and namespace on completion (via `t.Cleanup`)

Integration tests run via `make test-integration` and require a live Kubernetes cluster (kind or similar). Tests are skipped gracefully when no cluster is available.

Unit tests cover internal logic. Integration tests prove the feature works end-to-end against real Kubernetes resources.

---

## Delivery Summary

| Phase | Name | Focus | Testable Outcome |
|-------|------|-------|-----------------|
| 1 | Scaffold + Collection | Project structure, informers, raw resource index | CLI lists all observed resources with GVK/namespace/name |
| 2 | Ownership | Ownership resolver, Argo CD + Helm + K8s ownerRef detection | CLI shows ownership classification per resource |
| 3 | Relationships | Relationship graph construction | CLI shows resource relationships and reachability |
| 4 | Shapes | Shape engine, CRD-driven definitions, fingerprinting | CLI shows shape inventory with instance counts |
| 4b | Shape Grouping (Exact) | Unclassified → candidate families via deterministic fingerprints | CLI shows candidate groups with common core |
| 4c | Shape Grouping (Similarity) | Near-shape clustering, ranking, definition generation | CLI generates draft ShapeDefinitions from candidates |
| 4d | Candidate Affinity | Working classifications for candidates before formal shape promotion | CLI records/displays affinities, persists to SQLite |
| 5 | Knowledge API | Local HTTP API + SQLite persistence + CLI reporting | Full knowledge queryable via API and CLI |
| 6 | Janitor Rules | Observe-only rule evaluation, findings, lifecycle clocks | Findings created for unmanaged resources |
| 7 | Protocol + Mock Center | Communication client, open protocol, mock central | Edge sends heartbeat and events to mock center |
| 8 | Packaging + Docs | Helm chart, container image, RBAC, documentation | `helm install` deploys edge to any cluster |

---

## Phase 1: Scaffold + Collection

**Goal**: Project scaffolding, Kubernetes informers running, raw resource data indexed in memory.

### Deliverables

- Go module (`go.mod`) with controller-runtime, client-go
- `cmd/edge/main.go` — controller-runtime manager bootstrap with leader election disabled (for now)
- `api/v1alpha1/` — empty CRD type scaffolding (types registered, no CRDs deployed yet)
- `internal/edge/collector/` — Resource collector
  - Configurable GVK watch list (YAML config)
  - Informer factory per GVK
  - On add/update/delete → store in in-memory index
  - Collects: GVK, namespace, name, UID, resourceVersion, creationTimestamp, labels, annotations, managedFields, ownerReferences
  - Excludes: status fields, pod containers, health data
- `internal/edge/knowledge/` — In-memory ResourceIndex
  - `types.go` — ResourceIdentity, ResourceRecord (identity fields only at this phase)
  - `index.go` — Thread-safe map, Get/List/ByNamespace/ByGVK
- `cli/` — Basic CLI
  - `kos resources` — list all indexed resources (table: GVK, namespace, name, age)
  - `kos resources --kind Deployment` — filter by kind
  - `kos resources --namespace payments` — filter by namespace
- Default GVK watch list: Deployments, StatefulSets, DaemonSets, CronJobs, Jobs, Services, ConfigMaps, Secrets, ServiceAccounts, ClusterRoles, ClusterRoleBindings, Roles, RoleBindings, Ingresses, NetworkPolicies, PersistentVolumeClaims, Namespaces, CustomResourceDefinitions

### Testing

- Unit tests: index CRUD, GVK filtering
- Integration test: start manager against envtest, verify resources appear in index
- Manual: deploy to kind cluster, run `kos resources`, verify output matches `kubectl get all`

### Done When

- `kos resources` prints a table of all watched resources in a live cluster
- Count matches what Kubernetes reports for those GVKs

---

## Phase 2: Ownership

**Goal**: Determine who manages each resource. Argo CD, Helm, and Kubernetes ownerReference attribution.

### Deliverables

- `internal/edge/ownership/` — Ownership resolver
  - `types.go` — OwnershipResult, Evidence, OwnerRef, classifications (Managed, Inherited, AdHoc, Unknown, Orphaned, Conflicted, Excluded)
  - `resolver.go` — Ordered detector chain, processes each resource
  - `detectors/` — Pluggable detector implementations:
    - `ownerref.go` — Follow Kubernetes ownerReference chains to management root
    - `argocd.go` — Detect `argocd.argoproj.io/tracking-id` annotation, `argocd.argoproj.io/instance` label
    - `helm.go` — Detect Helm release Secrets (`owner=helm` label), attribute resources via `app.kubernetes.io/managed-by: Helm` + `helm.sh/release-name`
    - `managedfields.go` — Extract manager names from managedFields, detect kubectl/manual evidence
  - Each detector returns: evidence type, source field, confidence, authoritative flag
- `internal/edge/ownership/chain.go` — ownerReference chain traversal (follows UID links until root)
- ResourceRecord extended with `Ownership` field
- `kos ownership` CLI command — table: resource, classification, owner, confidence, evidence
- `kos ownership --classification AdHoc` — filter
- `kos ownership --summary` — counts per classification

### Key Design Decisions

- Detectors run in priority order: Argo CD → Helm → ownerReference chain → managedFields
- First authoritative match wins; corroborating evidence is accumulated
- Conflicting authoritative claims → Conflicted classification
- ownerReference to non-existent UID → Orphaned
- Only managedFields kubectl evidence + no other authoritative → AdHoc
- Every result retains: detector, source fields, confidence, traversal path

### Testing

- Unit tests: each detector against mock resource fixtures
- Unit tests: chain traversal with multi-level ownership (Deployment → ReplicaSet → Pod)
- Unit tests: conflict detection, orphan detection
- Integration: envtest with Argo CD-annotated resources, verify classifications

### Done When

- `kos ownership --summary` shows percentage breakdown by classification
- Argo CD-managed resources detected as Managed/Authoritative
- Helm-managed resources detected as Managed
- Resources owned via ownerReference chains detected as Inherited
- kubectl-created resources without other ownership detected as AdHoc

---

## Phase 3: Relationships

**Goal**: Build a directed graph of resource relationships beyond ownerReference.

### Deliverables

- `internal/edge/graph/` — Relationship graph
  - `types.go` — RelationType enum (Owns, ManagedBy, References, Uses, Selects, Binds, Mounts, BelongsToRelease, ReachableFromRoot)
  - `graph.go` — In-memory directed graph (adjacency list), add/remove edges, query by source/target/type
  - `builder.go` — Constructs graph from resource index:
    - ownerReferences → `Owns` edges
    - Resolved ownership → `ManagedBy` edges
    - ServiceAccount references → `Uses` edges
    - ConfigMap/Secret volume references → `Mounts` edges
    - Label selector matches → `Selects` edges
    - RoleBinding → ServiceAccount → `Binds` edges
    - ConfigMapRef/SecretRef in env → `References` edges
    - Helm release label → `BelongsToRelease` edges
  - `traversal.go` — BFS/DFS from any resource; reachability queries; path finding
- ResourceRecord extended with `Relationships []Relationship` field
- `kos relationships {kind} {namespace} {name}` — show edges for a resource
- `kos relationships --type Uses --namespace payments` — filter
- `kos reachable {kind} {namespace} {name}` — show all resources reachable from a root

### Testing

- Unit tests: graph add/remove/query operations
- Unit tests: builder against mock resource sets (Deployment with SA, ConfigMap, Service)
- Unit tests: traversal BFS/DFS, cycle handling, fan-out limits
- Integration: envtest with linked resources, verify graph completeness

### Done When

- `kos relationships Deployment payments payment-api` shows ServiceAccount, ConfigMap, Secret, Service connections
- `kos reachable Deployment argocd argocd-server` shows full subgraph reachable from ArgoCD server

---

## Phase 4: Shapes

**Goal**: Recognize structural patterns using declarative `ShapeDefinition` and `RelationshipDefinition` CRDs. The Go engine is a generic graph matcher — all domain intelligence lives in CRDs.

See: [Shape Definition Specification v2](2026-08-18-shape-definition-spec.md)

### Deliverables

- `api/v1alpha1/relationshipdefinition_types.go` and `shapedefinition_types.go`
- `config/crd/` — generated CRD manifests
- `config/samples/relationships/` — default RelationshipDefinitions
- `config/samples/shapes/` — default ShapeDefinitions
- `internal/edge/graph/relationships.go` — apply RelationshipDefinitions to construct edges
- `internal/edge/shape/compiler.go` — compile ShapeDefinitions, cache by generation
- `internal/edge/shape/matcher.go` — root selection, component matching, relationship verification, cardinality, constraints, traits
- `internal/edge/shape/cel.go` — restricted CEL environment
- `internal/edge/shape/canonical.go` — canonicalization per profile
- `internal/edge/shape/fingerprint.go` — SHA-256 of canonical + fingerprint traits
- CLI: `kos shapes`, `kos shapes definitions`, `kos shapes evaluate --explain`

### Testing

- Unit tests: RelationshipDefinition field resolution, compiler, matcher, CEL, canonicalization, fingerprint
- Integration: deploy definitions to cluster, verify shapes, custom override, conflict detection

### Done When

- `kos shapes` shows CRD-driven classifications
- `kos shapes evaluate --explain` shows match/reject reasoning
- Custom ShapeDefinition with higher priority overrides defaults
- Conflicted status reported when same-priority definitions match

---

## Phase 4b: Intelligent Shape Grouping — Exact

**Goal**: Organize unclassified resources into candidate structural families using deterministic fingerprinting.

See: [Intelligent Shape Grouping](2026-08-18-intelligent-shape-grouping.md)

### Deliverables

- `internal/edge/shape/segment.go` — graph segmentation of unclassified resources into candidate subgraphs
- `internal/edge/shape/generic_fingerprint.go` — anonymous canonicalization profile (`generic-structural-v1`)
- `internal/edge/shape/candidates.go` — exact fingerprint grouping, CandidateShapeGroup model
- `internal/edge/store/candidates.go` — SQLite storage for candidate groups and members
- CLI:
  - `kos shapes candidates` — list exact candidate groups with instance counts and confidence
  - `kos shapes candidate explain {id}` — show common core, variable components, member resources

### Testing

- Unit tests: graph segmentation produces correct subgraphs
- Unit tests: generic fingerprint is deterministic
- Unit tests: identical structures group together, different structures don't
- Integration: deploy multiple similar resources, verify they group into one candidate

### Done When

- `kos shapes candidates` shows grouped unclassified inventory
- Explanation shows common core and variable components with frequencies
- Identical structures produce same candidate group fingerprint

---

## Phase 4c: Intelligent Shape Grouping — Similarity and Generation

**Goal**: Cluster near-shapes, rank candidates, generate draft ShapeDefinitions.

### Deliverables

- `internal/edge/shape/similarity.go` — weighted structural distance model
- `internal/edge/shape/ranking.go` — candidate priority ranking (support, cohesion, spread)
- `internal/edge/shape/generate.go` — draft ShapeDefinition YAML generation from candidate
- `internal/edge/shape/validate.go` — dry-run definition test against live inventory
- `internal/edge/store/lineage.go` — candidate lineage tracking (split, merge, evolve)
- CLI:
  - `kos shapes candidate generate {id}` — produce draft ShapeDefinition YAML
  - `kos shapes definition test {file}` — dry-run: what would it classify?
  - `kos shapes candidates --confidence stable` — filter by confidence category

### Testing

- Unit tests: similarity model produces expected distances
- Unit tests: ranking sorts candidates correctly
- Unit tests: generated YAML is valid ShapeDefinition
- Integration: deploy similar-but-not-identical resources, verify similarity clustering
- Integration: generated definition dry-run produces expected classification

### Done When

- Near-shapes cluster into candidate families with explainable differences
- `kos shapes candidate generate` produces valid ShapeDefinition YAML
- `kos shapes definition test` shows would-classify counts and coverage percentage
- Confidence categories correctly distinguish exact, stable, probable, singleton, residue

---

## Phase 4d: Candidate Affinity

**Goal**: Allow operators to record working classifications for candidates before formal shape promotion. Persist affinities to SQLite. Display affinities in CLI and API.

### Deliverables

1. **Affinity data model** — `CandidateAffinity` struct with candidate ID, role, affinity, proposedName, confidence, rationale, source, observedAt.
2. **SQLite persistence** — `candidate_affinities` table storing assessments with history (multiple per candidate, timestamped).
3. **CLI: `kos candidates affinity set`** — Record an affinity assessment for a candidate.
4. **CLI: `kos candidates affinity list`** — Show affinities for a candidate or all candidates with affinities.
5. **API: `POST /api/v1/candidates/{id}/affinity`** — Record affinity via API.
6. **API: `GET /api/v1/candidates/{id}/affinity`** — Retrieve affinity history for a candidate.
7. **Integration with `kos candidates` output** — Show affinity column when affinities exist.

### Data Model

```go
type CandidateAffinity struct {
    CandidateID  string    // stable candidate identifier
    Role         string    // broad structural category
    Affinity     string    // archetype or resemblance
    ProposedName string    // optional working name
    Confidence   string    // Tentative, Likely
    Rationale    string    // human-readable reasoning
    Source       string    // Operator, AutoSuggestion
    ObservedAt   time.Time
}
```

### Invariants

- Recording an affinity must NOT alter the candidate's fingerprint.
- Affinities must NOT cause candidates to appear as named shapes.
- Multiple affinities per candidate are permitted (uncertainty is expected).
- Human vs automated assessments remain distinguishable via `source`.
- Affinity revision preserves prior assessments (append-only history).
- Affinities must NOT independently qualify resources for Janitor actions.

### CLI Interaction

```bash
# Record an affinity
kos candidates affinity set candidate-26b64f33bd03 \
  --role controller \
  --affinity "API Controller" \
  --confidence Tentative \
  --rationale "Long-running workload with RBAC and Service"

# List affinities
kos candidates affinity list

# Show affinities for one candidate
kos candidates affinity list candidate-26b64f33bd03
```

### Integration Tests

- Set affinity → verify persisted to SQLite.
- Set affinity → verify candidate fingerprint unchanged.
- Set multiple affinities → verify all visible.
- List affinities → verify grouping by role/affinity.
- Restart edge → verify affinities survive.
- Candidate with affinity → verify not treated as named shape.

---

## Phase 5: Knowledge API

**Goal**: Local HTTP API, SQLite persistence for lifecycle state, full CLI reporting.

### Deliverables

- `internal/edge/store/` — SQLite store
  - `schema.go` — Tables: lifecycle_clocks, findings, actions, spool_messages
  - `lifecycle.go` — first-observed timestamps per resource, per condition
  - Single `.db` file in configurable data directory
- `internal/edge/api/` — Local HTTP API (go-chi, localhost:9090 default)
  - `GET /api/v1/knowledge` — list resources with ownership, shape, lifecycle
  - `GET /api/v1/knowledge/{namespace}/{kind}/{name}` — single resource detail
  - `GET /api/v1/shapes` — shape catalog
  - `GET /api/v1/shapes/{shapeId}` — shape detail with members
  - `GET /api/v1/ownership/summary` — classification breakdown
  - `GET /api/v1/relationships/{namespace}/{kind}/{name}` — resource relationships
  - `GET /api/v1/report` — full cluster knowledge report (JSON)
- CLI updates:
  - `kos report` — generates and prints a structured cluster knowledge report
  - `kos report --format json` — machine-readable output
  - `kos report --format text` — human-readable summary
  - All existing commands use local API when available (falls back to in-process)
- Lifecycle clock tracking:
  - Records first time a resource is observed in each ownership classification
  - Persists to SQLite so clocks survive edge restart
  - Used by janitor rules in Phase 6

### Testing

- Unit tests: SQLite store CRUD, lifecycle clock persistence
- Integration: API endpoint tests against running edge
- CLI tests: verify output format matches API responses

### Done When

- `curl localhost:9090/api/v1/knowledge | jq .` returns full knowledge index
- `kos report` produces a readable cluster knowledge summary
- Lifecycle clocks persist across edge pod restart

---

## Phase 6: Janitor Rules (Observe-Only)

**Goal**: Rule evaluation engine, findings, lifecycle grace periods. No mutations.

### Deliverables

- `api/v1alpha1/` — CRD types deployed:
  - `ResourceOwner` — defines recognized management owners
  - `JanitorRule` — defines rules (observe-only lifecycle stages for this phase)
  - `JanitorFinding` — durable finding record
- `config/crd/` — Generated CRD manifests
- `config/rbac/` — ClusterRole for read-only edge
- `config/samples/` — Example ResourceOwner (ArgoCD, Helm), example JanitorRule (unmanaged-resources)
- `internal/edge/janitor/` — Rule engine
  - `types.go` — evaluator types, match spec, lifecycle stages
  - `engine.go` — loads rules from cluster, schedules evaluation
  - `evaluator_ownership.go` — Ownership evaluator (matches by classification)
  - `evaluator_retention.go` — ResourceRetention evaluator (matches by family count/age)
  - `lifecycle.go` — lifecycle clock evaluation (condition stable for N days?)
  - `findings.go` — creates/updates JanitorFinding CRs in janitor-system namespace
- `internal/edge/scheduler/` — Scheduler
  - Cron trigger support
  - KnowledgeChange trigger with debounce (default 30s)
  - Full reconciliation on schedule (default every 4 hours)
  - Leader election via Kubernetes Lease (only leader evaluates)
- CLI:
  - `kos findings` — list active findings
  - `kos findings --rule unmanaged-resources` — filter
  - `kos rules` — list configured rules and last evaluation time
- API updates:
  - `GET /api/v1/findings` — list findings
  - `GET /api/v1/rules` — list rules with evaluation status

### Key Constraints

- This phase is **observe-only**: maximum lifecycle action is `Report`
- No mutations to target resources
- No Mark, Neutralize, or Delete
- Findings are informational — they surface what rules would eventually act upon

### Testing

- Unit tests: evaluator matching logic, lifecycle clock progression
- Unit tests: rule loading and scheduling
- Integration: deploy sample ResourceOwner + JanitorRule to cluster, verify findings created
- Integration: verify findings resolve when condition clears

### Done When

- JanitorFinding CRs appear in `janitor-system` for resources matching rule conditions
- `kos findings` shows active findings with age and lifecycle stage
- Findings resolve automatically when the matching condition is no longer true
- Lifecycle clocks correctly track condition duration across evaluations

---

## Phase 7: Protocol + Mock Center

**Goal**: Open communication protocol, edge-initiated reporting, mock central for development.

### Deliverables

- `pkg/protocol/` — Open protocol types (shared package)
  - `types.go` — Heartbeat, Event, PolicyBundle, Message, Acknowledgement
  - `events.go` — OwnershipSummary, ShapeSummary, FindingEvent, ActionEvent
  - Wire format: JSON (version-tagged)
- `internal/edge/comm/` — Communication client
  - `client.go` — HTTP client, mTLS support, edge-initiated connections
  - `heartbeat.go` — periodic heartbeat (configurable interval, default 60s)
  - `events.go` — batch event upload (findings, shapes, ownership summaries)
  - `policy.go` — policy bundle pull by digest
  - `messages.go` — long-poll for controller messages
  - `spool.go` — SQLite-backed outbound spool, retry on failure, acknowledgement tracking
- `internal/edge/policy/` — Policy manager
  - `manager.go` — pull, verify digest, validate schema, stage, activate, rollback
  - `bundle.go` — bundle content model (ResourceOwners + JanitorRules + Posture)
  - Last-known-good preservation
- `cmd/mock-center/` — Mock central server
  - Accepts: /v1/edges/connect, /heartbeat, /events, /messages, /acknowledgements
  - Serves: /v1/policies/{digest} (static file-backed bundles)
  - Logs all received payloads to stdout (structured JSON)
  - Optional: writes received events to local JSON files for inspection
- Configuration:
  - Edge config gains `central.endpoint`, `central.tls.certFile`, `central.tls.keyFile`
  - Default: no central configured (edge runs fully autonomous)
- CLI:
  - `kos status` — shows edge operational state (connected, last heartbeat, policy revision, spool depth)

### Testing

- Unit tests: spool persistence, retry logic, acknowledgement
- Unit tests: policy bundle validation, staging, activation, rollback
- Integration: edge + mock center running together, verify heartbeat and event delivery
- Integration: policy pull from mock center, verify activation on edge

### Done When

- Edge sends heartbeats to mock center at configured interval
- Edge uploads ownership summaries, shape summaries, and findings as events
- Policy bundles served by mock center are pulled, validated, and activated by edge
- Spool correctly queues events during mock center downtime and replays on reconnect
- `kos status` shows connected state and policy revision

---

## Phase 8: Packaging + Documentation

**Goal**: Production-ready container image, Helm chart, RBAC, documentation.

### Deliverables

- `Dockerfile` — multi-stage build, scratch/distroless final image
- `Makefile` — targets: build, test, lint, docker-build, docker-push, generate (CRDs), install
- Helm chart (`deploy/helm/kube-open-shape/`):
  - Deployment with configurable replicas (leader election handles HA)
  - ServiceAccount + ClusterRole + ClusterRoleBinding (read-only by default)
  - ConfigMap for edge configuration (GVK watch list, central endpoint, intervals)
  - Optional PVC for SQLite persistence (or emptyDir with lifecycle clock loss on restart)
  - Optional Secret for mTLS certificates
  - CRD installation via helm hooks
- RBAC manifests:
  - `edge-reader` ClusterRole: list/watch for all configured GVKs
  - `edge-writer` ClusterRole: create/update JanitorFinding in janitor-system
  - Future: `edge-mutator` ClusterRole (Phase 2 product — not in open-source Phase 1)
- Documentation:
  - `docs/README.md` — project overview, quick start, architecture
  - `docs/getting-started.md` — install via Helm, first `kos` commands
  - `docs/configuration.md` — GVK list, intervals, central endpoint, TLS
  - `docs/ownership.md` — how ownership detection works, adding custom ResourceOwners
  - `docs/shapes.md` — how shapes are recognized, interpreting shape output
  - `docs/rules.md` — writing JanitorRules, lifecycle stages, safety
  - `docs/protocol.md` — open protocol specification for center implementors
  - `docs/contributing.md` — development setup, running tests, adding detectors
- GitHub Actions CI:
  - Lint, test, build on PR
  - Docker build + push on tag
  - CRD generation validation

### Testing

- Helm chart linting (`helm lint`)
- Helm template rendering tests
- End-to-end: `helm install` on kind cluster, verify edge starts and produces findings
- RBAC: verify edge cannot mutate resources (only observe)

### Done When

- `helm install kos ./deploy/helm/kube-open-shape` deploys a working edge
- `kos report` produces a cluster knowledge report within 5 minutes of install
- Documentation is sufficient for a Kubernetes operator to install, configure, and interpret output
- CI passes on a clean PR

---

## Phase Dependency Graph

```
Phase 1 (Scaffold)
    └── Phase 2 (Ownership)
            └── Phase 3 (Relationships)
                    └── Phase 4 (Shapes)
                            └── Phase 5 (Knowledge API)
                                    └── Phase 6 (Janitor Rules)
                                            └── Phase 7 (Protocol)
                                                    └── Phase 8 (Packaging)
```

Each phase depends on the previous one. However, **Phase 5 (Knowledge API) and Phase 7 (Protocol) can be developed in parallel** once Phase 4 is complete — the API and protocol are independent consumers of the same knowledge index.

---

## Estimated Effort

| Phase | Estimated Duration | Cumulative |
|-------|-------------------|-----------|
| 1 — Scaffold + Collection | 1–2 weeks | 2 weeks |
| 2 — Ownership | 2–3 weeks | 5 weeks |
| 3 — Relationships | 1–2 weeks | 7 weeks |
| 4 — Shapes | 2–3 weeks | 10 weeks |
| 5 — Knowledge API | 1–2 weeks | 12 weeks |
| 6 — Janitor Rules | 2–3 weeks | 15 weeks |
| 7 — Protocol + Mock Center | 2 weeks | 17 weeks |
| 8 — Packaging + Docs | 1–2 weeks | 19 weeks |

Total: approximately 16–19 weeks for a complete open-source edge.

---

## Open Questions for Phase 1

These should be resolved before starting implementation:

| # | Question | Default Proposal |
|---|----------|-----------------|
| 1 | Which GVKs to watch by default? | List in Phase 1 deliverables above |
| 2 | Are Pods watched? | No — only traversed via ownerReference for ownership resolution |
| 3 | CLI binary name? | `kos` |
| 4 | SQLite file location? | `/var/lib/kos/knowledge.db` (configurable) |
| 5 | Local API port? | `9090` (configurable) |
| 6 | JSON or YAML for CLI config? | YAML |
| 7 | Log library? | logrus (structured JSON in production) |
| 8 | Minimum Kubernetes version? | 1.27+ |

---

## Future Work

### CLI Output Formatting

Support kubectl-compatible structured output options in the global `--output` / `-o` flag:

- `-o jsonpath='{.nodes[*].resource.name}'` — JSONPath expressions against structured output
- `-o custom-columns=NAME:.resource.name,KIND:.resource.kind,NS:.resource.namespace` — custom column definitions
- `-o jsonpath-file=template.jsonpath` — JSONPath template from file

These require all commands to produce a consistent internal representation that can be projected through JSONPath or column templates before rendering. Implementation approach:

1. Each command produces a typed result struct (already done for JSON output).
2. A shared output formatter inspects `-o` value and dispatches to the appropriate renderer.
3. JSONPath uses `k8s.io/client-go/util/jsonpath` for compatibility with kubectl expressions.
4. Custom-columns parser splits the column spec and evaluates each JSONPath per row.

This unifies tabular, JSON, YAML, detail, jsonpath, and custom-columns into one output pipeline.

### kubectl-Compatible Global Flags

Add the following persistent flags to align with standard kubectl conventions:

| Flag | Short | Description |
|------|-------|-------------|
| `--selector` | `-l` | Filter resources by label selector (e.g., `-l app=nginx`) |
| `--context` | | Override kubeconfig context |
| `--kubeconfig` | | Path to kubeconfig file |
| `--sort-by` | | JSONPath expression for sorting results (e.g., `--sort-by=.metadata.creationTimestamp`) |
| `--field-selector` | | Filter by field (e.g., `--field-selector metadata.namespace=default`) |
| `--show-labels` | | Include labels column in tabular output |
| `--no-headers` | | Suppress column headers in tabular output |

Implementation notes:

- `--context` and `--kubeconfig` wire into `clientcmd.NewNonInteractiveDeferredLoadingClientConfig` overrides. Apply before any cluster connection.
- `--selector` / `-l` applies in-memory filtering against `ResourceRecord.Labels` using `k8s.io/apimachinery/pkg/labels` selector parsing.
- `--field-selector` applies in-memory against resource identity fields (kind, namespace, name, apiGroup).
- `--sort-by` requires a JSONPath evaluator over the internal result representation.
- `--show-labels` appends a LABELS column to tabular output with `key=value` pairs.
- `--no-headers` suppresses the header row in all tabwriter-based output.

These are global persistent flags applied uniformly across all commands.

### Multi-Manager Release Model

`kos releases` should normalize release identity across all Kubernetes deployment managers rather than being shaped around Helm semantics.

#### Supported managers

| Manager | Identity source | Revision concept |
|---------|----------------|-----------------|
| Helm | Release Secret, labels, annotations | Chart version, app version, Helm revision number |
| Argo CD | Application CR | Repository, path, target revision, resolved commit, sync status |
| Flux HelmRelease | HelmRelease CR | Chart reference, chart version, observed generation |
| Flux Kustomization | Kustomization CR | GitRepository, path, branch/tag, resolved commit |
| Operator | Custom resource | Operator version, generation |
| Raw manifest | Source bundle or commit if known | — |

#### Generic release model

KOS normalizes only concepts common across managers:

- Release identity
- Manager type and name
- Scope (namespace)
- Source/artifact reference
- Desired revision
- Resolved revision
- Manager-reported status
- Current resource set
- Logical application association
- Observed history

Provider-specific information remains attached but does not define the common model.

#### Default output

```
RELEASE           NAMESPACE         MANAGER  REVISION      STATUS    APPLICATION
argocd            argocd            Helm     1             deployed  argocd
cert-manager      cert-manager      Helm     1             deployed  cert-manager
payments-prod     payments          ArgoCD   a83f21c       synced    payments
platform-config   platform-system   Flux     main@21ab95   ready     platform
```

#### Wide output

```
RELEASE  NAMESPACE  MANAGER  REVISION  STATUS  SOURCE  UPDATED  MANAGED  INHERITED  APPLICATION  LAST CHANGE
```

SOURCE is a generic human-readable projection:
- Helm: `chart:argo-cd@10.4.0`
- Argo CD Git: `git:platform-config/apps/argocd@a83f21c`
- Argo CD Helm: `helm:argo-cd@10.4.0`
- Flux: `git:cluster-config/platform@21ab95`
- Operator: `crd:PostgresCluster/payments-db@generation-7`

#### Structured output

Detailed source fields available in JSON/YAML:

```json
{
  "manager": {"type": "Helm"},
  "source": {
    "type": "HelmChart",
    "name": "argo-cd",
    "version": "10.4.0",
    "appVersion": "v3.5.1"
  },
  "revision": {"managerRevision": "1"}
}
```

For Argo CD:

```json
{
  "manager": {"type": "ArgoCD", "name": "platform-argocd"},
  "source": {
    "type": "Git",
    "repository": "https://github.com/example/platform-config",
    "path": "apps/payments",
    "targetRevision": "main"
  },
  "revision": {"desired": "main", "resolved": "a83f21c"}
}
```

#### Describe is manager-aware

`kos describe releases argocd -n argocd` renders manager-specific detail:

Helm:
```
Manager:       Helm
Chart:         argo-cd
Chart Version: 10.4.0
App Version:   v3.5.1
Helm Revision: 1
```

Argo CD:
```
Manager:           Argo CD
Repository:        platform-config
Path:              apps/payments
Target Revision:   main
Resolved Revision: a83f21c
Sync Status:       Synced
```

#### History normalization

History means KOS-observed release progression, not only native Helm revisions:

```
TIME  MANAGER REVISION  SOURCE REVISION  RESOURCE DELTA  STRUCTURAL DELTA
```

A Helm revision, Git commit, Argo sync revision, or Kubernetes generation can all identify a transition.

#### Design principles

- Chart information belongs in Helm-specific detail and structured source metadata, not a required top-level column.
- KOS should not become a multi-manager wrapper shaped around Helm semantics.
- The generic default is: RELEASE | NAMESPACE | MANAGER | REVISION | STATUS | APPLICATION
- Source, resource counts, and time information go in `-o wide` and manager-specific describe output.

#### Implementation scope

Phase 1 (current): Helm-only release detection via labels and annotations.

Phase 2 (future): Add Argo CD Application CR watching, extract release identity from Application spec.

Phase 3 (future): Add Flux HelmRelease/Kustomization CR watching, normalize with generic model.

Each manager integration requires:
1. Adding the manager's CRD to the watch list.
2. A release extractor that maps CR fields to the generic release model.
3. A describe renderer for manager-specific detail.
4. History tracking via observed CR generation/revision transitions.

---

## Implementation Gaps — Identified via Testing (2026-08-19)

### Structure Axis

| ID | Gap | Severity | Test Reference |
|----|-----|----------|----------------|
| SHAPE-GAP-001 | Matcher does not exclude ownerRef-bearing resources from root selection. A ReplicaSet with a controller ownerReference can be matched as a shape root alongside its Deployment. | High | STRUCT-ADV-004 / `TestAdversarial_FrameworkResourceNotRoot` |
| SHAPE-GAP-002 | Generic fingerprint does not differentiate empty member sets from populated member sets. A bare Deployment with no components groups with a Deployment that has Service+ConfigMap+Secret. | Medium | STRUCT-ADV-001 / `TestAdversarial_RootKindAloneDoesNotGroup` |
| SHAPE-GAP-002b | Candidate fingerprints are non-deterministic across API requests. The relationship graph traversal uses map iteration, causing fingerprint instability between calls. Shapes endpoint is stable; candidates endpoint is not. | High | `TestEdgeAPI_Determinism/candidates_response_stable` / `TestCLI_Candidates/fingerprints_are_stable` |
| SHAPE-GAP-003 | No reverse traversal from resource to shape. `kos describe resource` does not show which shape instance a resource belongs to or which alias it fills. | Medium | STRUCT-REV-001 |
| SHAPE-GAP-004 | No named shape definitions exist. Only role classifiers (application, node-system) are loaded. True named shapes with relationship requirements, components, and alias bindings are not yet defined. | Low (design phase) | STRUCT-TAX-001 |
| SHAPE-GAP-005 | Candidate generation (`kos generate`) does not validate that the generated definition matches its source instances. No dry-run validation. | Low | STRUCT-GEN-002 |
| SHAPE-GAP-006 | Matcher does not evaluate relationship requirements for Structural mode definitions. Named shapes with `relationships[]` are registered but components are not verified against the graph. Root kind matching works; relationship-based component binding does not. | High | STRUCT-MATCH fixture-stateful / `kos-stateful-application` |

### Ownership Axis

| ID | Gap | Severity | Test Reference |
|----|-----|----------|----------------|
| OWN-GAP-001 | Helm DIRECT attribution inflated for ReplicaSets. Resources with inherited labels matched Helm rules before owner-chain propagation corrected attribution. Fixed in Phase D. | Resolved | — |
| OWN-GAP-002 | Authority record Secrets inflated managed-resource counts in summary. Fixed by excluding AuthorityRecord layer from counts. | Resolved | — |
| OWN-GAP-003 | Three generated Secrets (argocd-redis, cert-manager-webhook-ca, ingress-nginx-admission) remain unattributed. Require controller relationship or hook evidence. | Low | — |
| OWN-GAP-004 | Helm manifest membership extraction not implemented. Current engine relies on labels/annotations only. True manifest membership requires decoding Helm release Secret data (HelmManifestExtractor). | Medium | Ownership engine Phase C spec |

### Graph Axis

| ID | Gap | Severity | Test Reference |
|----|-----|----------|----------------|
| GRAPH-GAP-001 | envFrom ConfigMap references should produce `References` relationship, not `Mounts`. Fixed 2026-08-19. | Resolved | — |
| GRAPH-GAP-002 | No cross-namespace relationship support. Services with ExternalName or cross-namespace references are not modeled. | Low | — |

### Recommended Fix Priority

1. **SHAPE-GAP-002b** — Sort graph edges and member lists before fingerprint computation. Candidate fingerprints must be deterministic regardless of map iteration order. Without this, candidate IDs change between API calls.
2. **SHAPE-GAP-001** — Filter ownerRef-bearing resources from root selection in `matcher.go`. High impact: prevents framework resources from appearing as independent structural instances.
3. **SHAPE-GAP-002** — Include member-count or member-kind-set in the fingerprint hash. Prevents false grouping of bare roots with composed applications.
4. **OWN-GAP-004** — Implement HelmManifestExtractor to decode release Secrets and emit `release.manifestMember` facts for authoritative Helm membership.
5. **SHAPE-GAP-003** — Add shape membership to `kos describe resource` output.
6. **SHAPE-GAP-006** — Implement relationship evaluation in the matcher for Structural mode. The matcher currently only selects roots by kind; it must also verify that required relationships exist between root and component aliases using the graph.
7. **SHAPE-GAP-004** — Define at least one named shape (e.g., "Stateful Application" with StatefulSet + headless Service + PVC requirements) to exercise full structural matching.

---

## Consolidated Priorities — Remaining Work

### Tier 1: Correctness ~~(fix before new features)~~ — COMPLETED 2026-08-19

| # | Item | Axis | Status |
|---|------|------|--------|
| 1 | ~~SHAPE-GAP-002b: Deterministic candidate fingerprints~~ | Structure | **Done** — roots sorted before processing, members sorted in subgraph |
| 2 | ~~SHAPE-GAP-001: Exclude ownerRef-bearing resources from root selection~~ | Structure | **Done** — `hasControllerOwner()` filter in SegmentUnclassified |
| 3 | ~~SHAPE-GAP-002: Fingerprint differentiates member composition~~ | Structure | **Done** — Kinds map properly populated, different compositions produce different FPs |

### Tier 2: Completeness ~~(fill functional gaps)~~ — COMPLETED 2026-08-19

| # | Item | Axis | Status |
|---|------|------|--------|
| 4 | ~~OWN-GAP-004: HelmManifestExtractor~~ | Ownership | **Done** — verifies release Secret existence, emits `release.manifestMember` facts |
| 5 | ~~SHAPE-GAP-003: Resource-to-shape reverse traversal~~ | Structure | **Done** — `kos describe resource` shows Shape section |
| 6 | ~~SHAPE-GAP-004: First named shape definition~~ | Structure | **Done** — `kos-stateful-application` (StatefulSet+Service+PVC with required relationships) |
| 6b | ~~SHAPE-GAP-006: Matcher relationship evaluation~~ | Structure | **Done** — `resolveAliasKeys` resolves root alias from definition spec |
| 7 | ~~Phase 4d: Candidate affinity~~ | Structure | **Done** — SQLite persistence, `kos candidates affinity set/list`, shown in listings |

### Tier 3: New capabilities

| # | Item | Axis | Status |
|---|------|------|--------|
| 8 | ~~API ownership endpoint migration~~ | API | **Done** — `/api/v1/ownership/summary` uses new engine, returns authority-centric data |
| 8b | ~~API shapes endpoint fix~~ | API | **Done** — `/api/v1/shapes` returns shape data, `/api/v1/candidates` returns candidates with instances |
| 8c | ~~Bidirectional graph traversal for candidates~~ | Structure | **Done** — RBAC chain (ClusterRole, ClusterRoleBinding) now discovered via ancestors |
| 8d | ~~Candidate listing with PRIMARY/SUPPORTING/CONTEXT~~ | Structure | **Done** — replaces single CORE column, adds AFFINITY and RELATIONSHIPS (wide) |
| 8e | ~~Generate output as valid YAML with comment context~~ | Structure | **Done** — stdout is pipeable, context as # comments, affinity shown |
| 9 | Phase 6: Janitor Rules — observe-only rule evaluation, findings | Janitor | Not started |
| 10 | Release manager extensibility — ArgoCD Application CR watching | Deployment | Not started |
| 11 | Phase 7: Protocol + Mock Center — communication client | Protocol | Not started |
| 12 | Phase 8: Packaging — Helm chart, container image, RBAC | Operations | Not started |

### Tier 4: Design work (specification needed before implementation)

| # | Item | Axis | Status |
|---|------|------|--------|
| 13 | ~~Ownership engine spec (Phases A–D)~~ | Ownership | **Done** — fact model, catalogs, rules, resolution, extractors all implemented |
| 14 | Graph axis test strategy and traversal specification | Graph | Not started |
| 15 | Janitor safety model — qualification rules, blast radius, fail-closed semantics | Janitor | Not started |
| 16 | Fleet protocol specification — heartbeat, events, aggregation | Protocol | Not started |

### Current State Summary (2026-08-19)

| Axis | Implementation Status | Test Coverage | Remaining Gaps |
|------|----------------------|---------------|----------------|
| Organization | Complete | 22 integration tests pass | Minor: JSON/YAML output for all commands |
| Ownership | Engine complete (Phases A–D), CLI migrated | 11 engine unit + 20 CLI integration | 3 generated Secrets unattributed, 14 namespaces legitimately unknown |
| Structure | Matcher + candidates + named shapes + affinity working | 22 unit + 20 CLI + 16 API integration | No additional named shapes beyond Stateful Application |
| Graph | Bidirectional traversal, RBAC chain discovery | Covered via shape matching + relationship tests | Cross-namespace references |
| Deployment | Helm release extraction working | Covered via ownership tests | ArgoCD/Flux manager support |
| Janitor | Rule engine scaffolded | 3 edge API tests | No observe-only rule evaluation yet |

