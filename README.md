# Kube Open Shape

Kube Open Shape (`kos`) turns raw Kubernetes resources into navigable operational knowledge.

It discovers what is running, groups related resources into applications and releases, identifies lifecycle authority, maps structural relationships, recognizes recurring deployment shapes, and surfaces resources that may no longer belong.

KOS is CLI-first, cloud-neutral, and runs autonomously against a Kubernetes cluster. It does not require a central service.

> KOS is not a health or workload-monitoring system. It focuses on cluster composition, provenance, structure, relationships, drift, and safe lifecycle management.

## What Can KOS Tell Me?

- What resources, applications, and releases exist in this cluster?
- Which resources comprise an application?
- How did a resource get here?
- Which authority controls or reconciles it?
- Which resources depend on it?
- How is a workload exposed?
- What recurring architectural patterns exist?
- Which resources do not fit a known structure?
- What would be affected if an application were retired?
- Which findings are actionable, protected by a reconciler, or indeterminate?

## Project Status

Kube Open Shape is under active development and should currently be considered alpha software.

The edge collector, knowledge engine, CLI, local API, shape discovery, and observe-only Janitor are implemented. Neutralize and delete actions are not yet enabled.

| Capability | Status |
|---|---|
| Dynamic cluster collection | Available |
| Resource inventory and navigation | Available |
| Evidence-based ownership attribution | Available |
| Application and release grouping | Available |
| Structural relationship graph | Available |
| CRD-defined shapes | Available |
| Candidate discovery and affinity | Available |
| ShapeDefinition generation and dry-run testing | Available |
| Helm release discovery | Available |
| Argo CD Application and ApplicationSet discovery | Available |
| Authority handoff modeling | Available |
| Local HTTP API | Available |
| SQLite persistence | Available |
| Janitor findings and safety classification | Observe-only |
| Graph-derived execution plans | Planned |
| Neutralize and delete actions | Not implemented |
| Fleet aggregation | Planned |

## Quick Start

### Prerequisites

- Go toolchain
- Access to a Kubernetes cluster
- A valid current kubeconfig context
- Sufficient Kubernetes permissions to list and watch the resources being analyzed

Confirm the active context:

```bash
kubectl config current-context
```

### Build

```bash
make build
```

### Explore the cluster

```bash
# What resources exist?
bin/kos resources

# What belongs together?
bin/kos groups

# How did resources get here, and who controls them?
bin/kos ownership

# What releases are installed?
bin/kos releases

# What structural shapes are recognized?
bin/kos shapes

# What recurring structures remain unnamed?
bin/kos candidates

# What Janitor findings currently exist?
bin/kos findings
```

### Investigate a resource

```bash
bin/kos describe resource Deployment my-app -n default
```

A resource description includes its observed identity, ownership attribution, group membership, relationships, labels, and supporting evidence.

### Investigate unnamed structures

```bash
# Explain the evidence behind the first candidate
bin/kos candidates explain --first

# Generate a draft ShapeDefinition
bin/kos candidates generate --first

# Test the generated definition without applying it
bin/kos candidates test --first
```

Generated definitions are drafts. They do not become accepted structural knowledge or authorize Janitor actions without review.

### Export the knowledge graph

```bash
bin/kos graph export
```

### Run the edge controller

```bash
bin/edge
```

The edge controller continuously observes the cluster, maintains local knowledge, schedules Janitor evaluation, persists lifecycle state in SQLite, and exposes the local HTTP API on port `9090`.

The one-shot `kos` CLI performs the same analysis without requiring the long-running controller.

## Knowledge Model

KOS exposes four complementary ways to traverse a Kubernetes cluster.

| View | Questions |
|---|---|
| Organization | What exists, what belongs together, and who controls it? |
| Deployment | What releases are installed, from which manager, source, and revision? |
| Structure | What architectural shapes and roles comprise the cluster? |
| Graph | What depends on what, what is exposed, and what is the blast radius? |

These views share the same observed knowledge without conflating their meanings.

For example:

```text
Terraform
  └── provisions ──► Argo CD Application
                          └── reconciles ──► Group
                                                └── contains ──► Resources
```

Terraform explains how the Argo CD Application arrived. The Argo CD Application is the active reconciliation authority for the group it deploys. The Application resource is not treated as the structural root of that group.

## Core Concepts

### Resource

An observed Kubernetes API object.

### Group

A logical collection of related resources, such as an application or release. Group membership is established through evidence including labels, release metadata, controller tracking, and graph relationships.

### Release

A deployment lifecycle record produced by a release manager such as Helm or Argo CD.

### Lifecycle Authority

The system responsible for creating, maintaining, or reconciling a resource or group. Authority is evidence-based and contextual.

A resource may have one authority while acting as the authority for another group.

### Relationship

A typed, directed connection between resources or knowledge objects. Examples include:

- A Service selecting a workload.
- A workload using a ServiceAccount.
- A RoleBinding granting a Role.
- A workload referencing a ConfigMap or Secret.
- An Argo CD Application reconciling a group.
- An ApplicationSet generating an Application.

### Shape

A named structural composition defined by a `ShapeDefinition`. Shapes describe recurring Kubernetes architecture using roots, components, relationships, cardinality, and composition rules.

### Candidate

A recurring or significant structure that does not yet match an accepted shape. Candidates can receive tentative affinities and be used to generate draft definitions.

### Finding

The result of evaluating observed cluster knowledge against a Janitor rule. Findings remain visible even when safety constraints prevent action.

## Evidence and Confidence

KOS distinguishes observed facts from inferred knowledge.

Ownership, grouping, relationships, shapes, candidates, and findings retain their supporting evidence and confidence. Unknown or conflicting evidence remains visible; KOS does not invent ownership to make the inventory appear complete.

Examples of evidence include:

- Kubernetes owner references.
- Explicit spec references.
- Selector matches.
- Helm release manifests and metadata.
- Argo CD tracking information.
- Application and ApplicationSet relationships.
- Organization-defined labels and annotations.
- Structural graph relationships.

Candidate affinity, working classification, and generated ShapeDefinitions are provisional knowledge. They cannot independently authorize destructive behavior.

## Structural Shapes

Shapes allow operators to describe Kubernetes systems using terminology meaningful to them.

Examples might include:

- Small web application.
- Backend service.
- Stateful service.
- Controller.
- Operator.
- Node agent.
- Metrics exporter.
- Admission controller.
- Multi-component platform application.

A shape definition is stored as Kubernetes configuration rather than hardcoded Go logic.

```yaml
apiVersion: knowledge.kos.io/v1alpha1
kind: ShapeDefinition
metadata:
  name: web-application
spec:
  schemaVersion: 1
  definitionVersion: 1
  displayName: Web Application
  role: application

  roots:
    - alias: workload
      resource:
        apiGroups: ["apps"]
        kinds: ["Deployment"]

  components:
    - alias: service
      resource:
        apiGroups: [""]
        kinds: ["Service"]
      cardinality:
        min: 1

  relationships:
    - from: service
      type: SelectsWorkload
      to: workload
      required: true
```

KOS also discovers unnamed structures and groups exact semantic matches into candidates. Operators or AI-assisted tooling can assign affinities, generate definitions, test them against the cluster, and promote accepted knowledge after review.

## Relationship Model

Relationships are derived from explicit Kubernetes fields, selectors, ownership, release metadata, and reconciliation evidence.

| Relationship | Direction | Typical source |
|---|---|---|
| UsesServiceAccount | Workload → ServiceAccount | `spec.serviceAccountName` |
| SelectsWorkload | Service → Workload | Service selector and workload labels |
| BindsSubject | RoleBinding → ServiceAccount | `subjects` |
| GrantsRole | RoleBinding → Role | `roleRef` |
| Mounts | Workload → ConfigMap | Volumes and environment references |
| References | Workload → Secret | Volumes and environment references |
| ClaimsStorage | StatefulSet → PVC | Volume claim templates |
| UsesHeadlessService | StatefulSet → Service | `spec.serviceName` |
| Reconciles | Application → Group | Argo CD managed-resource evidence |
| Generates | ApplicationSet → Application | Argo CD generation evidence |
| Provisions | Authority → Control resource | Deployment provenance |

Provenance relationships such as `BelongsToRelease` and `ManagedBy` remain in the graph as contextual boundaries but are excluded from structural shape fingerprints.

Authority relationships such as `Reconciles` and `Generates` establish actionability constraints. They do not enter the Janitor teardown dependency DAG.

## Ownership and Authority

The ownership engine uses normalized facts, configurable catalogs, decision rules, and deterministic resolution.

It distinguishes:

- Direct attribution.
- Inherited attribution.
- Framework descendants.
- Authority records.
- Platform-generated resources.
- Active reconciliation.
- Provenance without continuous reconciliation.
- Conflicting evidence.
- No known authority.

KOS distinguishes deployment provenance from active reconciliation.

```text
Argo CD auto-sync:
  Reconciliation mode: Continuous
  State: Active
  Janitor actionability: Protected

Terraform-attributed resource:
  Reconciliation mode: None
  Janitor actionability: Continues through safety evaluation
```

Terraform attribution requires reliable metadata supplied by the deployment process. Without sufficient evidence, KOS reports no known authority rather than guessing.

## Janitor Safety Model

The Janitor evaluates cluster knowledge against rules and produces findings. The current implementation is observe-only.

Every finding has two orthogonal dimensions:

- **Status:** Active, Proposed, Approved, Executing, Executed, Failed, Suppressed, or Resolved.
- **Actionability:** Actionable, Protected, or Indeterminate.

The generic reconciliation model determines whether an active authority protects a resource or group.

| Mode | State | Actionability |
|---|---|---|
| Continuous | Active | Protected |
| Continuous | Unknown | Indeterminate |
| None | — | Continue safety evaluation |
| Unknown | — | Indeterminate |

Subsystem degradation blocks mutation but remains visible. Silence is not considered fail-closed behavior.

Future mutation follows this model:

```text
Rule
  → Finding
  → Actionability decision
  → Action boundary
  → Graph closure and ordering
  → Immutable execution plan
  → Operator approval
  → Pre-execution revalidation
  → Idempotent execution
  → Post-execution graph verification
```

Rules qualify a subject. The graph defines the action boundary and execution order. Safety constraints cap the action. Operators approve an immutable plan. Janitor executes and verifies that exact plan.

Candidate affinity, working classification, generated definitions, unknown authority, or incomplete graph knowledge can never independently authorize destructive action.

See the [Janitor Safety Model](docs/changelog/2026-08-19-janitor-safety-model.md) for the complete specification.

## Architecture

```text
Kubernetes API
      │
      ▼
Dynamic Collector
      │
      ▼
Resource Index
      │
      ▼
Knowledge Graph
      ├── Ownership and authority
      ├── Groups and releases
      ├── Relationships
      ├── Shapes and candidates
      ├── Janitor findings
      ├── SQLite persistence
      ├── Local HTTP API
      └── CLI
```

Repository structure:

```text
cmd/edge/       Long-running controller
cmd/kos/        One-shot CLI
cli/            CLI command implementations

internal/edge/
  collector/    Dynamic informers and reference extraction
  knowledge/    In-memory resource index
  ownership/    Fact extraction, catalogs, rules, and resolution
  graph/        Directed relationship graph
  release/      Release-manager integrations
  shape/        Compiler, matcher, candidates, and generation
  janitor/      Safety model, rule engine, and lifecycle clocks
  store/        SQLite persistence
  api/          Local HTTP API

api/v1alpha1/   Kubernetes API and CRD types
```

## Extensibility

KOS is designed to acquire domain knowledge through configuration rather than hardcoded product logic.

Current extension points include:

- `ShapeDefinition`
- `RelationshipDefinition`
- `JanitorRule`
- Ownership fact extractors
- Ownership catalogs and decision rules
- Release-manager integrations
- Candidate affinities
- Operational classification definitions

The intended knowledge-development loop is:

```text
Observe
  → discover candidate
  → inspect evidence
  → assign affinity
  → generate definition
  → dry-run against inventory
  → review
  → promote accepted knowledge
```

## Local API

The edge exposes a local HTTP API for querying collected knowledge, including resources, graph relationships, ownership, shapes, and findings.

Current endpoints are implemented under `/api/v1`. Refer to the source in `internal/edge/api/` for the current API surface while the contract remains alpha.

## Testing

```bash
# Unit tests
make test

# Integration tests against a Kubernetes cluster
make test-integration

# Helm integration tests
make test-helm-integration
```

Integration tests exercise real Kubernetes resources and require access to a disposable test cluster.

## Roadmap

Near-term development priorities include:

- Release-axis testing and hardening.
- Release history and progression.
- Consistent YAML and JSON output across commands.
- JSONPath and custom-column output.
- Additional release-manager integrations.
- Stable edge knowledge and communication contracts.
- Multi-cluster Fleet aggregation.
- Cross-cluster inventory, comparison, and drift.
- Operational-classification discovery and distribution.
- Graph-derived Janitor execution plans.
- Neutralize and conditional delete actions.
- AI-agent tooling grounded by the deterministic KOS knowledge API.

The edge will remain autonomous and useful without Fleet. Fleet is intended to aggregate normalized edge knowledge for multi-cluster visibility, history, comparison, classification, and reporting.

## Documentation

- [CLI Reference](docs/kos-cli-reference.md)
- [Janitor Safety Model](docs/changelog/2026-08-19-janitor-safety-model.md)
- [Edge Implementation Plan](docs/changelog/2026-08-18-edge-implementation-plan.md)
- [Shape Definition Specification](docs/changelog/2026-08-18-shape-definition-spec.md)
- [Intelligent Shape Grouping](docs/changelog/2026-08-18-intelligent-shape-grouping.md)
- [Product Implementation Plan](docs/changelog/2026-08-17-implementation-plan.md)

## Contributing

Contributions are welcome, particularly in:

- Relationship definitions.
- Ownership catalogs and rules.
- Shape definitions.
- Release-manager integrations.
- Kubernetes distribution testing.
- Documentation.
- Integration fixtures.

Please open an issue before beginning a large architectural change.

## Security

The Janitor currently operates in observe-only mode and does not perform Neutralize or Delete actions.

Please report security vulnerabilities privately rather than through a public issue. A formal security policy and reporting channel will be added as the project approaches a public release.

## License

Licensed under the [Apache License 2.0](LICENSE).