# Contributing to Kube Open Shape

Thank you for your interest in Kube Open Shape.

Kube Open Shape is an open-source, CLI-first Kubernetes knowledge engine. Contributions are welcome across code, documentation, relationship definitions, ownership rules, shape definitions, release-manager integrations, tests, and Kubernetes distribution support.

The project is currently alpha. Interfaces and internal packages may change while the knowledge model and operator experience stabilize.

## Ways to Contribute

Useful contributions include:

- Bug reports with reproducible examples.
- CLI usability improvements.
- Kubernetes relationship extraction.
- Ownership facts, catalogs, and decision rules.
- `ShapeDefinition` examples.
- Candidate grouping and matching improvements.
- Release-manager integrations.
- Janitor evaluators and safety tests.
- Integration fixtures for real Kubernetes products.
- Kubernetes distribution compatibility testing.
- Performance and memory improvements.
- Documentation and examples.
- Product and architectural discussion.

Small corrections can be submitted directly. Please open an issue before beginning a large feature or architectural change.

## Project Principles

Contributions should preserve the following architectural principles.

### Evidence Before Conclusions

KOS must retain the evidence supporting ownership, grouping, relationships, shapes, classifications, and findings.

Do not convert weak signals into authoritative conclusions merely to improve apparent coverage.

```text
Observed evidence
  → normalized facts
  → deterministic interpretation
  → explainable conclusion
```

Unknown and conflicting evidence should remain visible.

### Facts and Interpretation Are Separate

Collectors and extractors should produce normalized facts. Catalogs, rules, definitions, and resolution logic should interpret those facts.

When practical, product and organization knowledge should be represented through configuration rather than hardcoded conditionals in Go.

### Traversal Views Remain Distinct

KOS currently exposes four complementary traversal views:

- Organization.
- Deployment.
- Structure.
- Graph.

A resource can participate in all four views, but their meanings should not be conflated.

For example, an Argo CD Application can be provisioned by Terraform while acting as the reconciliation authority for a separate deployed group.

### The Edge Remains Autonomous

The open-source edge and CLI must remain useful without a central service.

Features may support future Fleet integration, but local collection, knowledge construction, navigation, rule evaluation, and reporting must not require Fleet connectivity.

### Fail Closed Without Becoming Silent

Uncertainty must block unsafe action, but it must not hide findings or degraded system state.

```text
Insufficient knowledge → visible Indeterminate result
Active reconciler      → visible Protected result
Subsystem failure      → visible degraded state
```

### Provisional Knowledge Cannot Authorize Mutation

The following are informational until reviewed and accepted:

- Candidate affinity.
- Working classification.
- Generated ShapeDefinitions.
- Inferred ownership without sufficient evidence.
- AI-generated recommendations.

They must not independently authorize Janitor Neutralize or Delete actions.

## Development Prerequisites

Typical development requires:

- Go.
- Make.
- Git.
- `kubectl`.
- Access to a disposable Kubernetes cluster.
- Helm for Helm integration tests.
- A local container runtime when using `kind`.

Use a non-production cluster for integration and Janitor development.

## Getting Started

Fork the repository and clone your fork:

```bash
git clone https://github.com/YOUR-USERNAME/kube-open-shape.git
cd kube-open-shape
```

Add the upstream repository:

```bash
git remote add upstream https://github.com/rmullinnix461332/kube-open-shape.git
```

Create a branch:

```bash
git checkout -b feature/short-description
```

Build the project:

```bash
make build
```

Run the unit tests:

```bash
make test
```

Before beginning integration testing, confirm that the active Kubernetes context points to a disposable cluster:

```bash
kubectl config current-context
```

## Repository Structure

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
docs/           Specifications and reference documentation
test/           Fixtures and integration tests
```

## Testing

### Unit Tests

```bash
make test
```

New behavior should include focused unit tests covering positive, negative, ambiguous, and failure cases.

### Integration Tests

```bash
make test-integration
```

Integration tests require access to a Kubernetes cluster.

Use a disposable cluster. Integration fixtures may install namespaces, workloads, RBAC resources, CRDs, and Helm releases.

### Helm Integration Tests

```bash
make test-helm-integration
```

Helm tests should validate:

- Release discovery.
- Revision and status.
- Manifest membership.
- Namespaced and cluster-scoped resources.
- Direct and inherited attribution.
- Authority records.
- Shared CRDs.
- Upgrade and rollback behavior where applicable.

### Test Expectations

A contribution should test more than its successful path.

Depending on the change, include coverage for:

- Missing evidence.
- Conflicting evidence.
- Partial relationship coverage.
- Resources with the same name in different namespaces.
- Cluster-scoped resources.
- Controller-created descendants.
- Shared resources.
- Unknown resource kinds.
- Deleted and recreated resources.
- Restart and persistence behavior.
- Invalid configuration.
- Degraded dependencies.
- Deterministic output.

## Go Contributions

Before submitting Go changes:

```bash
gofmt -w ./path/to/changed/files
make test
make build
```

Follow normal Go conventions:

- Prefer small, focused packages.
- Return useful errors with context.
- Avoid global mutable state.
- Preserve deterministic behavior.
- Keep Kubernetes-specific extraction separate from interpretation.
- Do not log credentials or Secret values.
- Avoid adding dependencies when the standard library or an existing dependency is sufficient.
- Document exported types and functions.
- Add tests for behavioral changes.

Do not perform unrelated mechanical refactoring in the same pull request as a functional change.

## CLI Contributions

The KOS CLI should feel familiar to Kubernetes operators.

Follow established `kubectl` conventions where they apply:

- `-n` and `--namespace`.
- `-A` and `--all-namespaces`.
- `-o` and `--output`.
- YAML and JSON output.
- JSONPath.
- Custom columns.
- `--no-headers`.
- Positional resource type and name where appropriate.
- `describe` for detailed, explanatory output.

Do not introduce a new flag when an established Kubernetes convention already expresses the same concept.

Human-readable output should:

- Lead from broad inventory to narrow detail.
- Use consistent terminology.
- Separate facts from inference.
- Expose evidence and uncertainty.
- Avoid presenting candidates as accepted shapes.
- Remain useful without requiring a web interface.

Machine-readable output should be rendered from the same typed result model used by human-readable output.

Changes to command syntax or output contracts should include updated documentation and tests.

## Relationship Contributions

Relationship definitions must identify:

- Source resource kind.
- Target resource kind.
- Exact evidence source.
- Direction.
- Confidence.
- Structural or contextual role.
- Teardown semantics, if applicable.

Prefer explicit Kubernetes fields over name-based inference.

Examples of strong evidence include:

- Owner references.
- `spec.serviceAccountName`.
- `spec.selector`.
- `spec.roleRef`.
- Volume references.
- Environment references.
- Argo CD resource tracking.
- Helm manifest membership.

Name similarity alone should not create an authoritative relationship.

Every new relationship should include fixtures demonstrating:

- A valid match.
- A non-match.
- Namespace behavior.
- Missing target behavior.
- Ambiguous or multiple-target behavior.
- Whether it participates in structural fingerprints.
- Whether it participates in Janitor dependency planning.

Authority relationships such as `Reconciles` and `Generates` must not automatically enter the teardown dependency DAG.

## Ownership Contributions

The ownership engine follows a fact-based model:

```text
Resource
  → extract normalized facts
  → evaluate catalogs and rules
  → create authority claims
  → resolve claims deterministically
  → preserve evidence and explanation
```

When adding ownership knowledge:

- Prefer new fact identifiers, catalogs, or decision rules over product-specific detectors.
- Define evidence strength explicitly.
- Distinguish provenance from active reconciliation.
- Distinguish direct from inherited attribution.
- Preserve conflicting claims.
- Do not treat absence of evidence as confirmed lack of authority.
- Ensure authority records are not accidentally treated as ordinary unmanaged resources.

A Terraform attribution label, for example, may establish provenance without establishing continuous reconciliation.

## Shape Contributions

A `ShapeDefinition` describes a recurring Kubernetes structure. It should not describe one installation merely by copying its names.

A useful shape definition should:

- Use stable structural evidence.
- Define clear roots.
- Assign meaningful component aliases.
- Use required relationships only when evidence supports them.
- Define realistic cardinality.
- Account for optional and variant resources.
- Avoid product-specific names unless the shape is intentionally product-specific.
- Include positive and negative fixtures.

The candidate workflow can accelerate definition development:

```bash
bin/kos candidates
bin/kos candidates explain --first
bin/kos candidates generate --first
bin/kos candidates test --first
```

Before contributing a generated definition:

1. Replace generated review placeholders.
2. Assign a meaningful display name and role.
3. Review every root and component.
4. Review every required relationship.
5. Test against intended instances.
6. Test against structurally similar non-instances.
7. Document remaining knowledge gaps.
8. Confirm that it does not conflict with higher-priority shapes.

Generated definitions are drafts, not accepted truth.

## Release-Manager Contributions

A release-manager integration should map manager-specific concepts into the normalized release model without forcing all managers into Helm semantics.

Document how the manager represents:

- Release identity.
- Revision or generation.
- Source.
- Desired version.
- Destination.
- Current status.
- History.
- Managed-resource inventory.
- Lifecycle authority.
- Reconciliation behavior.

Preserve manager-specific details when no meaningful normalized equivalent exists.

Release-manager tests should include unmanaged resources, partial metadata, historical records, cluster-scoped resources, and conflicting authority evidence.

## Janitor Contributions

Janitor changes require additional scrutiny because future phases may mutate cluster resources.

The core model is:

```text
Rule
  → Finding
  → Actionability decision
  → Action boundary
  → Graph closure and ordering
  → Immutable execution plan
  → Authorized approval
  → Pre-execution revalidation
  → Idempotent execution
  → Post-execution verification
```

Janitor contributions must preserve these invariants:

- Uncertainty blocks mutation.
- Active reconcilers protect managed resources.
- Unknown relationship semantics block destructive action.
- Persistent data requires explicit disposition.
- Shared resources remain protected unless every consumer is accounted for.
- Approval applies to an immutable plan digest.
- Material state changes invalidate approval.
- Execution must be idempotent and resumable.
- Partial execution must remain visible.
- Post-execution verification is mandatory.
- Provisional knowledge cannot authorize mutation.
- Degraded subsystems block mutation but remain visible.

Do not weaken a safety invariant merely to increase finding actionability or test coverage.

Neutralization strategies must be kind-specific and reversible. Unknown kinds must remain report-only.

## Documentation Contributions

Documentation changes are welcome.

When documenting behavior:

- Describe current behavior separately from planned behavior.
- Avoid presenting alpha capabilities as production-ready.
- Include concrete examples.
- Use operator terminology.
- Explain evidence and uncertainty.
- Keep commands synchronized with current CLI help.
- Prefer relative links within the repository.
- Update related specifications when behavior changes.

## AI-Assisted Contributions

AI-assisted development is welcome.

The human contributor remains responsible for:

- Correctness.
- Security.
- Licensing.
- Test coverage.
- Architectural consistency.
- Reviewing every generated change.
- Confirming that generated code does not contain incompatible copied material.

Do not submit generated code, definitions, tests, or documentation that you cannot explain and maintain.

AI-generated domain conclusions should be treated as proposals until validated against deterministic evidence.

## Pull Requests

Keep pull requests focused and reviewable.

A good pull request includes:

- A clear problem statement.
- The intended behavior.
- Relevant architectural context.
- Tests.
- Documentation updates.
- Example CLI output when output changes.
- Compatibility or migration notes.
- Security implications.
- Known limitations.

### Pull Request Checklist

Before submitting, confirm:

- [ ] The change has a focused scope.
- [ ] The project builds successfully.
- [ ] Unit tests pass.
- [ ] Relevant integration tests pass.
- [ ] New behavior includes tests.
- [ ] CLI changes follow Kubernetes conventions.
- [ ] Evidence remains explainable.
- [ ] Unknown or conflicting states remain visible.
- [ ] Janitor safety constraints are not weakened.
- [ ] Documentation is updated.
- [ ] No credentials, Secret values, or private cluster information are included.
- [ ] Commits include a DCO sign-off.

## Commit Sign-Off

This project uses the [Developer Certificate of Origin](https://developercertificate.org/) to document contributor provenance.

Sign each commit using:

```bash
git commit -s -m "Describe the change"
```

This adds:

```text
Signed-off-by: Your Name <your-email@example.com>
```

The sign-off certifies that you have the right to submit the contribution under the project license.

If necessary, sign an existing commit with:

```bash
git commit --amend --signoff
```

Do not use an email address belonging to another person.

## Licensing

Kube Open Shape is licensed under the Apache License 2.0.

By submitting a contribution, you agree that your contribution is licensed under the same terms and that you have the right to provide it.

No separate Contributor License Agreement is currently required.

## Security Reports

Do not report suspected vulnerabilities through public issues or pull requests.

Follow the instructions in [SECURITY.md](SECURITY.md) and use GitHub Private Vulnerability Reporting when available.

## Community Expectations

Be respectful, constructive, and specific.

Technical disagreement is welcome. Personal attacks, harassment, discrimination, and intentionally disruptive behavior are not.

Assume good intent while requiring evidence for technical conclusions.

The project may add a formal Code of Conduct as the contributor community grows.
