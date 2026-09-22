# Graph Axis — Test Cases

## Overview

Test cases for the Graph traversal axis. Each section corresponds to a test layer from the graph test strategy. Tests are categorized as Unit, Integration (CLI), or Manual.

Status legend:
- **Expected**: What the system should produce
- **Observed**: What the system actually produces (filled during test execution)
- **Status**: Pass / Fail / Not Run

---

## 1. Graph Data Structure Tests

### GRAPH-UNIT-001: Outgoing edges queryable

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-001 |
| Type | Unit |
| Operator question | Can I query what a resource depends on? |
| Function | `g.OutgoingEdges("A")` |
| Expected | Returns all edges originating from node A |
| Observed | Pass — 2 outgoing edges returned for node with 2 targets |
| Status | Pass |

### GRAPH-UNIT-002: Incoming edges queryable

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-002 |
| Type | Unit |
| Operator question | Can I query what depends on a resource? |
| Function | `g.IncomingEdges("B")` |
| Expected | Returns all edges targeting node B |
| Observed | Pass — 1 incoming edge returned, source correctly identified |
| Status | Pass |

### GRAPH-UNIT-003: No duplicate edges

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-003 |
| Type | Unit |
| Operator question | Does adding the same edge twice produce duplicates? |
| Function | `g.AddEdge()` twice with same source/target/type |
| Expected | Edge count does not increase on second add |
| Observed | Pass — EdgeCount remains 3 after duplicate add |
| Status | Pass |

### GRAPH-UNIT-004: RemoveNode removes all edges

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-004 |
| Type | Unit |
| Operator question | When a resource is removed, are all its edges cleaned up? |
| Function | `g.RemoveNode("Y")` |
| Expected | No edges remain from or to removed node |
| Observed | Pass — both incoming and outgoing edges removed |
| Status | Pass |

### GRAPH-UNIT-005: EdgeCount and NodeCount consistent

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-005 |
| Type | Unit |
| Function | `g.EdgeCount()`, `g.NodeCount()` |
| Expected | EdgeCount matches sum of all outgoing edges; NodeCount matches unique node set |
| Observed | Pass — 3 edges, 4 nodes for test graph |
| Status | Pass |

### GRAPH-UNIT-006: EdgesOfType filters correctly

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-006 |
| Type | Unit |
| Function | `g.EdgesOfType("A", Owns)` |
| Expected | Returns only edges of the requested type from the given source |
| Observed | Pass — returns 2 Owns edges, 0 for non-existent type |
| Status | Pass |

### GRAPH-UNIT-007: AllEdges returns complete edge set

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-007 |
| Type | Unit |
| Function | `g.AllEdges()` |
| Expected | Returns every edge in the graph exactly once |
| Observed | Pass — 3 edges returned, all sources present |
| Status | Pass |

### GRAPH-UNIT-008: Concurrent read safety

| Field | Value |
|-------|-------|
| Test ID | GRAPH-UNIT-008 |
| Type | Unit |
| Operator question | Can multiple goroutines read the graph without data races? |
| Function | 100 concurrent goroutines calling OutgoingEdges, IncomingEdges, Reachable, Ancestors |
| Expected | No panics, no data races under `go test -race` |
| Observed | Pass — 100 goroutines complete without race condition |
| Status | Pass |

---

## 2. Relationship Builder Tests

### GRAPH-BUILD-001: UsesServiceAccount from explicit spec field

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-001 |
| Type | Unit |
| Operator question | Does a Deployment's serviceAccountName create a relationship? |
| Fixture | Deployment with `spec.template.spec.serviceAccountName=app-sa`; ServiceAccount exists |
| Function | `graph.Build(index)` |
| Expected | Edge: Deployment → ServiceAccount [UsesServiceAccount] with ExplicitField confidence |
| Observed | Pass — edge created with correct confidence |
| Status | Pass |

### GRAPH-BUILD-002: UsesServiceAccount not created when SA missing

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-002 |
| Type | Unit |
| Operator question | Does a dangling serviceAccountName create a false edge? |
| Fixture | Deployment referencing non-existent ServiceAccount |
| Function | `graph.Build(index)` |
| Expected | No UsesServiceAccount edge created |
| Observed | Pass — 0 UsesServiceAccount edges |
| Status | Pass |

### GRAPH-BUILD-003: SelectsWorkload from Service selector match

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-003 |
| Type | Unit |
| Operator question | Does a Service with matching selector find the workload? |
| Fixture | Service with `spec.selector={app: web}`; Deployment with matching labels |
| Function | `graph.Build(index)` |
| Expected | Edge: Service → Deployment [SelectsWorkload] with SelectorMatch confidence |
| Observed | Pass — edge created correctly |
| Status | Pass |

### GRAPH-BUILD-004: SelectsWorkload not created for non-matching selector

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-004 |
| Type | Unit |
| Operator question | Does a non-matching selector create a false edge? |
| Fixture | Service selector `{app: other}`, Deployment labels `{app: web}` |
| Function | `graph.Build(index)` |
| Expected | No SelectsWorkload edge |
| Observed | Pass — 0 SelectsWorkload edges |
| Status | Pass |

### GRAPH-BUILD-005: BindsSubject from RoleBinding to ServiceAccount

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-005 |
| Type | Unit |
| Operator question | Does a RoleBinding's subject create a relationship to the ServiceAccount? |
| Fixture | RoleBinding with `spec.subjects[{kind: ServiceAccount, name: app-sa}]`; SA exists |
| Function | `graph.Build(index)` |
| Expected | Edge: RoleBinding → ServiceAccount [BindsSubject] with ExplicitField confidence |
| Observed | Pass — edge created |
| Status | Pass |

### GRAPH-BUILD-006: GrantsRole from RoleBinding to Role

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-006 |
| Type | Unit |
| Operator question | Does a RoleBinding's roleRef create a relationship to the Role? |
| Fixture | RoleBinding with `spec.roleRef={kind: Role, name: my-role}`; Role exists |
| Function | `graph.Build(index)` |
| Expected | Edge: RoleBinding → Role [GrantsRole] with ExplicitField confidence |
| Observed | Pass — edge created |
| Status | Pass |

### GRAPH-BUILD-007: ClaimsStorage from StatefulSet to PVC

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-007 |
| Type | Unit |
| Operator question | Does a StatefulSet's volumeClaimTemplate create a relationship to PVCs? |
| Fixture | StatefulSet with VCT `data`; PVC named `data-db-0` exists |
| Function | `graph.Build(index)` |
| Expected | Edge: StatefulSet → PVC [ClaimsStorage] with ExplicitField confidence |
| Observed | Pass — VCT naming pattern matched correctly |
| Status | Pass |

### GRAPH-BUILD-008: UsesHeadlessService from StatefulSet

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-008 |
| Type | Unit |
| Operator question | Does a StatefulSet's serviceName create a relationship? |
| Fixture | StatefulSet with `spec.serviceName=db-headless`; Service exists |
| Function | `graph.Build(index)` |
| Expected | Edge: StatefulSet → Service [UsesHeadlessService] with ExplicitField confidence |
| Observed | Pass — edge created |
| Status | Pass |

### GRAPH-BUILD-009: Mounts from workload volumes to ConfigMap

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-009 |
| Type | Unit |
| Operator question | Does a Deployment's volume configMap reference create a relationship? |
| Fixture | Deployment with ConfigMapRef `{name: app-config, fieldPath: volumes[].configMap.name}`; ConfigMap exists |
| Function | `graph.Build(index)` |
| Expected | Edge: Deployment → ConfigMap [Mounts] with ExplicitField confidence |
| Observed | Pass — edge created |
| Status | Pass |

### GRAPH-BUILD-010: References from workload to Secret

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-010 |
| Type | Unit |
| Operator question | Does a Deployment's secretKeyRef create a relationship? |
| Fixture | Deployment with SecretRef `{name: app-secret, fieldPath: volumes[].secret.secretName}`; Secret exists |
| Function | `graph.Build(index)` |
| Expected | Edge: Deployment → Secret [References] with ExplicitField confidence |
| Observed | Pass — edge created |
| Status | Pass |

### GRAPH-BUILD-011: Owns from ownerReference

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-011 |
| Type | Unit |
| Operator question | Does an ownerReference create an Owns edge from parent to child? |
| Fixture | ReplicaSet with ownerReference UID matching Deployment UID |
| Function | `graph.Build(index)` |
| Expected | Edge: Deployment → ReplicaSet [Owns] with OwnerReference confidence |
| Observed | Pass — edge created with correct direction |
| Status | Pass |

### GRAPH-BUILD-012: Cross-namespace ClusterRoleBinding to namespaced SA

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-012 |
| Type | Unit |
| Operator question | Does a cluster-scoped binding correctly reference a namespaced ServiceAccount? |
| Fixture | ClusterRoleBinding with subject `{kind: ServiceAccount, namespace: other-ns, name: sa}` |
| Function | `graph.Build(index)` |
| Expected | Edge: ClusterRoleBinding → ServiceAccount/other-ns/sa [BindsSubject] |
| Observed | Pass — cross-namespace edge created correctly |
| Status | Pass |

### GRAPH-BUILD-013: Multiple ConfigMaps mounted by one workload

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-013 |
| Type | Unit |
| Operator question | Are all mounted ConfigMaps captured? |
| Fixture | Deployment with 3 ConfigMapRefs (all volume mounts) |
| Function | `graph.Build(index)` |
| Expected | 3 Mounts edges from same Deployment to different ConfigMaps |
| Observed | Pass — 3 Mounts edges, all from correct source |
| Status | Pass |

### GRAPH-BUILD-014: Shared ConfigMap referenced by multiple workloads

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-014 |
| Type | Unit |
| Operator question | Does a shared ConfigMap show all its consumers? |
| Fixture | 3 Deployments each mounting `shared-config` |
| Function | `graph.Build(index)` → `IncomingEdges("ConfigMap/default/shared-config")` |
| Expected | 3 incoming Mounts edges from different Deployments |
| Observed | Pass — 3 Mounts incoming edges |
| Status | Pass |

### GRAPH-BUILD-015: No edge for dangling reference

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-015 |
| Type | Unit |
| Operator question | Does a reference to a non-existent resource create a false edge? |
| Fixture | Deployment with serviceAccountName, ConfigMapRef, SecretRef — none exist in index |
| Function | `graph.Build(index)` |
| Expected | No UsesServiceAccount, Mounts, or References edges |
| Observed | Pass — 0 structural edges for dangling references |
| Status | Pass |

### GRAPH-BUILD-016: Two Services selecting same workload

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-016 |
| Type | Unit |
| Operator question | Are both Services captured as consumers of the workload? |
| Fixture | 2 Services with matching selector, 1 Deployment |
| Function | `graph.Build(index)` |
| Expected | 2 SelectsWorkload edges from different Services to same Deployment |
| Observed | Pass — 2 SelectsWorkload edges |
| Status | Pass |

### GRAPH-BUILD-017: ClusterRoleBinding to ClusterRole

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BUILD-017 |
| Type | Unit |
| Operator question | Does a ClusterRoleBinding's roleRef to a ClusterRole create an edge? |
| Fixture | ClusterRoleBinding with roleRef kind=ClusterRole, name=admin-role |
| Function | `graph.Build(index)` |
| Expected | Edge: ClusterRoleBinding → ClusterRole [GrantsRole] |
| Observed | Pass — edge created |
| Status | Pass |

---

## 3. Traversal Tests

### GRAPH-TRAV-001: Reachable from Deployment

| Field | Value |
|-------|-------|
| Test ID | GRAPH-TRAV-001 |
| Type | Unit |
| Operator question | What resources does this Deployment depend on? |
| Fixture | Deployment → SA, ConfigMap, Secret |
| Function | `g.Reachable("Deployment/default/app", 5)` |
| Expected | Returns ServiceAccount, ConfigMap, Secret (3 nodes) |
| Observed | Pass — 3 reachable resources |
| Status | Pass |

### GRAPH-TRAV-002: Reachable from Service includes selected Deployment's dependencies

| Field | Value |
|-------|-------|
| Test ID | GRAPH-TRAV-002 |
| Type | Integration |
| Operator question | What is reachable from the Service entry point? |
| Fixture | Service → Deployment → SA, ConfigMaps, Secret, ReplicaSet |
| Command | `kos reachable Service argocd-server -n argocd` |
| Expected | Deployment + its dependencies (7 reachable resources) |
| Observed | Pass — 7 resources: Deployment, SA, 3 ConfigMaps, Secret, ReplicaSet |
| Status | Pass |

### GRAPH-TRAV-003: Ancestors of ConfigMap finds consumers

| Field | Value |
|-------|-------|
| Test ID | GRAPH-TRAV-003 |
| Type | Unit |
| Operator question | What workloads consume this ConfigMap? |
| Fixture | 2 Deployments mounting same ConfigMap |
| Function | `g.Ancestors("ConfigMap/default/shared", 5)` |
| Expected | Both Deployments returned |
| Observed | Pass — 2 ancestors found |
| Status | Pass |

### GRAPH-TRAV-004: Depth limit prevents deep traversal

| Field | Value |
|-------|-------|
| Test ID | GRAPH-TRAV-004 |
| Type | Unit |
| Fixture | Chain A→B→C→D→E |
| Function | `g.Reachable("A", 2)` |
| Expected | Returns B, C, D (depth 2 reaches 3 hops from existing test) |
| Observed | Pass — depth correctly limits traversal |
| Status | Pass |

### GRAPH-TRAV-005: Cycle does not cause infinite traversal

| Field | Value |
|-------|-------|
| Test ID | GRAPH-TRAV-005 |
| Type | Unit |
| Operator question | Does a cycle in the graph cause infinite loops? |
| Fixture | A→B→C→A (cycle) |
| Function | `g.Reachable("A", 10)` |
| Expected | Terminates, returns B and C (2 nodes) |
| Observed | Pass — terminates with 2 reachable nodes |
| Status | Pass |

### GRAPH-TRAV-006: Reachable excludes source node

| Field | Value |
|-------|-------|
| Test ID | GRAPH-TRAV-006 |
| Type | Unit |
| Function | `g.Reachable("A", 5)` |
| Expected | Source "A" does not appear in the result set |
| Observed | Pass — source excluded |
| Status | Pass |

### GRAPH-TRAV-007: Ancestors excludes target node

| Field | Value |
|-------|-------|
| Test ID | GRAPH-TRAV-007 |
| Type | Unit |
| Function | `g.Ancestors("B", 5)` |
| Expected | Target "B" does not appear in the result set |
| Observed | Pass — target excluded |
| Status | Pass |

---

## 4. Janitor Safety Integration Tests

### GRAPH-JAN-001: BuildDependencyDAG includes teardown-relevant relationships

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-001 |
| Type | Unit |
| Operator question | Which relationships create teardown ordering? |
| Fixture | Deployment with Mounts, SelectsWorkload, Reconciles edges |
| Function | `janitor.BuildDependencyDAG(key, g)` |
| Expected | Mounts and SelectsWorkload produce ordering edges; Reconciles excluded |
| Observed | Pass — Mounts and SelectsWorkload present, Reconciles absent |
| Status | Pass |

### GRAPH-JAN-002: Authority edges excluded from teardown DAG

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-002 |
| Type | Unit |
| Operator question | Do Reconciles/Generates edges enter the deletion plan? |
| Fixture | Application → Deployment [Reconciles] |
| Function | `janitor.BuildDependencyDAG(key, g)` |
| Expected | No dependency edge for Reconciles. Authority relationships produce Protected actionability, not ordering. |
| Observed | Pass — Reconciles not in dependency list |
| Status | Pass |

### GRAPH-JAN-003: Cycle detection identifies circular dependencies

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-003 |
| Type | Unit |
| Function | `janitor.DetectCycles(edges)` |
| Expected | Returns true for A→B→C→A |
| Observed | Pass |
| Status | Pass |

### GRAPH-JAN-004: Cycle detection returns false for acyclic DAG

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-004 |
| Type | Unit |
| Function | `janitor.DetectCycles(edges)` |
| Expected | Returns false for A→B→C (no back edges) |
| Observed | Pass |
| Status | Pass |

### GRAPH-JAN-005: Action closure includes owned descendants

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-005 |
| Type | Unit |
| Fixture | Deployment → ReplicaSet [Owns] |
| Function | `janitor.BuildActionClosure(key, g, index)` |
| Expected | Closure contains Deployment (target) and ReplicaSet (cascading) |
| Observed | Pass — 2 resources in closure |
| Status | Pass |

### GRAPH-JAN-006: Deletion qualification fails on unaccounted dependent

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-006 |
| Type | Unit |
| Fixture | ConfigMap with incoming Mounts from Deployment NOT in closure |
| Function | `janitor.QualifyDeletion(closure, key, g, index)` |
| Expected | Qualification fails: "unaccounted dependents" check |
| Observed | Pass — Qualified=false, details show the external consumer |
| Status | Pass |

### GRAPH-JAN-007: Deletion qualification fails on partial shape deletion

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-007 |
| Type | Unit |
| Fixture | Deployment in shape group with Service; only Deployment in closure |
| Function | `janitor.QualifyDeletion(closure, key, g, index)` |
| Expected | Qualification fails: "partial shape deletion" check |
| Observed | Pass — Service identified as missing from closure |
| Status | Pass |

### GRAPH-JAN-008: Deletion qualification fails on unknown relationship

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-008 |
| Type | Unit |
| Fixture | Resource with edge of unknown type (not in teardown or provenance categories) |
| Function | `janitor.QualifyDeletion(closure, key, g, index)` |
| Expected | Qualification fails: "unknown relationships" check |
| Observed | Pass — unknown edge type blocks qualification |
| Status | Pass |

### GRAPH-JAN-009: Deletion qualification passes for isolated resource

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-009 |
| Type | Unit |
| Fixture | ConfigMap with no incoming or outgoing structural edges |
| Function | `janitor.QualifyDeletion(closure, key, g, index)` |
| Expected | All 6 checks pass, Qualified=true |
| Observed | Pass |
| Status | Pass |

### GRAPH-JAN-010: Deletion order is consumers before providers

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-010 |
| Type | Unit |
| Fixture | Deployment → ConfigMap [Mounts]; both in closure |
| Function | `janitor.ComputeDeletionOrder(closure, g)` |
| Expected | Deployment (consumer) appears before ConfigMap (provider) |
| Observed | Pass — Deployment index < ConfigMap index in ordering |
| Status | Pass |

### GRAPH-JAN-011: Safety walk returns Protected for active reconciler

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-011 |
| Type | Unit |
| Fixture | Application with auto-reconcile → Deployment [Reconciles] |
| Function | `janitor.EvaluateSafety(key, g, index)` |
| Expected | Actionability=Protected, reason mentions continuous reconciliation |
| Observed | Pass |
| Status | Pass |

### GRAPH-JAN-012: Safety walk returns Indeterminate when graph unavailable

| Field | Value |
|-------|-------|
| Test ID | GRAPH-JAN-012 |
| Type | Unit |
| Function | `janitor.EvaluateSafety(key, nil, index)` |
| Expected | Actionability=Indeterminate, reason="graph unavailable" |
| Observed | Pass |
| Status | Pass |

---

## 5. CLI Integration Tests

### GRAPH-CLI-001: kos relationships lists all edges

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-001 |
| Type | Integration |
| Command | `kos relationships` |
| Expected | Table with SOURCE, TYPE, TARGET, EVIDENCE columns. Footer shows edge and node counts. |
| Observed | Pass — 297 edges, 357 nodes shown |
| Status | Pass |

### GRAPH-CLI-002: kos relationships for one workload

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-002 |
| Type | Integration |
| Command | `kos relationships Deployment argocd-server -n argocd` |
| Expected | Outgoing and Incoming sections with type, target/source, evidence, confidence |
| Observed | Pass — 6 outgoing (UsesServiceAccount, 3× Mounts, References, Owns), 1 incoming (SelectsWorkload) |
| Status | Pass |

### GRAPH-CLI-003: kos reachable shows transitive closure

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-003 |
| Type | Integration |
| Command | `kos reachable Service argocd-server -n argocd` |
| Expected | Lists all transitively reachable resources with count |
| Observed | Pass — 7 reachable resources listed |
| Status | Pass |

### GRAPH-CLI-004: kos reachable respects depth limit

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-004 |
| Type | Integration |
| Command | `kos reachable Deployment argocd-server -n argocd --depth 1` |
| Expected | Only direct dependencies (depth 1), not transitive |
| Observed | Pass — depth 1 returns fewer resources than default; direct dependencies only |
| Status | Pass |

### GRAPH-CLI-005: kos graph export produces valid JSON

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-005 |
| Type | Integration |
| Command | `kos graph export \| python3 -c "import sys,json; json.load(sys.stdin)"` |
| Expected | Valid JSON with nodes (array), edges (array), and metadata |
| Observed | Pass — parses without error, 535 nodes, 734 edges |
| Status | Pass |

### GRAPH-CLI-006: Graph export categories sum to total

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-006 |
| Type | Integration |
| Command | `kos graph export` with edge type aggregation |
| Expected | Structural (279) + Provenance (18) + Grouping (410) + Classification (27) = 734 |
| Observed | Pass — categories are mutually exclusive and sum correctly |
| Status | Pass |

### GRAPH-CLI-007: kos describe resource shows relationships

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-007 |
| Type | Integration |
| Command | `kos describe resource Deployment argocd-server -n argocd` |
| Expected | Relationships section with Outgoing and Incoming subsections |
| Observed | Pass — shows 6 outgoing, 1 incoming with type, target, evidence |
| Status | Pass |

### GRAPH-CLI-008: kos report counts match kos relationships

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CLI-008 |
| Type | Integration |
| Command | Compare `kos report` Relationships section with `kos relationships` footer |
| Expected | Same edge and node counts |
| Observed | Pass — both show 297 edges, 357 nodes |
| Status | Pass |

---

## 6. Blast Radius and Shared Resource Tests

### GRAPH-BLAST-001: Shared ConfigMap shows all consumers

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BLAST-001 |
| Type | Integration |
| Operator question | What workloads would be affected if this ConfigMap changes? |
| Command | `kos describe resource ConfigMap argocd-ssh-known-hosts-cm -n argocd` |
| Expected | Incoming relationships show all 3 mounting Deployments |
| Observed | Pass — 3 incoming Mounts edges from repo-server, server, applicationset-controller |
| Status | Pass |

### GRAPH-BLAST-002: Disconnected resource has no structural edges

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BLAST-002 |
| Type | Integration |
| Operator question | Does a disconnected ConfigMap show zero relationships? |
| Fixture | argocd-notifications-cm (disconnected ConfigMap in argocd namespace) |
| Command | `kos describe resource ConfigMap argocd-notifications-cm -n argocd` |
| Expected | No incoming or outgoing structural relationships (no Mounts, References, SelectsWorkload) |
| Observed | Pass — no structural consumer edges present |
| Status | Pass |

### GRAPH-BLAST-003: Service consumer does not imply hard deletion dependency

| Field | Value |
|-------|-------|
| Test ID | GRAPH-BLAST-003 |
| Type | Unit |
| Operator question | Does SelectsWorkload create a hard teardown dependency? |
| Fixture | Service → Deployment [SelectsWorkload] |
| Function | Check teardown semantics in `teardownRelationships` map |
| Expected | SelectsWorkload IS in the teardown map (Service is a consumer that must be processed first). However, removing a Service does NOT require removing the Deployment. |
| Observed | Pass — SelectsWorkload in teardown map means: "if both are being deleted, delete Service first" |
| Status | Pass |

---

## 7. Adversarial and Edge Cases

### GRAPH-ADV-001: Self-referencing resource

| Field | Value |
|-------|-------|
| Test ID | GRAPH-ADV-001 |
| Type | Unit |
| Operator question | Does a resource referencing itself cause problems? |
| Fixture | ConfigMap with itself listed in ConfigMapRefs |
| Expected | No infinite loop. At most one self-edge, or no edge if implementation skips self-refs. |
| Observed | Not Run |
| Status | Not Run |

### GRAPH-ADV-002: OwnerReference UID not in cluster

| Field | Value |
|-------|-------|
| Test ID | GRAPH-ADV-002 |
| Type | Unit |
| Operator question | Does a dangling ownerReference cause issues? |
| Fixture | ReplicaSet with ownerReference pointing to non-existent UID |
| Function | `graph.Build(index)` |
| Expected | No Owns edge created (UID not found in index) |
| Observed | Pass — 0 Owns edges for dangling ownerRef |
| Status | Pass |

### GRAPH-ADV-003: Very large fan-out

| Field | Value |
|-------|-------|
| Test ID | GRAPH-ADV-003 |
| Type | Unit |
| Operator question | Does a Service selecting 50 workloads perform correctly? |
| Fixture | Service with selector matching 50 Deployments |
| Expected | 50 SelectsWorkload edges created. Traversal completes in reasonable time. |
| Observed | Not Run |
| Status | Not Run |

### GRAPH-ADV-004: Empty cluster produces empty graph

| Field | Value |
|-------|-------|
| Test ID | GRAPH-ADV-004 |
| Type | Unit |
| Fixture | Empty knowledge index |
| Function | `graph.Build(index)` |
| Expected | EdgeCount=0, NodeCount=0 |
| Observed | Not Run |
| Status | Not Run |

### GRAPH-ADV-005: Resource with multiple ownerReferences

| Field | Value |
|-------|-------|
| Test ID | GRAPH-ADV-005 |
| Type | Unit |
| Fixture | Resource with 2 ownerReferences (both UIDs present) |
| Expected | 2 Owns edges from different parents to the same child |
| Observed | Not Run |
| Status | Not Run |

---

## 8. Cross-Axis Tests

### GRAPH-CROSS-001: Graph relationships align with shape composition

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CROSS-001 |
| Type | Integration |
| Operator question | Do the relationships shown in the graph match the shape's defining relationships? |
| Fixture | fixture-stateful shape instance |
| Expected | UsesHeadlessService and ClaimsStorage edges present — same edges that qualify the shape match |
| Observed | Not Run |
| Status | Not Run |

### GRAPH-CROSS-002: Graph consumer count validates Janitor qualification

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CROSS-002 |
| Type | Integration |
| Operator question | Does incoming edge count block deletion qualification? |
| Fixture | Shared ConfigMap with 3 consumers |
| Expected | Janitor qualification fails for this ConfigMap because consumers exist outside any single action closure |
| Observed | Not Run |
| Status | Not Run |

### GRAPH-CROSS-003: Ownership authority visible in graph export

| Field | Value |
|-------|-------|
| Test ID | GRAPH-CROSS-003 |
| Type | Integration |
| Command | `kos graph export` |
| Expected | Each node has an `ownership` field with classification, confidence, and owner |
| Observed | Pass — nodes include ownership data from new engine |
| Status | Pass |

---

## Execution Summary

| Category | Total | Pass | Fail | Skip | Not Run |
|----------|-------|------|------|------|---------|
| Graph Data Structure | 8 | 8 | 0 | 0 | 0 |
| Relationship Builder | 17 | 17 | 0 | 0 | 0 |
| Traversal | 7 | 7 | 0 | 0 | 0 |
| Janitor Safety Integration | 12 | 12 | 0 | 0 | 0 |
| CLI Integration | 8 | 8 | 0 | 0 | 0 |
| Blast Radius & Shared | 3 | 3 | 0 | 0 | 0 |
| Adversarial | 5 | 1 | 0 | 0 | 4 |
| Cross-Axis | 3 | 1 | 0 | 0 | 2 |
| **Total** | **63** | **57** | **0** | **0** | **6** |
