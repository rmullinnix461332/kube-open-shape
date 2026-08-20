# Logical Resource Grouping — Design Specification

## Purpose

KOS must recognize that multiple independently rooted Kubernetes workloads can collectively represent one logical deployed system.

Examples include:
- Multi-component applications
- Operators with controller and webhook workloads
- Monitoring stacks
- Ingress controllers
- Databases with proxies or exporters
- GitOps systems
- Helm releases containing several workload roots

Grouping must preserve both levels:

```
Logical system
├── Component workload A
├── Component workload B
└── Component workload C
```

A logical system is not itself a Kubernetes resource. It is a derived knowledge-graph entity supported by observed evidence.

## Core Principles

- Workload roots remain independently addressable.
- Grouping does not merge or discard component graphs.
- A resource can participate in multiple grouping dimensions.
- Group membership does not imply operational dependency.
- Labels provide declared association, not proof of consumption or control.
- Group identity must be scoped to prevent cross-namespace or cross-cluster collisions.
- Grouping rules must be extensible through configuration rather than product-specific Go code.

## Graph Entity

Introduce a synthetic node:

```json
{
  "id": "group:edge-local/namespace/application/example",
  "type": "LogicalResourceGroup",
  "groupType": "Application",
  "name": "example",
  "scope": {
    "clusterId": "edge-local",
    "namespace": "example"
  },
  "identity": {
    "strategy": "KubernetesRecommendedLabels",
    "key": "example"
  }
}
```

Suggested group types:
- Application
- Component
- Release
- System
- OwnershipBoundary
- Custom

Do not require every group to be an application. LogicalResourceGroup is the generic graph type; groupType provides its meaning.

## Membership Edge

```json
{
  "id": "edge:sha256:...",
  "type": "MemberOf",
  "from": "resource:edge-local/apps/Deployment/example/api",
  "to": "group:edge-local/example/application/example",
  "compositionRole": "Contextual",
  "evidence": {
    "type": "LabelAssociation",
    "fieldPath": "metadata.labels[app.kubernetes.io/part-of]",
    "observedValue": "example"
  },
  "model": "builtin:logical-grouping-v1"
}
```

MemberOf must not imply:
- Ownership
- Lifecycle control
- Network connectivity
- Runtime dependency
- Configuration consumption
- Common shape

It means only that available evidence declares the resources to be part of the same logical unit.

## Built-in Grouping Signals

### Kubernetes Recommended Labels

Interpret each label according to its defined purpose:

| Label | Meaning |
|-------|---------|
| app.kubernetes.io/part-of | Containing application or system |
| app.kubernetes.io/instance | Distinct installed instance |
| app.kubernetes.io/component | Component role within the containing system |
| app.kubernetes.io/name | Application or component name |
| app.kubernetes.io/managed-by | Management mechanism |
| app.kubernetes.io/version | Observed application version |

These labels must remain separate traits. Do not collapse them into one generic application label.

### Package-Manager Provenance

Helm release metadata can create a release group:

```
groupType: Release
identity:
  type: Helm
  namespace: example
  releaseName: example
```

Membership:
```
Resource ── MemberOfRelease ──→ ReleaseGroup
```

A release group is a lifecycle/provenance boundary. It is not automatically equivalent to an application or system group.

### Kubernetes Ownership

Owner references create operational ownership edges:
```
Resource ── OwnedBy ──→ KubernetesResource
```

Owner-reference graphs should not automatically become logical groups. They are controller-generated resource trees.

### Custom Metadata

Administrators must be able to define additional grouping sources through configuration:

```yaml
apiVersion: knowledge.kos.io/v1alpha1
kind: RelationshipDefinition
metadata:
  name: platform-system-group
spec:
  relationshipType: MemberOf
  source:
    resources:
      apiGroups: ["*"]
      kinds: ["*"]
  target:
    synthetic:
      type: LogicalResourceGroup
      groupType: System
  extraction:
    type: LabelValue
    fieldPath: metadata.labels["platform.example.io/system"]
  scope:
    - Cluster
    - Namespace
    - ExtractedValue
```

This keeps organizational conventions outside the core Go implementation.

## Group Identity

Group identity must be derived deterministically.

Default application group key:
```
clusterId + namespace + app.kubernetes.io/part-of + optional app.kubernetes.io/instance
```

Default release group key:
```
clusterId + namespace + packageManager + releaseName
```

Default custom group key:
```
clusterId + configured scope fields + extracted value
```

Canonical ID:
```
group:<clusterId>/<scope>/<groupType>/<normalized-key>
```

Store the original, non-normalized values separately.

## Evidence Precedence

Grouping signals may agree or conflict. Use evidence precedence:

```
Explicit custom relationship
> Package-manager release identity
> app.kubernetes.io/instance + part-of
> app.kubernetes.io/part-of
> configured label association
> naming convention
```

This precedence determines confidence, not destructive replacement. Preserve all observed group memberships when they represent different dimensions.

Example:
```
Resource
├── MemberOf → ApplicationGroup
├── MemberOf → ComponentGroup
└── MemberOfRelease → HelmReleaseGroup
```

Do not force these into a single parent relationship.

## Confidence

Each derived group and membership edge should carry confidence:

| Level | Meaning |
|-------|---------|
| Authoritative | Explicit custom grouping definition or authoritative package metadata |
| Corroborating | Multiple independent signals agree |
| Declared | One recognized declarative label |
| Inferred | Derived through weaker metadata association |
| Heuristic | Naming convention only |

## Group Construction Algorithm

1. Observe Kubernetes resources.
2. Extract configured grouping signals.
3. Normalize each grouping key.
4. Apply configured scope.
5. Create or resolve LogicalResourceGroup nodes.
6. Add evidence-bearing membership edges.
7. Combine matching evidence for identical memberships.
8. Detect conflicting identity evidence.
9. Retain workload/component graphs independently.
10. Persist groups and membership edges in local knowledge storage.

Grouping must be idempotent. Reprocessing the same snapshot must produce identical group and edge IDs.

## Conflict Handling

Conflicting signals must not be silently resolved.

Example:
```
part-of: payment-system
instance: inventory-prod
Helm release: customer-api
```

Record:
```json
{
  "state": "Conflicted",
  "conflicts": [{
    "signals": ["app.kubernetes.io/part-of", "app.kubernetes.io/instance", "HelmRelease"]
  }]
}
```

A conflict may produce multiple contextual memberships, but it must not create a false consolidated identity.

## Shape Interaction

Logical grouping and structural shape classification are independent.

Component-level shape:
```
Deployment + Service + ConfigMap
```

Aggregate shape:
```
LogicalResourceGroup
├── controller component
├── webhook component
└── stateful component
```

A ShapeDefinition may target either:
```yaml
spec:
  scope: ResourceRoot
```
or:
```yaml
spec:
  scope: LogicalGroup
```

A logical-group shape can define component cardinality:
```yaml
spec:
  scope: LogicalGroup
  groupTypes: ["Application", "System"]
  components:
    - alias: controllers
      shapeRole: controller
      cardinality:
        min: 1
    - alias: services
      shapeRole: application
      cardinality:
        min: 1
```

Initial implementation does not need aggregate ShapeDefinition matching. It only needs to create stable logical groups without losing component structure.

## Candidate Discovery Interaction

Candidate discovery operates at two levels:

- **Component candidates** — Repeated workload-root composition
- **Aggregate candidates** — Repeated logical-group composition

These should be separate commands or modes:
```bash
kos candidates --scope components
kos candidates --scope groups
```

Do not combine component and aggregate candidate fingerprints.

## CLI Behavior

```bash
kos groups
```

```
GROUP             TYPE         COMPONENTS  EVIDENCE
example           Application  4           part-of, instance, Helm
monitoring        System       7           part-of, Helm
custom-operator   Application  3           part-of
```

```bash
kos groups detail example
```

```
Group:      example
Type:       Application
Scope:      example namespace
Confidence: Corroborating

Evidence:
  app.kubernetes.io/part-of=example
  app.kubernetes.io/instance=example
  Helm release=example

Components:
  Deployment/example/api
  Deployment/example/controller
  StatefulSet/example/data
  Deployment/example/exporter
```

The existing `kos shapes` output should remain focused on accepted structural knowledge and should not become a group-reporting command.

## Graph Export

Logical groups become graph nodes:

```
KubernetesResource ── MemberOf ───────→ LogicalResourceGroup
KubernetesResource ── MemberOfRelease → ReleaseGroup
LogicalResourceGroup ── ClassifiedAs ─→ RoleClassifier
LogicalResourceGroup ── ConformsTo ───→ NamedShape
```

Graph summary should distinguish:
```json
{
  "summary": {
    "resourceNodes": 287,
    "logicalGroupNodes": 8,
    "releaseGroupNodes": 6,
    "classifierNodes": 3,
    "shapeNodes": 0,
    "edgeCount": 210
  }
}
```

## Initial Implementation Boundary

Implement first:
- Generic LogicalResourceGroup node
- MemberOf and MemberOfRelease edges
- Recommended Kubernetes label extraction
- Namespace-scoped deterministic group identity
- Evidence and confidence
- Graph export
- `kos groups` CLI
- Configurable label-based grouping through RelationshipDefinition

Defer:
- Aggregate shape matching
- Aggregate candidate generation
- Cross-namespace grouping
- Cross-cluster grouping
- Automated conflict remediation
- Behavioral or runtime dependency inference

This adds the missing system-level view without hardcoding any application, chart, operator, or deployment technology.
