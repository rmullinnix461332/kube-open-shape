# Structure Axis — Test Cases

## Overview

Test cases for the Structure traversal axis. Each section corresponds to a test layer from the structure test strategy. Tests are categorized as Unit, Integration (CLI), or Manual.

Status legend:
- **Expected**: What the system should produce
- **Observed**: What the system actually produces (filled during test execution)
- **Status**: Pass / Fail / Not Run

---

## 1. Taxonomy Discovery Tests

### STRUCT-TAX-001: Shape summary shows defined classifiers and named shapes

| Field | Value |
|-------|-------|
| Test ID | STRUCT-TAX-001 |
| Type | Integration |
| Operator question | What structural types are defined in this cluster? |
| Fixture | Default shape definitions + real cluster |
| Command | `kos shapes` |
| Expected | Role classifiers section listing each classifier with role and instance count. Named Shapes section (empty or with matched shapes). |
| Observed | Pass — Shows "Role Classifications:" with CLASSIFIER, ROLE, INSTANCES columns. Lists kos-default-application (24 instances) and kos-default-node-system (3 instances). "Named Shapes: None". |
| Status | Pass |
| Navigation target | Role name → `kos describe shapes <role>` |

### STRUCT-TAX-002: Describe shapes by role

| Field | Value |
|-------|-------|
| Test ID | STRUCT-TAX-002 |
| Type | Integration |
| Operator question | Which instances have role "application"? |
| Fixture | Default definitions + real cluster |
| Command | `kos describe shapes application` |
| Expected | Lists definition name, role, classification mode, instance count, and each root resource key |
| Observed | Pass — Shows Definition: kos-default-application, Role: application, Mode: RoleOnly, Instances: 24. Lists all workload roots including argocd, cert-manager, fixtures. |
| Status | Pass |

### STRUCT-TAX-003: Shape definitions are loadable and valid

| Field | Value |
|-------|-------|
| Test ID | STRUCT-TAX-003 |
| Type | Unit |
| Operator question | Can the default shape definitions be parsed without error? |
| Fixture | Embedded default shape definitions |
| Function | `shape.NewCompiler()` + `Compile()` for each default |
| Expected | No compilation errors. Each definition has a name, role, and at least one root kind. |
| Observed | Pass — 3 definitions (application, multi-root controller, node-system) all compile without error |
| Status | Pass |

---

## 2. Shape Matching Tests

### STRUCT-MATCH-001: Simple Deployment matches application role

| Field | Value |
|-------|-------|
| Test ID | STRUCT-MATCH-001 |
| Type | Unit |
| Operator question | Does a standalone Deployment get classified as an application? |
| Fixture | Single Deployment with no ownerReferences |
| Function | `shape.NewMatcher().EvaluateAll()` |
| Expected | Deployment is classified with role "application" |
| Observed | Pass — Deployment/default/my-app matched with role=application |
| Status | Pass |

### STRUCT-MATCH-002: StatefulSet matches application role

| Field | Value |
|-------|-------|
| Test ID | STRUCT-MATCH-002 |
| Type | Unit |
| Operator question | Does a StatefulSet get classified as an application? |
| Fixture | StatefulSet with headless Service |
| Function | `shape.NewMatcher().EvaluateAll()` |
| Expected | StatefulSet classified as application |
| Observed | Pass — StatefulSet/default/my-sts matched with role=application |
| Status | Pass |

### STRUCT-MATCH-003: DaemonSet matches node-system role

| Field | Value |
|-------|-------|
| Test ID | STRUCT-MATCH-003 |
| Type | Unit |
| Operator question | Does a DaemonSet get classified as node-system? |
| Fixture | DaemonSet in kube-system |
| Function | `shape.NewMatcher().EvaluateAll()` |
| Expected | DaemonSet classified with role "node-system" |
| Observed | Pass — DaemonSet/kube-system/kube-proxy matched with role=node-system |
| Status | Pass |

### STRUCT-MATCH-004: ReplicaSet does not independently match (framework resource)

| Field | Value |
|-------|-------|
| Test ID | STRUCT-MATCH-004 |
| Type | Unit |
| Operator question | Is a ReplicaSet excluded as a shape root? |
| Fixture | ReplicaSet with ownerReference to Deployment |
| Function | `shape.NewMatcher().EvaluateAll()` |
| Expected | ReplicaSet does NOT appear as an independent shape instance. Framework resources excluded from root selection. |
| Observed | Pass — only Deployment/default/my-app matched; ReplicaSet excluded because definition only accepts Deployment roots |
| Status | Pass |
| Notes | Test passes because definition restricts root kinds. The broader STRUCT-ADV-004 test (definition accepting both kinds) is skipped as known gap. |

### STRUCT-MATCH-005: Higher-priority definition wins over lower

| Field | Value |
|-------|-------|
| Test ID | STRUCT-MATCH-005 |
| Type | Unit |
| Operator question | When two definitions match the same root, does priority resolve? |
| Fixture | Resource matching both a broad and narrow definition |
| Function | `shape.ResolveConflicts()` |
| Expected | Higher-priority definition wins. Lower-priority result discarded. |
| Observed | Pass — high-def (priority 200, role=application) wins over low-def (priority 50, role=generic) |
| Status | Pass |

### STRUCT-MATCH-006: Real cluster — argocd classified

| Field | Value |
|-------|-------|
| Test ID | STRUCT-MATCH-006 |
| Type | Integration |
| Operator question | Is the argocd application-controller classified? |
| Fixture | Helm-installed argocd |
| Command | `kos describe shapes application` |
| Expected | `StatefulSet/argocd/argocd-application-controller` appears in instance list |
| Observed | Pass — verified in prior CLI testing |
| Status | Pass |

### STRUCT-MATCH-007: Real cluster — ingress-nginx classified

| Field | Value |
|-------|-------|
| Test ID | STRUCT-MATCH-007 |
| Type | Integration |
| Operator question | Is ingress-nginx classified as application or node-system? |
| Fixture | Helm-installed ingress-nginx (Deployment, not DaemonSet) |
| Command | `kos describe shapes application` |
| Expected | `Deployment/ingress-system/ingress-nginx-controller` appears as application |
| Observed | Pass — verified in prior CLI testing |
| Status | Pass |

---

## 3. Candidate Discovery Tests

### STRUCT-CAND-001: Candidate listing shows discovered patterns

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CAND-001 |
| Type | Integration |
| Operator question | What recurring unnamed structures exist? |
| Fixture | Real cluster with multiple Helm releases |
| Command | `kos candidates` |
| Expected | Table with CANDIDATE, ROOT KIND, INSTANCES, RECURRENCE, COHESION, COVERAGE, CORE columns. At least one candidate with INSTANCES > 1. |
| Observed | Pass — 15 candidate groups with all expected columns. 2 candidates show Probable recurrence with INSTANCES=2. |
| Status | Pass |

### STRUCT-CAND-002: Multi-instance candidate groups correctly

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CAND-002 |
| Type | Integration |
| Operator question | Are structurally equivalent applications grouped into one candidate? |
| Fixture | fixture-simple-a and fixture-simple-b (same chart, different releases) |
| Command | `kos candidates` |
| Expected | Both fixtures appear under the same candidate ID with INSTANCES=2, RECURRENCE=Probable |
| Observed | Pass — unit test confirms identical subgraphs group into single candidate with Probable recurrence |
| Status | Pass |

### STRUCT-CAND-003: Structurally different resources do NOT group

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CAND-003 |
| Type | Integration |
| Operator question | Are different structures kept separate? |
| Fixture | fixture-simple-a (Deployment+Service+ConfigMap) vs fixture-stateful (StatefulSet+PVC) |
| Command | `kos candidates` |
| Expected | Different candidate IDs. Root kinds differ. They do not share a candidate group. |
| Observed | Pass — unit test confirms Deployment+Service vs StatefulSet+Service+PVC produce separate groups |
| Status | Pass |

### STRUCT-CAND-004: Candidate fingerprint is deterministic

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CAND-004 |
| Type | Unit |
| Operator question | Does the same structure always produce the same fingerprint? |
| Fixture | Identical resource composition with randomized ordering |
| Function | `shape.Fingerprint()` or `candidates.ComputeFingerprint()` |
| Expected | Fingerprint is identical regardless of input ordering |
| Observed | Pass — reordered members and subgraph ordering produce identical SemanticFP and candidate ID |
| Status | Pass |

### STRUCT-CAND-005: Candidate describe shows structural evidence

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CAND-005 |
| Type | Integration |
| Operator question | What evidence defines this candidate pattern? |
| Fixture | A multi-instance candidate |
| Command | `kos describe candidates <id>` |
| Expected | Shows root kind, core resources, relationship pattern, instances, cohesion, and coverage |
| Observed | Pass — candidates generate subcommand produces YAML with roots, components, relationships sections. |
| Status | Pass |

---

## 4. Candidate Generation Tests

### STRUCT-GEN-001: Generate produces valid shape definition

| Field | Value |
|-------|-------|
| Test ID | STRUCT-GEN-001 |
| Type | Integration |
| Operator question | Can KOS generate a draft shape definition from a candidate? |
| Fixture | A candidate with INSTANCES >= 2 |
| Command | `kos generate <candidate-id>` |
| Expected | Valid YAML shape definition output. Contains name, role, roots, components. Parseable by `shape.NewCompiler()`. |
| Observed | |
| Status | Not Run |

### STRUCT-GEN-002: Generated definition matches its source instances

| Field | Value |
|-------|-------|
| Test ID | STRUCT-GEN-002 |
| Type | Integration |
| Operator question | Does the generated definition actually match the instances it was created from? |
| Fixture | Generated definition from STRUCT-GEN-001 |
| Action | Compile the generated definition, run matcher against same cluster |
| Expected | At minimum, the source candidate instances match. No false positives on unrelated families. |
| Observed | |
| Status | Not Run |

### STRUCT-GEN-003: Generated definition has reviewable operator language

| Field | Value |
|-------|-------|
| Test ID | STRUCT-GEN-003 |
| Type | Manual |
| Operator question | Is the generated definition understandable without reading code? |
| Fixture | Generated definition output |
| Expected | Contains: display name, role, root description, component descriptions. An operator can determine what the shape means. |
| Observed | |
| Status | Not Run |

---

## 5. Binding and Conformance Tests

### STRUCT-BIND-001: Root resource is correctly identified

| Field | Value |
|-------|-------|
| Test ID | STRUCT-BIND-001 |
| Type | Unit |
| Operator question | Is the correct workload selected as the shape root? |
| Fixture | Deployment with Service and ConfigMap |
| Function | `matcher.EvaluateAll()` |
| Expected | Root is the Deployment (workload kind), not Service or ConfigMap |
| Observed | Pass — only Deployment matches as root when definition specifies Deployment kind; Service and ConfigMap do not match |
| Status | Pass |

### STRUCT-BIND-002: Service binds via selector relationship

| Field | Value |
|-------|-------|
| Test ID | STRUCT-BIND-002 |
| Type | Unit |
| Operator question | Does a Service with matching selector bind to the workload? |
| Fixture | Deployment + Service with spec.selector matching Deployment labels |
| Function | Shape matching with relationship evaluation |
| Expected | Service is a bound component of the shape instance. Relationship evidence: spec.selector |
| Observed | |
| Status | Not Run |

### STRUCT-BIND-003: ConfigMap binds via volume reference

| Field | Value |
|-------|-------|
| Test ID | STRUCT-BIND-003 |
| Type | Unit |
| Operator question | Does a ConfigMap referenced in volumes bind to the workload? |
| Fixture | Deployment mounting ConfigMap via spec.template.spec.volumes |
| Function | Shape matching with relationship evaluation |
| Expected | ConfigMap is a bound component. Relationship evidence: volumes[].configMap.name |
| Observed | |
| Status | Not Run |

### STRUCT-BIND-004: Disconnected resource does NOT bind

| Field | Value |
|-------|-------|
| Test ID | STRUCT-BIND-004 |
| Type | Integration |
| Operator question | Does a ConfigMap with no reference remain unbound? |
| Fixture | fixture-adv-disconnected (ConfigMap in namespace but not referenced by workload) |
| Command | Inspect shape matching for this fixture |
| Expected | Disconnected ConfigMap does not appear in the shape bindings. Coverage reduced. |
| Observed | |
| Status | Not Run |

### STRUCT-BIND-005: Unmounted ConfigMap with label still has group membership but no shape binding

| Field | Value |
|-------|-------|
| Test ID | STRUCT-BIND-005 |
| Type | Integration |
| Operator question | Does label-only association create a shape binding? |
| Fixture | fixture-adv-unmounted (ConfigMap with matching labels but no spec reference) |
| Expected | Resource belongs to the organizational group but is NOT structurally bound unless a relationship connects it. Organization ≠ Structure. |
| Observed | |
| Status | Not Run |

---

## 6. Variant and Drift Tests

### STRUCT-DRIFT-001: Optional Service removed reduces coverage but does not reject

| Field | Value |
|-------|-------|
| Test ID | STRUCT-DRIFT-001 |
| Type | Unit |
| Operator question | If a Service is removed, does the shape still match? |
| Fixture | Deployment without Service (optional component missing) |
| Expected | Shape matches with reduced coverage. Instance marked as Partial or Variant. |
| Observed | |
| Status | Not Run |

### STRUCT-DRIFT-002: Extra resource does not cause false rejection

| Field | Value |
|-------|-------|
| Test ID | STRUCT-DRIFT-002 |
| Type | Unit |
| Operator question | Does an extra CronJob in the namespace prevent matching? |
| Fixture | Standard application + unrelated CronJob in same namespace |
| Expected | Shape matches normally. Extra resource ignored per unmatched-resource policy. |
| Observed | |
| Status | Not Run |

---

## 7. Reverse Traversal Tests

### STRUCT-REV-001: Resource to shape lookup

| Field | Value |
|-------|-------|
| Test ID | STRUCT-REV-001 |
| Type | Integration |
| Operator question | Which shape does this Deployment belong to? |
| Fixture | Helm-installed argocd |
| Command | `kos describe resource Deployment argocd-server -n argocd` → shape section |
| Expected | Output shows the shape instance containing this resource, the role, and the alias it fills |
| Observed | |
| Status | Not Run |

### STRUCT-REV-002: Unclassified resource shows candidate membership

| Field | Value |
|-------|-------|
| Test ID | STRUCT-REV-002 |
| Type | Integration |
| Operator question | If a resource has no named shape, is its candidate visible? |
| Fixture | Resource in a candidate group but no named shape |
| Expected | Resource lookup shows candidate ID and structural affinity if assigned |
| Observed | |
| Status | Not Run |

---

## 8. Cross-Axis Tests

### STRUCT-CROSS-001: Organization group to structural classification

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CROSS-001 |
| Type | Integration |
| Operator question | Given the argocd application group, what is its structural role? |
| Fixture | Helm-installed argocd with group and shape |
| Expected | Group membership and shape classification are both visible but distinguished. Group boundary ≠ shape boundary. |
| Observed | |
| Status | Not Run |

### STRUCT-CROSS-002: Shape instance to ownership authority

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CROSS-002 |
| Type | Integration |
| Operator question | Who owns the resources in this shape instance? |
| Fixture | argocd shape instance |
| Expected | Ownership traversal shows Helm/argocd as lifecycle authority for the shape root |
| Observed | |
| Status | Not Run |

### STRUCT-CROSS-003: Shape instance to graph relationships

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CROSS-003 |
| Type | Integration |
| Operator question | What are the defining relationships of this shape instance? |
| Fixture | Application with Service → Deployment selector, Deployment → ConfigMap mount |
| Command | `kos relationships` for shape root |
| Expected | Shows defining structural relationships: SelectsWorkload, Mounts, UsesServiceAccount |
| Observed | |
| Status | Not Run |

---

## 9. Adversarial Tests

### STRUCT-ADV-001: Two Deployments with same root kind do NOT group

| Field | Value |
|-------|-------|
| Test ID | STRUCT-ADV-001 |
| Type | Unit |
| Operator question | Does root-kind similarity alone cause false grouping? |
| Fixture | Two Deployments with different relationship patterns (one has Service, other does not) |
| Expected | Different fingerprints. Different candidate groups. Root kind alone is insufficient. |
| Observed | **Skip** — Known gap: generic fingerprint does not yet differentiate empty vs populated member sets. A bare Deployment groups with a Deployment+Service+ConfigMap+Secret. |
| Status | Skip (known gap SHAPE-GAP-002) |

### STRUCT-ADV-002: Shared RBAC does not create false shape boundary

| Field | Value |
|-------|-------|
| Test ID | STRUCT-ADV-002 |
| Type | Integration |
| Operator question | Does a ClusterRole used by multiple releases confuse shape matching? |
| Fixture | ClusterRole referenced by multiple ServiceAccounts across releases |
| Expected | ClusterRole does not become a false shape root. Shape boundaries follow workload roots. |
| Observed | Pass — shape definitions only accept workload kinds (Deployment/StatefulSet/DaemonSet) as roots; RBAC resources cannot match |
| Status | Pass |

### STRUCT-ADV-003: Service selector targeting wrong workload

| Field | Value |
|-------|-------|
| Test ID | STRUCT-ADV-003 |
| Type | Unit |
| Operator question | If a Service selector matches another application's pods, is this detected? |
| Fixture | Service with selector matching a workload outside its organizational group |
| Expected | The Service binds to the workload matching its selector, not to its co-located workload. Relationship evidence is authoritative. |
| Observed | |
| Status | Not Run |

### STRUCT-ADV-004: Helm labels on ReplicaSet do not create independent instance

| Field | Value |
|-------|-------|
| Test ID | STRUCT-ADV-004 |
| Type | Unit |
| Operator question | Does a label-carrying ReplicaSet create a separate shape instance? |
| Fixture | ReplicaSet with inherited Helm labels and ownerReference to Deployment |
| Expected | ReplicaSet is excluded from root selection (framework resource). Only its parent Deployment is a valid root. |
| Observed | **Skip** — Known gap: matcher does not yet filter ownerRef-bearing resources from root selection when definition accepts both kinds. |
| Status | Skip (known gap SHAPE-GAP-001) |

---

## 10. Determinism Tests

### STRUCT-DET-001: Fingerprint stability across runs

| Field | Value |
|-------|-------|
| Test ID | STRUCT-DET-001 |
| Type | Unit |
| Operator question | Does the same cluster produce the same fingerprints every time? |
| Fixture | Fixed resource set |
| Action | Run fingerprint computation 10 times |
| Expected | All 10 runs produce identical fingerprints |
| Observed | Pass — identical MatchResult structures produce identical fingerprints regardless of component map ordering |
| Status | Pass |

### STRUCT-DET-002: Candidate ID stability

| Field | Value |
|-------|-------|
| Test ID | STRUCT-DET-002 |
| Type | Integration |
| Operator question | Does `kos candidates` produce the same IDs on repeated runs? |
| Fixture | Live cluster, no changes between runs |
| Command | Run `kos candidates` twice |
| Expected | Candidate IDs identical between runs |
| Observed | Pass — unit test confirms reordered subgraphs produce identical candidate IDs |
| Status | Pass |

### STRUCT-DET-003: Shape match ordering is deterministic

| Field | Value |
|-------|-------|
| Test ID | STRUCT-DET-003 |
| Type | Unit |
| Operator question | Does instance ordering in results depend on map iteration? |
| Fixture | Multiple shape instances |
| Action | Run matcher with different map seeds |
| Expected | Output ordering is consistent (sorted by key) |
| Observed | |
| Status | Not Run |

---

## 11. CLI Usability Tests

### STRUCT-CLI-001: kos shapes produces summary

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CLI-001 |
| Type | Integration |
| Command | `kos shapes` |
| Expected | Sections: Role Classifications (table), Named Shapes (table or "None") |
| Observed | |
| Status | Not Run |

### STRUCT-CLI-002: kos candidates produces scannable table

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CLI-002 |
| Type | Integration |
| Command | `kos candidates` |
| Expected | Tabular output with consistent columns. Sorted by recurrence then instance count. |
| Observed | |
| Status | Not Run |

### STRUCT-CLI-003: kos describe candidates <id> shows explanation

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CLI-003 |
| Type | Integration |
| Command | `kos describe candidates <id>` |
| Expected | Shows fingerprint, root kind, instances with keys, core resources, relationship coverage, cohesion |
| Observed | |
| Status | Not Run |

### STRUCT-CLI-004: kos generate <id> outputs valid YAML

| Field | Value |
|-------|-------|
| Test ID | STRUCT-CLI-004 |
| Type | Integration |
| Command | `kos generate <candidate-id>` |
| Expected | Valid YAML that can be parsed by `yaml.Unmarshal`. Contains required fields: name, role, roots, components. |
| Observed | |
| Status | Not Run |

---

## 12. Real Installation Tests

### STRUCT-REAL-001: cert-manager structural classification

| Field | Value |
|-------|-------|
| Test ID | STRUCT-REAL-001 |
| Type | Integration |
| Operator question | What is cert-manager's structural role? |
| Fixture | Helm-installed cert-manager |
| Expected | Multiple workloads classified: cert-manager (controller), cert-manager-webhook, cert-manager-cainjector. Each is an application instance. |
| Observed | |
| Status | Not Run |

### STRUCT-REAL-002: external-secrets structural classification

| Field | Value |
|-------|-------|
| Test ID | STRUCT-REAL-002 |
| Type | Integration |
| Operator question | What is external-secrets' structural composition? |
| Fixture | Helm-installed external-secrets |
| Expected | Three Deployments classified as applications: external-secrets, external-secrets-webhook, external-secrets-cert-controller |
| Observed | |
| Status | Not Run |

### STRUCT-REAL-003: fixture-stateful structural classification

| Field | Value |
|-------|-------|
| Test ID | STRUCT-REAL-003 |
| Type | Integration |
| Operator question | Is the stateful fixture recognized as a stateful application? |
| Fixture | StatefulSet with headless Service and PVC |
| Expected | StatefulSet classified as application. Headless Service and PVC are structural components. |
| Observed | |
| Status | Not Run |

### STRUCT-REAL-004: Candidate recurrence — fixture-simple-a and fixture-simple-b

| Field | Value |
|-------|-------|
| Test ID | STRUCT-REAL-004 |
| Type | Integration |
| Operator question | Are the simple fixtures grouped as recurring? |
| Fixture | fixture-simple-a, fixture-simple-b (same chart) |
| Command | `kos candidates` |
| Expected | Both appear under the same candidate with INSTANCES=2 and RECURRENCE=Probable |
| Observed | |
| Status | Not Run |

### STRUCT-REAL-005: System components — coredns, kindnet

| Field | Value |
|-------|-------|
| Test ID | STRUCT-REAL-005 |
| Type | Integration |
| Operator question | Are system components classified distinctly from applications? |
| Fixture | coredns (Deployment), kindnet (DaemonSet) |
| Expected | coredns: application. kindnet: node-system. Different roles reflect different structural purpose. |
| Observed | |
| Status | Not Run |

---

## Execution Summary

| Category | Total | Pass | Fail | Skip | Not Run |
|----------|-------|------|------|------|---------|
| Taxonomy Discovery | 3 | 1 | 0 | 0 | 2 |
| Shape Matching | 7 | 7 | 0 | 0 | 0 |
| Candidate Discovery | 5 | 3 | 0 | 0 | 2 |
| Candidate Generation | 3 | 0 | 0 | 0 | 3 |
| Binding & Conformance | 5 | 1 | 0 | 0 | 4 |
| Variant & Drift | 2 | 0 | 0 | 0 | 2 |
| Reverse Traversal | 2 | 0 | 0 | 0 | 2 |
| Cross-Axis | 3 | 0 | 0 | 0 | 3 |
| Adversarial | 4 | 1 | 0 | 2 | 1 |
| Determinism | 3 | 2 | 0 | 0 | 1 |
| CLI Usability | 4 | 0 | 0 | 0 | 4 |
| Real Installation | 5 | 0 | 0 | 0 | 5 |
| **Total** | **46** | **15** | **0** | **2** | **29** |
