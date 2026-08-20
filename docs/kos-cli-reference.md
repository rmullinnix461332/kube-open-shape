# kos — Command Line Reference

Cluster knowledge CLI. Connects to the current kubeconfig context, discovers resources, builds a knowledge graph, and reports structural analysis.

## Global Flags

Available on all commands (kubectl-compatible):

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Filter by namespace |
| `--all-namespaces` | `-A` | Show resources across all namespaces |
| `--output` | `-o` | Output format: `json`, `yaml`, `wide` |

---

## kos resources

List observed resources.

```
kos resources [kind]
```

Positional arg filters by kind (case-insensitive).

```bash
kos resources                    # all resources
kos resources deployment         # Deployments only
kos resources deployment -n argocd
kos resources -A                 # all namespaces
```

---

## kos ownership

Show resource ownership classifications.

```
kos ownership [classification]
```

Default output is summary counts. Positional arg filters to a specific classification and shows per-resource listing. Use `-o detail` for all resources expanded.

Classifications: PlatformManaged, Managed, Inherited, AdHoc, Unknown, Orphaned, Conflicted

```bash
kos ownership                    # summary counts
kos ownership Unknown            # per-resource listing for Unknown
kos ownership Managed -n argocd  # managed resources in argocd
kos ownership -o detail          # all resources expanded
```

---

## kos relationships

Show the relationship graph.

```
kos relationships [kind] [name]
```

With kind and name, shows edges for a specific resource (requires `-n`). Without arguments, lists all edges (optionally filtered by `-n`).

```bash
kos relationships                          # all edges
kos relationships -n cert-manager          # edges in namespace
kos relationships Deployment payment-api -n payments  # specific resource
```

**Relationship types:**

| Type | Layer | Direction | Source Field |
|------|-------|-----------|--------------|
| UsesServiceAccount | Defining | Workload → ServiceAccount | spec.serviceAccountName |
| SelectsWorkload | Defining | Service → Workload | spec.selector |
| BindsSubject | Defining | RoleBinding → ServiceAccount | spec.subjects[].name |
| GrantsRole | Defining | RoleBinding → Role | spec.roleRef.name |
| ClaimsStorage | Defining | StatefulSet → PVC | spec.volumeClaimTemplates |
| UsesHeadlessService | Defining | StatefulSet → Service | spec.serviceName |
| Mounts | Defining | Workload → ConfigMap | spec.volumes/envFrom |
| References | Defining | Workload → Secret | spec.volumes/envFrom |
| Owns | Framework | Parent → Child | ownerReferences |
| BelongsToRelease | Contextual | Workload → Sibling | helm.sh/release-name |
| MemberOf | Contextual | Resource → LogicalGroup | app.kubernetes.io labels |
| MemberOfRelease | Contextual | Resource → ReleaseGroup | Helm release metadata |

**Evidence confidence:** ExplicitField, SelectorMatch, LabelAssociation, NamingConvention

---

## kos reachable

Show all resources reachable from a root via BFS graph traversal.

```
kos reachable <kind> <name> -n <namespace>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--depth` | 5 | Maximum traversal depth |

```bash
kos reachable Deployment argocd-server -n argocd --depth 3
```

---

## kos shapes

Show the cluster shape inventory.

```
kos shapes [role]
```

Positional arg filters by role. Output separates Role Classifications from Named Shapes.

```bash
kos shapes                       # all shapes
kos shapes node-system           # filter by role
kos shapes -o json
```

---

## kos candidates

List candidate shape groups from structurally unnamed resources.

```
kos candidates
```

```bash
kos candidates                   # table (default)
kos candidates -o json
kos candidates -o name           # IDs only
```

---

## kos candidates explain

Show detailed composition of a candidate group.

```
kos candidates explain [candidate-id]
```

| Flag | Description |
|------|-------------|
| `--first` | Explain the highest-ranked candidate |

---

## kos candidates generate

Generate a draft ShapeDefinition YAML. YAML to stdout, guidance to stderr.

```
kos candidates generate [candidate-id]
```

| Flag | Description |
|------|-------------|
| `--first` | Generate from the highest-ranked candidate |

```bash
kos candidates generate --first > draft-shape.yaml
```

---

## kos candidates test

Compile generated definition and match against all eligible roots using the real matcher.

```
kos candidates test [candidate-id]
```

| Flag | Description |
|------|-------------|
| `--first` | Test the highest-ranked candidate |

Output shows target validation, classification impact (accepted/rejected with explanations), and knowledge quality.

---

## kos groups

List logical application groups.

```
kos groups [group-name]
```

Positional arg filters to a specific group. Default shows Application type only.

| Flag | Description |
|------|-------------|
| `--type` | Filter by group type (Application, Release, System) |

```bash
kos groups                       # all application groups
kos groups argocd                # just the argocd row
kos groups -o json
kos groups -n observability      # filter by home namespace
```

---

## kos releases

List Helm releases.

```
kos releases [release-name]
```

Positional arg filters to a specific release.

```bash
kos releases                     # all releases
kos releases argocd              # just the argocd row
kos releases -o json
kos releases -n observability
```

---

## kos findings

List active janitor findings.

```
kos findings
```

| Flag | Description |
|------|-------------|
| `--rule` | Filter by rule name |
| `--severity` | Filter by severity (Info, Warning, Critical) |

```bash
kos findings
kos findings --rule unmanaged-resources
kos findings --severity Critical
```

---

## kos rules

List configured janitor rules with finding counts.

```
kos rules
```

Default rules: unmanaged-resources (Warning, 7d grace), adhoc-resources (Info, 14d grace), orphaned-resources (Critical, no grace).

---

## kos describe

Show detailed human-readable description of a resource type. Equivalent to `kubectl describe`.

```
kos describe <type> [name]
```

Types: `groups`, `releases`, `shapes`, `ownership`

```bash
kos describe groups argocd           # component hierarchy for argocd
kos describe groups                  # all application groups expanded
kos describe releases argocd         # release resource listing
kos describe shapes application      # shape instances for role
kos describe ownership Managed       # per-resource detail for classification
kos describe ownership Managed -n argocd
```

---

## kos report

Generate a cluster knowledge report.

```
kos report
```

```bash
kos report                       # text summary
kos report -o json               # machine-readable
```

---

## kos graph export

Export the full knowledge graph as a JSON snapshot.

```
kos graph export
```

| Flag | Description |
|------|-------------|
| `--cluster-id` | Cluster identifier (default: kubeconfig context) |
| `--include-candidates` | Include candidate discovery state |

```bash
kos graph export --cluster-id my-cluster > graph.json
```

---

## Resource Key Format

```
Kind/Namespace/Name       (namespaced)
Kind/Name                 (cluster-scoped)
```

---

## Composition Roles

| Role | Edge Types | Meaning |
|------|-----------|---------|
| Defining | Mounts, SelectsWorkload, UsesServiceAccount, BindsSubject, GrantsRole, ClaimsStorage, UsesHeadlessService, References | Structural composition |
| Framework | Owns | Generated controller machinery |
| Contextual | BelongsToRelease, MemberOf, MemberOfRelease | Boundary/provenance |
| Taxonomic | ClassifiedAs, ConformsTo | Role/shape classification |

---

## Workflow

```bash
kos resources deployment -n payments
kos ownership
kos relationships Deployment payment-api -n payments
kos reachable Deployment payment-api -n payments
kos shapes
kos candidates
kos candidates test --first
kos describe groups argocd
kos describe releases argocd
kos groups
kos releases
kos findings
kos report
kos graph export > graph.json
```
