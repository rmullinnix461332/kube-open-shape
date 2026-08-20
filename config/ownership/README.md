# Ownership Knowledge Packs

KOS loads ownership catalogs and rules from this directory to extend the built-in ownership engine. Files are merged with embedded defaults at startup.

## Directory Structure

```
config/ownership/
  catalogs/       *.yaml files defining named value sets
  rules/          *.yaml files defining ownership decision rules
```

## Catalogs

Catalogs are named sets of values that rules reference. Two types are supported:

### exactSet

Matches when a value is exactly present in the list:

```yaml
catalogs:
  myOperatorCRDs:
    type: exactSet
    version: "1.0"
    values:
      - mycrd.example.io
      - anothercrd.example.io
```

### prefix

Matches when a value starts with the specified prefix:

```yaml
catalogs:
  myOrgNamespaces:
    type: prefix
    value: "team-"
```

Multi-prefix via values list:

```yaml
catalogs:
  internalPrefixes:
    type: prefix
    values:
      - "internal-"
      - "platform-"
```

## Rules

Rules evaluate facts about resources and produce ownership candidates.

### Rule Structure

```yaml
decisions:
  - name: my-rule-name          # unique identifier
    priority: 500               # numeric; higher = evaluated first in tiebreaks
    when:
      all:                      # ALL conditions must match
        - field: <fact-field>
          <operator>: <value>
    result:
      authority:
        type: <authority-type>  # Helm, ArgoCD, KubernetesController, etc.
        name: <name>            # static name OR
        nameFrom: <binding>     # dynamic binding from facts
        namespace: <ns>         # static OR
        namespaceFrom: <binding>
      claimLayer: <layer>       # REQUIRED for authority-producing rules
      evidenceStrength: <str>   # Authoritative, Corroborating, Supporting
      authorityState: <state>   # Verified, Detected, Missing
      attribution: <attr>       # Direct, Inherited
      resourceRole: <role>      # optional: AuthorityRecord, RuntimeArtifact, etc.
```

### Condition Operators

| Operator | Meaning |
|----------|---------|
| `equals: "value"` | Fact value must exactly equal the string |
| `exists: true` | Fact with this field must exist (any value) |
| `inCatalog: "catalogName"` | Fact value must be in the named catalog |
| `matchesCatalog: "catalogName"` | Fact value must match the catalog (prefix match) |

### Fact Fields Available

Standard (emitted by MetadataExtractor):
- `resource.kind`, `resource.name`, `resource.namespace`
- `metadata.label` (with `key` attribute for specific label)
- `metadata.annotation` (with `key` attribute)
- `metadata.managedField` (with `manager`, `operation` attributes)

Parameterized field syntax for conditions:
- `metadata.label["helm.sh/release-name"]` — matches label fact with that key
- `metadata.annotation["argocd.argoproj.io/tracking-id"]`

Derived (from specialized extractors):
- `runtime.ownerChainRoot` — key of the ownerRef chain root
- `runtime.ownerChainDepth` — number of hops
- `release.name`, `release.namespace` — from Helm release Secrets
- `release.metadataRecord` — true if the resource IS a release Secret
- `argocd.trackingClaim.appName`, `argocd.trackingClaim.appNS`
- `lease.resolvedController` — resolved controller for a Lease
- `pvc.statefulSetOwner` — StatefulSet key for VCT-derived PVCs

### Binding References (nameFrom / namespaceFrom)

Bindings resolve authority names dynamically from facts:

```yaml
# Bind from a specific label value
nameFrom: metadata.label["helm.sh/release-name"]

# Bind from a fact attribute
nameFrom: release.name
namespaceFrom: release.namespace

# Bind from extractor output
nameFrom: lease.resolvedController
nameFrom: argocd.trackingClaim.appName
```

### Claim Layers

Every authority-producing rule MUST declare a `claimLayer`:

| Layer | Use For |
|-------|---------|
| `LifecycleAuthority` | The primary declarative owner (Helm release, ArgoCD app, bootstrap) |
| `RuntimeController` | Kubernetes controller that maintains the resource at runtime |
| `HigherLevelReconciler` | Authority that manages another authority object |
| `AuthorityRecord` | The resource IS the authority record (e.g., Helm release Secret) |

Contention is only detected within the same layer. Different layers form a valid chain.

### Evidence Strength

| Strength | Meaning |
|----------|---------|
| `Authoritative` | Definitive proof (release manifest, bootstrapping label) |
| `Corroborating` | Strong indicator but not definitive (name+namespace catalog, chart label) |
| `Supporting` | Weak signal, cannot determine authority alone (managedFields mutation) |

### Example: Custom Operator Pack

```yaml
# config/ownership/catalogs/my-operator.yaml
catalogs:
  myOperatorManagedKinds:
    type: exactSet
    values:
      - MyDatabase
      - MyDatabaseBackup
      - MyDatabaseRestore
```

```yaml
# config/ownership/rules/my-operator.yaml
decisions:
  - name: my-operator-managed-resources
    priority: 850
    when:
      all:
        - field: resource.kind
          inCatalog: myOperatorManagedKinds
    result:
      authority:
        type: Operator
        name: my-database-operator
      claimLayer: LifecycleAuthority
      evidenceStrength: Corroborating
      authorityState: Detected
      attribution: Direct
```

## Validation

Rules are validated at engine startup:
- Duplicate rule names are rejected
- Authority-producing rules without `claimLayer` are rejected
- References to missing catalogs are rejected (fail-closed)
- Duplicate catalog names are rejected

Invalid configuration prevents engine startup rather than silently producing incorrect ownership.

## Priority Guidelines

| Range | Use For |
|-------|---------|
| 1100–1200 | Platform/controller-generated resources (kube-root-ca.crt, default SA) |
| 900–1100 | Helm release records and strong declarative membership |
| 700–900 | Bootstrap labels, ArgoCD tracking, controller SA catalogs |
| 500–700 | Distribution components, Lease attribution, PVC derivation |
| 100–400 | Corroborating/supporting evidence, chart labels, managed-by conventions |

Priority is used ONLY as a tiebreaker when evidence strength is equal. Stronger evidence always wins regardless of priority.
