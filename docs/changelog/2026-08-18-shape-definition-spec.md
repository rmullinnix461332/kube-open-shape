# Shape Definition Specification — v2

## Overview

Shape definitions declare the **graph composition** that constitutes a recognizable structural pattern. The Go engine is a generic, deterministic graph matcher. All domain intelligence — root selection, component composition, relationship requirements, cardinality, constraints, traits, and canonicalization — lives in declarative CRDs.

Two CRD types work together:

| CRD | Purpose |
|-----|---------|
| `RelationshipDefinition` | Declares how the generic graph is constructed from Kubernetes resource fields |
| `ShapeDefinition` | Declares a named graph pattern to match against the generic graph |

The Go engine:
1. Observes resources
2. Applies `RelationshipDefinition` rules to construct a generic relationship graph
3. Uses each `ShapeDefinition` to select candidate roots, match components, evaluate constraints, emit traits, and assign a role
4. Canonicalizes matched composition and calculates a fingerprint
5. Records definition revision, match explanation, and conflicts

---

## 1. Processing Model

```
Go observes resources
    ↓
RelationshipDefinitions construct generic graph
    ↓
ShapeDefinition selects candidate roots
    ↓
ShapeDefinition declares graph composition (components + relationships)
    ↓
Engine matches components from graph using aliases
    ↓
Engine applies cardinality constraints
    ↓
Engine evaluates CEL constraints against matched components
    ↓
ShapeDefinition emits traits via CEL expressions
    ↓
Go canonicalizes matched composition (per profile)
    ↓
Go calculates shape fingerprint
    ↓
Engine records definition revision, match explanation
    ↓
Engine reports conflicts or leaves unclassified
```

---

## 2. RelationshipDefinition CRD

Declares how edges are constructed in the generic resource graph. Replaces hardcoded relationship extraction.

```yaml
apiVersion: knowledge.kos.io/v1alpha1
kind: RelationshipDefinition
metadata:
  name: deployment-uses-service-account
spec:
  schemaVersion: 1
  definitionVersion: 1

  type: UsesServiceAccount

  source:
    apiGroups: ["apps"]
    kinds: ["Deployment", "StatefulSet", "DaemonSet"]

  target:
    apiGroups: [""]
    kinds: ["ServiceAccount"]

  references:
    - sourcePath: spec.template.spec.serviceAccountName
      targetPath: metadata.name
      targetNamespace: Source

  defaults:
    targetName: default
```

### 2.1 Additional Examples

```yaml
---
apiVersion: knowledge.kos.io/v1alpha1
kind: RelationshipDefinition
metadata:
  name: workload-references-configmap
spec:
  schemaVersion: 1
  definitionVersion: 1
  type: References
  source:
    apiGroups: ["apps"]
    kinds: ["Deployment", "StatefulSet", "DaemonSet"]
  target:
    apiGroups: [""]
    kinds: ["ConfigMap"]
  references:
    - sourcePath: spec.template.spec.volumes[*].configMap.name
      targetPath: metadata.name
      targetNamespace: Source
    - sourcePath: spec.template.spec.containers[*].envFrom[*].configMapRef.name
      targetPath: metadata.name
      targetNamespace: Source
---
apiVersion: knowledge.kos.io/v1alpha1
kind: RelationshipDefinition
metadata:
  name: rolebinding-binds-serviceaccount
spec:
  schemaVersion: 1
  definitionVersion: 1
  type: BindsServiceAccount
  source:
    apiGroups: ["rbac.authorization.k8s.io"]
    kinds: ["RoleBinding", "ClusterRoleBinding"]
  target:
    apiGroups: [""]
    kinds: ["ServiceAccount"]
  references:
    - sourcePath: subjects[*].name
      targetPath: metadata.name
      targetNamespace: subjects[*].namespace
      filter:
        sourcePath: subjects[*].kind
        equals: ServiceAccount
---
apiVersion: knowledge.kos.io/v1alpha1
kind: RelationshipDefinition
metadata:
  name: rolebinding-grants-role
spec:
  schemaVersion: 1
  definitionVersion: 1
  type: GrantsRole
  source:
    apiGroups: ["rbac.authorization.k8s.io"]
    kinds: ["RoleBinding", "ClusterRoleBinding"]
  target:
    apiGroups: ["rbac.authorization.k8s.io"]
    kinds: ["Role", "ClusterRole"]
  references:
    - sourcePath: roleRef.name
      targetPath: metadata.name
      targetNamespace: Source
---
apiVersion: knowledge.kos.io/v1alpha1
kind: RelationshipDefinition
metadata:
  name: deployment-uses-lease
spec:
  schemaVersion: 1
  definitionVersion: 1
  type: UsesLease
  source:
    apiGroups: ["apps"]
    kinds: ["Deployment", "StatefulSet"]
  target:
    apiGroups: ["coordination.k8s.io"]
    kinds: ["Lease"]
  references:
    - sourcePath: metadata.name
      targetPath: metadata.name
      targetNamespace: Source
```

### 2.2 RelationshipDefinition Go Types

```go
type RelationshipDefinitionSpec struct {
    SchemaVersion     int                `json:"schemaVersion"`
    DefinitionVersion int                `json:"definitionVersion"`
    Type              string             `json:"type"`
    Source            ResourceSelector   `json:"source"`
    Target            ResourceSelector   `json:"target"`
    References        []ReferenceMapping `json:"references"`
    Defaults          *ReferenceDefaults `json:"defaults,omitempty"`
}

type ResourceSelector struct {
    APIGroups []string `json:"apiGroups"`
    Kinds     []string `json:"kinds"`
    Versions  []string `json:"versions,omitempty"` // optional, for specificity
}

type ReferenceMapping struct {
    SourcePath      string           `json:"sourcePath"`      // JSONPath-like field reference
    TargetPath      string           `json:"targetPath"`
    TargetNamespace string           `json:"targetNamespace"` // "Source" or explicit ns or path
    Filter          *ReferenceFilter `json:"filter,omitempty"`
}

type ReferenceFilter struct {
    SourcePath string `json:"sourcePath"`
    Equals     string `json:"equals"`
}

type ReferenceDefaults struct {
    TargetName string `json:"targetName,omitempty"`
}
```

### 2.3 CRD Status

```go
type RelationshipDefinitionStatus struct {
    ObservedGeneration int64              `json:"observedGeneration"`
    Phase              string             `json:"phase"` // Ready, Invalid
    CompiledDigest     string             `json:"compiledDigest"`
    EdgesCreated       int                `json:"edgesCreated"`
    LastCompiledAt     *metav1.Time       `json:"lastCompiledAt"`
    Conditions         []metav1.Condition `json:"conditions"`
}
```

---

## 3. ShapeDefinition CRD

Declares a named graph pattern — a composition of aliased components connected by typed relationships.

```yaml
apiVersion: knowledge.kos.io/v1alpha1
kind: ShapeDefinition
metadata:
  name: leader-elected-controller
spec:
  schemaVersion: 1
  definitionVersion: 3
  displayName: Leader-Elected Controller
  role: Controller
  priority: 100

  roots:
    - alias: controller
      resource:
        apiGroups: ["apps"]
        kinds: ["Deployment", "StatefulSet"]
      selector:
        matchLabels:
          app.kubernetes.io/component: controller

  components:
    - alias: serviceAccount
      resource:
        apiGroups: [""]
        kinds: ["ServiceAccount"]
      cardinality: {min: 1, max: 1}

    - alias: role
      resource:
        apiGroups: ["rbac.authorization.k8s.io"]
        kinds: ["Role", "ClusterRole"]
      cardinality: {min: 1}

    - alias: binding
      resource:
        apiGroups: ["rbac.authorization.k8s.io"]
        kinds: ["RoleBinding", "ClusterRoleBinding"]
      cardinality: {min: 1}

    - alias: lease
      resource:
        apiGroups: ["coordination.k8s.io"]
        kinds: ["Lease"]
      cardinality: {min: 0, max: 1}

  relationships:
    - from: controller
      type: UsesServiceAccount
      to: serviceAccount
      required: true

    - from: binding
      type: BindsServiceAccount
      to: serviceAccount
      required: true

    - from: binding
      type: GrantsRole
      to: role
      required: true

    - from: controller
      type: UsesLease
      to: lease
      required: false

  constraints:
    - name: dedicated-service-account
      expression: >
        serviceAccount[0].metadata.name != "default"

  traits:
    - name: clusterScopedRBAC
      type: boolean
      fingerprint: true
      expression: >
        role.exists(r, r.kind == "ClusterRole")

    - name: leaderElection
      type: boolean
      fingerprint: true
      expression: >
        lease.size() > 0

  composition:
    unmatchedResources: IncludeAsVariant

  canonicalization:
    profileRefs:
      - kubernetes-structural-v1
    exclude:
      - metadata.name
      - metadata.namespace
      - metadata.uid
      - metadata.resourceVersion
      - metadata.creationTimestamp
      - spec.replicas
      - containers[*].image
```

### 3.1 ShapeDefinition Go Types

```go
type ShapeDefinitionSpec struct {
    SchemaVersion     int                    `json:"schemaVersion"`
    DefinitionVersion int                    `json:"definitionVersion"`
    DisplayName       string                 `json:"displayName"`
    Role              string                 `json:"role"`
    Priority          int                    `json:"priority"`
    Roots             []RootSpec             `json:"roots"`
    Components        []ComponentSpec        `json:"components"`
    Relationships     []RelationshipSpec     `json:"relationships"`
    Constraints       []ConstraintSpec       `json:"constraints,omitempty"`
    Traits            []TraitSpec            `json:"traits,omitempty"`
    Composition       CompositionSpec        `json:"composition"`
    Canonicalization  CanonicalizationSpec   `json:"canonicalization"`
}

type RootSpec struct {
    Alias    string            `json:"alias"`
    Resource ResourceSelector  `json:"resource"`
    Selector *LabelSelector    `json:"selector,omitempty"`
}

type ComponentSpec struct {
    Alias       string           `json:"alias"`
    Resource    ResourceSelector `json:"resource"`
    Cardinality CardinalitySpec  `json:"cardinality"`
}

type CardinalitySpec struct {
    Min int `json:"min"`
    Max int `json:"max,omitempty"` // 0 = unbounded
}

type RelationshipSpec struct {
    From     string `json:"from"`     // alias
    Type     string `json:"type"`     // must match a RelationshipDefinition.spec.type
    To       string `json:"to"`       // alias
    Required bool   `json:"required"`
}

type ConstraintSpec struct {
    Name       string `json:"name"`
    Expression string `json:"expression"` // CEL
}

type TraitSpec struct {
    Name        string `json:"name"`
    Type        string `json:"type"`        // boolean, integer, string
    Fingerprint bool   `json:"fingerprint"` // include in fingerprint calculation
    Expression  string `json:"expression"`  // CEL
}

type CompositionSpec struct {
    UnmatchedResources string `json:"unmatchedResources"` // Ignore, IncludeAsVariant, Reject
}

type CanonicalizationSpec struct {
    ProfileRefs []string `json:"profileRefs,omitempty"`
    Include     []string `json:"include,omitempty"`
    Exclude     []string `json:"exclude,omitempty"`
}
```

### 3.2 CRD Status

```go
type ShapeDefinitionStatus struct {
    ObservedGeneration int64              `json:"observedGeneration"`
    Phase              string             `json:"phase"` // Ready, Invalid, Conflicted
    CompiledDigest     string             `json:"compiledDigest"`
    MatchedInstances   int                `json:"matchedInstances"`
    LastCompiledAt     *metav1.Time       `json:"lastCompiledAt"`
    Complexity         ComplexityInfo     `json:"complexity"`
    Conditions         []metav1.Condition `json:"conditions"`
}

type ComplexityInfo struct {
    ComponentAliases         int `json:"componentAliases"`
    RelationshipConstraints  int `json:"relationshipConstraints"`
    MaximumTraversalDepth    int `json:"maximumTraversalDepth"`
}
```

---

## 4. CEL Activation Context

CEL expressions receive a restricted context containing only normalized resource and graph data:

```
// Available per alias
serviceAccount     []Resource    // matched resources for this alias
role               []Resource    // matched resources for this alias
binding            []Resource    // matched resources for this alias
controller         []Resource    // root resources
lease              []Resource    // matched resources for this alias

// Each Resource contains:
//   .kind          string
//   .apiGroup      string
//   .metadata.name string
//   .metadata.namespace string
//   .metadata.labels map[string]string
//   .metadata.annotations map[string]string
//   .spec          map (selected declared fields only)
```

CEL must NOT receive:
- Network access
- Filesystem access
- Current time
- Kubernetes mutation functions
- Arbitrary client calls
- Unbounded recursive traversal

---

## 5. Match Algorithm

For each `ShapeDefinition`:

1. **Compile** — validate, compile CEL expressions, cache by generation
2. **Select roots** — find resources matching `roots[].resource` and optional `selector`
3. **For each candidate root**:
   a. Follow graph edges to find resources matching each `components[].resource`
   b. Verify `relationships[]` — required edges must exist between aliased components
   c. Verify `cardinality` — each component must have count within [min, max]
   d. Evaluate `constraints[]` — all CEL expressions must return true
   e. Emit `traits[]` — evaluate CEL expressions for each trait
4. **Composition** — handle unmatched resources per `unmatchedResources` policy
5. **Canonicalize** — apply profile, remove excluded fields, sort deterministically
6. **Fingerprint** — SHA-256 of canonical representation including fingerprint-tagged traits
7. **Record** — definition name, version, digest, match explanation

---

## 6. Priority and Conflict Resolution

- Each definition has an explicit `priority` (integer, higher wins)
- All definitions are evaluated for every candidate root
- The highest-priority match wins
- If multiple definitions match at the **same** highest priority → `Conflicted`
- Conflicted state is reported, not silently resolved
- Alphabetical name ordering is **never** used as a tiebreaker

Conflict reporting:
```yaml
classification:
  state: Conflicted
  candidates:
    - definition: controller-with-webhook
      priority: 100
    - definition: generic-operator
      priority: 100
```

---

## 7. Versioning

`metadata.resourceVersion` is an etcd concurrency token, not a semantic version. Every definition carries:

```yaml
spec:
  schemaVersion: 1      # CRD schema version
  definitionVersion: 3  # semantic version of this definition's content
```

Every shape match records:
```yaml
definition:
  name: leader-elected-controller
  version: 3
  digest: sha256:...
canonicalization:
  profile: kubernetes-structural-v1
  revision: 2
```

Compiled definitions are cached by `observedGeneration`. A new generation triggers recompilation and re-evaluation of affected roots.

---

## 8. Open vs Closed Composition

`composition.unmatchedResources` determines what happens to resources connected to the root that are not declared as components:

| Value | Behavior |
|-------|----------|
| `Ignore` | Extra resources are excluded from the shape entirely |
| `IncludeAsVariant` | Extra resources are included in canonicalization — shapes with extra resources get different fingerprints |
| `Reject` | If extra resources are found, the definition does not match |

Default: `IncludeAsVariant` — the definition classifies the role, while the exact fingerprint captures structural differences.

---

## 9. Engine Limits

```yaml
engineLimits:
  maximumDefinitions: 500
  maximumComponentsPerDefinition: 32
  maximumRelationshipsPerDefinition: 64
  maximumTraversalDepth: 3
  maximumCandidateNodesPerRoot: 1000
  maximumCandidateMatchesPerRoot: 100
  maximumCELCost: 10000
  maximumRegexLength: 512
```

Uses Go's RE2 regex engine (linear-time, no backtracking).

---

## 10. Installation

Default definitions are NOT self-installed by the controller on startup. They are installed via:

- Helm chart (bundled as CRD manifests)
- Kustomize bundle
- Community knowledge packs
- Explicit CLI: `kos pack install kubernetes-core`

Installed CRDs are visible, GitOps-manageable, and auditable.

---

## 11. Extensibility Model

| CRD | What it externalizes |
|-----|---------------------|
| `ResourceOwner` | Management ownership detection |
| `RelationshipDefinition` | Graph edge construction from resource fields |
| `ShapeDefinition` | Recurring graph composition, root selection, constraints, traits |
| `JanitorRule` | Conditions and lifecycle policy |

The Go binary becomes a generic engine that:
- Watches resources
- Constructs graphs per `RelationshipDefinition`
- Matches patterns per `ShapeDefinition`
- Evaluates rules per `JanitorRule`
- Reports findings

All domain intelligence lives in CRDs.

---

## 12. Impact on Phase 4 Implementation

Phase 4 deliverables are updated to:

- `api/v1alpha1/relationshipdefinition_types.go` — CRD type
- `api/v1alpha1/shapedefinition_types.go` — CRD type
- `internal/edge/graph/relationships.go` — apply RelationshipDefinitions to construct graph edges (replaces hardcoded builder)
- `internal/edge/shape/compiler.go` — compile ShapeDefinitions (validate, compile CEL, cache by generation)
- `internal/edge/shape/matcher.go` — match compiled definitions against graph (root selection → component matching → relationship verification → cardinality → constraints → traits)
- `internal/edge/shape/cel.go` — restricted CEL environment setup and evaluation
- `internal/edge/shape/canonical.go` — canonicalization per profile and definition
- `internal/edge/shape/fingerprint.go` — SHA-256 of canonical + fingerprint-tagged traits
- `config/crd/` — generated CRD manifests for both types
- `config/samples/relationships/` — default relationship definitions (SA, ConfigMap, Secret, RoleBinding, Lease)
- `config/samples/shapes/` — default shape definitions (controller, operator, node-system, scheduled, application)
- CLI: `kos shapes definitions`, `kos shapes evaluate --explain`

---

## Candidate Affinity

### Summary

Candidate affinity is an intermediate semantic classification applied to an observed candidate pattern before a formal ShapeDefinition exists.

It records:

> "This candidate feels like a controller/operator/node agent," without claiming that it already matches an accepted shape.

### Position in Lifecycle

```text
Observed resources
    ↓
Candidate grouping
    ↓
Candidate affinity
    ↓
Operator review and refinement
    ↓
ShapeDefinition generation
    ↓
Validation and promotion
```

### Data Model

An affinity assessment contains:

```yaml
candidate: candidate-e53cb4adb95a
role: controller
affinity: API Controller
proposedName: Configuration Controller
confidence: Tentative
rationale: Long-running workload with RBAC and configuration references
source: Operator
observedAt: <timestamp>
```

Fields:

| Field | Purpose |
|-------|---------|
| candidate | Stable candidate identifier |
| role | Broad structural category |
| affinity | The archetype or existing shape it resembles |
| proposedName | Optional working name |
| confidence | Tentative, Likely, or another bounded scale |
| rationale | Human-readable reasoning |
| source | Operator, automated suggestion, or imported knowledge |
| observedAt | Assessment timestamp |

### Required Semantics

- Affinity is interpretation, not observed structural evidence.
- It must not alter the candidate's semantic or mechanical fingerprint.
- It must not cause the candidate to appear as a named shape.
- It must not qualify as conformance with an existing shape.
- Human assessments and automated suggestions remain distinguishable.
- Multiple affinities may coexist when classification is uncertain.
- Assessments can be revised while preserving prior history.
- Mechanically different candidates may share the same affinity.
- Similar affinities help identify candidates that may be merged into one future definition.

### Example

```text
Candidate: candidate-123

Working Affinities:
  Operator       Likely       Operator assessment
  Controller     Possible     Automated suggestion
```

### Relationship to Existing Concepts

| Concept | Meaning |
|---------|---------|
| Candidate fingerprint | What was structurally observed |
| Candidate affinity | What the structure may mean |
| Named shape | Accepted structural vocabulary |
| Shape match | Validated conformance to that vocabulary |

### Safety Boundary

Candidate affinity is advisory knowledge only.

It may support:
- Organizing candidates.
- Prioritizing shape-definition work.
- Comparing similar patterns.
- Suggesting names and roles.
- Generating draft definitions.

It must NOT independently authorize Janitor actions, particularly neutralize or delete. Only accepted shape definitions and validated matches may participate in destructive qualification.

### Success Criteria

Candidate affinity is successful when operators can:

- Organize recurring unnamed patterns by perceived purpose.
- Record "this feels like A" without prematurely defining A.
- Find multiple candidates that may represent variants of one concept.
- Preserve uncertainty and conflicting assessments.
- Move deliberately from observed structure to an accepted, testable definition.
