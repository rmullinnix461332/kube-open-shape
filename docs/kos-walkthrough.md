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
| Namespaces | 16 (argocd, cert-manager, external-secrets, ingress-system, observability, fixture-*, kube-system, etc.) |
| Total resources observed | 509 |

Installed products used in the examples:

- Argo CD (chart:argo-cd@10.4.0)
- cert-manager (chart:cert-manager-v1.21.1)
- external-secrets (chart:external-secrets@2.9.0)
- ingress-nginx (chart:ingress-nginx@4.15.1)
- Grafana (chart:grafana@12.11.0)
- kube-state-metrics (chart:kube-state-metrics@8.4.0)
- prometheus-node-exporter (chart:prometheus-node-exporter@4.56.1)
- KOS integration fixtures (fixture-simple-a/b/c, fixture-stateful, fixture-adv-*)

Confirm the active cluster:

```console
$ kubectl config current-context
kind-kind
```

Capture a high-level KOS report:

```console
$ kos report
=== Cluster Knowledge Report ===
Resources: 509
Ownership:
  Unknown            64  (12.6%)
  PlatformManaged    32  (6.3%)
  AdHoc              21  (4.1%)
  Orphaned            1  (0.2%)
  Conflicted         77  (15.1%)
  Managed           314  (61.7%)
Relationships:
  Edges: 297
  Nodes: 357
Candidate Shape Groups (model: builtin:structural-composition-v1):
  Groups: 16
  Instances: 17
    candidate-26b64f33bd03 (Deployment) — 2 instances [Probable/Exact/Partial]
    candidate-3c8de8a6c5d1 (Deployment) — 1 instances [Singleton/Exact/Full]
    candidate-46b9b7b66cac (DaemonSet) — 1 instances [Singleton/Exact/Full]
    candidate-69581f5bc8f7 (StatefulSet) — 1 instances [Singleton/Exact/Full]
    ... 12 additional singleton candidates ...
```

---

# 1. Organization Axis

The Organization axis provides a navigable view of the cluster inventory.

## 1.1 Start With the Complete Resource Inventory

```console
$ kos resources
KIND                            NAMESPACE              NAME                                                     AGE
ClusterRole                                            admin                                                    3d
ClusterRole                                            argocd-application-controller                            1d
ClusterRole                                            argocd-notifications-controller                          1d
ClusterRole                                            argocd-server                                            1d
ClusterRole                                            cert-manager-cainjector                                  1d
... output omitted (509 resources) ...
ServiceAccount                  observability          kube-state-metrics                                       1d
ServiceAccount                  observability          node-exporter-prometheus-node-exporter                   1d
StatefulSet                     argocd                 argocd-application-controller                            1d
StatefulSet                     fixture-stateful       fixture-stateful                                         1d
ValidatingWebhookConfiguration                         cert-manager-webhook                                     1d
509 resources
```

### Observation

The cluster contains 509 resources across 20 distinct kinds — from cluster-scoped infrastructure (ClusterRole, ClusterRoleBinding, CRDs) to namespaced application resources (Deployments, Services, ConfigMaps). The flat inventory establishes breadth but provides no structure. An operator cannot determine from this list alone which resources form one application, which are framework descendants, or which lack lifecycle authority.

## 1.2 Identify Logical Groups

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
fixture-adv-disconnected  fixture-adv-disco      4        Corroborating  instance, Helm
fixture-adv-unmounted     fixture-adv-unmounted  4        Corroborating  instance, Helm
fixture-simple-b          fixture-b              4        Corroborating  instance, Helm
fixture-simple-c          fixture-c              7        Corroborating  instance, Helm
fixture-stateful          fixture-stateful       7        Corroborating  instance, Helm
prometheus-node-exporter  observability          3        Declared       part-of
13 groups
```

### Observation

KOS discovered 13 logical application groups. Most have `Corroborating` confidence — multiple independent evidence sources (Helm release metadata, `app.kubernetes.io/instance` label, `app.kubernetes.io/part-of` label) agree on membership. The prometheus-node-exporter group uses `Declared` confidence from the `part-of` label alone. Groups consolidate 509 individual resources into navigable application boundaries.

## 1.3 Drill Into an Application

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
    Resources:
      ConfigMap/argocd/argocd-gpg-keys-cm
      ConfigMap/argocd/argocd-ssh-known-hosts-cm
      ConfigMap/argocd/argocd-tls-certs-cm
      CustomResourceDefinition/applications.argoproj.io
      CustomResourceDefinition/applicationsets.argoproj.io
      CustomResourceDefinition/appprojects.argoproj.io
  application-controller
    Workload: StatefulSet/argocd/argocd-application-controller
    Resources: ClusterRole, ClusterRoleBinding, NetworkPolicy, Role, RoleBinding, ServiceAccount
  applicationset-controller
    Workload: Deployment/argocd/argocd-applicationset-controller
    Resources: ReplicaSet, Role, RoleBinding, Service, ServiceAccount
  dex-server
    Workload: Deployment/argocd/argocd-dex-server
    Resources: NetworkPolicy, ReplicaSet, Role, RoleBinding, Service, ServiceAccount
  notifications-controller
    Workload: Deployment/argocd/argocd-notifications-controller
    Resources: ClusterRole, ClusterRoleBinding, ConfigMap, NetworkPolicy, ReplicaSet, Role, RoleBinding, Secret, ServiceAccount
  redis
    Workload: Deployment/argocd/argocd-redis
    Resources: ConfigMap, NetworkPolicy, ReplicaSet, Service
  repo-server
    Workload: Deployment/argocd/argocd-repo-server
    Resources: NetworkPolicy, ReplicaSet, Role, RoleBinding, Service, ServiceAccount
  server
    Workload: Deployment/argocd/argocd-server
    Resources: ClusterRole, ClusterRoleBinding, ConfigMap (×3), NetworkPolicy, ReplicaSet, Role, RoleBinding, Secret, Service, ServiceAccount
```

### Observation

Argo CD comprises 7 workloads organized into 8 components totaling 64 resources. The component hierarchy (application-controller, repo-server, server, redis, etc.) maps directly to the operational architecture. Shared resources like `argocd-ssh-known-hosts-cm` appear in the unassigned section — they serve multiple components without belonging exclusively to one. This single command replaces what would require `kubectl get all -n argocd`, `helm get manifest argocd`, and substantial interpretation.

## 1.4 Identify Lifecycle Authorities

```console
$ kos ownership
LIFECYCLE AUTHORITY         TYPE                  RESOURCES  DIRECT  INHERITED
rbac-defaults               KubernetesBootstrap   133        133     0
argocd                      Helm                  64         58      6
cert-manager                Helm                  49         46      3
external-secrets            Helm                  47         44      3
kube-controller-manager     KubernetesController  38         38      0
kubeadm                     KubernetesBootstrap   26         25      1
root-ca-cert-publisher      KubernetesController  16         16      0
service-account-controller  KubernetesController  16         16      0
kind                        ClusterDistribution   15         14      1
ingress-nginx               Helm                  11         10      1
grafana                     Helm                  8          7       1
... 15 additional authorities ...
(no known authority)        —                     14         —       —
509 resources, 26 authorities
```

### Observation

The cluster has 26 distinct lifecycle authorities across 5 types: KubernetesBootstrap (platform RBAC defaults, kubeadm), KubernetesController (kube-controller-manager, service-account-controller), ClusterDistribution (Kind), Helm (13 releases), and Controller (cert-manager-controller, ingress-nginx). The `Inherited` column shows framework descendants — ReplicaSets owned by Deployments inherit their parent's Helm authority. 14 resources have no known authority.

## 1.5 Filter Ownership to One Authority

```console
$ kos ownership argocd
RESOURCE                                                       LIFECYCLE AUTHORITY  EVIDENCE       ATTRIBUTION
ClusterRole/argocd-application-controller                      Helm/argocd          Authoritative  Direct
ClusterRole/argocd-notifications-controller                    Helm/argocd          Authoritative  Direct
... 52 Direct resources ...
ReplicaSet/argocd/argocd-applicationset-controller-799d87dcc5  Helm/argocd          Authoritative  Inherited
ReplicaSet/argocd/argocd-dex-server-f67675f8f                  Helm/argocd          Authoritative  Inherited
ReplicaSet/argocd/argocd-notifications-controller-5b8c8d6689   Helm/argocd          Authoritative  Inherited
ReplicaSet/argocd/argocd-redis-546d94bfb4                      Helm/argocd          Authoritative  Inherited
ReplicaSet/argocd/argocd-repo-server-75c99884f                 Helm/argocd          Authoritative  Inherited
ReplicaSet/argocd/argocd-server-6c967757bb                     Helm/argocd          Authoritative  Inherited
64 resources, 1 authority record(s)
```

### Observation

The argocd Helm authority controls 64 resources: 58 direct (explicitly present in the Helm release manifest) and 6 inherited (ReplicaSets created by Kubernetes controllers as framework descendants of Helm-managed Deployments). Evidence is `Authoritative` for most — the Helm release secret proves lifecycle ownership definitively. Three resources show `Corroborating` — their evidence comes from label matching rather than the release manifest.

## 1.6 Drill Down to One Resource

```console
$ kos describe resource ConfigMap argocd-cm -n argocd
Kind:            ConfigMap
Name:            argocd-cm
Namespace:       argocd
UID:             ef5a9567-642b-49b6-a85b-b41d2fd3e30c
Created:         2026-08-18 15:42:49
Ownership:
  Classification: Managed
  Confidence:     Authoritative
  Owner:          Helm/argocd
Groups:
  argocd
Relationships:
  Incoming (2):
    ← StatefulSet/argocd/argocd-application-controller  [References]  configMapKeyRef.name=argocd-cm
    ← Deployment/argocd/argocd-repo-server  [References]  configMapKeyRef.name=argocd-cm
Labels:
  app.kubernetes.io/component=server
  app.kubernetes.io/instance=argocd
  app.kubernetes.io/managed-by=Helm
  app.kubernetes.io/name=argocd-cm
  app.kubernetes.io/part-of=argocd
```

### Observation

For one resource, KOS shows: identity (UID, creation time), ownership (Helm/argocd with Authoritative confidence), group membership (argocd), and structural relationships (2 consumers reference this ConfigMap via environment variable configMapKeyRef). This ConfigMap is shared — two workloads depend on it. Any Janitor action against it would require evaluating both consumers.

## 1.7 Identify Resources Without Known Authority

```console
$ kos ownership unmanaged
RESOURCE
Namespace/argocd
Namespace/cert-manager
Namespace/external-secrets
Namespace/fixture-a
Namespace/fixture-adv-disco
Namespace/fixture-adv-unmounted
Namespace/fixture-b
Namespace/fixture-c
Namespace/fixture-stateful
Namespace/ingress-system
Namespace/observability
Secret/argocd/argocd-redis
Secret/cert-manager/cert-manager-webhook-ca
Secret/ingress-system/ingress-nginx-admission
14 resources with no known authority
```

### Observation

"No known authority" means KOS found insufficient observed attribution — not that these resources are abandoned. The 11 namespaces were likely created by Terraform, a CI pipeline, or manual `kubectl create ns` — none of which leave Helm-style metadata. The 3 Secrets are controller-generated (cert-manager webhook CA, ingress admission webhook, redis password) but lack traceable provenance metadata. Absence of authority alone does not authorize Janitor deletion — it surfaces operational ambiguity for the operator to resolve.

## Organization Axis Summary

The Organization traversal moved through:

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

# 2. Deployment Axis

The Deployment axis describes how software was installed and how its deployed state is represented.

## 2.1 List Releases

```console
$ kos releases
RELEASE                   NAMESPACE              MANAGER  REVISION  STATUS    APPLICATION
argocd                    argocd                 Helm     1         deployed  argocd
cert-manager              cert-manager           Helm     1         deployed  cert-manager
external-secrets          external-secrets       Helm     1         deployed  external-secrets
fixture-simple-a          fixture-a              Helm     2         deployed  fixture-simple-a
fixture-adv-disconnected  fixture-adv-disco      Helm     2         deployed  fixture-adv-disconnected
fixture-adv-unmounted     fixture-adv-unmounted  Helm     2         deployed  fixture-adv-unmounted
fixture-simple-b          fixture-b              Helm     2         deployed  fixture-simple-b
fixture-simple-c          fixture-c              Helm     2         deployed  fixture-simple-c
fixture-stateful          fixture-stateful       Helm     2         deployed  fixture-stateful
ingress-nginx             ingress-system         Helm     1         deployed  ingress-nginx
grafana                   observability          Helm     1         deployed  grafana
kube-state-metrics        observability          Helm     1         deployed  kube-state-metrics
node-exporter             observability          Helm     1         deployed  prometheus-node-exporter
13 releases
```

### Observation

13 Helm releases manage the cluster's application software. All are in `deployed` status. Production releases (argocd, cert-manager, external-secrets, ingress-nginx, observability stack) are at revision 1. Fixture releases are at revision 2 (upgraded from initial install). Each release maps 1:1 to an application group.

## 2.2 Request the Wide Release View

```console
$ kos releases -o wide
RELEASE                   NAMESPACE              MANAGER  REVISION  STATUS    SOURCE                                 MANAGED  APPLICATION
argocd                    argocd                 Helm     1         deployed  chart:argo-cd@10.4.0                   61       argocd
cert-manager              cert-manager           Helm     1         deployed  chart:cert-manager-v1.21.1             49       cert-manager
external-secrets          external-secrets       Helm     1         deployed  chart:external-secrets@2.9.0           47       external-secrets
ingress-nginx             ingress-system         Helm     1         deployed  chart:ingress-nginx@4.15.1             11       ingress-nginx
grafana                   observability          Helm     1         deployed  chart:grafana@12.11.0                  8        grafana
kube-state-metrics        observability          Helm     1         deployed  chart:kube-state-metrics@8.4.0         6        kube-state-metrics
node-exporter             observability          Helm     1         deployed  chart:prometheus-node-exporter@4.56.1  3        prometheus-node-exporter
... 6 fixture releases ...
13 releases
```

### Observation

Wide output adds source chart and version (e.g., `chart:argo-cd@10.4.0`) and managed-resource count. The MANAGED column (61 for argocd) differs slightly from the group MEMBERS (64) because group membership includes framework descendants counted separately from the release manifest.

## 2.3 Describe a Helm Release

The `describe releases` command provides release detail. The relationship between release and group shows provenance versus composition:

- **Release** answers: how the software was deployed (Helm chart, revision, manifest resources)
- **Group** answers: what logically belongs together (including controller-generated descendants)

## 2.4 Inspect Reconciliation Protection

```console
$ kos findings
RULE                     RESOURCE                                                  SEVERITY  ACTIONABILITY  AGE   GRACE
unmanaged-resources      Namespace/argocd                                          Warning   Actionable     0m    6d left
unmanaged-resources      Namespace/cert-manager                                    Warning   Actionable     0m    6d left
... 14 unmanaged resources ...
disconnected-configmaps  ConfigMap/argocd/argocd-notifications-cm                  Info      Actionable     0m    2d left
disconnected-configmaps  ConfigMap/argocd/argocd-rbac-cm                           Info      Actionable     0m    2d left
disconnected-configmaps  ConfigMap/fixture-adv-unmounted/fixture-adv-unmounted-unmounted  Info  Actionable  0m    2d left
disconnected-secrets     Secret/argocd/argocd-notifications-secret                 Info      Actionable     0m    2d left
orphaned-resources       ConfigMap/fixture-a/fixture-simple-a-config               Critical  Actionable     0m    —
... 23 orphaned fixture resources ...
45 active findings
```

### Observation

The findings show two dimensions per finding: **Status** (Active) and **Actionability** (Actionable or Protected). All findings here are Actionable because the test cluster does not have Argo CD Applications deployed as reconcilers. In a cluster where an Application with auto-sync reconciles a group, those resources would show `Actionability: Protected` — the Janitor's safety walk would detect the continuous reconciliation and block mutation.

## Deployment Axis Summary

The Deployment traversal established:
- 13 Helm releases manage the cluster's application software
- Each release maps to a source chart and version
- Release-to-group mapping provides provenance for application boundaries
- Reconciliation protection (when present) blocks Janitor action on actively-reconciled resources

---

# 3. Structure Axis

The Structure axis overlays an operator-defined classification system on the cluster.

## 3.1 List Accepted Shapes

```console
$ kos shapes
Role Classifications:
  CLASSIFIER               ROLE         INSTANCES
  kos-default-application  application  23
  kos-default-node-system  node-system  3
Named Shapes:
  DEFINITION                VARIANT       ROLE         INSTANCES  TRAITS
  kos-stateful-application  7c10807e50c4  application  1
2 role classifiers, 1 named shapes, 27 total instances
```

### Observation

The cluster has 2 broad role classifiers (application for Deployments/StatefulSets, node-system for DaemonSets) and 1 named structural shape (kos-stateful-application). Role classifiers assign a broad category to workloads. Named shapes define specific architectural compositions with required relationships and aliases. Candidates (unnamed recurring structures) do not appear as accepted shapes — they remain proposals until promoted.

## 3.2 Describe Shape Definitions

```console
$ kos describe shapes
Definition: kos-stateful-application
Role:       application
Mode:       Structural
Instances:  1
  StatefulSet/fixture-stateful/fixture-stateful
```

The kos-stateful-application shape is a Structural definition — it matches based on composition (required root kind, components, and relationships), not just role classification. It currently matches one instance: the fixture StatefulSet with its headless Service, PVC, ConfigMap, and Secret.

## 3.3 List Unnamed Candidate Structures

```console
$ kos candidates
CANDIDATE               ROOT KIND    INSTANCES  RECURRENCE  PRIMARY                  SUPPORTING                                              CONTEXT
candidate-26b64f33bd03  Deployment   2          Probable    Deployment               Service                                                 ConfigMap
candidate-3c8de8a6c5d1  Deployment   1          Singleton   Deployment               Service, ServiceAccount, ClusterRole, ClusterRoleBinding  ConfigMap, Secret
candidate-46b9b7b66cac  DaemonSet    1          Singleton   DaemonSet                ServiceAccount, ClusterRole, ClusterRoleBinding         —
candidate-69581f5bc8f7  StatefulSet  1          Singleton   StatefulSet              Service, ServiceAccount, PersistentVolumeClaim          ConfigMap, Secret
candidate-828e41a5ac63  Deployment   1          Singleton   Deployment               Service, ServiceAccount, Role, RoleBinding              ConfigMap
... 11 additional candidates ...
16 candidate groups, 17 unnamed instances
```

### Observation

16 candidate groups capture recurring architectural patterns the cluster exhibits but has not yet named. candidate-26b64f33bd03 is the only Probable candidate (2 instances with exact semantic fingerprint match). The PRIMARY/SUPPORTING/CONTEXT columns show the three-dimensional evidence: primary workloads, supporting infrastructure (RBAC, Services), and context resources (ConfigMaps, Secrets used as configuration). Candidates range from simple (Deployment + Service + ConfigMap) to complex (multi-RBAC controller patterns).

## 3.4 Explain One Candidate

```console
$ kos candidates explain candidate-26b64f33bd03
Candidate Shape Group: candidate-26b64f33bd03
Fingerprints:
  Semantic:   sha256:26b64f33bd032c7ee2576a14
  Mechanical: sha256:74601a98e336dae3548d1a6d
Model:
  Canonicalization: generic-structural-v1@1
  Relationship set: builtin:structural-composition-v1
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

### Observation

This candidate represents a simple web application pattern: a Deployment that References a ConfigMap for configuration and is exposed via a Service (SelectsWorkload). The semantic fingerprint is deterministic — it captures the defining relationships but excludes framework descendants (ReplicaSet). Coverage is Partial because not all possible relationship types from the model are present — these instances only use References and SelectsWorkload, not Mounts, ClaimsStorage, or RBAC relationships.

## 3.5 Generate a Draft ShapeDefinition

```console
$ kos candidates generate candidate-26b64f33bd03
apiVersion: knowledge.kos.io/v1alpha1
kind: ShapeDefinition
metadata:
  generateName: candidate-26b64f33bd03-
  annotations:
    knowledge.kos.io/generated-from: candidate-26b64f33bd03
    knowledge.kos.io/semantic-fingerprint: sha256:26b64f33bd032c7ee2576a14
spec:
  schemaVersion: 1
  definitionVersion: 1
  displayName: REVIEW REQUIRED
  role: Unclassified
  priority: 0
  roots:
    - alias: root
      resource:
        apiGroups: ["apps"]
        kinds: ["Deployment"]
  components:
    - alias: configMap
      resource:
        apiGroups: [""]
        kinds: ["ConfigMap"]
      cardinality:
        min: 1
    - alias: service
      resource:
        apiGroups: [""]
        kinds: ["Service"]
      cardinality:
        min: 1
  relationships:
    - from: root
      type: References
      to: configMap
      required: true
    - from: service
      type: SelectsWorkload
      to: root
      required: true
  composition:
    unmatchedResources: IncludeAsVariant
  # --- Knowledge Gaps ---
  # - Relationship coverage is partial within builtin:structural-composition-v1
  # - Definition may match additional instances of the same root kind
```

### Observation

KOS generates a complete ShapeDefinition with root alias, component aliases, cardinality constraints, and required relationships. The operator must review and name it (`displayName: REVIEW REQUIRED`), assign a role, and decide whether the knowledge gaps are acceptable. The definition correctly requires both the References relationship (Deployment → ConfigMap) and the SelectsWorkload relationship (Service → Deployment).

## 3.6 Test the Draft Definition

```console
$ kos candidates test candidate-26b64f33bd03
Definition Test: candidate-26b64f33bd03
Compiled:        draft-candidate-26b64f33bd03
Mode:            Structural
Target Validation:
  Source instances:   2
  Matched by def:    2/2
Classification Impact:
  Additional matches:  3
  Rejected roots:      17
  Eligible roots accepted:  5/22
  Accepted (additional):
    ✓ Deployment/argocd/argocd-repo-server
    ✓ Deployment/argocd/argocd-dex-server
    ✓ Deployment/fixture-c/fixture-simple-c
  Rejected:
    ✗ Deployment/observability/kube-state-metrics — configMap has 0 instances, min required 1
    ✗ Deployment/argocd/argocd-notifications-controller — service has 0 instances, min required 1
    ✗ Deployment/observability/grafana — required relationship root -[References]-> configMap not found
    ... 14 additional rejections ...
Knowledge Quality:
  Recurrence:              Probable (2 instances)
  Structural cohesion:     Exact
  Observed-edge coverage:  Partial
```

### Observation

The dry-run shows the definition would match 5 total instances: the 2 source instances plus 3 additional (argocd-repo-server, argocd-dex-server, fixture-simple-c). It correctly rejects 17 Deployments that lack either a ConfigMap reference or a selecting Service. The operator can evaluate whether the additional matches are acceptable or whether the definition needs tightening (e.g., adding a Mounts relationship requirement to distinguish from simple References).

## Structure Axis Summary

The Structure traversal moved through:

```text
Accepted roles and shapes (2 classifiers, 1 named shape)
  → shape definition detail
  → unnamed candidates (16 groups, 17 instances)
  → candidate evidence (fingerprints, relationships, traits)
  → generated definition (valid YAML with review markers)
  → dry-run validation (5 matches, 17 rejections)
```

---

# 4. Graph Axis

The Graph axis exposes how resources are connected.

## 4.1 Summarize the Graph

From the report above:
- 535 nodes (resources + classifiers)
- 734 edges (structural, provenance, grouping)
- 297 structural relationship edges across 357 unique nodes

## 4.2 Inspect Relationships for One Workload

```console
$ kos relationships Deployment argocd-server -n argocd
Relationships for: Deployment/argocd/argocd-server
  Outgoing (6):
    → ServiceAccount/argocd/argocd-server  [UsesServiceAccount]  serviceAccountName=argocd-server (ExplicitField)
    → ConfigMap/argocd/argocd-ssh-known-hosts-cm  [Mounts]  configMap.name=argocd-ssh-known-hosts-cm (ExplicitField)
    → ConfigMap/argocd/argocd-tls-certs-cm  [Mounts]  configMap.name=argocd-tls-certs-cm (ExplicitField)
    → ConfigMap/argocd/argocd-cmd-params-cm  [Mounts]  configMap.name=argocd-cmd-params-cm (ExplicitField)
    → Secret/argocd/argocd-redis  [References]  secretKeyRef.name=argocd-redis (ExplicitField)
    → ReplicaSet/argocd/argocd-server-6c967757bb  [Owns]  ownerReferences (OwnerReference)
  Incoming (1):
    ← Service/argocd/argocd-server  [SelectsWorkload]  spec.selector (SelectorMatch)
```

### Observation

The argocd-server Deployment has 6 outgoing relationships (UsesServiceAccount, 3× Mounts, 1× References, 1× Owns) and 1 incoming (Service selects it). All structural relationships are sourced from explicit Kubernetes spec fields — no inferred or naming-convention edges. The ReplicaSet is a framework descendant (Owns via ownerReference). The 3 mounted ConfigMaps and 1 referenced Secret represent the workload's configuration dependencies.

## 4.3 Traverse From a Shared Configuration Resource

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

### Observation

This ConfigMap has 3 consumers. It cannot be evaluated by age or name alone — the graph proves it is actively referenced by 3 workloads. Any Janitor action would fail the "no consumers outside action closure" qualification check. This is why graph knowledge is essential for safe mutation.

## 4.4 Follow an Access Path

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

### Observation

Starting from the Service entry point, the graph traverses:

```text
Service/argocd/argocd-server
  → Deployment/argocd/argocd-server  [SelectsWorkload]
    → ServiceAccount/argocd/argocd-server  [UsesServiceAccount]
    → ConfigMap/argocd/argocd-ssh-known-hosts-cm  [Mounts]
    → ConfigMap/argocd/argocd-tls-certs-cm  [Mounts]
    → ConfigMap/argocd/argocd-cmd-params-cm  [Mounts]
    → Secret/argocd/argocd-redis  [References]
    → ReplicaSet/argocd/argocd-server-6c967757bb  [Owns]
```

This is the blast radius of the argocd-server Service — all resources transitively reachable from it.

## 4.5 Demonstrate Teardown Ordering

From the argocd-server relationships:

```text
Expected teardown principle (consumers before providers):
  Service/argocd/argocd-server         (consumer of Deployment via SelectsWorkload)
  Deployment/argocd/argocd-server      (consumer of ConfigMaps via Mounts/References)
  ReplicaSet/argocd/argocd-server-*    (framework descendant, cascading via Owns)
  ConfigMap/argocd/argocd-cmd-params-cm  (provider)
  ServiceAccount/argocd/argocd-server    (provider)
```

The graph-derived ordering ensures consumers are removed before the providers they depend on. Framework descendants (ReplicaSet) cascade automatically via Kubernetes garbage collection.

## 4.6 Identify Graph Uncertainty

```console
$ kos candidates explain candidate-26b64f33bd03
...
Evidence:
  Coverage: Partial
...
```

Partial coverage means this candidate uses only 2 of the available structural relationship types (References, SelectsWorkload). Other relationship types (Mounts, UsesServiceAccount, ClaimsStorage, BindsSubject) are not present in these instances. For destructive action, this incomplete coverage would mean the Janitor cannot definitively rule out unobserved dependencies — it would require operator review before promotion to a named shape.

## Graph Axis Summary

The Graph traversal moved through:

```text
Graph summary (297 edges, 357 nodes)
  → workload relationships (7 edges for argocd-server)
  → shared resource consumers (argocd-ssh-known-hosts-cm: 3 consumers)
  → access path traversal (Service → 7 reachable resources)
  → teardown ordering (consumers before providers)
  → graph uncertainty (partial coverage blocks autonomous action)
```

---

# Conclusion

The four axes are different projections of the same observed cluster knowledge.

```text
Organization
  What exists, what belongs together, and who controls it?

Deployment
  How was it installed, from what source, and under which lifecycle manager?

Structure
  What recognizable architectural patterns comprise it?

Graph
  How is it connected, exposed, and affected by change?
```

A typical investigation moves naturally between them:

```text
Application group (argocd: 64 resources)
  → lifecycle authority (Helm/argocd)
  → release (chart:argo-cd@10.4.0, revision 1)
  → structural composition (7 workloads across 8 components)
  → individual resource (ConfigMap argocd-cm: 2 consumers)
  → dependency graph (Service → Deployment → ConfigMaps → Secret)
  → findings and actionability (14 unmanaged, 45 total findings)
```

KOS does not replace `kubectl`, Helm, or Argo CD. It supplies the navigable knowledge layer that connects the information those tools expose individually.
