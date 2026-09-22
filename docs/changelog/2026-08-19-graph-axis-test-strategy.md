# Graph Axis Test Strategy

## Purpose

Define the test strategy for the Graph axis — the relationship model that connects observed Kubernetes resources, enables dependency traversal, determines blast radius, and provides the execution ordering knowledge required by the Janitor safety model.

The Graph axis is the foundation for:
- Dependency closure (what is reachable from a resource)
- Blast radius (what consumers are affected by a change)
- Teardown ordering (which resources must be removed first)
- Janitor qualification (shared resources, unknown relationships, cycle detection)
- Shape composition (which relationships define a structural fingerprint)

## Scope

### What the Graph axis produces

| Capability | Implementation |
|-----------|---------------|
| Directed relationship edges | `graph.Build(index)` → structural + provenance edges |
| Edge confidence grading | ExplicitField, OwnerReference, SelectorMatch |
| Forward traversal (reachable) | `graph.Reachable(source, depth)` |
| Reverse traversal (ancestors) | `graph.Ancestors(target, depth)` |
| Per-resource relationship listing | `kos relationships <kind> <name> -n <ns>` |
| Full graph listing | `kos relationships` |
| JSON export with metadata | `kos graph export` |
| Dependency DAG for Janitor | `janitor.BuildDependencyDAG(key, graph)` |
| Cycle detection | `janitor.DetectCycles(edges)` |
| Action closure expansion | `janitor.BuildActionClosure(key, graph, index)` |
| Deletion qualification | `janitor.QualifyDeletion(closure, key, graph, index)` |

### Relationship types under test

| Category | Types | Source |
|----------|-------|--------|
| Structural | UsesServiceAccount, SelectsWorkload, BindsSubject, GrantsRole, ClaimsStorage, UsesHeadlessService, Mounts, References | Explicit spec fields, selector matching |
| Framework | Owns | ownerReferences |
| Provenance | BelongsToRelease, ManagedBy | Helm labels, ArgoCD annotations |
| Grouping | MemberOf, MemberOfRelease | Application group membership |
| Authority | Reconciles, Generates, Provisions | ArgoCD Application/ApplicationSet CRs |
| Classification | ClassifiedAs | Shape matcher results |

## Test Levels

### Level 1: Unit tests (`internal/edge/graph/`)

**Graph data structure operations:**

| Test | Description | Status |
|------|-------------|--------|
| GRAPH-UNIT-001 | AddEdge creates directed edge, queryable from both ends | Done |
| GRAPH-UNIT-002 | No duplicate edges for same source/target/type | Done |
| GRAPH-UNIT-003 | RemoveNode removes all incoming and outgoing edges | Done |
| GRAPH-UNIT-004 | EdgeCount and NodeCount are consistent | Done |
| GRAPH-UNIT-005 | Reachable BFS respects maxDepth | Done |
| GRAPH-UNIT-006 | Reachable from leaf node returns empty | Done |
| GRAPH-UNIT-007 | Ancestors reverse-BFS finds upstream providers | Done |
| GRAPH-UNIT-008 | EdgesOfType filters correctly | Needed |
| GRAPH-UNIT-009 | AllEdges returns complete edge set | Needed |
| GRAPH-UNIT-010 | Concurrent read safety (RWMutex) | Needed |

**Relationship builder tests:**

| Test | Description | Status |
|------|-------------|--------|
| GRAPH-BUILD-001 | UsesServiceAccount from explicit spec field | Needed |
| GRAPH-BUILD-002 | UsesServiceAccount not created when SA doesn't exist in index | Needed |
| GRAPH-BUILD-003 | SelectsWorkload from Service selector to matching workload | Needed |
| GRAPH-BUILD-004 | SelectsWorkload not created for non-matching selectors | Needed |
| GRAPH-BUILD-005 | BindsSubject from RoleBinding to ServiceAccount | Needed |
| GRAPH-BUILD-006 | GrantsRole from RoleBinding to Role/ClusterRole | Needed |
| GRAPH-BUILD-007 | ClaimsStorage from StatefulSet to PVC | Needed |
| GRAPH-BUILD-008 | UsesHeadlessService from StatefulSet to headless Service | Needed |
| GRAPH-BUILD-009 | Mounts from workload volumes to ConfigMap | Needed |
| GRAPH-BUILD-010 | Mounts from envFrom.configMapRef | Needed |
| GRAPH-BUILD-011 | References from workload to Secret (volumes) | Needed |
| GRAPH-BUILD-012 | References from env.valueFrom.secretKeyRef | Needed |
| GRAPH-BUILD-013 | Owns from ownerReference on child resource | Needed |
| GRAPH-BUILD-014 | BelongsToRelease from Helm release label detection | Needed |
| GRAPH-BUILD-015 | Reconciles from ArgoCD Application to managed resources | Needed |
| GRAPH-BUILD-016 | Generates from ApplicationSet to Application | Needed |
| GRAPH-BUILD-017 | Cross-namespace BindsSubject (ClusterRoleBinding → namespaced SA) | Needed |
| GRAPH-BUILD-018 | Multiple ConfigMaps mounted by one workload | Needed |
| GRAPH-BUILD-019 | Shared ConfigMap referenced by multiple workloads | Needed |
| GRAPH-BUILD-020 | No edge created for dangling reference (target not in index) | Needed |

### Level 2: Traversal correctness tests

| Test | Description | Status |
|------|-------------|--------|
| GRAPH-TRAV-001 | Reachable from Deployment reaches SA, ConfigMaps, Secret, ReplicaSet | Needed |
| GRAPH-TRAV-002 | Reachable from Service reaches selected Deployment and its dependencies | Needed |
| GRAPH-TRAV-003 | Ancestors of ConfigMap finds all workloads that mount it | Needed |
| GRAPH-TRAV-004 | Depth limit prevents traversal beyond specified depth | Done |
| GRAPH-TRAV-005 | Cycle in graph does not cause infinite traversal | Needed |
| GRAPH-TRAV-006 | Provenance edges (BelongsToRelease) are traversable | Needed |
| GRAPH-TRAV-007 | Authority edges (Reconciles) are traversable | Needed |
| GRAPH-TRAV-008 | MemberOf edges are traversable (for shape instance membership) | Needed |
| GRAPH-TRAV-009 | Reachable excludes the source node itself | Needed |
| GRAPH-TRAV-010 | Ancestors excludes the target node itself | Needed |

### Level 3: Janitor integration tests

| Test | Description | Status |
|------|-------------|--------|
| GRAPH-JAN-001 | BuildDependencyDAG includes only teardown-relevant relationships | Done |
| GRAPH-JAN-002 | Authority edges (Reconciles, Generates) excluded from DAG | Done |
| GRAPH-JAN-003 | DetectCycles identifies circular dependencies | Done |
| GRAPH-JAN-004 | DetectCycles returns false for acyclic DAG | Done |
| GRAPH-JAN-005 | BuildActionClosure includes Owns-chain descendants | Done |
| GRAPH-JAN-006 | BuildActionClosure excludes persistent data | Done |
| GRAPH-JAN-007 | QualifyDeletion fails on unaccounted dependent | Done |
| GRAPH-JAN-008 | QualifyDeletion fails on partial shape deletion | Done |
| GRAPH-JAN-009 | QualifyDeletion fails on unknown relationship type | Done |
| GRAPH-JAN-010 | QualifyDeletion passes for isolated resource with no consumers | Done |
| GRAPH-JAN-011 | ComputeDeletionOrder produces consumers-before-providers ordering | Done |
| GRAPH-JAN-012 | HasBlockingDependencies identifies consumers outside closure | Needed |
| GRAPH-JAN-013 | QualifyDeletion fails when shared resource has external consumers | Needed |
| GRAPH-JAN-014 | Safety walk returns Protected for Reconciles edge | Done |
| GRAPH-JAN-015 | Safety walk returns Indeterminate when graph unavailable | Done |

### Level 4: CLI integration tests

| Test | Description | Status |
|------|-------------|--------|
| GRAPH-CLI-001 | `kos relationships` lists all edges with correct format | Needed |
| GRAPH-CLI-002 | `kos relationships Deployment <name> -n <ns>` shows outgoing and incoming | Needed |
| GRAPH-CLI-003 | `kos reachable <kind> <name> -n <ns>` shows transitive closure | Needed |
| GRAPH-CLI-004 | `kos reachable --depth 1` limits traversal depth | Needed |
| GRAPH-CLI-005 | `kos graph export` produces valid JSON with nodes, edges, metadata | Needed |
| GRAPH-CLI-006 | `kos graph export` edge types are correct and categories sum to total | Needed |
| GRAPH-CLI-007 | `kos describe resource` shows incoming and outgoing relationships | Needed |
| GRAPH-CLI-008 | `kos report` relationship counts match `kos relationships` footer | Needed |

### Level 5: Adversarial and edge cases

| Test | Description | Status |
|------|-------------|--------|
| GRAPH-ADV-001 | Self-referencing resource (ConfigMap referencing itself) | Needed |
| GRAPH-ADV-002 | Circular ownerReference chain | Needed |
| GRAPH-ADV-003 | Resource referencing non-existent target (dangling ref) | Needed |
| GRAPH-ADV-004 | Very large fan-out (100+ edges from one Service) | Needed |
| GRAPH-ADV-005 | Cross-namespace reference (ClusterRoleBinding → namespaced SA) | Needed |
| GRAPH-ADV-006 | Cluster-scoped resource with no namespace | Needed |
| GRAPH-ADV-007 | Resource with multiple ownerReferences | Needed |
| GRAPH-ADV-008 | Empty cluster (no resources) → empty graph | Needed |
| GRAPH-ADV-009 | Resource deleted between graph build and traversal | Needed |
| GRAPH-ADV-010 | Two Services selecting the same workload | Needed |

## Fixtures Required

### Controlled fixtures (existing)

| Fixture | Graph coverage |
|---------|---------------|
| fixture-simple-a | Deployment → ConfigMap (References), Service → Deployment (SelectsWorkload) |
| fixture-simple-c | Adds RBAC: Role, RoleBinding, ServiceAccount + BindsSubject, GrantsRole, UsesServiceAccount |
| fixture-stateful | StatefulSet → Service (UsesHeadlessService), StatefulSet → PVC (ClaimsStorage) |
| fixture-adv-disconnected | ConfigMap with no structural edges (tests disconnected detection) |
| fixture-adv-unmounted | ConfigMap exists but not Mounted (tests relationship accuracy) |

### Additional fixtures needed

| Fixture | Purpose |
|---------|---------|
| fixture-shared-config | One ConfigMap mounted by 3 Deployments (blast radius, shared resource qualification) |
| fixture-cross-namespace | ClusterRoleBinding referencing SA in different namespace (cross-namespace edge) |
| fixture-deep-chain | 5-level ownerReference chain (depth-limited traversal) |
| fixture-circular-dep | Resources with mutual references (cycle detection) |
| fixture-multi-selector | Two Services selecting the same Deployment (fan-in) |

## Confidence Model Verification

Each relationship must be tested for its confidence grading:

| Confidence | Source | Verification |
|-----------|--------|-------------|
| ExplicitField | Direct spec field reference (serviceAccountName, volumes[].configMap.name) | Value matches the exact spec path |
| OwnerReference | metadata.ownerReferences[].uid | UID exists in cluster |
| SelectorMatch | spec.selector matches pod template labels | Label-set intersection produces match |

Tests must verify:
- Confidence is correctly assigned per relationship type
- No NamingConvention or LabelAssociation confidence is assigned (KOS does not use these)
- ExplicitField edges include the specific source field in Evidence

## Graph Export Verification

The `kos graph export` JSON must satisfy:

| Property | Verification |
|----------|-------------|
| Node count | 509 resources + 26 group nodes = 535 |
| Edge count | Structural (279) + Provenance (18) + Grouping (410) + Classification (27) = 734 |
| Categories are mutually exclusive | No edge appears in two categories |
| Every edge source exists as a node | No dangling source references |
| Every edge target exists as a node | No dangling target references |
| Snapshot metadata present | Model version, observation timestamp, cluster ID |

## Teardown Semantics Verification

The Janitor's dependency DAG uses only a subset of relationship types. Tests must verify:

| Relationship | Teardown contribution | Ordering |
|-------------|----------------------|----------|
| Mounts | Hard dependency | Consumer (workload) before provider (ConfigMap) |
| References | Hard dependency | Consumer (workload) before provider (Secret) |
| UsesServiceAccount | Hard dependency | Consumer (workload) before provider (SA) |
| SelectsWorkload | Informational for teardown | Service removal does not require Deployment removal |
| BindsSubject | Hard dependency | Binding before subject |
| GrantsRole | Hard dependency | Binding before role |
| ClaimsStorage | Hard dependency | Workload before PVC |
| UsesHeadlessService | Hard dependency | StatefulSet before Service |
| Owns | Cascading (K8s GC) | Not explicit in execution DAG |
| Reconciles | Authority (blocks, not orders) | Never enters teardown DAG |
| Generates | Authority (blocks, not orders) | Never enters teardown DAG |
| BelongsToRelease | Provenance boundary | Never enters teardown DAG |
| MemberOf | Grouping | Never enters teardown DAG |

## Exit Criteria

The Graph axis is considered hardened when:

1. All Level 1 unit tests pass (GRAPH-UNIT-001 through 010, GRAPH-BUILD-001 through 020)
2. All Level 2 traversal tests pass (GRAPH-TRAV-001 through 010)
3. All Level 3 Janitor integration tests pass (GRAPH-JAN-001 through 015)
4. All Level 4 CLI tests pass (GRAPH-CLI-001 through 008)
5. At least 5 adversarial cases pass (GRAPH-ADV subset)
6. Graph export JSON validates against the category reconciliation
7. Teardown semantics verified for all 13 relationship types
8. No graph operation causes panic or data race under concurrent access
9. Traversal terminates for any graph structure (no infinite loops)
10. Relationship builder produces no false positives against controlled fixtures

## Implementation Priority

| Priority | Tests | Rationale |
|----------|-------|-----------|
| 1 (immediate) | GRAPH-BUILD-001 through 020 | Builder correctness is the foundation |
| 2 (next) | GRAPH-TRAV-001 through 010 | Traversal correctness enables CLI and Janitor |
| 3 (important) | GRAPH-CLI-001 through 008 | User-facing validation |
| 4 (hardening) | GRAPH-ADV-001 through 010 | Robustness under unusual conditions |
| 5 (ongoing) | GRAPH-JAN-012, 013 | Janitor safety model completeness |
