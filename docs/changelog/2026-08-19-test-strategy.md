# KOS General Test and Hardening Strategy

## 1. Purpose

This strategy defines how KOS traversal axes are tested and hardened.

Testing proceeds one traversal axis at a time. Each axis is exercised against controlled fixtures, adversarial fixtures, and real Kubernetes installations. Identified gaps are corrected before expanding to the next axis.

The initial focus is the Organization axis because it most closely aligns with current Kubernetes operator workflows and provides the base navigation model for the other axes.

## 2. Traversal Axes

KOS provides four complementary ways to understand a cluster.

| Axis | Primary question | Scope |
|------|-----------------|-------|
| Organization | What is installed, where is it, and which resources belong together? | Namespaces, applications, groups, components, workloads, and resources |
| Deployment | How did it get here, what version is installed, and how has it changed? | Releases, revisions, managers, ownership, current state, and history |
| Structure | What types of systems and patterns comprise the cluster? | Role classifiers, named shapes, shape instances, bindings, variants, drift, and candidates |
| Graph | How are resources connected, exposed, and affected by change? | Relationships, access points, reachability, dependencies, blast radius, and orphans |

Janitor is a cross-cutting action plane that consumes accepted knowledge from all four axes.

## 3. Iterative Hardening Approach

Each traversal axis follows the same cycle:

1. Establish representative controlled fixtures.
2. Run the applicable KOS commands.
3. Validate broad-to-narrow navigation.
4. Validate narrow-to-broad navigation.
5. Compare output with Kubernetes source objects.
6. Identify missing, inaccurate, ambiguous, or misleading knowledge.
7. Correct the knowledge model or presentation.
8. Add regression tests for the discovered gap.
9. Introduce adversarial cases.
10. Test against real Helm-installed applications.
11. Repeat until the axis exit criteria are satisfied.

A defect discovered during exploratory testing must result in a repeatable regression test.

## 4. General Testing Goals

Every traversal axis must satisfy the following goals.

### 4.1 Intuitive navigation

Operators must be able to move:

- From cluster-level summaries to detailed resources.
- From individual resources back to their higher-level context.
- Between related entities without reconstructing identifiers manually.
- Into another traversal axis when appropriate.
- Back to the original context after cross-axis traversal.

Every entity displayed by KOS must have a stable and unambiguous identity that can be used by another command.

### 4.2 Accuracy

Output must accurately represent the Kubernetes objects and deterministic conclusions in the local knowledge store.

KOS must not:

- Invent relationships unsupported by Kubernetes semantics.
- Silently merge ambiguous identities.
- Present inferred conclusions as authoritative.
- Present incomplete discovery as complete.
- Use naming or label similarity as proof of operational behavior.

### 4.3 Completeness

Within the edge's configured permissions and collection scope, output must account for:

- Namespaced resources.
- Cluster-scoped resources.
- Generated resources.
- Shared resources.
- Unassigned resources.
- Cross-namespace relationships.
- Multiple grouping and ownership dimensions.
- Resources without recognized metadata.

If permissions or configuration prevent complete discovery, KOS must identify the result as incomplete.

### 4.4 Explainability

Every derived conclusion must be traceable to evidence.

Examples include:

- Explicit Kubernetes fields.
- Selectors.
- Owner references.
- Package metadata.
- Recommended Kubernetes labels.
- ShapeDefinition matches.
- Ownership inheritance.
- Configured relationship definitions.

Default output should summarize conclusions. Detailed output must expose the evidence and model version when requested.

### 4.5 Determinism

Given the same knowledge snapshot and model versions:

- Entity IDs must remain stable.
- Relationship IDs must remain stable.
- Ordering must remain stable.
- Counts must remain stable.
- Confidence must remain stable.
- Repeated exports must be semantically identical.
- Restarting the edge must not change conclusions.

### 4.6 Kubernetes CLI consistency

KOS must reuse kubectl conventions where equivalent behavior exists.

This includes:

- `-n`, `--namespace`
- `-A`, `--all-namespaces`
- `-l`, `--selector`
- `--field-selector`
- `--sort-by`
- `--no-headers`
- `--context`
- `--kubeconfig`
- `-o json`
- `-o yaml`
- `-o wide`
- `-o jsonpath`
- `-o custom-columns`

KOS-specific flags should only represent knowledge operations without a kubectl equivalent.

`describe` is used for expanded human-readable knowledge. Output flags are reserved for table or serialized representations.

## 5. Core Invariants

The following invariants apply across all axes.

### 5.1 Aggregate reconciliation

Every summary count must reconcile with its detailed inventory.

If a group reports:
```
7 workloads
8 components
64 resources
```

the detailed result must contain exactly:
- Seven unique workload roots.
- Eight distinct declared components.
- Sixty-four unique resources.

### 5.2 Unique membership

A resource must not be counted twice in the same metric because it is reachable through multiple relationships.

Multiple legitimate memberships must remain distinguishable, such as:
- Application membership.
- Component membership.
- Release membership.
- Shape binding.
- Ownership.

### 5.3 Unambiguous lookup

If a supplied identity resolves to more than one entity, KOS must return an ambiguity error and list the valid choices.

KOS must never silently select the first match.

### 5.4 Bidirectional traversal

Every downward traversal must have a corresponding upward traversal when the knowledge exists.

Examples:
```
Application → Component → Workload → Resource
Resource → Workload → Component → Application

ShapeDefinition → ShapeInstance → Resource binding
Resource → ShapeInstance → ShapeDefinition
```

### 5.5 Evidence integrity

Evidence type must match the assertion it supports.

Examples:
- ExplicitField may support Mounts.
- SelectorMatch may support SelectsWorkload.
- OwnerReference may support Owns.
- LabelAssociation may support MemberOf or BelongsToRelease.

Label association must not support Mounts, References, or SelectsWorkload.

### 5.6 Human and structured output parity

Human-readable, JSON, YAML, JSONPath, and custom-column output must derive from the same canonical projection.

Formatting must not change:
- Membership.
- Counts.
- Confidence.
- Identity.
- Classification.
- Relationships.

## 6. Test Layers

### 6.1 Unit tests

Unit tests validate isolated behavior such as:

- Identity generation.
- Label extraction.
- Relationship extraction.
- Confidence calculation.
- Scope resolution.
- Alias binding.
- Cardinality evaluation.
- Output sorting.
- Evidence categorization.
- Group and release identity.

### 6.2 Controlled fixture integration tests

Controlled fixtures establish expected positive behavior.

Examples:
- Two identical simple applications.
- An application with RBAC.
- A stateful application with storage.
- Multiple components in one application.
- One component containing multiple workloads.
- A component containing no workload.
- An application spanning multiple namespaces.
- Cluster-scoped resources associated with a namespaced application.

### 6.3 Adversarial tests

Adversarial fixtures prove that KOS does not infer unsupported knowledge.

Examples:
- Service exists but does not select the workload.
- ConfigMap exists but is not mounted.
- Resources share labels but are operationally unrelated.
- Same application name exists in multiple namespaces.
- Application and release share the same name.
- Conflicting part-of, instance, and Helm evidence.
- Same instance label is reused by unrelated systems.
- Resource moves between groups.
- Release-managed resource lacks recommended labels.

### 6.4 Real-world installation tests

Real Helm charts validate behavior against uncontrolled metadata and resource composition.

The corpus should include:
- Multi-component control planes.
- Operators and webhooks.
- DaemonSet-based node agents.
- Stateful applications.
- Ingress controllers.
- Monitoring applications.
- Applications with CRDs and cluster-scoped RBAC.

Real-world tests identify missing abstractions and relationship types. They must not be handled through product-specific hardcoded logic.

### 6.5 Persistence and reconciliation tests

Validate behavior across:

- Edge restart.
- SQLite rebuild.
- Resource creation.
- Resource deletion.
- Resource recreation with a new UID.
- Label addition, removal, and modification.
- Component reassignment.
- Helm upgrade.
- Namespace change.
- Definition version change.
- Model version change.

### 6.6 Degraded-environment tests

Validate behavior when:

- RBAC prevents listing one or more resource kinds.
- A CRD is unavailable.
- A namespace is excluded.
- The local snapshot is stale.
- An API discovery request fails.
- A referenced resource is missing.
- The Kubernetes API is temporarily unavailable.

KOS must distinguish incomplete knowledge from a verified absence.

### 6.7 Scale and performance tests

Test representative inventories such as:

- 100 namespaces
- 1,000 logical groups
- 10,000–50,000 resources
- Multiple cross-namespace applications
- Cluster-scoped resources
- High relationship density

Measure:

- Initial collection duration.
- Incremental reconciliation duration.
- SQLite size.
- Memory consumption.
- `kos groups` latency.
- `kos describe` latency.
- Relationship traversal latency.
- Knowledge graph export duration.

CLI commands should query indexed projections rather than reconstructing the complete graph for every request.

## 7. Organization Axis Test Strategy

Organization is the first axis to be hardened.

### 7.1 Organization scope

Organization provides the traditional Kubernetes inventory view with the navigation layer missing from kubectl:

```
Cluster → Namespace → Application/group → Component → Workload → Supporting resource
```

### 7.2 Applicable commands

```
kos groups
kos resources
kos describe groups
kos describe component
kos describe resource
```

### 7.3 Navigation tests

Validate:

- Cluster → Group
- Group → Component
- Component → Workload
- Workload → Supporting resource
- Resource → Workload
- Resource → Component
- Resource → Group

Each displayed identity must be usable in the next command.

### 7.4 Grouping tests

Validate:

- `app.kubernetes.io/part-of`.
- `app.kubernetes.io/instance`.
- `app.kubernetes.io/component`.
- Helm release evidence.
- Custom grouping metadata.
- Shared and unassigned resources.
- Cross-namespace membership.
- Cluster-scoped membership.
- Multiple releases contributing to one application.
- One release contributing to multiple logical groups.
- Conflicting grouping signals.

### 7.5 Scope tests

Validate:

```
kos groups
kos groups -n argocd
kos groups -A
kos resources
kos resources deployment
kos resources deployment -n argocd
```

Confirm:

- Default scope is documented and consistent.
- Namespace filtering does not corrupt group identity.
- Cluster-scoped resources remain correctly associated.
- Home namespace and member namespaces remain distinct.
- Kubeconfig and context selection match kubectl conventions.

### 7.6 Organization output tests

Default list output must be concise and navigable.

Detailed output must distinguish:

- Workload count.
- Declared component count.
- Resource count.
- Shared or unassigned resources.
- Home namespace.
- Member namespaces.
- Confidence.
- Evidence.
- Related releases.
- Available cross-axis navigation.

A missing component label should display as unknown or undeclared, not imply that the application contains no components.

### 7.7 Cross-axis handoff tests

Organization views may summarize other axes but must not absorb their detailed responsibilities.

Validate navigation from a group into:

- **Deployment:** Associated releases and current revision
- **Structure:** Named and unnamed workload shapes
- **Graph:** Access points, relationships, orphans, and blast radius

Example command paths:

```bash
kos describe groups argocd
kos describe releases argocd -n argocd
kos shapes --group argocd
kos access --group argocd
kos relationships --group argocd
```

The original group identity and scope must remain available during the handoff.

## 8. Organization Axis Exit Criteria

Organization is considered hardened when:

- Every recognized resource can be navigated upward to its organizational context.
- Every group can be navigated downward to unique resources.
- Group, component, workload, and resource counts reconcile.
- Cross-namespace and cluster-scoped membership is represented accurately.
- Shared and unassigned resources are visible.
- Ambiguous identities are never silently resolved.
- Conflicting grouping evidence is retained and explained.
- Incomplete discovery is clearly identified.
- Human and structured output agree.
- Results remain stable across restart and reconciliation.
- Real Helm installations produce useful, explainable grouping.
- Adversarial fixtures do not create false membership.
- Cross-axis handoffs preserve group identity and scope.

## 9. Test Case Format

Test cases should use the following structure:

```
Test ID:
Traversal axis:
Capability:
Operator question:
Fixture or environment:
Preconditions:
Command:
Expected human output:
Expected structured output:
Expected navigation targets:
Expected evidence:
Expected confidence:
Reconciliation invariants:
Negative assertions:
State-transition behavior:
Performance expectation:
```

Each test should identify the operator question being answered. Tests should validate knowledge behavior, not merely command execution.

## 10. Non-Goals

This testing strategy does not validate:

- Pod health.
- Container readiness.
- Job success.
- Application logs.
- Runtime traffic.
- Runtime threat detection.
- General observability or monitoring.

KOS tests declared Kubernetes state, derived knowledge, lifecycle, structure, connectivity, ownership, and governed actions.
