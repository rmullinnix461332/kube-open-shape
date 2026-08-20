# Intelligent Shape Grouping — Design Specification

## Overview

Shape definitions classify known structure. Intelligent grouping organizes **unknown** structure into candidate families so operators are not staring at hundreds of unrelated "unclassified" resources.

The distinction:

| Concept | Source | Purpose |
|---------|--------|---------|
| `ShapeDefinition` | Administrator-approved | Semantic knowledge |
| `CandidateShapeGroup` | System-discovered | Structural similarity |
| `ShapeInstance` | Engine-matched | Concrete resources implementing either |

---

## 1. Terminology

- **CandidateShapeGroup** — system-discovered structural similarity group (stored in SQLite, not CRDs)
- **Resource Family** — reserved for lifecycle groups (Helm revisions, etc.) — not used here
- **Common Core** — resource types and relationships present in 100% of group members
- **Variable Components** — resources present in some but not all members
- **Cohesion** — measure of structural similarity within a group (0.0–1.0)

---

## 2. Discovery Pipeline

```
Unclassified resources
    ↓
Graph segmentation (ownership, ownerRef, references, namespace)
    ↓
Candidate structural roots (Deployments, StatefulSets, etc.)
    ↓
Generic canonicalization (strip names, UIDs, timestamps, images)
    ↓
Exact structural fingerprints
    ↓
Exact-match groups
    ↓
Explainable similarity clustering
    ↓
CandidateShapeGroups
    ↓
Suggested ShapeDefinition skeleton
```

---

## 3. Graph Segmentation

Before comparing shapes, determine which resources plausibly belong together.

**Boundary heuristics (priority order):**

1. Recognized management owner (one Argo Application, Helm release)
2. Kubernetes ownerReference tree
3. Explicit resource references (configMapRef, secretRef, serviceAccountName)
4. Selector/binding relationships
5. Namespace boundary
6. Generic structural-root heuristics

**Candidate root kinds:**

- Deployment, StatefulSet, DaemonSet, CronJob
- Recognized custom-resource root
- Operator/controller Deployment
- Argo or Helm package root
- Cluster-scoped resource with no inbound management edge
- Graph node with no inbound structural edges

**Resource membership:**

- A resource may be a member of one candidate graph
- Or shared by several candidate graphs (referenced, not owned)
- Or unattached (residue)

Shared resources are referenced by multiple groups rather than arbitrarily assigned to one.

---

## 4. Generic Anonymous Fingerprint

Even without a ShapeDefinition, the engine applies a generic canonicalization profile.

### 4.1 Relationship Classification

Graph content is divided into three structural layers:

| Layer | Purpose | Examples | Fingerprint weight |
|-------|---------|----------|-------------------|
| **Framework** | Generated Kubernetes machinery | Deployment→ReplicaSet, StatefulSet→ControllerRevision, CronJob→Job | 0 (excluded from semantic fingerprint) |
| **Defining** | Relationships that distinguish what the deployment *is* | Deployment→ServiceAccount, RoleBinding→ServiceAccount, RoleBinding→ClusterRole, Deployment→ConfigMap, Service→Deployment, Deployment→Lease | 1.0 (drives grouping) |
| **Contextual** | Useful for explanation, not exact fingerprints | Managed by Helm/Argo, hostPath, hostNetwork, privileged, cluster-scoped RBAC, image repo, node tolerations | 0.25 |

Framework relationships (ownerReference chains from controller kinds) are useful for ownership traversal but carry little information for distinguishing shapes.

### 4.2 Framework Relationships (excluded from semantic fingerprint)

```
Deployment → Owns → ReplicaSet
StatefulSet → Owns → ControllerRevision
CronJob → Owns → Job
Job → Owns → Pod
DaemonSet → Owns → ControllerRevision
ReplicaSet → Owns → Pod
```

### 4.3 Dual Fingerprinting

Every candidate maintains two fingerprints:

| Fingerprint | Includes | Use |
|-------------|----------|-----|
| **Mechanical** | All relationships including framework | Graph verification, ownership traversal |
| **Semantic** | Only defining + weighted contextual relationships | Grouping, naming, shape identity |

The semantic fingerprint drives candidate grouping. The mechanical fingerprint is retained for explanation.

### 4.4 Canonicalization Profile

```yaml
canonicalization:
  profile: generic-structural-v1
  relationshipClasses:
    framework:
      fingerprintWeight: 0
      retainForExplanation: true
    defining:
      fingerprintWeight: 1
    contextual:
      fingerprintWeight: 0.25
  retain:
    - apiGroup
    - kind
    - scope
    - relationship.type (defining only)
    - relationship.direction
    - resourceCardinality (defining resources only)
    - selected structural traits
  discard:
    - name
    - namespace
    - uid
    - resourceVersion
    - timestamps
    - generated hashes
    - image tag
    - literal configuration values
    - framework relationship targets (ReplicaSet, ControllerRevision, Pod)
```

**Example anonymous signature:**

```yaml
root:
  kind: Deployment
members:
  Deployment: 1
  ServiceAccount: 1
  ClusterRole: 1
  ClusterRoleBinding: 1
  Service: 1
relationships:
  UsesServiceAccount: 1
  BindsServiceAccount: 1
  GrantsRole: 1
  SelectsWorkload: 1
traits:
  clusterScopedResources: true
  dedicatedServiceAccount: true
  externallyExposed: false
```

Identical signatures form an **exact group** immediately:

```
candidate-f7a21
  Exact instances: 18
  Clusters: 4
  Management owners: Helm 12, Argo CD 6
```

This is deterministic and implemented first.

---

## 5. Similarity Clustering

Exact fingerprints fragment patterns when optional resources differ. The second layer compares structural features.

**Weighted distance model:**

| Feature | Weight |
|---------|--------|
| Root GVK | Required match |
| Relationship topology | 35% |
| Resource-kind composition | 25% |
| Root compatibility | 15% |
| Structural traits | 15% |
| Cardinality similarity | 10% |

**Explainable output:**

```
Candidate group candidate-controller-03
  18 instances share:
    Deployment
    ServiceAccount
    ClusterRole
    ClusterRoleBinding
    UsesServiceAccount
    BindsServiceAccount
    GrantsRole

  Variants:
    15/18 include a Service
    12/18 include a Lease
    3/18 include a ValidatingWebhookConfiguration

  Similarity: 0.91

  Differences:
    Lease is optional across the group
    Webhook configuration forms a three-instance variant
```

---

## 6. Common Core and Variation

For every candidate group, calculate occurrence frequency:

```yaml
commonCore:
  resources:
    Deployment: 1.00
    ServiceAccount: 1.00
    ClusterRole: 1.00
    ClusterRoleBinding: 1.00
  relationships:
    UsesServiceAccount: 1.00
    BindsServiceAccount: 1.00
    GrantsRole: 1.00

variableComponents:
  Service:
    frequency: 0.83
  Lease:
    frequency: 0.67
  ValidatingWebhookConfiguration:
    frequency: 0.17

cardinality:
  ConfigMap:
    min: 1
    median: 2
    max: 4
    mode: 2
```

**Frequency interpretation (configurable thresholds):**

| Frequency | Interpretation |
|-----------|---------------|
| 100% | Required candidate |
| 60–99% | Optional candidate |
| 1–59% | Variant or incidental |
| 0% | Absent |

---

## 7. Candidate Ranking

Not every anonymous shape deserves a definition. Rank candidates by:

| Factor | Description |
|--------|-------------|
| Instance count | More = higher priority |
| Cluster count | Cross-cluster = more meaningful |
| Stability over time | Persistent structure ranks higher |
| Low structural variance | High cohesion = cleaner definition |
| Multiple management owners | Appears across Helm, Argo, etc. |
| Percentage of unclassified explained | Coverage impact |
| Structural distinctiveness | Not just "a Deployment with a ConfigMap" |
| Cluster-scoped resources present | Security/governance significance |
| Growth rate | Growing patterns deserve attention |
| Distance from existing approved shape | Near-matches may be variants |

**Priority score:**

```
support × cross-cluster-spread × cohesion × persistence
```

Expose each component individually, not only the combined score.

---

## 8. Draft Definition Generation

The system generates, but **never automatically activates**, a proposed ShapeDefinition.

**CLI workflow:**

```bash
kos shapes candidates                          # List candidate groups
kos shapes candidate explain candidate-7       # Show composition details
kos shapes candidate generate candidate-7      # Generate draft YAML
kos shapes definition test candidate-7.yaml    # Dry-run against cluster
kos shapes definition apply candidate-7.yaml   # Apply as CRD
```

**Generated YAML includes:**

```yaml
apiVersion: knowledge.kos.io/v1alpha1
kind: ShapeDefinition
metadata:
  generateName: candidate-7-
  annotations:
    knowledge.kos.io/generated-from: candidate-7
spec:
  definitionVersion: 1
  displayName: REVIEW REQUIRED
  role: Unclassified
  priority: 0
  # ... composition derived from common core ...
  provenance:
    observedInstances: 42
    observedClusters: 8
    observationWindow: 30d
    cohesion: 0.96
```

**Administrator workflow:**
1. Review representative instances
2. Name the shape
3. Assign semantic role
4. Adjust required and optional components
5. Choose fingerprint traits
6. Apply the definition
7. Evaluate how much inventory it matches
8. Promote into a community or organizational pack

---

## 9. Validation Before Activation

```bash
kos shapes definition test candidate-controller.yaml
```

Output:
```
Would classify:
  42 current candidate instances
  0 currently approved shape instances
  3 structurally divergent instances

False-positive review:
  2 instances have an unrelated shared ClusterRole
  1 instance spans two management owners

Coverage:
  417 resources
  14.2% of currently unclassified inventory
```

---

## 10. Confidence Categories

| Category | Criteria |
|----------|----------|
| **Exact group** | Identical generic fingerprint |
| **Stable family** | High support, high cohesion, persisted over time |
| **Probable family** | Meaningful similarity but unresolved variation |
| **Singleton** | One observed structure |
| **Residue** | Disconnected or weakly related resources |
| **Conflict** | Resources plausibly belong to multiple families |

Only **Exact group** and **Stable family** should be strong candidates for definition generation.

---

## 11. Storage

Candidate groups are **derived knowledge** stored in SQLite. They are NOT CRDs.

Rationale:
- Candidates change as observations accumulate
- CRD churn would be disruptive to GitOps
- Only promoted definitions become CRDs

SQLite tables:
```sql
CREATE TABLE candidate_groups (
    id TEXT PRIMARY KEY,
    root_kind TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    member_count INTEGER,
    cohesion REAL,
    confidence TEXT,
    common_core_json TEXT,
    variable_components_json TEXT,
    first_observed_at TIMESTAMP,
    last_observed_at TIMESTAMP,
    stable_since TIMESTAMP
);

CREATE TABLE candidate_members (
    group_id TEXT NOT NULL,
    resource_key TEXT NOT NULL,
    FOREIGN KEY (group_id) REFERENCES candidate_groups(id)
);

CREATE TABLE candidate_lineage (
    group_id TEXT NOT NULL,
    operation TEXT NOT NULL,  -- Split, Merge, Evolved
    parent_ids TEXT NOT NULL, -- JSON array
    reason TEXT,
    occurred_at TIMESTAMP
);
```

---

## 12. Lineage Tracking

Candidate group identities evolve:

```
candidate-7 → split into candidate-7a and candidate-7b
candidate-11 + candidate-14 → merged into candidate-22
```

Stable identity based on:
- Canonicalization version
- Core structural signature
- Similarity-model version

Not on sequential database IDs alone.

---

## 13. Edge vs Central

**Edge performs:**
- Local graph segmentation
- Generic fingerprints
- Exact grouping
- Local similarity
- Candidate explanations
- Draft definition generation

This preserves standalone open-source value.

**Central adds (Phase 2+):**
- Cross-cluster candidate grouping
- Longer observation windows
- Shape-family evolution
- Organization-wide prevalence
- Comparison with known fleet shapes
- Optional community/global benchmarking
- Definition distribution and adoption reporting

---

## 14. Expected Product Presentation

The system transforms:

> "We have 386 things the system doesn't understand."

into:

```
Unclassified inventory
  386 resources
  27 candidate graphs

Grouped into:
  4 stable candidate families covering 301 resources
  3 probable families covering 49 resources
  12 singletons covering 21 resources
  8 residue groups covering 15 resources
```

"Four recurring structural patterns explain 78% of the unknown inventory. Here is their common composition, variation, provenance, and a generated starting point for formal definitions."

---

## 15. Implementation Phasing

This feature spans two implementation phases:

### Phase 4b: Exact Grouping (add to existing Phase 4)

- Graph segmentation of unclassified resources
- Generic canonicalization profile (`generic-structural-v1`)
- Exact fingerprint grouping
- `kos shapes candidates` — list exact groups
- `kos shapes candidate explain {id}` — show composition
- SQLite storage for candidate groups
- Integration tests

### Phase 4c: Similarity and Generation

- Weighted similarity model
- Common core / variable component analysis
- Confidence categories (exact, stable, probable, singleton, residue)
- Candidate ranking
- `kos shapes candidate generate {id}` — produce draft YAML
- `kos shapes definition test {file}` — dry-run validation
- Lineage tracking (split, merge, evolved)
- Integration tests
