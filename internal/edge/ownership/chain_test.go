package ownership

import (
	"testing"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func makeChainRecord(kind, ns, name string, uid types.UID, labels, annotations map[string]string, ownerRefs []knowledge.OwnerReference) *knowledge.ResourceRecord {
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       uid,
		},
		Labels:          labels,
		Annotations:     annotations,
		OwnerReferences: ownerRefs,
	}
}

func TestResolveChain_DirectHelmAuthority(t *testing.T) {
	tests := []struct {
		name          string
		record        *knowledge.ResourceRecord
		expectedAuth  string
		expectedAttr  Attribution
		expectedState AuthorityState
		expectedClass Classification
	}{
		{
			name: "Deployment with helm.sh/chart is Direct/Verified",
			record: makeChainRecord("Deployment", "argocd", "argocd-server", "uid-1",
				map[string]string{"helm.sh/chart": "argo-cd-10.4.0", "app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/instance": "argocd"},
				map[string]string{"meta.helm.sh/release-name": "argocd"},
				nil),
			expectedAuth:  "Helm",
			expectedAttr:  AttributionDirect,
			expectedState: StateVerified,
			expectedClass: Managed,
		},
		{
			name: "ConfigMap with helm.sh/chart is Direct/Verified",
			record: makeChainRecord("ConfigMap", "argocd", "argocd-cm", "uid-2",
				map[string]string{"helm.sh/chart": "argo-cd-10.4.0", "app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/instance": "argocd"},
				nil, nil),
			expectedAuth:  "Helm",
			expectedAttr:  AttributionDirect,
			expectedState: StateVerified,
			expectedClass: Managed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			index.Upsert(tt.record)

			resolver := NewResolver()
			rec := resolver.ResolveChain(tt.record, index)

			require.NotNil(t, rec.LifecycleAuthority)
			assert.Equal(t, tt.expectedAuth, rec.LifecycleAuthority.Type)
			assert.Equal(t, tt.expectedAttr, rec.Attribution)
			assert.Equal(t, tt.expectedState, rec.AuthorityState)
			assert.Equal(t, tt.expectedClass, rec.DeriveClassification())
		})
	}
}

func TestResolveChain_InheritedThroughOwnerRef(t *testing.T) {
	index := knowledge.NewIndex()

	deployment := makeChainRecord("Deployment", "argocd", "argocd-server", "uid-deploy",
		map[string]string{"helm.sh/chart": "argo-cd-10.4.0", "app.kubernetes.io/instance": "argocd"},
		map[string]string{"meta.helm.sh/release-name": "argocd"},
		nil)
	index.Upsert(deployment)

	replicaSet := makeChainRecord("ReplicaSet", "argocd", "argocd-server-abc", "uid-rs",
		map[string]string{},
		nil,
		[]knowledge.OwnerReference{{
			Kind:       "Deployment",
			Name:       "argocd-server",
			UID:        "uid-deploy",
			Controller: true,
		}})
	index.Upsert(replicaSet)

	resolver := NewResolver()
	rec := resolver.ResolveChain(replicaSet, index)

	assert.Equal(t, AttributionInherited, rec.Attribution)
	require.Len(t, rec.RuntimeChain, 1)
	assert.Equal(t, "Deployment/argocd/argocd-server", rec.RuntimeChain[0].ResourceKey)
	assert.Equal(t, "ownerReference", rec.RuntimeChain[0].Relationship)

	require.NotNil(t, rec.LifecycleAuthority)
	assert.Equal(t, "Helm", rec.LifecycleAuthority.Type)
	assert.Equal(t, "argocd", rec.LifecycleAuthority.Name)
	assert.Equal(t, Inherited, rec.DeriveClassification())
}

func TestResolveChain_NoAuthority(t *testing.T) {
	index := knowledge.NewIndex()
	rec := makeChainRecord("ConfigMap", "default", "orphan", "uid-orphan",
		map[string]string{"app": "something"},
		nil, nil)
	index.Upsert(rec)

	resolver := NewResolver()
	chain := resolver.ResolveChain(rec, index)

	assert.Nil(t, chain.LifecycleAuthority)
	assert.Equal(t, StateNoAuthority, chain.AuthorityState)
	assert.Equal(t, AttributionDirect, chain.Attribution)
	assert.Equal(t, Unknown, chain.DeriveClassification())
}

func TestResolveChain_PlatformManaged(t *testing.T) {
	tests := []struct {
		name   string
		record *knowledge.ResourceRecord
	}{
		{
			name:   "kube-root-ca.crt",
			record: makeChainRecord("ConfigMap", "default", "kube-root-ca.crt", "uid-ca", nil, nil, nil),
		},
		{
			name:   "default ServiceAccount",
			record: makeChainRecord("ServiceAccount", "default", "default", "uid-sa", nil, nil, nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			index.Upsert(tt.record)

			resolver := NewResolver()
			chain := resolver.ResolveChain(tt.record, index)

			require.NotNil(t, chain.LifecycleAuthority)
			assert.Equal(t, "Platform", chain.LifecycleAuthority.Type)
			assert.Equal(t, StateVerified, chain.AuthorityState)
			assert.Equal(t, PlatformManaged, chain.DeriveClassification())
		})
	}
}

func TestResolveChain_BrokenOwnerRef(t *testing.T) {
	index := knowledge.NewIndex()

	// ReplicaSet with ownerRef pointing to non-existent Deployment
	rs := makeChainRecord("ReplicaSet", "default", "orphan-rs", "uid-rs",
		nil, nil,
		[]knowledge.OwnerReference{{
			Kind:       "Deployment",
			Name:       "deleted-deploy",
			UID:        "uid-nonexistent",
			Controller: true,
		}})
	index.Upsert(rs)

	resolver := NewResolver()
	chain := resolver.ResolveChain(rs, index)

	// Chain should show the broken link
	require.Len(t, chain.RuntimeChain, 1)
	assert.Contains(t, chain.RuntimeChain[0].Relationship, "missing")
	assert.Equal(t, AttributionInherited, chain.Attribution)
	assert.Equal(t, StateNoAuthority, chain.AuthorityState)
}

func TestResolveChain_DeriveResult(t *testing.T) {
	index := knowledge.NewIndex()
	rec := makeChainRecord("Deployment", "default", "app", "uid-app",
		map[string]string{"helm.sh/release-name": "myrelease"},
		nil, nil)
	index.Upsert(rec)

	resolver := NewResolver()
	chain := resolver.ResolveChain(rec, index)
	result := chain.DeriveResult()

	assert.Equal(t, Managed, result.Classification)
	assert.Equal(t, Authoritative, result.Confidence)
	require.NotNil(t, result.Owner)
	assert.Equal(t, "Helm", result.Owner.Type)
	assert.Equal(t, "myrelease", result.Owner.Name)
}

func TestResolveAllChains(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeChainRecord("ConfigMap", "default", "kube-root-ca.crt", "uid-ca", nil, nil, nil))
	index.Upsert(makeChainRecord("Deployment", "default", "app", "uid-app",
		map[string]string{"helm.sh/release-name": "myrelease"}, nil, nil))
	index.Upsert(makeChainRecord("ConfigMap", "default", "random", "uid-rand", nil, nil, nil))

	resolver := NewResolver()
	chains := resolver.ResolveAllChains(index)

	assert.Equal(t, 3, len(chains))
	assert.Equal(t, PlatformManaged, chains["ConfigMap/default/kube-root-ca.crt"].DeriveClassification())
	assert.Equal(t, Managed, chains["Deployment/default/app"].DeriveClassification())
	assert.Equal(t, Unknown, chains["ConfigMap/default/random"].DeriveClassification())
}
