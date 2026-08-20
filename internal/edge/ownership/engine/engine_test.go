package engine

import (
	"testing"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// --- helpers ---

func makeRecord(kind, ns, name string, uid types.UID, labels, annotations map[string]string) *knowledge.ResourceRecord {
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       uid,
		},
		Labels:      labels,
		Annotations: annotations,
	}
}

func testCatalogs() *CatalogRegistry {
	reg := NewCatalogRegistry()
	reg.Register(&Catalog{Name: "helmReleaseSecretPattern", Type: CatalogPrefix, Value: "sh.helm.release.v1."})
	reg.Register(&Catalog{Name: "kubernetesControllerServiceAccounts", Type: CatalogExactSet, Values: []string{
		"deployment-controller", "replicaset-controller", "coredns",
	}})
	return reg
}

func testRules() []DecisionRule {
	trueVal := true
	return []DecisionRule{
		{
			Name:     "platform-kube-root-ca",
			Priority: 1200,
			When: RuleCondition{All: []FieldCondition{
				{Field: "resource.kind", Equals: "ConfigMap"},
				{Field: "resource.name", Equals: "kube-root-ca.crt"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "KubernetesController", Name: "root-ca-cert-publisher"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrDirect,
			},
		},
		{
			Name:     "platform-default-sa",
			Priority: 1200,
			When: RuleCondition{All: []FieldCondition{
				{Field: "resource.kind", Equals: "ServiceAccount"},
				{Field: "resource.name", Equals: "default"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "KubernetesController", Name: "service-account-controller"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrDirect,
			},
		},
		{
			Name:     "helm-release-record",
			Priority: 1100,
			When: RuleCondition{All: []FieldCondition{
				{Field: "resource.kind", Equals: "Secret"},
				{Field: "resource.name", MatchCatalog: "helmReleaseSecretPattern"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "Helm", NameFrom: "release.name"},
				ClaimLayer:       ClaimAuthorityRecord,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrDirect,
				ResourceRole:     "AuthorityRecord",
			},
		},
		{
			Name:     "kubernetes-bootstrapping-label",
			Priority: 800,
			When: RuleCondition{All: []FieldCondition{
				{Field: "metadata.label", Equals: "rbac-defaults"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "KubernetesBootstrap", Name: "rbac-defaults"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrDirect,
			},
		},
		{
			Name:     "kubernetes-controller-sa",
			Priority: 700,
			When: RuleCondition{All: []FieldCondition{
				{Field: "resource.kind", Equals: "ServiceAccount"},
				{Field: "resource.namespace", Equals: "kube-system"},
				{Field: "resource.name", InCatalog: "kubernetesControllerServiceAccounts"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "KubernetesController", Name: "kube-controller-manager"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthCorroborating,
				AuthorityState:   AuthStateDetected,
				Attribution:      AttrDirect,
			},
		},
		{
			Name:     "managed-field-supporting",
			Priority: 100,
			When: RuleCondition{All: []FieldCondition{
				{Field: "metadata.managedField", Exists: &trueVal},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{},
				EvidenceStrength: StrengthSupporting,
			},
		},
	}
}

// --- Tests ---

func TestEngine_PlatformKubeRootCA(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("ConfigMap", "default", "kube-root-ca.crt", "uid-1", nil, nil))

	eng := mustEngine(t, testCatalogs(), testRules())
	results := eng.EvaluateAll(index)

	r := results["ConfigMap/default/kube-root-ca.crt"]
	require.NotNil(t, r)
	require.NotNil(t, r.LifecycleAuthority)
	assert.Equal(t, "KubernetesController", r.LifecycleAuthority.Authority.Type)
	assert.Equal(t, "root-ca-cert-publisher", r.LifecycleAuthority.Authority.Name)
	assert.Equal(t, StrengthAuthoritative, r.LifecycleAuthority.EvidenceStrength)
	assert.False(t, r.Contended)
}

func TestEngine_PlatformDefaultSA(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("ServiceAccount", "argocd", "default", "uid-2", nil, nil))

	eng := mustEngine(t, testCatalogs(), testRules())
	results := eng.EvaluateAll(index)

	r := results["ServiceAccount/argocd/default"]
	require.NotNil(t, r)
	require.NotNil(t, r.LifecycleAuthority)
	assert.Equal(t, "service-account-controller", r.LifecycleAuthority.Authority.Name)
}

func TestEngine_BootstrapLabel(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("ClusterRole", "", "system:controller:deployment-controller", "uid-3",
		map[string]string{"kubernetes.io/bootstrapping": "rbac-defaults"}, nil))

	eng := mustEngine(t, testCatalogs(), testRules())
	results := eng.EvaluateAll(index)

	r := results["ClusterRole/system:controller:deployment-controller"]
	require.NotNil(t, r)
	require.NotNil(t, r.LifecycleAuthority)
	assert.Equal(t, "KubernetesBootstrap", r.LifecycleAuthority.Authority.Type)
	assert.Equal(t, "rbac-defaults", r.LifecycleAuthority.Authority.Name)
	assert.Equal(t, StrengthAuthoritative, r.LifecycleAuthority.EvidenceStrength)
}

func TestEngine_ControllerSA_Corroborating(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("ServiceAccount", "kube-system", "deployment-controller", "uid-4", nil, nil))

	eng := mustEngine(t, testCatalogs(), testRules())
	results := eng.EvaluateAll(index)

	r := results["ServiceAccount/kube-system/deployment-controller"]
	require.NotNil(t, r)
	require.NotNil(t, r.LifecycleAuthority)
	assert.Equal(t, "KubernetesController", r.LifecycleAuthority.Authority.Type)
	// Evidence is only corroborating
	assert.Equal(t, StrengthCorroborating, r.LifecycleAuthority.EvidenceStrength)
	assert.Equal(t, AuthStateDetected, r.LifecycleAuthority.AuthorityState)
}

func TestEngine_HelmReleaseRecord(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Secret", "argocd", "sh.helm.release.v1.argocd.v1", "uid-5",
		map[string]string{"owner": "helm", "name": "argocd"}, nil))

	eng := mustEngine(t, testCatalogs(), testRules())
	results := eng.EvaluateAll(index)

	r := results["Secret/argocd/sh.helm.release.v1.argocd.v1"]
	require.NotNil(t, r)
	require.NotNil(t, r.AuthorityRecord)
	assert.Equal(t, "Helm", r.AuthorityRecord.Authority.Type)
	assert.Equal(t, "AuthorityRecord", r.AuthorityRecord.ResourceRole)
}

func TestEngine_Contention_SameLayer(t *testing.T) {
	// Two rules both claim LifecycleAuthority with Authoritative strength
	catalogs := testCatalogs()
	rules := []DecisionRule{
		{
			Name:     "helm-release-a",
			Priority: 1000,
			When: RuleCondition{All: []FieldCondition{
				{Field: "metadata.label", Equals: "release-a"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "Helm", Name: "release-a", Namespace: "ns-a"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrDirect,
			},
		},
		{
			Name:     "helm-release-b",
			Priority: 1000,
			When: RuleCondition{All: []FieldCondition{
				{Field: "metadata.label", Equals: "release-b"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "Helm", Name: "release-b", Namespace: "ns-b"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrDirect,
			},
		},
	}

	index := knowledge.NewIndex()
	index.Upsert(makeRecord("ConfigMap", "shared", "contended-resource", "uid-10",
		map[string]string{"helm.sh/release-name": "release-a", "other-release": "release-b"}, nil))

	// Manually build facts that trigger both rules
	store := NewFactStore()
	key := "ConfigMap/shared/contended-resource"
	store.Add(Fact{Subject: key, Field: "metadata.label", Value: "release-a", Attributes: map[string]string{"key": "helm.sh/release-name"}})
	store.Add(Fact{Subject: key, Field: "metadata.label", Value: "release-b", Attributes: map[string]string{"key": "other-release"}})

	candidates := EvaluateAllRules(rules, store.ForResource(key), catalogs)
	require.Len(t, candidates, 2)

	result := Resolve(key, candidates)
	assert.True(t, result.Contended)
	assert.Equal(t, ClaimLifecycleAuthority, result.ContendedLayer)
}

func TestEngine_NoContention_DifferentLayers(t *testing.T) {
	// RuntimeController + LifecycleAuthority on same resource = valid chain
	catalogs := testCatalogs()
	rules := []DecisionRule{
		{
			Name:     "runtime-controller",
			Priority: 900,
			When: RuleCondition{All: []FieldCondition{
				{Field: "resource.kind", Equals: "ReplicaSet"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "KubernetesController", Name: "deployment-controller"},
				ClaimLayer:       ClaimRuntimeController,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrInherited,
			},
		},
		{
			Name:     "helm-lifecycle",
			Priority: 1000,
			When: RuleCondition{All: []FieldCondition{
				{Field: "metadata.label", Equals: "myrelease"},
			}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "Helm", Name: "myrelease"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthAuthoritative,
				AuthorityState:   AuthStateVerified,
				Attribution:      AttrInherited,
			},
		},
	}

	store := NewFactStore()
	key := "ReplicaSet/default/my-rs-abc"
	store.Add(Fact{Subject: key, Field: "resource.kind", Value: "ReplicaSet"})
	store.Add(Fact{Subject: key, Field: "metadata.label", Value: "myrelease", Attributes: map[string]string{"key": "helm.sh/release-name"}})

	candidates := EvaluateAllRules(rules, store.ForResource(key), catalogs)
	require.Len(t, candidates, 2)

	result := Resolve(key, candidates)
	assert.False(t, result.Contended, "different layers should not contend")
	require.NotNil(t, result.RuntimeController)
	require.NotNil(t, result.LifecycleAuthority)
	assert.Equal(t, "KubernetesController", result.RuntimeController.Authority.Type)
	assert.Equal(t, "Helm", result.LifecycleAuthority.Authority.Type)
}

func TestEngine_ManagedFieldsDoNotAssignAuthority(t *testing.T) {
	// A rule with Supporting evidence should not produce a lifecycle authority
	catalogs := testCatalogs()
	trueVal := true
	rules := []DecisionRule{
		{
			Name:     "managed-field-only",
			Priority: 100,
			When: RuleCondition{All: []FieldCondition{
				{Field: "metadata.managedField", Exists: &trueVal},
			}},
			Result: RuleResult{
				// No authority type — supporting evidence only
				Authority:        AuthorityResult{},
				EvidenceStrength: StrengthSupporting,
			},
		},
	}

	store := NewFactStore()
	key := "ConfigMap/default/random"
	store.Add(Fact{Subject: key, Field: "metadata.managedField", Value: "kube-apiserver", Attributes: map[string]string{"manager": "kube-apiserver"}})

	candidates := EvaluateAllRules(rules, store.ForResource(key), catalogs)
	require.Len(t, candidates, 1)

	result := Resolve(key, candidates)
	// Supporting evidence with empty authority produces no resolved authority
	assert.True(t, result.NoAuthority)
	assert.Nil(t, result.LifecycleAuthority)
}

func TestEngine_NoAuthority(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Namespace", "", "fixture-a", "uid-ns", nil, nil))

	eng := mustEngine(t, testCatalogs(), testRules())
	results := eng.EvaluateAll(index)

	r := results["Namespace/fixture-a"]
	require.NotNil(t, r)
	assert.True(t, r.NoAuthority)
}

func TestValidateRules_MissingClaimLayer(t *testing.T) {
	catalogs := testCatalogs()
	rules := []DecisionRule{
		{
			Name:     "bad-rule",
			Priority: 500,
			When:     RuleCondition{All: []FieldCondition{{Field: "resource.kind", Equals: "Pod"}}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "Helm", Name: "x"},
				EvidenceStrength: StrengthAuthoritative,
				// Missing ClaimLayer
			},
		},
	}

	errs := ValidateRules(rules, catalogs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing claimLayer")
}

func TestValidateRules_MissingCatalog(t *testing.T) {
	catalogs := NewCatalogRegistry() // empty
	rules := []DecisionRule{
		{
			Name:     "uses-missing-catalog",
			Priority: 500,
			When:     RuleCondition{All: []FieldCondition{{Field: "resource.name", InCatalog: "nonexistent"}}},
			Result: RuleResult{
				Authority:        AuthorityResult{Type: "X", Name: "y"},
				ClaimLayer:       ClaimLifecycleAuthority,
				EvidenceStrength: StrengthAuthoritative,
			},
		},
	}

	errs := ValidateRules(rules, catalogs)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing catalog")
}

// --- helper ---

func mustEngine(t *testing.T, catalogs *CatalogRegistry, rules []DecisionRule) *DecisionEngine {
	t.Helper()
	// Use MetadataExtractor for real fact extraction
	eng, err := NewDecisionEngine(
		[]FactExtractor{&metadataExtractorAdapter{}},
		catalogs,
		rules,
	)
	require.NoError(t, err)
	return eng
}

// metadataExtractorAdapter wraps the extractors package extractor for test use within engine package.
type metadataExtractorAdapter struct{}

func (e *metadataExtractorAdapter) Name() string { return "Metadata" }

func (e *metadataExtractorAdapter) Extract(index *knowledge.Index) []Fact {
	var facts []Fact
	for _, rec := range index.List() {
		key := rec.Key()

		facts = append(facts, Fact{Subject: key, Field: "resource.kind", Value: rec.Identity.GVK.Kind, Source: key})
		facts = append(facts, Fact{Subject: key, Field: "resource.name", Value: rec.Identity.Name, Source: key})
		facts = append(facts, Fact{Subject: key, Field: "resource.namespace", Value: rec.Identity.Namespace, Source: key})

		for k, v := range rec.Labels {
			facts = append(facts, Fact{Subject: key, Field: "metadata.label", Value: v, Attributes: map[string]string{"key": k}, Source: key})
		}
		for k, v := range rec.Annotations {
			facts = append(facts, Fact{Subject: key, Field: "metadata.annotation", Value: v, Attributes: map[string]string{"key": k}, Source: key})
		}
		for _, mf := range rec.ManagedFields {
			facts = append(facts, Fact{Subject: key, Field: "metadata.managedField", Value: mf.Manager, Attributes: map[string]string{"manager": mf.Manager, "operation": mf.Operation}, Source: key})
		}
	}
	return facts
}
