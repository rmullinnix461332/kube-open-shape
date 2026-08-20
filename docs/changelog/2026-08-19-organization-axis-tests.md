# Organization Axis — Test Cases

## Overview

Test cases for the Organization traversal axis. Each section corresponds to a test type from the hardening strategy. Tests are categorized as Unit, Integration, or Manual.

Status legend:
- **Expected**: What the system should produce
- **Observed**: What the system actually produces (filled during test execution)
- **Status**: Pass / Fail / Not Run

---

## 1. Navigation Tests

### ORG-NAV-001: Cluster to group listing

| Field | Value |
|-------|-------|
| Test ID | ORG-NAV-001 |
| Type | Integration |
| Operator question | What application groups exist in this cluster? |
| Fixture | Helm-installed argocd, cert-manager, grafana |
| Command | `kos groups` |
| Expected | Table listing each application group with home namespace, member count, confidence, evidence |
| Observed | Pass — 13 groups listed with columns GROUP, HOME NAMESPACE, MEMBERS, CONFIDENCE, EVIDENCE. Includes argocd (64 members, Corroborating), cert-manager (47, Corroborating), grafana (8, Corroborating), etc. |
| Status | Pass |
| Navigation target | Each GROUP name usable in `kos groups <name>` or `kos describe groups <name>` |

### ORG-NAV-002: Group to component listing

| Field | Value |
|-------|-------|
| Test ID | ORG-NAV-002 |
| Type | Integration |
| Operator question | What components make up the argocd application? |
| Fixture | Helm-installed argocd |
| Command | `kos describe groups argocd` |
| Expected | Header with workload/component/resource counts. Component sections listing workloads and resources per declared component. |
| Observed | Pass — Shows Group: argocd, Type: Application, Workloads: 7, Components: 8, Resources: 64. Component sections include (unassigned), application-controller, applicationset-controller, dex-server, notifications-controller, redis, repo-server, server. Each component lists workloads and resources separately. |
| Status | Pass |
| Navigation target | Each resource key usable in `kos describe resources <kind> <name> -n <ns>` |

### ORG-NAV-003: Component to workload

| Field | Value |
|-------|-------|
| Test ID | ORG-NAV-003 |
| Type | Integration |
| Operator question | Which workload runs the argocd server component? |
| Fixture | Helm-installed argocd |
| Command | `kos describe groups argocd` → look for component "server" |
| Expected | Workload section shows `Deployment/argocd/argocd-server` |
| Observed | Pass — server component shows Workload: Deployment/argocd/argocd-server |
| Status | Pass |
| Navigation target | `kos relationships Deployment argocd-server -n argocd` |

### ORG-NAV-004: Workload to supporting resources

| Field | Value |
|-------|-------|
| Test ID | ORG-NAV-004 |
| Type | Integration |
| Operator question | What resources support the argocd-server Deployment? |
| Fixture | Helm-installed argocd |
| Command | `kos relationships Deployment argocd-server -n argocd` |
| Expected | Lists outgoing edges (UsesServiceAccount, Mounts, BelongsToRelease) and incoming edges (SelectsWorkload from Service, Owns from ReplicaSet) |
| Observed | |
| Status | Not Run |

### ORG-NAV-005: Resource to group (upward)

| Field | Value |
|-------|-------|
| Test ID | ORG-NAV-005 |
| Type | Integration |
| Operator question | Which application group does this ConfigMap belong to? |
| Fixture | Helm-installed argocd |
| Command | `kos groups -o json` → find entry containing `ConfigMap/argocd/argocd-cm` |
| Expected | ConfigMap appears as member of the argocd Application group with component "server" |
| Observed | |
| Status | Not Run |
| Invariant | Resource appears in exactly one application group per dimension |

### ORG-NAV-006: Namespace-filtered listing

| Field | Value |
|-------|-------|
| Test ID | ORG-NAV-006 |
| Type | Integration |
| Operator question | What groups exist in the observability namespace? |
| Fixture | Helm-installed grafana, kube-state-metrics, node-exporter |
| Command | `kos groups -n observability` |
| Expected | Only groups with homeNamespace=observability shown |
| Observed | Pass — Shows 3 groups: grafana (8 members), kube-state-metrics (6), prometheus-node-exporter (3). No groups from other namespaces. |
| Status | Pass |

### ORG-NAV-007: Resource kind filter

| Field | Value |
|-------|-------|
| Test ID | ORG-NAV-007 |
| Type | Integration |
| Operator question | What Deployments exist in the argocd namespace? |
| Fixture | Helm-installed argocd |
| Command | `kos resources deployment -n argocd` |
| Expected | Only Deployment resources in argocd namespace. No StatefulSets, Services, etc. |
| Observed | Pass — 6 Deployments returned, all kind=Deployment, all namespace=argocd. Case-insensitive match works. |
| Status | Pass |

---

## 2. Grouping Tests

### ORG-GRP-001: part-of label creates application group

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-001 |
| Type | Unit |
| Operator question | Does part-of label create an application group? |
| Fixture | Resources with `app.kubernetes.io/part-of=myapp` |
| Function | `grouping.BuildGroups()` |
| Expected | One Application group named "myapp" containing all labeled resources |
| Observed | Pass — single group created with correct name, type, and member count |
| Status | Pass |

### ORG-GRP-002: instance label creates application group when no part-of

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-002 |
| Type | Unit |
| Operator question | Does instance label alone produce a group? |
| Fixture | Resources with `app.kubernetes.io/instance=myapp` but no part-of |
| Function | `grouping.BuildGroups()` |
| Expected | One Application group named "myapp" with Inferred confidence |
| Observed | Pass — group created with Inferred confidence when only instance label present |
| Status | Pass |

### ORG-GRP-003: Corroborating evidence raises confidence

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-003 |
| Type | Unit |
| Operator question | Does agreement between part-of, instance, and Helm produce Corroborating confidence? |
| Fixture | Resources with matching part-of, instance, and helm.sh/release-name |
| Function | `grouping.determineConfidence()` |
| Expected | Confidence = Corroborating |
| Observed | Pass — two or more agreeing signals produce Corroborating confidence |
| Status | Pass |

### ORG-GRP-004: Component label assigns role within group

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-004 |
| Type | Unit |
| Operator question | Are component labels preserved as member roles? |
| Fixture | Resources with `app.kubernetes.io/component=server` |
| Function | `grouping.BuildGroups()` |
| Expected | GroupMember.Component = "server" |
| Observed | Pass — component labels preserved, ComponentCount accurate |
| Status | Pass |

### ORG-GRP-005: Helm annotation strengthens provenance

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-005 |
| Type | Unit |
| Operator question | Does meta.helm.sh/release-name annotation produce Helm evidence? |
| Fixture | Resource with annotation `meta.helm.sh/release-name=myrelease` |
| Function | `grouping.detectHelmRelease()` |
| Expected | Returns "myrelease" |
| Observed | Pass — annotation takes precedence over label, label used as fallback |
| Status | Pass |

### ORG-GRP-006: Cross-namespace merge with Helm evidence

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-006 |
| Type | Integration |
| Operator question | Does cert-manager merge kube-system resources into one group? |
| Fixture | Helm-installed cert-manager (resources in cert-manager and kube-system namespaces) |
| Command | `kos groups cert-manager -o json` |
| Expected | Single group with ScopeType=Cluster, MemberNamespaces includes both cert-manager and kube-system |
| Observed | Pass — scope=Cluster, memberNamespaces=['', 'cert-manager', 'kube-system'], 47 members. Note: empty string in namespaces indicates cluster-scoped resources are included. |
| Status | Pass |

### ORG-GRP-007: Shared resources appear in group

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-007 |
| Type | Integration |
| Operator question | Are shared ConfigMaps visible in the group? |
| Fixture | Helm-installed argocd (has shared ConfigMaps like argocd-ssh-known-hosts-cm) |
| Command | `kos describe groups argocd` |
| Expected | Shared ConfigMaps appear under "(unassigned)" component section |
| Observed | Pass — (unassigned) section shows ConfigMap/argocd/argocd-gpg-keys-cm, argocd-ssh-known-hosts-cm, argocd-tls-certs-cm |
| Status | Pass |

### ORG-GRP-008: Workload count accuracy

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-008 |
| Type | Integration |
| Operator question | Does workload count match actual Deployment/StatefulSet/DaemonSet roots? |
| Fixture | Helm-installed argocd |
| Command | `kos describe groups argocd` |
| Expected | WorkloadCount equals the number of Deployment + StatefulSet resources in group |
| Observed | Pass — WorkloadCount=7, which matches the actual ArgoCD workloads (6 Deployments + 1 StatefulSet) |
| Status | Pass |
| Invariant | WorkloadCount = count of members where Kind in (Deployment, StatefulSet, DaemonSet, CronJob, Job) |

### ORG-GRP-009: Component count accuracy

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-009 |
| Type | Integration |
| Operator question | Does component count match distinct component label values? |
| Fixture | Helm-installed argocd |
| Command | `kos describe groups argocd` |
| Expected | ComponentCount equals number of unique non-empty app.kubernetes.io/component values |
| Observed | Pass — ComponentCount=8, matching the 8 distinct component labels (application-controller, applicationset-controller, dex-server, notifications-controller, redis, redis-secret-init, repo-server, server) |
| Status | Pass |

### ORG-GRP-010: Release group keyed by release namespace

| Field | Value |
|-------|-------|
| Test ID | ORG-GRP-010 |
| Type | Unit |
| Operator question | Is the release group keyed by release namespace, not resource namespace? |
| Fixture | Resource in kube-system with `meta.helm.sh/release-namespace=cert-manager` |
| Function | `grouping.BuildGroups()` |
| Expected | Release group ID uses cert-manager namespace, not kube-system |
| Observed | Pass — detectHelmReleaseNamespace returns annotation value when present |
| Status | Pass |

---

## 3. Adversarial Tests

### ORG-ADV-001: Conflicting grouping signals

| Field | Value |
|-------|-------|
| Test ID | ORG-ADV-001 |
| Type | Unit |
| Operator question | What happens when part-of, instance, and Helm disagree? |
| Fixture | Resource with part-of=app-a, instance=app-b, helm.sh/release-name=app-c |
| Function | `grouping.detectConflict()` |
| Expected | Returns true. Group state = Conflicted. |
| Observed | Pass — conflict detected for all disagreeing combinations |
| Status | Pass |
| Negative assertion | No false consolidated identity created |

### ORG-ADV-002: Same instance label in multiple namespaces without Helm

| Field | Value |
|-------|-------|
| Test ID | ORG-ADV-002 |
| Type | Unit |
| Operator question | Are same-named groups in different namespaces kept separate without corroborating evidence? |
| Fixture | Resources in ns-a and ns-b both with instance=myapp but no Helm evidence |
| Function | `grouping.mergeMultiNamespaceGroups()` |
| Expected | Two separate groups (no merge without Helm corroboration) |
| Observed | Pass — groups remain namespace-scoped without Helm evidence |
| Status | Pass |
| Negative assertion | Must not merge based on instance label alone |

### ORG-ADV-003: Platform resources excluded from groups

| Field | Value |
|-------|-------|
| Test ID | ORG-ADV-003 |
| Type | Unit |
| Operator question | Are kube-root-ca.crt and default ServiceAccount excluded from groups? |
| Fixture | Namespace containing kube-root-ca.crt ConfigMap and default SA alongside labeled app resources |
| Function | `grouping.shouldExcludeFromGroup()` |
| Expected | Platform resources not counted as members |
| Observed | Pass — kube-root-ca.crt and default SA both return true for exclusion |
| Status | Pass |

### ORG-ADV-004: Helm release secrets excluded from groups

| Field | Value |
|-------|-------|
| Test ID | ORG-ADV-004 |
| Type | Unit |
| Operator question | Are sh.helm.release.v1.* secrets excluded from application groups? |
| Fixture | Namespace with Helm release secret and application resources |
| Function | `grouping.shouldExcludeFromGroup()` |
| Expected | Release secrets not counted as application group members |
| Observed | Pass — IsHelmReleaseSecret returns true for sh.helm.release.v1.* names |
| Status | Pass |

### ORG-ADV-005: Ambiguous group name returns error

| Field | Value |
|-------|-------|
| Test ID | ORG-ADV-005 |
| Type | Integration |
| Operator question | What happens when a group name matches multiple groups? |
| Fixture | Two application groups with same name in different namespaces |
| Command | `kos describe groups myapp` (without -n) |
| Expected | Error listing ambiguous matches with suggestion to use -n |
| Observed | |
| Status | Not Run |
| Negative assertion | Must not silently select first match |

### ORG-ADV-006: Application and release share same name

| Field | Value |
|-------|-------|
| Test ID | ORG-ADV-006 |
| Type | Integration |
| Operator question | Does describe groups resolve only Application type? |
| Fixture | Application group "argocd" and Release group "argocd" both exist |
| Command | `kos describe groups argocd` |
| Expected | Shows only the Application group, not the Release |
| Observed | |
| Status | Not Run |

---

## 4. Scope and Filtering Tests

### ORG-SCO-001: Default scope shows all application groups

| Field | Value |
|-------|-------|
| Test ID | ORG-SCO-001 |
| Type | Integration |
| Operator question | Does `kos groups` without flags show all application groups? |
| Fixture | Multiple Helm-installed applications |
| Command | `kos groups` |
| Expected | All Application-type groups listed regardless of namespace |
| Observed | Pass — 13 application groups listed across all namespaces |
| Status | Pass |

### ORG-SCO-002: Namespace filter on groups

| Field | Value |
|-------|-------|
| Test ID | ORG-SCO-002 |
| Type | Integration |
| Operator question | Does -n filter groups by home namespace? |
| Fixture | Groups in argocd, observability, cert-manager namespaces |
| Command | `kos groups -n observability` |
| Expected | Only groups with homeNamespace=observability |
| Observed | Pass — 3 groups shown (grafana, kube-state-metrics, prometheus-node-exporter), no argocd or cert-manager |
| Status | Pass |
| Negative assertion | Groups from other namespaces not shown |

### ORG-SCO-003: Namespace filter on resources

| Field | Value |
|-------|-------|
| Test ID | ORG-SCO-003 |
| Type | Integration |
| Operator question | Does -n filter resources correctly? |
| Fixture | Resources across multiple namespaces |
| Command | `kos resources -n argocd` |
| Expected | Only resources in argocd namespace |
| Observed | Pass — 59 resources shown, all in argocd namespace |
| Status | Pass |

### ORG-SCO-004: Kind filter is case-insensitive

| Field | Value |
|-------|-------|
| Test ID | ORG-SCO-004 |
| Type | Unit |
| Operator question | Does `kos resources deployment` match "Deployment"? |
| Fixture | Resources with kind "Deployment" |
| Function | `matchesKindFilter("Deployment", "deployment")` |
| Expected | Returns true |
| Observed | Pass — case-insensitive matching works for all cases |
| Status | Pass |

### ORG-SCO-005: Type filter on groups

| Field | Value |
|-------|-------|
| Test ID | ORG-SCO-005 |
| Type | Integration |
| Operator question | Does --type filter group type? |
| Fixture | Both Application and Release groups exist |
| Command | `kos groups --type Release` |
| Expected | Only Release-type groups shown |
| Observed | Pass — release type filter accepted, shows matching groups |
| Status | Pass |

---

## 5. Output Parity Tests

### ORG-OUT-001: JSON output matches table content

| Field | Value |
|-------|-------|
| Test ID | ORG-OUT-001 |
| Type | Integration |
| Operator question | Does -o json contain the same groups as default table? |
| Fixture | Helm-installed applications |
| Command | `kos groups` vs `kos groups -o json` |
| Expected | Same group names, same member counts, same confidence levels |
| Observed | Pass — JSON output contains all groups with matching names, resourceCount, confidence fields |
| Status | Pass |
| Invariant | len(json_output) == number of table rows |

### ORG-OUT-002: YAML output matches JSON content

| Field | Value |
|-------|-------|
| Test ID | ORG-OUT-002 |
| Type | Integration |
| Operator question | Does -o yaml produce equivalent content to -o json? |
| Fixture | Helm-installed applications |
| Command | `kos groups -o json` vs `kos groups -o yaml` |
| Expected | Same data, different serialization format |
| Observed | Pass — YAML output contains componentCount, confidence, evidence, groupType, members matching JSON structure |
| Status | Pass |

### ORG-OUT-003: Positional filter applies to structured output

| Field | Value |
|-------|-------|
| Test ID | ORG-OUT-003 |
| Type | Integration |
| Operator question | Does `kos groups argocd -o json` return only argocd? |
| Fixture | Multiple application groups |
| Command | `kos groups argocd -o json` |
| Expected | JSON array with exactly one element (argocd) |
| Observed | Pass — Returns array with 1 element, name=argocd, resourceCount=64 |
| Status | Pass |
| Negative assertion | Must not return all groups |

### ORG-OUT-004: Describe output includes evidence

| Field | Value |
|-------|-------|
| Test ID | ORG-OUT-004 |
| Type | Integration |
| Operator question | Does describe show the evidence that formed the group? |
| Fixture | Helm-installed argocd |
| Command | `kos describe groups argocd` |
| Expected | Evidence section lists field paths and observed values (part-of, instance, helm-release) |
| Observed | Pass — Evidence section shows helm-release=argocd, metadata.labels[app.kubernetes.io/instance]=argocd, metadata.labels[app.kubernetes.io/part-of]=argocd |
| Status | Pass |

### ORG-OUT-005: Describe groups shows component hierarchy

| Field | Value |
|-------|-------|
| Test ID | ORG-OUT-005 |
| Type | Integration |
| Operator question | Does describe present resources grouped by declared component? |
| Fixture | Helm-installed argocd (has app.kubernetes.io/component labels) |
| Command | `kos describe groups argocd` |
| Expected | Components section with named subsections, each containing Workload and Resources |
| Observed | Pass — Components section shows 8 named components + (unassigned). Each shows Workload and Resources subsections. |
| Status | Pass |

---

## 6. Aggregate Reconciliation Tests

### ORG-REC-001: Resource count matches member list

| Field | Value |
|-------|-------|
| Test ID | ORG-REC-001 |
| Type | Integration |
| Operator question | Does ResourceCount equal the number of members in detail? |
| Fixture | Helm-installed argocd |
| Commands | `kos groups argocd -o json` → check resourceCount vs len(members) |
| Expected | resourceCount == len(members) |
| Observed | Pass — resourceCount=64, members array length=64 |
| Status | Pass |
| Invariant | Summary count must reconcile with detail |

### ORG-REC-002: Workload count matches workload-kind members

| Field | Value |
|-------|-------|
| Test ID | ORG-REC-002 |
| Type | Integration |
| Operator question | Does WorkloadCount match count of Deployment/StatefulSet/DaemonSet/CronJob/Job members? |
| Fixture | Helm-installed argocd |
| Commands | `kos groups argocd -o json` → count members where kind is workload |
| Expected | workloadCount == count of workload-kind members |
| Observed | Pass — workloadCount=7 matches 7 workload-kind members (6 Deployments + 1 StatefulSet) |
| Status | Pass |

### ORG-REC-003: Component count matches distinct component labels

| Field | Value |
|-------|-------|
| Test ID | ORG-REC-003 |
| Type | Integration |
| Operator question | Does ComponentCount match unique component values? |
| Fixture | Helm-installed argocd |
| Commands | `kos groups argocd -o json` → count unique non-empty component fields |
| Expected | componentCount == count of unique component values |
| Observed | Pass — componentCount=8 matches 8 unique component label values |
| Status | Pass |

### ORG-REC-004: No resource counted twice in same group

| Field | Value |
|-------|-------|
| Test ID | ORG-REC-004 |
| Type | Unit |
| Operator question | Can a resource appear twice in one group's member list? |
| Fixture | Resource with both part-of and instance pointing to same group |
| Function | `grouping.BuildGroups()` → check member uniqueness |
| Expected | Each resource key appears exactly once per group |
| Observed | Pass — resource with all three signals (part-of, instance, helm) appears once |
| Status | Pass |

---

## 7. Determinism Tests

### ORG-DET-001: Group IDs stable across runs

| Field | Value |
|-------|-------|
| Test ID | ORG-DET-001 |
| Type | Integration |
| Operator question | Do group IDs remain the same between CLI invocations? |
| Fixture | Helm-installed applications (static cluster) |
| Commands | Run `kos groups -o json` twice, compare IDs |
| Expected | Identical IDs in both runs |
| Observed | Pass — two consecutive runs produce identical JSON output |
| Status | Pass |

### ORG-DET-002: Member ordering stable

| Field | Value |
|-------|-------|
| Test ID | ORG-DET-002 |
| Type | Integration |
| Operator question | Is the member list in the same order each time? |
| Fixture | Helm-installed argocd |
| Commands | Run `kos groups argocd -o json` twice, compare member arrays |
| Expected | Identical ordering (sorted by component then resource key) |
| Observed | Pass — TestCLI_Determinism confirms identical JSON output across consecutive runs |
| Status | Pass |

### ORG-DET-003: Group listing order stable

| Field | Value |
|-------|-------|
| Test ID | ORG-DET-003 |
| Type | Integration |
| Operator question | Is the group list in the same order each time? |
| Fixture | Multiple Helm-installed applications |
| Commands | Run `kos groups` twice, compare table rows |
| Expected | Identical row ordering (sorted by group ID) |
| Observed | Pass — TestCLI_Determinism confirms identical output across runs |
| Status | Pass |

---

## 8. Real-World Installation Tests

### ORG-REAL-001: ArgoCD multi-component grouping

| Field | Value |
|-------|-------|
| Test ID | ORG-REAL-001 |
| Type | Manual |
| Operator question | Does ArgoCD produce a coherent multi-component application group? |
| Fixture | `helm install argocd argo/argo-cd` |
| Command | `kos describe groups argocd` |
| Expected | 7+ workloads, 8+ components, 50+ resources. Components include server, repo-server, application-controller, etc. |
| Observed | Pass — 7 workloads, 8 components, 64 resources. Components: application-controller, applicationset-controller, dex-server, notifications-controller, redis, redis-secret-init, repo-server, server. Shared resources in (unassigned). |
| Status | Pass |

### ORG-REAL-002: cert-manager cross-namespace grouping

| Field | Value |
|-------|-------|
| Test ID | ORG-REAL-002 |
| Type | Manual |
| Operator question | Does cert-manager merge its kube-system resources into one group? |
| Fixture | `helm install cert-manager` |
| Command | `kos describe groups cert-manager` |
| Expected | Single group with Scope=Cluster, MemberNamespaces includes cert-manager and kube-system |
| Observed | Pass — Single group, Scope: Cluster, Namespaces: cert-manager, kube-system. 3 workloads, 5 components, 47 resources. |
| Status | Pass |

### ORG-REAL-003: Monitoring stack in shared namespace

| Field | Value |
|-------|-------|
| Test ID | ORG-REAL-003 |
| Type | Manual |
| Operator question | Are grafana, kube-state-metrics, and node-exporter separate groups in observability? |
| Fixture | All three installed in observability namespace |
| Command | `kos groups -n observability` |
| Expected | Three distinct groups, not merged into one |
| Observed | Pass — Three separate groups: grafana (8 members, Corroborating), kube-state-metrics (6, Corroborating), prometheus-node-exporter (3, Declared). |
| Status | Pass |

### ORG-REAL-004: DaemonSet-based node agent

| Field | Value |
|-------|-------|
| Test ID | ORG-REAL-004 |
| Type | Manual |
| Operator question | Does node-exporter appear as a group with its DaemonSet? |
| Fixture | `helm install node-exporter` |
| Command | `kos describe groups prometheus-node-exporter` |
| Expected | Group contains DaemonSet and associated ServiceAccount, Service |
| Observed | Pass — Group contains DaemonSet, ServiceAccount, and Service (3 resources total). Confidence: Declared (part-of label only). |
| Status | Pass |

### ORG-REAL-005: Ingress controller

| Field | Value |
|-------|-------|
| Test ID | ORG-REAL-005 |
| Type | Manual |
| Operator question | Does ingress-nginx produce a coherent application group? |
| Fixture | `helm install ingress-nginx` |
| Command | `kos describe groups ingress-nginx` |
| Expected | Group with controller Deployment, admission webhook, services |
| Observed | Pass — Group with 10 members including controller Deployment, admission Service, ValidatingWebhookConfiguration (after adding webhook kinds to watch list). Defect resolved: ValidatingWebhookConfiguration and MutatingWebhookConfiguration added to default GVK watch list. |
| Status | Pass (defect resolved) |

### ORG-REAL-006: Release listing matches Helm

| Field | Value |
|-------|-------|
| Test ID | ORG-REAL-006 |
| Type | Manual |
| Operator question | Does `kos releases` list all Helm releases? |
| Fixture | Multiple Helm-installed applications |
| Command | `kos releases` vs `helm list -A` |
| Expected | Every Helm release appears. Resource counts are reasonable. |
| Observed | Pass — All 13 Helm releases listed with MANAGER=Helm, REVISION, STATUS=deployed, and APPLICATION linkage. Enhancement applied: architecture refactored to use Manager interface supporting multi-manager model; `-o wide` shows SOURCE and MANAGED columns; application matching improved to handle chart-name vs release-name differences (e.g., node-exporter → prometheus-node-exporter). |
| Status | Pass (enhanced) |

---

## 9. Persistence and Reconciliation Tests

### ORG-PER-001: Groups stable after edge restart

| Field | Value |
|-------|-------|
| Test ID | ORG-PER-001 |
| Type | Integration |
| Operator question | Do groups remain the same after restarting the edge? |
| Fixture | Running edge with Helm-installed applications |
| Steps | 1. Run `kos groups -o json` 2. Restart edge 3. Run `kos groups -o json` again |
| Expected | Identical output |
| Observed | |
| Status | Not Run |

### ORG-PER-002: New resource appears in correct group

| Field | Value |
|-------|-------|
| Test ID | ORG-PER-002 |
| Type | Integration |
| Operator question | Does a newly created resource with app labels appear in its group? |
| Fixture | Create a ConfigMap with `app.kubernetes.io/instance=argocd` in argocd namespace |
| Steps | 1. Create resource 2. Wait for sync 3. Check `kos groups argocd -o json` |
| Expected | New resource appears as member of argocd group |
| Observed | |
| Status | Not Run |

### ORG-PER-003: Deleted resource removed from group

| Field | Value |
|-------|-------|
| Test ID | ORG-PER-003 |
| Type | Integration |
| Operator question | Does a deleted resource disappear from its group? |
| Fixture | Delete a resource from argocd namespace |
| Steps | 1. Note current members 2. Delete resource 3. Wait for sync 4. Check members |
| Expected | Deleted resource no longer in member list. Counts decremented. |
| Observed | |
| Status | Not Run |

---

## 10. Cross-Axis Handoff Tests

### ORG-XAX-001: Group to releases

| Field | Value |
|-------|-------|
| Test ID | ORG-XAX-001 |
| Type | Integration |
| Operator question | Can I navigate from an application group to its associated release? |
| Fixture | Helm-installed argocd |
| Commands | `kos groups argocd` → `kos releases argocd` |
| Expected | Release exists with same name, shows managed resources count |
| Observed | Pass — `kos releases argocd` shows release with 64 resources and APPLICATION=argocd |
| Status | Pass |

### ORG-XAX-002: Group to relationships

| Field | Value |
|-------|-------|
| Test ID | ORG-XAX-002 |
| Type | Integration |
| Operator question | Can I see relationships for a workload discovered through groups? |
| Fixture | Helm-installed argocd |
| Commands | `kos describe groups argocd` → pick workload → `kos relationships Deployment argocd-server -n argocd` |
| Expected | Relationships shown for the workload with evidence and confidence |
| Observed | Not Run (requires additional cluster invocation) |
| Status | Not Run |

### ORG-XAX-003: Group to shapes

| Field | Value |
|-------|-------|
| Test ID | ORG-XAX-003 |
| Type | Integration |
| Operator question | Can I see shape classification for workloads in a group? |
| Fixture | Helm-installed argocd |
| Commands | `kos describe groups argocd` → `kos shapes` |
| Expected | Workloads from the group appear classified in shapes output |
| Observed | |
| Status | Not Run |

### ORG-XAX-004: Group to graph export

| Field | Value |
|-------|-------|
| Test ID | ORG-XAX-004 |
| Type | Integration |
| Operator question | Do group nodes and MemberOf edges appear in graph export? |
| Fixture | Helm-installed argocd |
| Command | `kos graph export` / Edge API `GET /api/v1/graph` |
| Expected | LogicalResourceGroup nodes present. MemberOf edges connect resources to groups. |
| Observed | Pass — CLI exports 26 LogicalResourceGroup nodes, 400 MemberOf/MemberOfRelease edges, 27 ClassifiedAs edges. Edge API produces equivalent output. |
| Status | Pass |
| Invariant | Number of MemberOf edges == total members across all groups |
