# Janitor Safety Model — Specification

## Purpose

The janitor evaluates cluster resources against rules and produces findings. When findings persist beyond grace periods, the janitor constructs execution plans and presents them for operator approval. The safety model defines what actions are permissible, under what conditions, and what constraints prevent destructive behavior.

The central requirement: **the janitor must never cause operational harm through incorrect, premature, or unauthorized action.**

The conceptual model:

> Rules qualify a subject. The graph defines the action boundary and execution order. Safety constraints cap the action. Operators approve an immutable plan. Janitor executes and verifies that exact plan.

---

## Execution Flow

```
Finding
  → Proposed Action
  → Graph-Derived Execution Plan
  → Safety Validation
  → Approval of Exact Plan (immutable digest)
  → Pre-Execution Revalidation (digest must still match)
  → Execution
  → Post-Execution Verification
```

### Execution Plan Contents

An execution plan is an immutable document containing:

| Field | Purpose |
|-------|---------|
| Action boundary | Resource, shape instance, group, or release |
| Exact resources and UIDs | Prevents acting on the wrong object |
| Deletion/neutralization order | Dependency-aware DAG |
| Shared resources excluded | With explicit disposition |
| Persistent resources | With explicit retention/deletion disposition |
| Active authority constraints | Why the plan is safe |
| Expected Kubernetes cascading behavior | What the controller will do |
| Relationship model version | How edges were interpreted |
| Graph snapshot digest | Exact relevant nodes and edges approved |
| Observation timestamp | When the graph was observed |
| Rule version | Rule definition that qualified the subject |
| Configuration version | Janitor settings at plan time |
| Plan digest | Cryptographic hash of the complete plan |

Approval applies to the **plan digest**. Any material state change invalidates the approval.

---

## Finding Model

### Separate Lifecycle from Actionability

Findings carry two orthogonal dimensions:

```yaml
finding:
  status: Active | Proposed | Approved | Executing | Executed | Failed | Suppressed | Resolved
  actionability: Actionable | Protected | Indeterminate
```

| Dimension | Values | Meaning |
|-----------|--------|---------|
| **Status** (lifecycle) | Active, Proposed, Approved, Executing, Executed, Failed, Suppressed, Resolved | Where is this finding in the action lifecycle? |
| **Actionability** (safety) | Actionable, Protected, Indeterminate | Is it safe to act on this finding? |

Examples:

```
Status: Active
Actionability: Protected
Reason: Active Argo reconciliation (Continuous + Active)

Status: Proposed
Actionability: Actionable
Plan: sha256:abc123

Status: Active
Actionability: Indeterminate
Reason: Authority walk failed; cannot verify safety
```

---

## Action Capabilities

Actions are capabilities, not a mandatory sequential workflow. A rule requests an action, which is constrained down to an effective action:

```
Rule Requested Action
  constrained by Rule MaxAction
  constrained by Global MaxAction
  constrained by Safety Caps
  constrained by Classification
  constrained by Authority
  = Effective Action
```

| Action | Reversibility | Impact | Description |
|--------|--------------|--------|-------------|
| Observe | None | None | Record the finding; take no action |
| Report | None | None | Surface the finding to operators via API/CLI |
| Annotate | Reversible | Low | Apply a KOS annotation to mark for review |
| Neutralize | Partially reversible | Medium | Scale to zero, suspend, or disable |
| Delete | Irreversible | High | Remove the resource from the cluster |

### Annotation, Not Label

Labels are NOT universally low-risk. A label can change behavior if it participates in Service selectors, NetworkPolicy, admission policy, operator reconciliation, or scheduling. The janitor uses a reserved KOS annotation:

```
knowledge.kos.io/finding: disconnected-configmap
```

Even this annotation is treated as a mutation requiring safety validation.

### Metadata Domain

All KOS annotations use the `knowledge.kos.io/` domain consistently.

---

## Neutralization Safety

Neutralize requires explicit specification per resource kind:

| Requirement | Description |
|-------------|-------------|
| Strategies registered by kind | Only known kinds can be neutralized |
| Unknown kinds | Cannot be neutralized; Report only |
| Original state persisted | In the execution plan (for restoration) |
| Idempotent | Repeated neutralization produces the same result |
| Restoration plan defined | How to reverse the neutralization |
| Pre-neutralization state revalidated | Before execution |
| Partial neutralization | Produces Failed, not Executed |
| Active reconciler | Blocks neutralization |
| Storage and configuration | Not modified unless strategy explicitly requires |

Neutralization strategies:

| Kind | Strategy | Persisted State |
|------|----------|-----------------|
| Deployment | `spec.replicas=0` | Previous replica count |
| StatefulSet | `spec.replicas=0` | Previous replica count |
| CronJob | `spec.suspend=true` | Previous suspend value |
| Unknown CR | Unsupported | Report only |

---

## Dependency-Aware Ordering

The graph constructs an execution DAG using **teardown-relevant relationships only**.

### Relationships with teardown semantics

| Relationship | Ordering | Rationale |
|-------------|----------|-----------|
| Consumer → Provider (Mounts, References) | Delete consumer first | Provider removal breaks consumers |
| Ingress → Service (SelectsWorkload) | Delete Ingress first | Service removal orphans Ingress |
| Workload → ServiceAccount | Delete workload first | SA removal breaks workload |
| Custom Resource → CRD | Delete CRs first | CRD removal cascades to all CRs |
| RoleBinding → Role | Delete binding first | Role removal breaks binding |

### Relationships excluded from teardown DAG

| Relationship | Reason |
|-------------|--------|
| Reconciles (Application → Group) | Authority relationship; not consumer/provider |
| Generates (ApplicationSet → Application) | Authority relationship |
| Provisions | Provenance; not teardown-relevant |
| BelongsToRelease | Provenance boundary |
| MemberOf | Grouping, not dependency |

Authority relationships mean: **Protected, not teardown-ordered.** An active Reconciles edge produces `Actionability: Protected`. It never enters the deletion DAG.

### Ordering requirements

The janitor must:
- Delete consumers before providers
- Detect graph cycles (block if found)
- Block when ordering is indeterminate
- Account for Kubernetes cascading deletion (ownerReferences)
- Recompute after each material execution phase
- Verify residue when finished

---

## Generic Reconciliation Model

Safety rules use generic authority properties, not manager-specific logic:

```yaml
Authority:
  reconciliationMode: Continuous | None | Unknown
  state: Active | Inactive | Unknown
  evidence: [...]
```

| Mode | State | Actionability |
|------|-------|---------------|
| Continuous | Active | Protected |
| Continuous | Inactive | Indeterminate |
| Continuous | Unknown | Indeterminate |
| None | — | Actionable (safety qualification continues) |
| Unknown | — | Indeterminate |

Sources of reconciliation evidence:
- ArgoCD auto-sync → `Continuous + Active`
- Active operator watching a CR → `Continuous + Active`
- ApplicationSet generating Applications → `Continuous + Active`
- Terraform (no continuous watch) → `None`
- Helm metadata (provenance only) → `None`
- Unknown authority → `Unknown`

---

## Plan Invalidation

### Invalidation triggers

Approval is invalidated if ANY action-relevant state changes:

| State | Trigger |
|-------|---------|
| Resource UID | Different object |
| Resource generation | Spec changed |
| Action-relevant spec fingerprint | Material spec mutation |
| Relevant labels/annotations | May affect behavior |
| Ownership and authority fingerprint | Authority changed |
| Group/shape membership | Structural context changed |
| Dependency-graph snapshot digest | Relationships changed |
| Classification fingerprint | Operational classification changed |
| Approval TTL exceeded | Time expired |

`resourceVersion` is used as an optimistic execution precondition but does NOT alone invalidate a plan (status-only changes are common and immaterial).

### Pre-execution check

```
if currentPlanDigest != approvedPlanDigest:
    invalidate approval
    finding status → Active
    re-evaluation may create a new plan
```

### Approval expiration

```
Approval expires
  → plan becomes Expired
  → finding remains Active
  → re-evaluation may create a new plan
```

An old proposal does NOT remain indefinitely approvable.

---

## Approval Record

```yaml
approval:
  actor: operator@example.com
  authorizationSource: RBAC/kubectl
  timestamp: 2026-08-20T14:30:00Z
  decision: Approved | Rejected
  reason: "Confirmed disconnected after 30 days"
  planDigest: sha256:abc123...
  expiration: 2026-08-21T14:30:00Z
```

The system must verify that the actor is authorized for the action boundary and scope, particularly for cluster-scoped actions.

---

## Invalid Configuration Handling

| Scenario | Behavior |
|----------|----------|
| Invalid configuration update | Reject transaction; continue with last-known-good; report configuration error |
| Invalid initial configuration | Start edge in degraded observe-only mode; block all mutation; report configuration error |

The edge itself does not become unavailable because janitor configuration is invalid.

---

## Fail-Closed Semantics

Uncertainty produces inaction, NOT invisibility:

| Condition | Findings | Actions |
|-----------|----------|---------|
| Ownership engine fails | Existing findings remain visible; new ownership findings → Indeterminate | All mutation blocked |
| Graph unavailable | Non-graph rules may still produce findings; graph findings → Indeterminate | All Neutralize/Delete blocked |
| Safety walk cannot reach authority | Findings visible | Actionability: Indeterminate |
| Unaccounted hard dependency | Findings visible | Block destructive action |
| Shared consumer outside action closure | Findings visible | Block destructive action |
| Unknown relationship semantics | Findings visible | Block destructive action |
| Approval timeout | Findings remain Active | Plan expires; no execution |
| Conflicting rules | Findings visible | More restrictive cap wins |
| Invalid configuration | Edge starts/continues in observe-only | Last-known-good active |

---

## Operational Classification Protection

Accepted operational classifications may restrict actionability:

| Classification | Restriction |
|---------------|-------------|
| Data: Confidential | Delete prohibited |
| Environment: Production | Neutralize/Delete require stronger approval |
| Persistence: Durable | Storage excluded from plans |
| Criticality: Tier 0 | Report only |
| Disposal: Ephemeral | Removes a classification restriction; does NOT independently authorize deletion |

> Ephemeral may remove a classification restriction, but never independently authorizes deletion. Qualification still requires an explicit rule and complete safety validation.

Accepted classifications can **reduce** actionability. Provisional classifications (candidate affinity, working classification) can NEVER **authorize** action.

---

## Namespace Protection

Protected namespaces produce findings but block mutation:

| Namespace | Evaluation | Reporting | Mutation |
|-----------|-----------|-----------|----------|
| kube-system | Allowed | Allowed | Blocked by default |
| kube-public | Allowed | Allowed | Blocked by default |
| kube-node-lease | Allowed | Allowed | Blocked by default |
| `knowledge.kos.io/protected=true` | Allowed | Allowed | Blocked |
| All others | Allowed | Allowed | Subject to safety model |

Namespace protection is configurable. System namespaces are defaults that can be extended.

---

## Authority Record Scoping

Authority records are excluded from resource-cleanup evaluators unless a rule explicitly targets authority-record lifecycle.

A future release-residue rule may legitimately report:
- Helm authority record exists
- Release has no managed resources
- Last revision is superseded or uninstalled

---

## Shape Instance Protection

A resource belonging to a matched shape instance cannot be destructively evaluated in isolation. The complete instance must be evaluated, after which individual members may be retained or included based on their role and relationships.

This applies to ALL shape members, not only required aliases.

---

## Phase 4 Deletion Qualification

A valid deletion plan requires:

| Requirement | Meaning |
|-------------|---------|
| No unaccounted hard dependents | All consumers identified and included or retained |
| No consumers outside action closure | Every dependent is addressed |
| No partial deletion of shape instance | Complete instance evaluated |
| No shared resources without disposition | Explicit retain/delete for each |
| No persistent data without disposition | Storage explicitly handled |
| No unknown relationship semantics | All edges must have defined teardown behavior |

---

## Invariants

1. Approval applies to an immutable execution-plan digest.
2. Any material state change invalidates approval.
3. No destructive action occurs without a complete dependency plan.
4. Unknown relationship semantics block destructive action.
5. Persistent data requires explicit disposition.
6. Shared resources remain outside the action closure unless all consumers are included.
7. Subsystem degradation blocks mutation but remains visible.
8. Every execution is idempotent and safely resumable after restart.
9. Partial execution is recorded and requires reconciliation before retry.
10. Post-execution graph verification is mandatory.
11. Accepted operational classifications may restrict action; provisional classifications never authorize it.
12. Invalid distributed configuration is rejected transactionally; last-known-good remains active.
13. The janitor must never act on a resource with Actionability: Protected or Indeterminate.
14. The janitor must never act based solely on candidate affinity or working classification.
15. The janitor must never act on a shape instance member without evaluating the complete instance.
16. Every action must be traceable: rule → finding → plan → evidence → approval → execution → verification.
17. Authority relationships (Reconciles, Generates) never enter the teardown DAG.
18. Neutralization requires a registered strategy for the resource kind.

---

## Implementation Phases

### Phase 1 (Current): Observe-Only
- Rules evaluate and produce findings
- Findings carry status (Active/Resolved) and actionability (Actionable/Protected/Indeterminate)
- No escalation beyond observation
- Safety walk classifies findings
- Grace periods tracked but not acted upon
- Subsystem failures → Indeterminate, not silent

### Phase 2: Annotate Actions
- Findings that expire grace → propose Annotate action
- KOS annotation (`knowledge.kos.io/finding`) is the only mutation
- Plan contains: resource key, UID, annotation key/value
- Operator may approve or suppress
- Approval record with actor, timestamp, expiration

### Phase 3: Execution Plans + Neutralize
- Graph-derived execution plans with dependency DAG
- Registered neutralization strategies per kind
- Immutable plan digests for approval
- Pre-execution revalidation
- Operator approval required
- Post-execution verification
- Original state persisted for restoration
- Partial execution → Failed status

### Phase 4: Conditional Delete
- Complete action-closure evaluation
- No unaccounted hard dependents
- No partial shape instance deletion
- No shared resources without disposition
- No persistent data without disposition
- Full dependency ordering
- Operator approval always required
- Audit trail: rule → finding → plan → approval → execution → verification
