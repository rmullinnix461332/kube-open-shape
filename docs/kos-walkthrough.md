# Exploring a Kubernetes Cluster with Kube Open Shape

Kubernetes exposes individual resources effectively, but understanding how those resources form applications, releases, architectural structures, and dependency graphs normally requires many commands and substantial operator interpretation.

Kube Open Shape (`kos`) builds a local knowledge graph from observed Kubernetes resources and makes that knowledge navigable through four complementary traversal axes:

1. **Organization** — What exists, what belongs together, and who controls it?
2. **Deployment** — What was deployed, by which manager, from what source and revision?
3. **Structure** — What architectural patterns comprise the cluster?
4. **Graph** — What depends on what, what is exposed, and what is the blast radius?

This walkthrough explores one cluster through each axis.

> KOS is not a health-monitoring tool. The walkthrough focuses on cluster composition, provenance, structure, relationships, and lifecycle knowledge.

---

## Test Environment

| Property | Value |
|---|---|
| Kubernetes distribution | Kind (Kubernetes in Docker) |
| Kubernetes version | v1.36.1 |
| KOS version or commit | a79e04f (main) |
| Cluster context | kind-kind |
| Namespaces | 16 |
| Total resources observed | 509 |

Installed products:

- Argo CD (chart:argo-cd@10.4.0)
- cert-manager (chart:cert-manager-v1.21.1)
- external-secrets (chart:external-secrets@2.9.0)
- ingress-nginx (chart:ingress-nginx@4.15.1)
- Grafana (chart:grafana@12.11.0)
- kube-state-metrics (chart:kube-state-metrics@8.4.0)
- prometheus-node-exporter (chart:prometheus-node-exporter@4.56.1)
- KOS integration fixtures (fixture-simple-a/b/c, fixture-stateful, fixture-adv-*)

> This cluster contains Argo CD itself but does not have Argo-managed Applications deployed. The Deployment axis currently demonstrates Helm-only lifecycle management. Argo-managed release examples (Application/ApplicationSet authority handoff) will be added when a fixture is available.

```console
$ kubectl config current-context
kind-kind
```

```console
$ kos report
=== Cluster Knowledge Report ===
Resources: 509
Ownership:
  Managed               471  (92.5%)
  No Known Authority     14  (2.8%)
  Contended              24  (4.7%)
  Authorities            26
Relationships:
  Edges: 297
  Nodes: 357
Candidate Shape Groups (model: builtin:structural-composition-v1):
  Groups: 16
  Instances: 17
    candidate-26b64f33bd03 (Deployment) — 2 instances [Probable/Exact/Partial]
    ... 15 additional candidates ...
```

> The Relationships section reports structural edges only (UsesServiceAccount, SelectsWorkload, Mounts, References, Owns, etc.). The full knowledge graph export (`kos graph export`) includes contextual edges (MemberOf, MemberOfRelease, BelongsToRelease, ClassifiedAs) for a total of 734 edges across 535 nodes.

---

## 1. Organization Axis

The Organization axis provides a navigable view of the cluster inventory.

### 1.1 Start With the Complete Resource Inventory

```console
$ kos resources
KIND                            NAMESPACE              NAME                                                     AGE
ClusterRole                                            admin                                                    3d
ClusterRole                                            argocd-application-controller                            1d
ClusterRole                                            cert-manager-cainjector                                  1d
... output omitted (509 resources across 20 kinds) ...
StatefulSet                     argocd                 argocd-application-controller                            1d
StatefulSet                     fixture-stateful       fixture-stateful                                         1d
ValidatingWebhookConfiguration                         cert-manager-webhook                                     1d
509 resources
```

#### Observation

The cluster contains 509 resources across 20 distinct kinds — from cluster-scoped infrastructure (ClusterRole, ClusterRoleBinding, CRDs) to namespaced application resources (Deployments, Services, ConfigMaps). The flat inventory establishes breadth but provides no structure. An operator cannot determine from this list alone which resources form one application or which lack lifecycle authority.

### 1.2 Identify Logical Groups

```console
$ kos groups
GROUP                     HOME NAMESPACE         MEMBERS  CONFIDENCE     EVIDENCE
argocd                    argocd                 64       Corroborating  part-of, instance, Helm
cert-manager              cert-manager           49       Corroborating  instance, Helm
external-secrets          external-secrets       22       Corroborating  instance, Helm
grafana                   observability          8        Corroborating  instance, Helm
ingress-nginx             ingress-system         11       Corroborating  part-of, instance, Helm
kube-state-metrics        observability          6        Corroborating  part-of, instance, Helm
fixture-simple-a          fixture-a              4        Corroborating  instance, Helm
... 5 additional fixture groups ...
prometheus-node-exporter  observability          3        Declared       part-of
13 groups
```

#### Observation

KOS discovered 13 logical application groups. Most have `Corroborating` confidence — multiple independent evidence sources (Helm release metadata, `app.kubernetes.io/instance` label, `app.kubernetes.io/part-of` label) agree on membership. Groups organize related application resources into navigable boundaries while leaving platform, shared, and currently ungrouped resources visible in the broader inventory.

### 1.3 Drill Into an Application

```console
$ kos describe groups argocd
Group:          argocd
Type:           Application
Home Namespace: argocd
Confidence:     Corroborating
Workloads:      7
Components:     8
Resources:      64
Evidence:
  helm-release=argocd
  metadata.labels[app.kubernetes.io/instance]=argocd
  metadata.labels[app.kubernetes.io/part-of]=argocd
Components:
  (unassigned)
    Resources: 3 ConfigMaps (shared), 3 CRDs
  application-controller
    Workload: StatefulSet/argocd/argocd-application-controller
    Resources: ClusterRole, ClusterRoleBinding, NetworkPolicy, Role, RoleBinding, ServiceAccount
  applicationset-controller
    Workload: Deployment/argocd/argocd-applicationset-controller
    Resources: ReplicaSet, Role, RoleBinding, Service, ServiceAccount
  server
    Workload: Deployment/argocd/argocd-server
    Resources: ClusterRole, ClusterRoleBinding, ConfigMap (×3), NetworkPolicy, ReplicaSet, Role,
               RoleBinding, Secret, Service, ServiceAccount
  ... 4 additional components (dex-server, notifications-controller, redis, repo-server) ...
```

#### Observation

Argo CD comprises 7 workloads organized into 8 components totaling 64 resources. The component hierarchy maps directly to the operational architecture. Shared resources (argocd-ssh-known-hosts-cm, argocd-tls-certs-cm) appear in the unassigned section — they serve multiple components. This single command replaces what would require `kubectl get all -n argocd`, `helm get manifest argocd`, and substantial interpretation.

### 1.4 Identify Lifecycle Authorities

```console
$ kos ownership
LIFECYCLE AUTHORITY         TYPE                  RESOURCES  DIRECT  INHERITED
rbac-defaults               KubernetesBootstrap   133        133     0
argocd                      Helm                  64         58      6
cert-manager                Helm                  49         46      3
external-secrets            Helm                  47         44      3
kube-controller-manager     KubernetesController  38         38      0
kubeadm                     KubernetesBootstrap   26         25      1
... 20 additional authorities ...
(no known authority)        —                     14         —       —
509 resources, 26 known authorities
```

#### Observation

The cluster has 26 known lifecycle authorities across 5 types: KubernetesBootstrap (platform RBAC defaults, kubeadm), KubernetesController (kube-controller-manager, service-account-controller), ClusterDistribution (Kind), Helm (13 releases), and Controller (cert-manager-controller, ingress-nginx). The `Inherited` column shows framework descendants — ReplicaSets owned by Deployments inherit their parent's Helm authority. 14 resources have no known authority.

### 1.5 Filter Ownership to One Authority

```console
$ kos ownership argocd
RESOURCE                                                       LIFECYCLE AUTHORITY  EVIDENCE       ATTRIBUTION
ClusterRole/argocd-application-controller                      Helm/argocd          Authoritative  Direct
ClusterRole/argocd-notifications-controller                    Helm/argocd          Authoritative  Direct
... 52 Direct resources ...
ReplicaSet/argocd/argocd-server-6c967757bb                     Helm/argocd          Authoritative  Inherited
... 5 additional Inherited ReplicaSets ...
64 resources, 1 authority record(s)
```

#### Observation

The argocd Helm authority controls 64 resources: 58 direct (explicitly in the Helm release manifest) and 6 inherited (ReplicaSets created by Kubernetes controllers as framework descendants of Helm-managed Deployments).

### 1.6 Drill Down to One Resource

```console
$ kos describe resource Deployment argocd-server -n argocd
Kind:            Deployment
Name:            argocd-server
Namespace:       argocd
UID:             be0c499c-9b75-4997-83c1-80f5a2819044
Created:         2026-08-18 15:42:50
Ownership:
  Classification: Managed
  Confidence:     Authoritative
  Owner:          Helm/argocd
Groups:
  argocd
Shape:
  Definition: kos-default-application
  Role:       application
Relationships:
  Outgoing (6):
    → ServiceAccount/argocd/argocd-server  [UsesServiceAccount]  (ExplicitField)
    → ConfigMap/argocd/argocd-ssh-known-hosts-cm  [Mounts]  (ExplicitField)
    → ConfigMap/argocd/argocd-tls-certs-cm  [Mounts]  (ExplicitField)
    → ConfigMap/argocd/argocd-cmd-params-cm  [Mounts]  (ExplicitField)
    → Secret/argocd/argocd-redis  [References]  (ExplicitField)
    → ReplicaSet/argocd/argocd-server-6c967757bb  [Owns]  (OwnerReference)
  Incoming (1):
    ← Service/argocd/argocd-server  [SelectsWorkload]  (SelectorMatch)
```

#### Observation

For one resource, KOS shows: identity (UID, creation time), ownership (Helm/argocd with Authoritative confidence), group membership (argocd), shape classification (application), and structural relationships (6 outgoing dependencies, 1 incoming consumer). The Deployment reaches three ConfigMaps through Mounts relationships and one Secret through References — graph knowledge that is not visible from `kubectl get` alone.

### 1.7 Identify Resources Without Known Authority

```console
$ kos ownership unmanaged
RESOURCE
Namespace/argocd
Namespace/cert-manager
Namespace/external-secrets
... 8 additional namespaces ...
Secret/argocd/argocd-redis
Secret/cert-manager/cert-manager-webhook-ca
Secret/ingress-system/ingress-nginx-admission
14 resources with no known authority
```

#### Observation

"No known authority" means KOS found insufficient observed attribution — not that these resources are abandoned. The 11 namespaces may have been created by Terraform, a CI pipeline, `helm --create-namespace`, or manual `kubectl create ns` — none of which leave Helm-style ownership metadata. The three Secrets were generated during installation or runtime but lack sufficient provenance metadata. Absence of authority alone does not authorize Janitor deletion — it surfaces operational ambiguity for the operator to resolve.

### Organization Axis Summary

```text
Cluster inventory (509 resources)
  → logical groups (13 applications)
  → application hierarchy (argocd: 7 workloads, 8 components)
  → lifecycle authorities (26 authorities)
  → authority inventory (argocd: 64 resources)
  → individual resource evidence
  → unexplained inventory (14 resources)
```

---

## 2. Deployment Axis

The Deployment axis describes how software was installed and how its deployed state is represented.

### 2.1 List Releases

```console
$ kos releases
RELEASE                   NAMESPACE              MANAGER  REVISION  STATUS    APPLICATION
argocd                    argocd                 Helm     1         deployed  argocd
cert-manager              cert-manager           Helm     1         deployed  cert-manager
external-secrets          external-secrets       Helm     1         deployed  external-secrets
ingress-nginx             ingress-system         Helm     1         deployed  ingress-nginx
grafana                   observability          Helm     1         deployed  grafana
kube-state-metrics        observability          Helm     1         deployed  kube-state-metrics
node-exporter             observability          Helm     1         deployed  prometheus-node-exporter
... 6 fixture releases at revision 2 ...
13 releases
```

### 2.2 Request the Wide Release View

```console
$ kos releases -o wide
RELEASE           NAMESPACE         MANAGER  REVISION  STATUS    SOURCE                                 MANAGED  APPLICATION
argocd            argocd            Helm     1         deployed  chart:argo-cd@10.4.0                   61       argocd
cert-manager      cert-manager      Helm     1         deployed  chart:cert-manager-v1.21.1             49       cert-manager
external-secrets  external-secrets  Helm     1         deployed  chart:external-secrets@2.9.0           47       external-secrets
ingress-nginx     ingress-system    Helm     1         deployed  chart:ingress-nginx@4.15.1             11       ingress-nginx
grafana           observability     Helm     1         deployed  chart:grafana@12.11.0                  8        grafana
kube-state-metrics observability    Helm     1         deployed  chart:kube-state-metrics@8.4.0         6        kube-state-metrics
node-exporter     observability     Helm     1         deployed  chart:prometheus-node-exporter@4.56.1  3        prometheus-node-exporter
```

#### Observation

Wide output adds source chart and version and managed-resource count. The MANAGED column (61 for argocd) may differ slightly from the group MEMBERS (64) because group membership includes framework descendants (ReplicaSets) and shared resources that are counted differently from the release manifest inventory.

### 2.3 Describe a Helm Release

```console
$ kos describe releases argocd
Release:         argocd
Namespace:       argocd
Manager:         Helm
Status:          deployed
Revision:        1
Managed:         61
Source:
  Chart:         argo-cd
  Chart Version: 10.4.0
```

#### Observation

The release record answers: how was this software installed (Helm), what version (chart argo-cd@10.4.0), what revision (1, never upgraded), and how many resources does the release manifest contain (61). This is the lifecycle record behind the `Helm/argocd` authority.

### 2.4 Compare Release Inventory With Group Inventory

```console
$ kos describe groups argocd
Group:          argocd
Type:           Application
Home Namespace: argocd
Confidence:     Corroborating
Workloads:      7
Components:     8
Resources:      64
Evidence:
  helm-release=argocd
  metadata.labels[app.kubernetes.io/instance]=argocd
  metadata.labels[app.kubernetes.io/part-of]=argocd
Components:
  (unassigned)
    Resources: 3 ConfigMaps (shared), 3 CRDs
  application-controller
    Workload: StatefulSet/argocd/argocd-application-controller
    Resources: ClusterRole, ClusterRoleBinding, NetworkPolicy, Role, RoleBinding, ServiceAccount
  server
    Workload: Deployment/argocd/argocd-server
    Resources: ClusterRole, ClusterRoleBinding, ConfigMap (×3), NetworkPolicy, ReplicaSet, Role,
               RoleBinding, Secret, Service, ServiceAccount
  ... 5 additional components ...
```

#### Observation

- **Release** answers: how the software was deployed (Helm chart, revision, manifest resources)
- **Group** answers: what logically belongs together (including controller-generated descendants)

The argocd release manages 61 resources directly. The argocd group contains 64 members because group membership also includes 6 ReplicaSets (framework descendants created by Kubernetes controllers) and excludes the authority record (Helm release Secret) from the member count. Different questions produce different totals — both are correct.

### 2.5 Janitor Findings and Actionability

```console
$ kos findings
RULE                     RESOURCE                                          SEVERITY  ACTIONABILITY  AGE   GRACE
unmanaged-resources      Namespace/argocd                                  Warning   Actionable     0m    6d left
unmanaged-resources      Namespace/cert-manager                            Warning   Actionable     0m    6d left
... 12 additional unmanaged-resources findings ...
disconnected-configmaps  ConfigMap/argocd/argocd-notifications-cm          Info      Actionable     0m    2d left
disconnected-configmaps  ConfigMap/argocd/argocd-rbac-cm                   Info      Actionable     0m    2d left
disconnected-configmaps  ConfigMap/fixture-adv-unmounted/...unmounted      Info      Actionable     0m    2d left
disconnected-configmaps  ConfigMap/ingress-system/ingress-nginx-controller Info      Actionable     0m    2d left
disconnected-secrets     Secret/argocd/argocd-notifications-secret         Info      Actionable     0m    2d left
disconnected-secrets     Secret/argocd/argocd-secret                       Info      Actionable     0m    2d left
disconnected-secrets     Secret/cert-manager/cert-manager-webhook-ca       Info      Actionable     0m    2d left
orphaned-resources       ConfigMap/fixture-a/fixture-simple-a-config       Critical  Actionable     0m    —
... 23 additional orphaned-resources findings ...
45 active findings
```

#### Observation

Findings carry two orthogonal dimensions: **Status** (Active — the rule still matches) and **Actionability** (Actionable, Protected, or Indeterminate).

**Actionable means KOS has not identified a safety condition that forces the finding to Protected or Indeterminate.** It does not mean the resource is safe to delete. Current default rules are capped at Annotate, and the Janitor requires operator approval before any mutation.

The `orphaned-resources` rule (24 findings) and the `unmanaged-resources` rule (14 findings) have different definitions:
- **unmanaged-resources**: the ownership engine found no lifecycle authority at all (NoAuthority)
- **orphaned-resources**: the ownership engine found contended or broken authority chains (Contended)

The fixture resources appear as Orphaned because they are deployed by Helm but their individual resources use label metadata that conflicts with the Helm release ownership model — producing contended authority determination.

### Deployment Axis Summary

```text
Release inventory (13 Helm releases)
  → release source and version (chart:argo-cd@10.4.0)
  → release-to-group mapping
  → release detail (61 managed resources)
  → Janitor findings and actionability
```

> This walkthrough demonstrates Helm-only lifecycle management. When Argo-managed Applications are deployed, the Deployment axis will additionally demonstrate: Application → Group reconciliation, ApplicationSet generation, and the authority handoff chain (Terraform → ApplicationSet → Application → Group → Resources).

---

## 3. Structure Axis

The Structure axis overlays an operator-defined classification system on the cluster.

### 3.1 List Accepted Shapes

```console
$ kos shapes
Role Classifications:
  CLASSIFIER               ROLE         INSTANCES
  kos-default-application  application  23
  kos-default-node-system  node-system  3
Named Shapes:
  DEFINITION                VARIANT       ROLE         INSTANCES  TRAITS
  kos-stateful-application  7c10807e50c4  application  1
2 role classifiers, 1 named shape, 27 total instances
```

#### Observation

The cluster has 2 broad role classifiers (application for Deployments/StatefulSets, node-system for DaemonSets) and 1 named structural shape. Role classifiers assign a broad category. Named shapes define specific architectural compositions with required relationships and aliases. Candidates (unnamed recurring structures) do not appear as accepted shapes.

### 3.2 Describe the Named Shape Definition

The embedded default `kos-stateful-application` ShapeDefinition is:

```yaml
# Embedded ShapeDefinition (kos-stateful-application)
spec:
  classificationMode: Structural
  displayName: Stateful Application
  role: application
  priority: 300
  roots:
    - alias: workload
      resource: { apiGroups: ["apps"], kinds: ["StatefulSet"] }
  components:
    - alias: headlessService
      resource: { apiGroups: [""], kinds: ["Service"] }
      cardinality: { min: 1 }
    - alias: storage
      resource: { apiGroups: [""], kinds: ["PersistentVolumeClaim"] }
      cardinality: { min: 1 }
  relationships:
    - { from: workload, type: UsesHeadlessService, to: headlessService, required: true }
    - { from: workload, type: ClaimsStorage, to: storage, required: true }
  composition:
    unmatchedResources: IncludeAsVariant
```

This defines: a StatefulSet root, with a required headless Service and required PVC storage, bound by explicit relationships.

```console
$ kos describe shapes
Definition: kos-stateful-application
Role:       application
Mode:       Structural
Instances:  1
  StatefulSet/fixture-stateful/fixture-stateful
```

#### Observation

The definition matches one instance: the fixture StatefulSet that has both a headless Service (UsesHeadlessService) and a PersistentVolumeClaim (ClaimsStorage). Other StatefulSets in the cluster (argocd-application-controller) do not match because they lack the required storage relationship.

### 3.3 List Unnamed Candidate Structures

```console
$ kos candidates
CANDIDATE               ROOT KIND    INSTANCES  RECURRENCE  PRIMARY                  SUPPORTING                                              CONTEXT
candidate-26b64f33bd03  Deployment   2          Probable    Deployment               Service                                                 ConfigMap
candidate-3c8de8a6c5d1  Deployment   1          Singleton   Deployment               Service, ServiceAccount, ClusterRole, ClusterRoleBinding  ConfigMap, Secret
candidate-46b9b7b66cac  DaemonSet    1          Singleton   DaemonSet                ServiceAccount, ClusterRole, ClusterRoleBinding         —
candidate-69581f5bc8f7  StatefulSet  1          Singleton   StatefulSet              Service, ServiceAccount, PersistentVolumeClaim          ConfigMap, Secret
... 12 additional singleton candidates ...
16 candidate groups, 17 unnamed instances
```

### 3.4 Explain One Candidate

```console
$ kos candidates explain candidate-26b64f33bd03
Candidate Shape Group: candidate-26b64f33bd03
Fingerprints:
  Semantic:   sha256:26b64f33bd032c7ee2576a14
  Mechanical: sha256:74601a98e336dae3548d1a6d
Evidence:
  Recurrence: Probable (2 instances)
  Cohesion:   Exact
  Coverage:   Partial
Grouping Basis: Exact semantic fingerprint (defining relationships)
Defining Resources:
  Deployment: 100%
  ConfigMap: 100%
  Service: 100%
Framework Resources (excluded from semantic fingerprint):
  ReplicaSet: 100%
Defining Relationships:
  References: 100%
  SelectsWorkload: 100%
Structural Traits:
  exposesService
Instances:
  Deployment/fixture-a/fixture-simple-a (3 related resources)
  Deployment/fixture-b/fixture-simple-b (3 related resources)
```

#### Observation

This candidate represents a pattern: a Deployment that References a ConfigMap and is exposed via a Service (SelectsWorkload). `Partial` is the candidate engine's evidence-coverage assessment. It indicates that the observed defining relationships may not fully distinguish this candidate from other structures. It does not imply that every supported relationship type should be present — a simple web application does not need RBAC, storage, or every relationship the model supports.

### 3.5 Generate a Draft ShapeDefinition

```console
$ kos candidates generate candidate-26b64f33bd03
apiVersion: knowledge.kos.io/v1alpha1
kind: ShapeDefinition
spec:
  displayName: REVIEW REQUIRED
  role: Unclassified
  roots:
    - alias: root
      resource: { apiGroups: ["apps"], kinds: ["Deployment"] }
  components:
    - alias: configMap
      resource: { apiGroups: [""], kinds: ["ConfigMap"] }
      cardinality: { min: 1 }
    - alias: service
      resource: { apiGroups: [""], kinds: ["Service"] }
      cardinality: { min: 1 }
  relationships:
    - { from: root, type: References, to: configMap, required: true }
    - { from: service, type: SelectsWorkload, to: root, required: true }
  # --- Knowledge Gaps ---
  # - Relationship coverage is partial within builtin:structural-composition-v1
  # - Definition may match additional instances of the same root kind
```

### 3.6 Test the Draft Definition

```console
$ kos candidates test candidate-26b64f33bd03
Definition Test: candidate-26b64f33bd03
Target Validation:
  Source instances:   2
  Matched by def:    2/2
Classification Impact:
  Additional matches:  3
  Rejected roots:      17
  Accepted (additional):
    ✓ Deployment/argocd/argocd-repo-server
    ✓ Deployment/argocd/argocd-dex-server
    ✓ Deployment/fixture-c/fixture-simple-c
  Rejected (sample):
    ✗ Deployment/observability/kube-state-metrics — configMap has 0 instances
    ✗ Deployment/argocd/argocd-notifications-controller — service has 0 instances
    ✗ Deployment/observability/grafana — required relationship root→configMap not found
```

#### Observation

The dry-run shows the definition would match 5 total instances: the 2 source instances plus 3 additional. It correctly rejects 17 Deployments that lack either a ConfigMap reference or a selecting Service.

The three additional matches show that the generated definition is broader than a tentative "Simple Web App" affinity. The operator must either accept a broader shape name or identify another relationship or trait shared by the source instances that excludes the additional matches. This is exactly why the dry-run is valuable.

### Structure Axis Summary

```text
Accepted roles and shapes (2 classifiers, 1 named shape)
  → shape definition grammar
  → matched instance binding
  → unnamed candidates (16 groups, 17 instances)
  → candidate evidence (fingerprints, relationships, traits)
  → generated definition
  → dry-run validation (acceptance and rejection)
```

---

## 4. Graph Axis

The Graph axis exposes how resources are connected.

> This cluster does not contain Ingress resources pointing to application Services. The examples below demonstrate dependency traversal and blast radius but not external exposure paths. An externally-exposed fixture would be needed to demonstrate Ingress → Service → Workload traversal.

### 4.1 Summarize the Graph

From the report:
- **Structural edges**: 297 (UsesServiceAccount, SelectsWorkload, Mounts, References, Owns, BindsSubject, GrantsRole, ClaimsStorage, UsesHeadlessService)
- **All edges** (including contextual): 734 (adds MemberOfRelease: 217, MemberOf: 193, BelongsToRelease: 18, ClassifiedAs: 27)
- **Nodes**: 357 structural / 535 total

### 4.2 Inspect Relationships for One Workload

```console
$ kos relationships Deployment argocd-server -n argocd
Relationships for: Deployment/argocd/argocd-server
  Outgoing (6):
    → ServiceAccount/argocd/argocd-server  [UsesServiceAccount]  serviceAccountName=argocd-server (ExplicitField)
    → ConfigMap/argocd/argocd-ssh-known-hosts-cm  [Mounts]  configMap.name (ExplicitField)
    → ConfigMap/argocd/argocd-tls-certs-cm  [Mounts]  configMap.name (ExplicitField)
    → ConfigMap/argocd/argocd-cmd-params-cm  [Mounts]  configMap.name (ExplicitField)
    → Secret/argocd/argocd-redis  [References]  secretKeyRef.name (ExplicitField)
    → ReplicaSet/argocd/argocd-server-6c967757bb  [Owns]  ownerReferences (OwnerReference)
  Incoming (1):
    ← Service/argocd/argocd-server  [SelectsWorkload]  spec.selector (SelectorMatch)
```

#### Observation

Relationships are derived from explicit spec fields, owner references, and deterministic selector matching. None of the relationships shown rely on resource-name similarity. The 3 mounted ConfigMaps and 1 referenced Secret represent the workload's configuration dependencies. The ReplicaSet is a framework descendant (Owns via ownerReference — Kubernetes will garbage-collect it if the Deployment is deleted).

### 4.3 Inspect a Shared Configuration Resource (Blast Radius)

```console
$ kos describe resource ConfigMap argocd-ssh-known-hosts-cm -n argocd
Kind:            ConfigMap
Name:            argocd-ssh-known-hosts-cm
Namespace:       argocd
Ownership:       Managed (Helm/argocd, Authoritative)
Relationships:
  Incoming (3):
    ← Deployment/argocd/argocd-repo-server  [Mounts]
    ← Deployment/argocd/argocd-server  [Mounts]
    ← Deployment/argocd/argocd-applicationset-controller  [Mounts]
```

#### Observation

This ConfigMap has 3 consumers. Its incoming edges identify three Deployments that mount it. This is the blast radius of changing or removing this ConfigMap — three workloads would be affected. Graph knowledge proves it is actively referenced and cannot be evaluated by age or name alone. Any destructive Janitor action against the ConfigMap would fail the "no consumers outside action closure" qualification check. An annotation action may still be valid.

### 4.4 Traverse a Dependency Path

```console
$ kos reachable Service argocd-server -n argocd
Resources reachable from Service/argocd/argocd-server (depth=5):
  Deployment/argocd/argocd-server
  ServiceAccount/argocd/argocd-server
  ConfigMap/argocd/argocd-ssh-known-hosts-cm
  ConfigMap/argocd/argocd-tls-certs-cm
  ConfigMap/argocd/argocd-cmd-params-cm
  Secret/argocd/argocd-redis
  ReplicaSet/argocd/argocd-server-6c967757bb
7 reachable resources
```

#### Observation

The dependency closure starting from the Service traverses:

```text
Service/argocd/argocd-server
  → Deployment/argocd/argocd-server  [SelectsWorkload]
    → ServiceAccount  [UsesServiceAccount]
    → ConfigMap (×3)  [Mounts]
    → Secret          [References]
    → ReplicaSet      [Owns]
```

This is the downstream dependency neighborhood of the Service — all resources transitively reachable. It does not establish external exposure (no Ingress points to this Service in the current cluster configuration).

### 4.5 Demonstrate Teardown Ordering Knowledge

From the argocd-server relationships, the graph contains the information needed to derive teardown ordering:

```text
Exposure:
  Service may be removed first when disabling access

Workload root:
  Deployment is the primary deletion target

Framework descendants:
  ReplicaSet is expected to cascade through ownerReferences

Supporting providers:
  ConfigMaps and ServiceAccount are considered only after all consumers
  have been removed or included in the action closure

Shared dependencies:
  Retain when consumers exist outside the action closure
```

> This is an illustration of ordering information already present in the graph, not a complete or executable Janitor plan. A Service selecting a Deployment does not itself require that the Deployment be deleted. Likewise, the ReplicaSet is normally an expected cascade, not a separate explicit execution step.

The general principle for teardown ordering:

```text
Consumers before providers
RoleBinding before Role
Custom Resources before CRD
```

Note: removing a Service does not require deleting its selected Deployment. Each relationship type defines its own teardown semantics — not all edges contribute hard ordering constraints to an execution DAG.

### Graph Axis Summary

```text
Graph summary (297 structural edges, 357 nodes)
  → workload relationships (7 edges for argocd-server)
  → shared resource consumers (argocd-ssh-known-hosts-cm: 3 consumers)
  → dependency path traversal (Service → 7 reachable resources)
  → teardown ordering knowledge
```

| Question | Command |
|---|---|
| How large is the graph? | `kos report` |
| What relationships exist? | `kos relationships` |
| What is connected to one workload? | `kos relationships Deployment <name> -n <ns>` |
| What consumes a resource? | `kos describe resource ...` (incoming edges) |
| What is reachable downstream? | `kos reachable <kind> <name> -n <ns>` |
| What teardown ordering is required? | Relationship traversal (graph-derived) |

---

## Conclusion

The four axes are different projections of the same observed cluster knowledge.

```text
Organization — What exists, what belongs together, and who controls it?
Deployment   — How was it installed, from what source, and under which lifecycle manager?
Structure    — What recognizable architectural patterns comprise it?
Graph        — How is it connected, exposed, and affected by change?
```

A typical investigation moves naturally between them:

```text
Argo CD:
  Application group (64 resources)
    → Helm authority (chart:argo-cd@10.4.0, revision 1)
    → component hierarchy (7 workloads, 8 components)
    → individual resource (ConfigMap argocd-ssh-known-hosts-cm: 3 consumers)
    → dependency graph (Service → Deployment → ConfigMaps → Secret)

Stateful fixture:
  Application group (7 resources)
    → Stateful Application shape (bound instance)
    → headless Service + PVC + ConfigMap + Secret
    → findings (orphaned — contended authority)
```

KOS does not replace `kubectl`, Helm, or Argo CD. It supplies the navigable knowledge layer that connects the information those tools expose individually.
