# Structure Traversal Axis — Test Strategy

## 1. Purpose

The Structure traversal axis applies an operator-defined classification system to the resources and resource families in a Kubernetes cluster.

It must answer:

* What structural types comprise this cluster?
* What terminology has been defined?
* What makes a resource family qualify as a particular shape?
* Which installed resource families match each shape?
* How does each resource fill a role within the matched shape?
* What structural variants or drift exist among instances?
* Which recurring structures remain unnamed?
* Can the remaining structures be classified provisionally before a formal shape is defined?
* Does the resulting taxonomy make the cluster's purpose easier to understand and communicate?

The Structure axis turns Kubernetes inventory into an architectural description of the cluster.

## 2. Scope

This strategy covers:

* Role classifiers.
* Named shape definitions.
* Shape instances.
* Resource and alias bindings.
* Shape matching and conformance explanations.
* Variants and structural drift.
* Candidate discovery and grouping.
* Candidate explanation.
* Candidate working classifications and structural affinities.
* Candidate-to-shape promotion.
* Reverse traversal from a resource to its structural classification.
* Cross-axis traversal into Organization, Graph, Deployment, and Janitor knowledge.
* Persistence, determinism, performance, and CLI usability.

## 3. Non-Goals

The Structure axis does not determine:

* Resource health.
* Pod readiness.
* Job success.
* Application performance.
* Team ownership.
* Release revision or deployment status.
* Network reachability or blast radius.
* Whether a candidate is safe for automatic cleanup.

These concerns may be exposed through another traversal axis, but they must not affect structural classification unless explicitly included in a shape definition.

## 4. Structural Taxonomy

The tests must preserve the following conceptual hierarchy:

```text
Role
└── Named Shape
    └── Shape Instance
        └── Resource Bindings

Unclassified Inventory
└── Candidate Pattern
    └── Candidate Instances
        └── Working Classification / Structural Affinity
            └── Proposed Shape Definition
```

### 4.1 Role

A role is a broad classification of purpose, such as:

```text
application
controller
operator
node-system
observability
ingress
storage
security
```

A role does not define a complete resource composition.

### 4.2 Named Shape

A named shape is an accepted structural definition using terminology meaningful to the operator or organization.

Examples:

```text
GitOps Controller
Certificate Operator
Ingress Controller
Node Agent
Metrics Collector
Stateful Application
```

A named shape defines roots, components, relationships, cardinality, traits, and composition behavior.

### 4.3 Shape Instance

A shape instance is a concrete resource family that conforms to a named shape.

Examples:

```text
GitOps Controller
└── argocd

Certificate Operator
└── cert-manager
```

### 4.4 Resource Bindings

Bindings explain how concrete resources fill aliases in a shape definition.

```text
Shape: GitOps Controller
Instance: argocd

Bindings:
  controller:
    StatefulSet/argocd/argocd-application-controller

  api:
    Deployment/argocd/argocd-server
    Service/argocd/argocd-server

  repository:
    Deployment/argocd/argocd-repo-server

  configuration:
    ConfigMap/argocd/argocd-cm

  credentials:
    Secret/argocd/argocd-secret
```

### 4.5 Candidate Pattern

A candidate is a recurring observed structure for which no accepted named shape exists.

A candidate is evidence, not accepted taxonomy.

### 4.6 Working Classification

A working classification records an operator's current interpretation of a candidate.

Examples:

```text
Role: controller
Affinity: API Controller
Confidence: Tentative
Source: Operator
```

A working classification must not cause the candidate to appear as a matched named shape.

## 5. Core Test Objectives

### 5.1 Navigation

Verify that operators can navigate:

* Broad to narrow.
* Summary to explanation.
* Definition to instance.
* Instance to resource binding.
* Resource back to shape.
* Unclassified inventory to candidate.
* Candidate to working classification.
* Candidate to proposed definition.
* Structure into another traversal axis when appropriate.

### 5.2 Accuracy

Verify that:

* Shape matches are correct.
* Non-matches are correctly rejected.
* Required resources and relationships are enforced.
* Optional resources do not cause false rejection.
* Framework resources do not distort semantic identity.
* Resource bindings point to the correct concrete resources.
* Candidate fingerprints reflect actual structural evidence.
* Confidence and coverage accurately describe knowledge quality.

### 5.3 Explainability

Every structural conclusion must be explainable.

The system must be able to show:

* Which definition matched.
* Which definition version was used.
* Which roots matched.
* Which aliases were bound.
* Which required relationships were satisfied.
* Which optional components were present.
* Which requirements were missing.
* Which unmatched resources were included as variants.
* Which evidence caused a candidate grouping.
* Which evidence was absent or insufficient.
* Why another definition did not win.

### 5.4 Communication

Verify that the taxonomy can describe the cluster at an architectural level.

A structural summary should support statements such as:

```text
This cluster contains:
  1 GitOps controller
  2 certificate operators
  1 ingress controller
  3 observability collectors
  2 node agents
  6 application workloads
```

The output should communicate more than a count of Kubernetes resources.

## 6. Required Traversal Paths

### 6.1 Taxonomy to Resource

```text
Defined roles and shapes
    ↓
Named shape
    ↓
Shape definition
    ↓
Matched instances
    ↓
Selected instance
    ↓
Alias/resource bindings
    ↓
Concrete resource
```

### 6.2 Resource to Taxonomy

```text
Concrete resource
    ↓
Shape instance membership
    ↓
Alias filled by the resource
    ↓
Matched shape
    ↓
Definition requirement
    ↓
Role
```

### 6.3 Unclassified Inventory to Definition

```text
Unclassified resource families
    ↓
Candidate groups
    ↓
Candidate explanation
    ↓
Observed instances
    ↓
Working classification
    ↓
Generated draft
    ↓
Dry-run validation
    ↓
Accepted named shape
```

### 6.4 Structure to Other Axes

```text
Shape instance
    ├── Organization: application, group, component, resources
    ├── Deployment: release, source, revision, history
    ├── Graph: relationships, exposure, dependencies, blast radius
    └── Janitor: qualification against accepted structural facts
```

## 7. Structural Invariants

The following invariants must always hold.

### 7.1 Roles and Shapes

* A role classification is not automatically a named shape.
* A named shape must reference a valid accepted definition.
* A shape instance must identify the exact definition and version used.
* A resource may have a broad role without matching a named shape.
* A role classifier must not hide unnamed structural variation.

### 7.2 Candidates

* Candidates must not appear as named shapes.
* Candidate identifiers must be deterministic for identical normalized evidence.
* Candidate fingerprints must not change due to resource ordering.
* Candidate assessment must not change the underlying fingerprint.
* A working classification must remain distinct from a validated shape match.
* Candidate confidence must not exceed the available evidence.
* Root-kind similarity alone must not imply meaningful structural similarity.
* Candidates must never independently authorize destructive Janitor actions.

### 7.3 Bindings

* Every required alias must have a valid binding.
* Binding cardinality must comply with the definition.
* Every bound resource must exist in the knowledge index.
* A resource binding must identify the evidence that caused the binding.
* Framework descendants must not replace the workload root.
* Bindings must remain stable when irrelevant metadata changes.

### 7.4 Matching

* Higher-priority definitions may win only when they actually match.
* Equal-priority overlapping definitions must be handled deterministically.
* A partial match must not be presented as full conformance.
* Missing relationships must be visible.
* Unknown evidence must not be treated as satisfied evidence.
* Extra resources must follow the definition's unmatched-resource policy.
* Definition changes must cause controlled re-evaluation.

### 7.5 Explanation

* Every match must have a reproducible conformance trace.
* Explanations must use the evidence evaluated by the matcher.
* Explanations must not reconstruct a different result after evaluation.
* Failed requirements must be shown without exposing sensitive values.
* Confidence labels must correspond to documented evidence thresholds.

## 8. Iterative Test Method

Use one vertical structural path at a time:

```text
Select structural capability
    ↓
Run controlled fixtures
    ↓
Run real installations
    ↓
Compare expected and observed classification
    ↓
Inspect definition, bindings, and evidence
    ↓
Identify model or usability gaps
    ↓
Apply localized correction
    ↓
Repeat regression suite
```

Do not restructure the entire axis in response to a single fixture. A structural change is justified only when testing exposes a broken invariant or a concept the existing model cannot represent.

## 9. Test Layers

### 9.1 Unit Tests

Test isolated behavior for:

* Shape-definition parsing.
* Schema validation.
* Root selection.
* Component selection.
* Relationship matching.
* Cardinality.
* Trait evaluation.
* Priority resolution.
* Definition-version handling.
* Fingerprint canonicalization.
* Candidate grouping.
* Candidate confidence.
* Alias binding.
* Conformance-trace generation.
* Working-classification persistence.
* Deterministic ordering.

### 9.2 Controlled Fixture Tests

Create minimal fixtures representing:

* Deployment-only application.
* Deployment and Service.
* Deployment, Service, ConfigMap, Secret, and ServiceAccount.
* StatefulSet with Service and PVC.
* DaemonSet node agent.
* Controller with RBAC.
* Operator with CRDs and controller workload.
* Multi-component controller.
* Optional component present and absent.
* Disconnected component.
* Unmounted ConfigMap or Secret.
* Excess resource variant.
* Missing required relationship.
* Multiple possible roots.
* Two definitions competing for one resource family.

Each fixture must have an explicit expected classification, binding map, and explanation.

### 9.3 Real Installation Tests

Use the existing real-chart corpus:

* Argo CD.
* cert-manager.
* external-secrets.
* ingress-nginx.
* Grafana.
* kube-state-metrics.
* node-exporter.
* Kubernetes and Kind system components.
* Local-path provisioner.

Validate that real installations produce understandable structural classifications rather than merely successful matches.

### 9.4 Adversarial Tests

Test cases designed to produce false positives:

* Two Deployments sharing only a root kind.
* Two applications with identical standard Kubernetes labels.
* A Service whose selector targets another application.
* A ConfigMap referenced by multiple unrelated workloads.
* Copied Helm labels on a runtime-generated resource.
* Shared RBAC resources.
* Multiple charts installing the same CRD.
* Structurally similar resources with different purposes.
* Similar names with unrelated relationships.
* Missing relationship evidence.
* Stale owner references.
* Cross-namespace references.
* Cluster-scoped resources shared by multiple instances.

The matcher must prefer insufficient knowledge over false structural certainty.

### 9.5 Persistence Tests

Verify that:

* Accepted definitions survive restart.
* Working candidate classifications survive restart.
* Candidate identity remains stable after restart.
* Match results can be reproduced from persisted facts.
* Definition-version changes trigger re-evaluation.
* Removed definitions do not leave stale named-shape instances.
* Explanation traces reference the correct definition version.
* Historical candidate assessments remain distinguishable from current assessments.

### 9.6 Determinism Tests

Run identical input repeatedly with:

* Random resource ordering.
* Random relationship ordering.
* Random map iteration.
* Restarted processes.
* Rebuilt indexes.
* Equivalent YAML field ordering.

The same input and definition set must produce:

* Identical fingerprints.
* Identical candidate groups.
* Identical winning definitions.
* Identical alias bindings.
* Identical explanation ordering.

### 9.7 Scale Tests

Test representative cluster sizes:

```text
Small:    500 resources
Medium:   5,000 resources
Large:   50,000 resources
Fleet-like local corpus: multiple cluster snapshots
```

Measure:

* Collection-to-classification time.
* Candidate discovery time.
* Matching time per definition.
* Memory consumption.
* SQLite growth.
* Re-evaluation cost after one resource changes.
* Re-evaluation cost after one definition changes.
* Explanation-generation latency.

A localized resource change should not require unnecessary full-cluster recomputation.

## 10. Named Shape Tests

For every accepted shape definition, verify:

* The definition has a clear display name.
* The role is meaningful.
* Roots are neither overly broad nor overly narrow.
* Required components are truly defining.
* Optional components represent valid variation.
* Relationships bind aliases unambiguously.
* Cardinality matches expected architecture.
* Framework resources are treated consistently.
* Unmatched-resource behavior is explicit.
* The definition distinguishes its intended instances from unrelated families.
* The definition can explain its own requirements in operator language.

A shape is not complete merely because it matches fixtures. It must communicate a useful architectural concept.

## 11. Instance Conformance Tests

For every matched instance, verify that the explanation can answer:

* What shape matched?
* Why did it match?
* Which root resource established the instance?
* Which resources fill each alias?
* Which relationships connect them?
* Which evidence was authoritative, corroborating, or inferred?
* Which components were optional?
* Which additional resources were treated as variants?
* Did another definition also match?
* Why did the selected definition win?
* Is the instance conforming, variant, partial, or divergent?

The expected binding map should be a first-class test assertion, not only the shape name.

## 12. Variant and Drift Tests

Test differences between instances of the same named shape:

* Optional resource added.
* Optional resource removed.
* Required resource missing.
* Cardinality changed.
* Relationship target changed.
* Service removed.
* ConfigMap replaced by Secret.
* RBAC expanded or reduced.
* CRD version changed.
* Conversion webhook added.
* Sidecar added.
* Exposure resource added.
* Component split into multiple workloads.

Classify differences as appropriate:

```text
Conforming
Variant
Partial
Divergent
```

Structural drift must be based on definition-relevant evidence, not transient runtime state.

## 13. Candidate Discovery Tests

Candidate testing must validate:

### 13.1 Grouping

* Recurring instances with equivalent semantic structure group together.
* Mechanically different but semantically equivalent instances can group when permitted.
* Structurally different instances do not group solely because they share a root kind.
* Framework resources do not create unnecessary candidate splits.
* Defining relationships influence the semantic fingerprint.
* Missing relationships reduce coverage and confidence.

### 13.2 Explanation

Candidate explanation must show:

* Semantic fingerprint.
* Mechanical fingerprint.
* Root kind.
* Instance count.
* Recurrence.
* Cohesion.
* Relationship coverage.
* Defining resources.
* Framework resources.
* Common relationships.
* Distinguishing evidence.
* Knowledge gaps.
* Observed instances.
* Reason for grouping.

### 13.3 Generation

A generated definition must:

* Be valid against the ShapeDefinition schema.
* Preserve the candidate's observed root.
* Generate aliases for supported components.
* Generate relationships only when aliases can be bound.
* Avoid invalid placeholder targets.
* Include provenance outside the functional definition.
* Clearly indicate required operator review.
* Avoid promoting weak root-only evidence as a strong definition.

### 13.4 Dry-Run Validation

Dry-run testing must report:

* Candidate instances matched.
* Other unclassified instances matched.
* Existing classified instances matched.
* Higher-priority conflicts.
* Inventory coverage.
* Relationship coverage.
* False-positive risk.
* Promotion blockers.

## 14. Candidate Working Classification Tests

Verify that an operator can record:

```text
Role
Structural affinity
Proposed name
Confidence
Rationale
Source
Timestamp
```

Test that:

* Classification does not alter the candidate fingerprint.
* Classification does not create a named shape.
* Multiple tentative affinities can coexist.
* Human classification remains distinguishable from automated suggestion.
* Classification can be revised without deleting prior evidence.
* Candidates can be grouped by working classification.
* Similar classifications can reveal possible merge opportunities.
* One classification can be applied to mechanically different candidates.
* Conflicting classifications remain visible.
* Promotion requires an accepted ShapeDefinition, not merely a classification.

## 15. Cross-Axis Tests

### 15.1 Organization to Structure

Starting from an application group:

* Identify its structural role.
* Identify any named shape instance.
* Show which grouped resources fill shape aliases.
* Preserve the distinction between membership and structural meaning.

### 15.2 Structure to Organization

Starting from a shape instance:

* Identify the application or group containing it.
* Identify its components and workloads.
* Avoid assuming that a shape and group have identical boundaries.

### 15.3 Structure to Graph

Starting from a shape instance:

* Traverse defining relationships.
* Identify exposure points.
* Identify dependencies.
* Identify shared resources.
* Identify graph orphans without treating them as shape failures unless required by the definition.

### 15.4 Structure to Deployment

Starting from a shape instance:

* Identify the deployment or release authority.
* Preserve the distinction between structural identity and release lifecycle.
* Avoid using chart names as structural definitions unless explicitly configured.

### 15.5 Structure to Janitor

Verify that:

* Accepted shapes can qualify resources for Janitor evaluation.
* Definition version is preserved in the qualification evidence.
* Partial and divergent instances can be reported.
* Candidates cannot authorize neutralize or delete actions.
* Working classifications cannot authorize destructive actions.
* Ambiguous shape matches fail closed.

## 16. CLI and Output Usability Tests

Verify that output supports:

* High-level structural summary.
* Defined taxonomy discovery.
* Definition explanation.
* Instance inventory.
* Instance conformance explanation.
* Resource-to-shape reverse lookup.
* Candidate listing.
* Candidate explanation.
* Working-classification visibility.
* Generated-definition review.
* Dry-run validation.
* Machine-readable output consistency.

Output must:

* Use stable terminology.
* Distinguish roles, shapes, instances, candidates, and affinities.
* Avoid exposing candidates through the named-shapes view.
* Separate observed facts from operator interpretation.
* Clearly label partial knowledge.
* Use deterministic ordering.
* Avoid internal implementation terminology where operator terminology exists.
* Avoid requiring operators to infer alias bindings from raw resource lists.

Do not add new CLI verbs or flags as part of test remediation unless the current command model cannot express a required traversal.

## 17. Regression Requirements

Every correction must rerun:

* Shape parser tests.
* Matcher tests.
* Binding tests.
* Candidate fingerprint tests.
* Candidate grouping tests.
* Explanation tests.
* Persistence tests.
* Organization-to-Structure traversal tests.
* Graph relationship tests used by shape matching.
* Janitor safety tests.
* Real-chart corpus tests.

A fix for one shape must not silently change unrelated fingerprints or candidate groups.

## 18. Exit Criteria

The Structure axis is functionally complete when:

* Operators can discover the defined taxonomy.
* Every named shape can explain what defines it.
* Every matched instance can explain how resources fill the definition.
* Operators can traverse from shape to resource and resource to shape.
* Role classifications remain distinct from named shapes.
* Candidates remain distinct from accepted taxonomy.
* Recurring unnamed structures are grouped deterministically.
* Candidates can receive working classifications without premature promotion.
* Generated definitions are valid and reviewable.
* Dry-run promotion exposes false-positive risk and conflicts.
* Variants and divergence are visible.
* Cross-axis transitions preserve domain boundaries.
* Candidate and tentative knowledge cannot authorize destructive Janitor behavior.
* Real installations produce meaningful architectural descriptions.
* Output is accurate, comprehensive, explainable, and intuitive.

## 19. Test Case Template

```markdown
### STRUCT-<AREA>-<NUMBER>: <Title>

**Purpose**

Describe the structural behavior being validated.

**Fixture or Cluster State**

List required resources, relationships, definitions, and prior classifications.

**Traversal Start**

Identify the broadest point where the operator begins.

**Traversal Steps**

Describe each transition from summary to detail or detail to related knowledge.

**Expected Structural Result**

State the expected role, shape, instance, candidate, or working classification.

**Expected Bindings**

List each shape alias and the concrete resources expected to fill it.

**Expected Evidence**

List the facts and relationships supporting the result.

**Expected Explanation**

State what the operator must be able to understand from the output.

**Negative Assertions**

List classifications, bindings, or conclusions that must not occur.

**Cross-Axis Assertions**

Identify any valid transition into Organization, Deployment, Graph, or Janitor.

**Persistence Assertions**

State whether the result must remain stable after restart or re-index.

**Determinism Assertions**

State which identifiers, fingerprints, ordering, and bindings must remain stable.

**Result**

Not Run | Pass | Fail | Blocked

**Notes**

Record discrepancies, ambiguities, and follow-up work.
```

## 20. Initial Execution Order

Execute Structure testing in this order:

1. Taxonomy discovery and definition explanation.
2. Named-shape matching.
3. Instance binding and conformance explanation.
4. Reverse traversal from resources.
5. Variant and drift behavior.
6. Candidate grouping and explanation.
7. Candidate working classification.
8. Definition generation and dry-run promotion.
9. Cross-axis traversal.
10. Persistence and determinism.
11. Adversarial real-chart testing.
12. Scale and performance.

This order establishes trusted accepted structure before testing discovery and promotion of new structure.
