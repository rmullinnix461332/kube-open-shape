# Application Detail Extension Model (High-Level)

Status: Draft — high-level only. Field-level schema, normalization rules, and
CRD definitions are deferred to a later revision.

## Purpose

Let operators teach KOS application-specific knowledge **declaratively**, by
extracting allowlisted fields from Kubernetes CRDs, without running any
application-specific code on the edge. This fills the visibility gap named in the
fleet protocol spec: `ApplicationSummary` reports what KOS can observe from
Kubernetes; this extension model is how KOS is taught to observe more — but only
what is already exposed through Kubernetes APIs.

## The Extension Promise

> KOS can be taught application-specific knowledge declaratively when that
> knowledge is exposed through Kubernetes APIs. KOS does not require or assume the
> ability to interrogate applications directly.

That is the entire boundary. Declarative CRD extraction is the extension model.
Executable collectors are not.

## Declarative CRD Extraction

An operator defines an `ApplicationDetailDefinition` that says:

- Which CRD represents an application-specific source
- Which fields are safe and relevant (allowlist)
- How those fields attach to a KOS application/group
- Which values are comparable across the fleet
- Whether values represent **declared** or **observed** state

Conceptual example:

```yaml
apiVersion: kos.io/v1alpha1
kind: ApplicationDetailDefinition
metadata:
  name: jenkins-operator-details
spec:
  source:
    apiVersion: jenkins.io/v1alpha2
    kind: Jenkins

  application:
    product: Jenkins
    association:
      strategy: resource-group

  fields:
    - name: plugins
      path: .spec.master.plugins
      type: list
      state: declared
      comparable: true

    - name: baseConfiguration
      path: .spec.configurationAsCode.configurations
      type: list
      state: declared
      comparable: true

    - name: observedVersion
      path: .status.jenkinsVersion
      type: version
      state: observed
      comparable: true
```

## Extraction Engine (generic)

The engine is product-agnostic. It never contains Jenkins-specific (or any
product-specific) logic — all product knowledge lives in the definition.

```
Observe registered CRD
  → match ApplicationDetailDefinition
  → extract allowlisted fields
  → normalize values
  → attach evidence to application
  → include approved details in the ApplicationSummary fleet projection
```

## Evidence Semantics: Declared vs. Observed

Each field is tagged `declared` or `observed`, and the distinction is preserved
end to end:

- `spec.*` fields are **declared** state → fleet says "declared plugins."
- `status.*` fields are **observed** state → fleet may say "observed version."

KOS must **not** turn declared configuration into an unsupported runtime claim.
If plugins appear under `spec`, the fleet reports "declared plugins," never
"installed plugins," unless the operator maps an `observed` status field.

## What the Definition Controls

At a high level, an `ApplicationDetailDefinition` governs:

| Concern | Purpose |
|---------|---------|
| Source CRD | Which apiVersion/kind is the detail source |
| Application association | How extracted detail attaches to a KOS application/group |
| Field allowlist + path | Exactly which fields are extracted (nothing else) |
| Field type + normalization | How raw values become comparable values |
| Declared vs. observed | Evidence semantics for each field |
| Comparability | Whether a field participates in fleet comparison |
| Sensitivity classification | Whether a field is fleet-visible |
| List ordering + uniqueness | Deterministic normalization of list fields |
| Schema/version compatibility | Which CRD versions the definition applies to |
| Missing-field interpretation | What absence means (unset vs. not-applicable) |
| Maximum size + cardinality | Bounds to prevent unbounded fleet payloads |

## Fleet Questions This Enables

With declared/observed detail attached, the fleet can answer:

```
Jenkins installations
  → versions
  → declared plugin sets
  → plugin-set variants
  → configuration profiles
  → declared-versus-observed differences
```

## Explicitly Out of Scope: Executable Collectors

Code that runs on the edge to interrogate an application directly is a **different
feature** and is **not a priority — it may never be implemented**. Executable
collectors would introduce:

- Application credentials on the edge
- Outbound network access to applications
- Arbitrary code execution
- Collector packaging and lifecycle
- Version compatibility between collector and application
- Failure isolation
- A substantially larger security boundary

That conflicts with the edge's simplicity and trust model. Declarative CRD
extraction provides meaningful extensibility without turning the edge into an
agent/plugin runtime.

## Relationship to the Fleet Protocol

This model is the mechanism behind the "normalized material traits" and the
visibility limit described in the fleet protocol spec:

- Extracted, allowlisted, normalized fields become part of the
  `ApplicationSummary` inventory event.
- Sensitivity classification and comparability are policy-governed — the same
  "policy-approved traits" principle already stated for `ApplicationSummary`.
- The visibility limit still holds: KOS only sees what a CRD exposes. If a product
  does not expose plugins/config through a Kubernetes API, KOS cannot report them
  — and does not attempt to.

## Explicitly Out of Scope (this draft)

Deferred to a detailed revision:

- The `ApplicationDetailDefinition` CRD schema and field definitions
- Path expression syntax and normalization rules per field type
- Association-strategy definitions (beyond naming `resource-group`)
- Sensitivity classification levels and their enforcement
- CRD version-compatibility and migration semantics
- Size/cardinality limit values
