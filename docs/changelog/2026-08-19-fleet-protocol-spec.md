# Fleet Protocol Specification (High-Level)

Status: Draft — high-level only. Message shapes, wire encoding, transport, and
security are intentionally deferred to a later revision.

## Purpose

Define the open protocol between a KOS **edge** (per-cluster knowledge agent) and
a **center** (fleet aggregator). The protocol lets many autonomous edges report
knowledge upward and receive policy downward, without the center ever driving
cluster mutations directly.

## Design Principles

1. **Edge-initiated.** The edge opens all connections. The center never dials the
   edge. This keeps edges deployable behind NAT/firewalls with outbound-only access.
2. **Edge autonomy.** An edge is fully functional with no center configured. The
   center is an optional aggregation and policy-distribution layer, not a controller.
3. **Knowledge up, policy down.** Edges send observed knowledge (summaries,
   findings, action records). The center sends policy (what to look for, what is
   permitted). The center does not send imperative commands to act on a resource.
4. **Idempotent and resumable.** Every exchange tolerates retry, duplication, and
   center downtime. The edge spools locally and replays on reconnect.
5. **Digest-addressed policy.** Policy is content-addressed so an edge can verify
   exactly what it activated and roll back to last-known-good.

## Participants

| Participant | Role |
|-------------|------|
| Edge | Observes one cluster, builds the knowledge graph, evaluates rules, initiates all communication |
| Center | Aggregates knowledge across many edges, distributes policy bundles, holds no cluster credentials |

## Communication Directions

```
Edge  ──(knowledge)──▶  Center     heartbeats, summaries, findings, action records
Edge  ◀──(policy)────   Center     policy bundles pulled by digest, controller messages
```

## Event Types

The protocol has two upward channels (edge → center) and two downward channels
(center → edge). Each carries a small number of event types.

### Upward: Telemetry (edge → center)

| Event | Purpose | Key information (high level) |
|-------|---------|------------------------------|
| **Heartbeat** | Liveness and current posture | Edge identity, cluster identity, agent version, timestamp, active policy revision, spool depth, subsystem health |
| **OwnershipSummary** | Fleet **posture**: lifecycle authorities | Counts by authority type, managed vs. no-known-authority vs. contended totals, number of distinct authorities |
| **ShapeSummary** | Fleet **posture**: structural composition | Named shape instance counts, role classifier counts, candidate group counts |
| **ApplicationSummary** | Fleet **inventory**: one installed application instance | Product + instance identity, namespace, group reference, versions, deployment source/revision, lifecycle authority + reconciliation mode, structural profile, normalized material traits, findings summary, observed timestamp |
| **FindingEvent** | Janitor findings surfaced upward | Rule identity, resource reference, severity, status, actionability, grace state — never raw resource contents |
| **ActionRecordEvent** | Lifecycle record of a janitor plan | Plan digest, action type, target reference, lifecycle state (proposed → approved → executed/failed), approval identity, result |

`ActionRecordEvent` reports the lifecycle of a proposed janitor plan as it moves
through approval and execution. `proposed` is a lifecycle state, not an action
outcome — the event gives the center visibility into plans that were proposed,
approved, executed, or failed, without the center participating in the decision.

Telemetry is a projection of already-accepted edge knowledge. It carries
summaries and references, not full resource manifests or secret data.

## Posture vs. Inventory

The upward events split into two kinds of fleet knowledge:

- **Posture** (`OwnershipSummary`, `ShapeSummary`) — aggregate *counts* for one
  cluster. Answers "how much" and "what mix," not "what is installed."
- **Inventory** (`ApplicationSummary`) — an installation-level projection, one
  event per installed application instance. Answers "what is actually running,
  which versions, and how do installations differ."

Posture alone lets the fleet know counts; it cannot answer "where is Jenkins
installed and which versions are running." `ApplicationSummary` is the missing
installation-level knowledge projection that makes genuine fleet inventory and
comparison possible.

### ApplicationSummary

One event per installed application instance. High-level content:

| Field group | Key information |
|-------------|-----------------|
| Identity | Edge/cluster identity, product identity (e.g. Jenkins), instance identity, namespace, group reference |
| Versions | Application version, image reference, package/chart version |
| Deployment | Manager (Helm/ArgoCD/…), source, revision, reconciliation mode |
| Lifecycle | Lifecycle authority, reconciliation mode |
| Structure | Structural profile: matched shapes, replica/topology model, external exposure, persistence model |
| Traits | Normalized material-traits profile + trait fingerprint |
| Findings | Findings summary for the instance |
| Timestamp | Observed timestamp |

A conceptual fleet record:

```
product: jenkins
instance: jenkins-prod
cluster: prod-east
namespace: cicd

versions:
  application: 2.479.3
  chart: 5.7.12
  image: jenkins/jenkins:2.479.3-lts

deployment:
  manager: ArgoCD
  source: platform/jenkins
  revision: a813bc2
  reconciliation: Continuous

structure:
  shapes: [StatefulApplication]
  replicas: 1
  exposure: Ingress
  persistence: PersistentVolume
  database: Embedded

traits:
  fingerprint: sha256:...
  authentication: OIDC
  configurationProfile: managed
```

### Why normalized traits, not just a fingerprint

A fingerprint tells the fleet that two installations *differ*, not *why*. To
answer real comparison questions the edge sends a **restricted set of
policy-approved, normalized traits** alongside the fingerprint:

| Fleet question | Required data |
|----------------|---------------|
| Where is Jenkins installed? | Product identity, cluster, namespace, instance |
| Which versions are running? | Application, image, chart/source revision |
| Which installations differ? | Normalized structural and material traits |
| Exactly how do they differ? | Comparable trait values, not merely a fingerprint |

Useful material differences (illustrative, for Jenkins): application/image
version, chart or deployment revision, deployment authority and reconciliation
mode, external exposure, authentication mode, persistence model, replica/topology
model, resource profile, relevant policy findings.

Which traits are collected and reported is governed by policy — the center
defines the approved trait set; the edge does not export arbitrary detail.

### Visibility limit

`ApplicationSummary` reports what KOS can observe from Kubernetes: workloads,
versions, deployment provenance, structure, and Kubernetes-level configuration
shape. It **cannot** report application-internal state — for Jenkins that means
plugins, internal security configuration, or Jenkins-managed agents are not
visible unless that knowledge is exposed through a Kubernetes API. The event
describes the Kubernetes-observable installation, not the application's internal
inventory.

KOS can be *taught* to extract more application-specific detail declaratively,
when that detail is exposed through a CRD, via the Application Detail Extension
model (see `2026-08-19-application-detail-extension-spec.md`). KOS does not run
application-specific code to interrogate applications directly.

### Inventory boundaries preserved

`ApplicationSummary` stays within the same information boundary as the other
telemetry: identities, versions, provenance, normalized traits, and summaries
cross the wire. Raw ConfigMaps, environment values, manifests, and Secret
contents remain on the edge.

### Downward: Policy and Control (center → edge)

| Event | Purpose | Key information (high level) |
|-------|---------|------------------------------|
| **PolicyBundle** | The rules and posture an edge should apply | Bundle digest, ResourceOwner definitions, JanitorRules, posture/action caps, schema version |
| **ControllerMessage** | Out-of-band protocol-behavior request | Message kind (request re-evaluation, refresh summaries, request full snapshot, adjust reporting interval), correlation id |

**Policy is advertised, not pushed.** The center advertises the *desired policy
digest*. The edge initiates retrieval of the corresponding bundle. "Policy down"
describes the direction of information, not a center-initiated connection — the
edge still opens every connection and decides when to pull.

**ControllerMessage boundary.** Controller messages may alter protocol behavior —
such as requesting re-evaluation, refreshing summaries, requesting a full
telemetry snapshot, or changing reporting intervals — but **cannot identify a
Kubernetes resource for mutation or prescribe a janitor action**. There is no
`ControllerMessage{ delete: resource X }`. Actionability and execution remain the
edge's responsibility, gated by the edge's own safety model and operator approval.

**Full snapshot semantics.** A "full snapshot" is a full *telemetry* snapshot: it
replaces the center's prior summaries for that edge. It does **not** include raw
manifests, configuration bodies, Secret values, or the edge's internal knowledge
graph. "Request full snapshot" means "re-send your complete summary set," never
"upload the cluster."

### Control-flow envelope (all channels)

| Element | Purpose |
|---------|---------|
| **Message** | Common envelope: protocol version, edge identity, edge incarnation, cluster identity, event type, sequence, event id, timestamp |
| **Acknowledgement** | Center confirms receipt of upward events so the edge can advance its spool |

**Edge incarnation.** The envelope carries an *incarnation* identifier generated
when the edge's durable protocol state is initialized (e.g. a fresh install or a
rebuilt database). Deduplication uses **edge identity + incarnation + sequence**,
so a rebuilt edge that restarts sequence numbering at zero is not mistaken for a
replay of the previous incarnation. Each event also carries a stable event id.

## Exchange Lifecycle (high level)

```
1. Connect        Edge announces identity and capabilities; center returns
                  the current expected policy revision.
2. Heartbeat      Edge sends periodic heartbeats with posture and policy revision.
3. Report         Edge batches telemetry events (summaries, findings, actions)
                  and uploads them; center acknowledges.
4. Policy pull    If the center's advertised digest differs from the edge's active
                  revision, the edge pulls the bundle, verifies digest AND issuer
                  authenticity, stages, then activates.
5. Messages       Edge long-polls for controller messages and handles them.
6. Spool/replay   During center downtime the edge spools upward events locally
                  and replays them (in order, de-duplicated) on reconnect.
```

## Key Information Boundaries

What crosses the wire, and what does not:

| Crosses to center | Stays on edge |
|-------------------|---------------|
| Cluster/edge identity, agent version | Cluster credentials (edge holds none for the center) |
| Ownership/shape/finding **summaries and references** | Full resource manifests, spec bodies |
| Application inventory: identity, versions, provenance, **policy-approved normalized traits** | Raw ConfigMaps, env values, Secret contents |
| Plan digests and action outcomes | Application-internal state (e.g. Jenkins plugins) unless a product collector supplies it |
| Subsystem health, policy revision, spool depth | Raw knowledge-graph internals |

Policy flows down as content-addressed bundles; knowledge flows up as summaries
and references. Neither direction transfers raw secret or configuration data.

## Autonomy and Failure Behavior (high level)

- **No center configured:** edge runs fully autonomous; all telemetry is local only.
- **Center unreachable:** edge continues evaluating, spools upward events, and
  keeps applying its last-known-good policy.
- **Invalid policy bundle:** edge rejects the update, keeps last-known-good, and
  reports the configuration error on the next heartbeat.
- **Duplicate/replayed events:** center de-duplicates by edge identity +
  incarnation + sequence.

## Policy Trust Invariant

An edge activates a policy bundle **only** when both hold:

1. The bundle's content digest matches the digest the center advertised, and
2. The bundle's authenticity is verified against an explicitly trusted issuer.

Retrieval success alone never authorizes activation. A digest proves *content
identity*, not *who authorized it* — both must be established before a bundle
becomes active. The specific signing and issuer-trust mechanism is deferred to
the detailed revision, but this invariant is fixed now.

## Explicitly Out of Scope (this draft)

Deferred to a detailed protocol revision:

- Concrete message schemas and field-level definitions
- Wire format and versioning rules (JSON vs. protobuf, version negotiation)
- Transport and security (mTLS, authentication, authorization)
- Batching sizes, heartbeat/report intervals, backoff and retry policy
- Spool storage format and retention
- Multi-tenancy, edge grouping, and center-side aggregation model
- Rate limiting and flow control

## Relationship to Phase 7 Deliverables

This spec frames the event model for the Phase 7 implementation:

- `pkg/protocol/` — the Message envelope and the event types named above
- `internal/edge/comm/` — connect, heartbeat, report, policy pull, messages, spool
- `internal/edge/policy/` — bundle verify/stage/activate/rollback (last-known-good)
- `cmd/mock-center/` — a development center that accepts telemetry and serves bundles
- `kos status` — surfaces heartbeat state, policy revision, and spool depth

Detailed message definitions will be specified before implementation begins.
