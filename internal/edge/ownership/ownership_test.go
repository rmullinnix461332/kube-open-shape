package ownership

import (
	"testing"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func makeRecord(kind, ns, name string, labels, annotations map[string]string) *knowledge.ResourceRecord {
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
			UID:       types.UID("uid-" + ns + "-" + name),
		},
		Labels:      labels,
		Annotations: annotations,
	}
}

func TestPlatformDetector(t *testing.T) {
	tests := []struct {
		name     string
		record   *knowledge.ResourceRecord
		detected bool
	}{
		{
			name:     "kube-root-ca.crt is platform",
			record:   makeRecord("ConfigMap", "default", "kube-root-ca.crt", nil, nil),
			detected: true,
		},
		{
			name:     "default ServiceAccount is platform",
			record:   makeRecord("ServiceAccount", "default", "default", nil, nil),
			detected: true,
		},
		{
			name:     "normal ConfigMap is not platform",
			record:   makeRecord("ConfigMap", "default", "app-config", nil, nil),
			detected: false,
		},
		{
			name:     "named ServiceAccount is not platform",
			record:   makeRecord("ServiceAccount", "default", "my-app", nil, nil),
			detected: false,
		},
	}

	det := &PlatformDetector{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := det.Detect(tt.record, nil)
			if tt.detected {
				assert.NotEmpty(t, evidence)
				assert.True(t, evidence[0].Authoritative)
			} else {
				assert.Empty(t, evidence)
			}
		})
	}
}

func TestArgoCDDetector(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		detected    bool
	}{
		{
			name:        "tracking-id annotation detected",
			annotations: map[string]string{"argocd.argoproj.io/tracking-id": "app:apps/Deployment:ns/name"},
			detected:    true,
		},
		{
			name:        "no argocd annotation",
			annotations: map[string]string{"other": "value"},
			detected:    false,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			detected:    false,
		},
	}

	det := &ArgoCDDetector{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := makeRecord("Deployment", "default", "app", nil, tt.annotations)
			evidence := det.Detect(rec, nil)
			if tt.detected {
				assert.NotEmpty(t, evidence)
				assert.True(t, evidence[0].Authoritative)
			} else {
				assert.Empty(t, evidence)
			}
		})
	}
}

func TestHelmDetector(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		detected bool
	}{
		{
			name:     "managed-by Helm + release-name detected",
			labels:   map[string]string{"app.kubernetes.io/managed-by": "Helm", "helm.sh/release-name": "myrelease"},
			detected: true,
		},
		{
			name:     "release-name only detected",
			labels:   map[string]string{"helm.sh/release-name": "myrelease"},
			detected: true,
		},
		{
			name:     "managed-by only (evidence but no owner resolution)",
			labels:   map[string]string{"app.kubernetes.io/managed-by": "Helm"},
			detected: true,
		},
		{
			name:     "no helm labels",
			labels:   map[string]string{"app": "myapp"},
			detected: false,
		},
	}

	det := &HelmDetector{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := makeRecord("Deployment", "default", "app", tt.labels, nil)
			evidence := det.Detect(rec, nil)
			if tt.detected {
				assert.NotEmpty(t, evidence)
			} else {
				assert.Empty(t, evidence)
			}
		})
	}
}

func TestResolver_PlatformManaged(t *testing.T) {
	tests := []struct {
		name           string
		record         *knowledge.ResourceRecord
		classification Classification
	}{
		{
			name:           "kube-root-ca.crt classified PlatformManaged",
			record:         makeRecord("ConfigMap", "default", "kube-root-ca.crt", nil, nil),
			classification: PlatformManaged,
		},
		{
			name:           "default SA classified PlatformManaged",
			record:         makeRecord("ServiceAccount", "default", "default", nil, nil),
			classification: PlatformManaged,
		},
	}

	resolver := NewResolver()
	index := knowledge.NewIndex()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index.Upsert(tt.record)
			result := resolver.Resolve(tt.record, index)
			assert.Equal(t, tt.classification, result.Classification)
			assert.Equal(t, Authoritative, result.Confidence)
		})
	}
}

func TestResolver_HelmManaged(t *testing.T) {
	rec := makeRecord("Deployment", "default", "app", map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"helm.sh/release-name":         "myrelease",
	}, nil)

	resolver := NewResolver()
	index := knowledge.NewIndex()
	index.Upsert(rec)

	result := resolver.Resolve(rec, index)
	assert.Equal(t, Managed, result.Classification)
	require.NotNil(t, result.Owner)
	assert.Equal(t, "Helm", result.Owner.Type)
	assert.Equal(t, "myrelease", result.Owner.Name)
}

func TestResolver_ArgoCDManaged(t *testing.T) {
	rec := makeRecord("Deployment", "default", "app", nil, map[string]string{
		"argocd.argoproj.io/tracking-id": "myapp:apps/Deployment:default/app",
	})

	resolver := NewResolver()
	index := knowledge.NewIndex()
	index.Upsert(rec)

	result := resolver.Resolve(rec, index)
	assert.Equal(t, Managed, result.Classification)
	require.NotNil(t, result.Owner)
	assert.Equal(t, "ArgoCD", result.Owner.Type)
}

func TestResolver_Unknown(t *testing.T) {
	rec := makeRecord("ConfigMap", "default", "orphan-config", map[string]string{
		"app": "something",
	}, nil)

	resolver := NewResolver()
	index := knowledge.NewIndex()
	index.Upsert(rec)

	result := resolver.Resolve(rec, index)
	assert.Equal(t, Unknown, result.Classification)
}

func TestResolver_ResolveAll(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("ConfigMap", "default", "kube-root-ca.crt", nil, nil))
	index.Upsert(makeRecord("Deployment", "default", "app", map[string]string{
		"helm.sh/release-name": "myrelease",
	}, nil))
	index.Upsert(makeRecord("ConfigMap", "default", "random", nil, nil))

	resolver := NewResolver()
	results := resolver.ResolveAll(index)

	assert.Equal(t, 3, len(results))
	assert.Equal(t, PlatformManaged, results["ConfigMap/default/kube-root-ca.crt"].Classification)
	assert.Equal(t, Managed, results["Deployment/default/app"].Classification)
	assert.Equal(t, Unknown, results["ConfigMap/default/random"].Classification)
}
