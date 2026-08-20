package grouping

import (
	"testing"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func makeRecord(kind, ns, name string, labels map[string]string) *knowledge.ResourceRecord {
	if labels == nil {
		labels = map[string]string{}
	}
	return &knowledge.ResourceRecord{
		Identity: knowledge.ResourceIdentity{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
			Namespace: ns,
			Name:      name,
			UID:       types.UID("uid-" + ns + "-" + name),
		},
		Labels:      labels,
		Annotations: map[string]string{},
	}
}

func TestBuildGroups_PartOfCreatesApplicationGroup(t *testing.T) {
	tests := []struct {
		name           string
		records        []*knowledge.ResourceRecord
		expectedGroups int
		expectedName   string
		expectedType   string
	}{
		{
			name: "single resource with part-of creates one group",
			records: []*knowledge.ResourceRecord{
				makeRecord("Deployment", "default", "api", map[string]string{
					"app.kubernetes.io/part-of": "myapp",
				}),
			},
			expectedGroups: 1,
			expectedName:   "myapp",
			expectedType:   GroupTypeApplication,
		},
		{
			name: "multiple resources with same part-of form one group",
			records: []*knowledge.ResourceRecord{
				makeRecord("Deployment", "default", "api", map[string]string{
					"app.kubernetes.io/part-of": "myapp",
				}),
				makeRecord("Service", "default", "api-svc", map[string]string{
					"app.kubernetes.io/part-of": "myapp",
				}),
				makeRecord("ConfigMap", "default", "api-config", map[string]string{
					"app.kubernetes.io/part-of": "myapp",
				}),
			},
			expectedGroups: 1,
			expectedName:   "myapp",
			expectedType:   GroupTypeApplication,
		},
		{
			name: "different part-of values create separate groups",
			records: []*knowledge.ResourceRecord{
				makeRecord("Deployment", "default", "api", map[string]string{
					"app.kubernetes.io/part-of": "app-a",
				}),
				makeRecord("Deployment", "default", "worker", map[string]string{
					"app.kubernetes.io/part-of": "app-b",
				}),
			},
			expectedGroups: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			for _, r := range tt.records {
				index.Upsert(r)
			}

			groups := BuildGroups(index, "test-cluster")

			// Filter to Application type only
			var appGroups []*LogicalResourceGroup
			for _, g := range groups {
				if g.GroupType == GroupTypeApplication {
					appGroups = append(appGroups, g)
				}
			}

			assert.Equal(t, tt.expectedGroups, len(appGroups))
			if tt.expectedName != "" && len(appGroups) > 0 {
				assert.Equal(t, tt.expectedName, appGroups[0].Name)
				assert.Equal(t, tt.expectedType, appGroups[0].GroupType)
			}
		})
	}
}

func TestBuildGroups_InstanceLabel(t *testing.T) {
	tests := []struct {
		name               string
		records            []*knowledge.ResourceRecord
		expectedGroups     int
		expectedConfidence string
	}{
		{
			name: "instance-only creates group with Inferred confidence",
			records: []*knowledge.ResourceRecord{
				makeRecord("Deployment", "default", "api", map[string]string{
					"app.kubernetes.io/instance": "myapp",
				}),
			},
			expectedGroups:     1,
			expectedConfidence: ConfidenceInferred,
		},
		{
			name: "instance ignored when part-of is present",
			records: []*knowledge.ResourceRecord{
				makeRecord("Deployment", "default", "api", map[string]string{
					"app.kubernetes.io/part-of":  "myapp",
					"app.kubernetes.io/instance": "myapp",
				}),
			},
			expectedGroups:     1,
			expectedConfidence: ConfidenceCorroborating,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			for _, r := range tt.records {
				index.Upsert(r)
			}

			groups := BuildGroups(index, "test-cluster")
			var appGroups []*LogicalResourceGroup
			for _, g := range groups {
				if g.GroupType == GroupTypeApplication {
					appGroups = append(appGroups, g)
				}
			}

			require.Equal(t, tt.expectedGroups, len(appGroups))
			assert.Equal(t, tt.expectedConfidence, appGroups[0].Confidence)
		})
	}
}

func TestBuildGroups_HelmRelease(t *testing.T) {
	tests := []struct {
		name                string
		records             []*knowledge.ResourceRecord
		expectedReleases    int
		expectedReleaseName string
	}{
		{
			name: "helm label creates release group",
			records: []*knowledge.ResourceRecord{
				makeRecord("Deployment", "default", "api", map[string]string{
					"helm.sh/release-name": "myrelease",
				}),
				makeRecord("Service", "default", "api-svc", map[string]string{
					"helm.sh/release-name": "myrelease",
				}),
			},
			expectedReleases:    1,
			expectedReleaseName: "myrelease",
		},
		{
			name: "helm annotation creates release group",
			records: []*knowledge.ResourceRecord{
				makeRecord("Deployment", "default", "api", map[string]string{}),
			},
			expectedReleases:    1,
			expectedReleaseName: "myrelease",
		},
	}

	// For the annotation test, set it manually
	tests[1].records[0].Annotations = map[string]string{
		"meta.helm.sh/release-name": "myrelease",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := knowledge.NewIndex()
			for _, r := range tt.records {
				index.Upsert(r)
			}

			groups := BuildGroups(index, "test-cluster")
			var releaseGroups []*LogicalResourceGroup
			for _, g := range groups {
				if g.GroupType == GroupTypeRelease {
					releaseGroups = append(releaseGroups, g)
				}
			}

			require.Equal(t, tt.expectedReleases, len(releaseGroups))
			if tt.expectedReleaseName != "" {
				assert.Equal(t, tt.expectedReleaseName, releaseGroups[0].Name)
			}
		})
	}
}

func TestBuildGroups_ComponentLabel(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "api", map[string]string{
		"app.kubernetes.io/part-of":   "myapp",
		"app.kubernetes.io/component": "server",
	}))
	index.Upsert(makeRecord("Deployment", "default", "worker", map[string]string{
		"app.kubernetes.io/part-of":   "myapp",
		"app.kubernetes.io/component": "worker",
	}))

	groups := BuildGroups(index, "test-cluster")
	var appGroups []*LogicalResourceGroup
	for _, g := range groups {
		if g.GroupType == GroupTypeApplication {
			appGroups = append(appGroups, g)
		}
	}

	require.Equal(t, 1, len(appGroups))
	assert.Equal(t, 2, appGroups[0].ComponentCount)
	assert.Equal(t, 2, appGroups[0].WorkloadCount)
}

func TestBuildGroups_PlatformExclusion(t *testing.T) {
	tests := []struct {
		name     string
		record   *knowledge.ResourceRecord
		excluded bool
	}{
		{
			name:     "kube-root-ca.crt excluded",
			record:   makeRecord("ConfigMap", "default", "kube-root-ca.crt", map[string]string{"app.kubernetes.io/instance": "myapp"}),
			excluded: true,
		},
		{
			name:     "default ServiceAccount excluded",
			record:   makeRecord("ServiceAccount", "default", "default", map[string]string{"app.kubernetes.io/instance": "myapp"}),
			excluded: true,
		},
		{
			name:     "normal resource included",
			record:   makeRecord("Deployment", "default", "api", map[string]string{"app.kubernetes.io/instance": "myapp"}),
			excluded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.excluded, shouldExcludeFromGroup(tt.record))
		})
	}
}

func TestBuildGroups_Counts(t *testing.T) {
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "api", map[string]string{
		"app.kubernetes.io/instance":  "myapp",
		"app.kubernetes.io/component": "api",
	}))
	index.Upsert(makeRecord("StatefulSet", "default", "db", map[string]string{
		"app.kubernetes.io/instance":  "myapp",
		"app.kubernetes.io/component": "database",
	}))
	index.Upsert(makeRecord("Service", "default", "api-svc", map[string]string{
		"app.kubernetes.io/instance":  "myapp",
		"app.kubernetes.io/component": "api",
	}))
	index.Upsert(makeRecord("ConfigMap", "default", "config", map[string]string{
		"app.kubernetes.io/instance": "myapp",
	}))

	groups := BuildGroups(index, "test-cluster")
	var appGroups []*LogicalResourceGroup
	for _, g := range groups {
		if g.GroupType == GroupTypeApplication {
			appGroups = append(appGroups, g)
		}
	}

	require.Equal(t, 1, len(appGroups))
	grp := appGroups[0]

	assert.Equal(t, 4, grp.ResourceCount, "total resources")
	assert.Equal(t, 2, grp.WorkloadCount, "workloads (Deployment + StatefulSet)")
	assert.Equal(t, 2, grp.ComponentCount, "components (api + database)")
}

func TestBuildGroups_Determinism(t *testing.T) {
	index := knowledge.NewIndex()
	for i := 0; i < 10; i++ {
		index.Upsert(makeRecord("Deployment", "default", "app-"+string(rune('a'+i)), map[string]string{
			"app.kubernetes.io/instance": "bigapp",
		}))
	}

	groups1 := BuildGroups(index, "test-cluster")
	groups2 := BuildGroups(index, "test-cluster")

	require.Equal(t, len(groups1), len(groups2))
	for i := range groups1 {
		assert.Equal(t, groups1[i].ID, groups2[i].ID)
		require.Equal(t, len(groups1[i].Members), len(groups2[i].Members))
		for j := range groups1[i].Members {
			assert.Equal(t, groups1[i].Members[j].ResourceKey, groups2[i].Members[j].ResourceKey)
		}
	}
}

func TestDetectConflict(t *testing.T) {
	tests := []struct {
		name     string
		partOf   string
		instance string
		helm     string
		conflict bool
	}{
		{"all agree", "myapp", "myapp", "myapp", false},
		{"part-of and instance disagree", "app-a", "app-b", "", true},
		{"helm disagrees", "myapp", "myapp", "other-release", true},
		{"only one value", "myapp", "", "", false},
		{"all empty", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.conflict, detectConflict(tt.partOf, tt.instance, tt.helm))
		})
	}
}

func TestDetermineConfidence(t *testing.T) {
	tests := []struct {
		name       string
		partOf     bool
		instance   bool
		helm       bool
		confidence string
	}{
		{"all three", true, true, true, ConfidenceCorroborating},
		{"part-of + instance", true, true, false, ConfidenceCorroborating},
		{"part-of + helm", true, false, true, ConfidenceCorroborating},
		{"part-of only", true, false, false, ConfidenceDeclared},
		{"helm only", false, false, true, ConfidenceDeclared},
		{"instance only", false, true, false, ConfidenceInferred},
		{"none", false, false, false, ConfidenceHeuristic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.confidence, determineConfidence(tt.partOf, tt.instance, tt.helm))
		})
	}
}

func TestNormalizeGroupKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MyApp", "myapp"},
		{"  spaces  ", "spaces"},
		{"already-lower", "already-lower"},
		{"UPPER", "upper"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeGroupKey(tt.input))
		})
	}
}

func TestDetectHelmRelease(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		expected    string
	}{
		{
			name:        "annotation takes precedence",
			labels:      map[string]string{"helm.sh/release-name": "from-label"},
			annotations: map[string]string{"meta.helm.sh/release-name": "from-annotation"},
			expected:    "from-annotation",
		},
		{
			name:     "label fallback",
			labels:   map[string]string{"helm.sh/release-name": "from-label"},
			expected: "from-label",
		},
		{
			name:     "managed-by=Helm uses instance",
			labels:   map[string]string{"app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/instance": "proxy-instance"},
			expected: "proxy-instance",
		},
		{
			name:     "no helm evidence",
			labels:   map[string]string{"app.kubernetes.io/name": "myapp"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &knowledge.ResourceRecord{
				Labels:      tt.labels,
				Annotations: tt.annotations,
			}
			if rec.Annotations == nil {
				rec.Annotations = map[string]string{}
			}
			assert.Equal(t, tt.expected, detectHelmRelease(rec))
		})
	}
}

func TestBuildGroups_NoDuplicateMembers(t *testing.T) {
	// A resource with both part-of and instance pointing to the same group
	// should appear only once in the member list.
	index := knowledge.NewIndex()
	index.Upsert(makeRecord("Deployment", "default", "api", map[string]string{
		"app.kubernetes.io/part-of":  "myapp",
		"app.kubernetes.io/instance": "myapp",
		"helm.sh/release-name":       "myapp",
	}))

	groups := BuildGroups(index, "test-cluster")
	var appGroups []*LogicalResourceGroup
	for _, g := range groups {
		if g.GroupType == GroupTypeApplication {
			appGroups = append(appGroups, g)
		}
	}

	require.Equal(t, 1, len(appGroups))
	assert.Equal(t, 1, len(appGroups[0].Members), "resource should appear once even with multiple signals")
}
