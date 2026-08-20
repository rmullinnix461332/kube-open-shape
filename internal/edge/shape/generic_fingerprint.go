package shape

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
)

// Relationship classification layers
const (
	LayerFramework  = "framework"
	LayerDefining   = "defining"
	LayerContextual = "contextual"
	LayerProvenance = "provenance"
)

// Framework kinds — generated controller children excluded from semantic fingerprint
var frameworkKinds = map[string]bool{
	"ReplicaSet":         true,
	"ControllerRevision": true,
	"Pod":                true,
}

// Framework relationship patterns — ownerRef chains to generated resources
var frameworkRelationships = map[string]map[string]bool{
	"Deployment":  {"ReplicaSet": true},
	"StatefulSet": {"ControllerRevision": true},
	"DaemonSet":   {"ControllerRevision": true},
	"CronJob":     {"Job": true},
	"Job":         {"Pod": true},
	"ReplicaSet":  {"Pod": true},
}

// Provenance relationship types — boundary/ownership, not composition
var provenanceRelationTypes = map[graph.RelationType]bool{
	graph.BelongsToRelease: true,
	graph.ManagedBy:        true,
	graph.Reconciles:       true,
	graph.Generates:        true,
	graph.Provisions:       true,
}

// AnonymousSignature is a generic structural representation of a candidate subgraph
type AnonymousSignature struct {
	RootKind                string          `json:"rootKind"`
	DefiningMembers         map[string]int  `json:"definingMembers"`       // non-framework kind → count
	FrameworkMembers        map[string]int  `json:"frameworkMembers"`      // framework kind → count
	DefiningRelationships   map[string]int  `json:"definingRelationships"` // defining relType → count
	FrameworkRelationships  map[string]int  `json:"frameworkRelationships"`
	ProvenanceRelationships map[string]int  `json:"provenanceRelationships,omitempty"`
	Traits                  map[string]bool `json:"traits"`
	DirectedEdges           []DirectedEdge  `json:"-"` // not part of fingerprint; used by generator
}

// DualFingerprint holds both mechanical and semantic fingerprints
type DualFingerprint struct {
	Mechanical string // Includes everything
	Semantic   string // Excludes framework and provenance, drives grouping
}

// GenericFingerprint generates deterministic structural fingerprints for a candidate subgraph
func GenericFingerprint(subgraph CandidateSubgraph, g *graph.Graph) (AnonymousSignature, DualFingerprint) {
	sig := AnonymousSignature{
		DefiningMembers:         make(map[string]int),
		FrameworkMembers:        make(map[string]int),
		DefiningRelationships:   make(map[string]int),
		FrameworkRelationships:  make(map[string]int),
		ProvenanceRelationships: make(map[string]int),
		Traits:                  make(map[string]bool),
	}

	// Root kind
	parts := splitKey(subgraph.Root)
	if len(parts) > 0 {
		sig.RootKind = parts[0]
	}

	// Classify member kinds into framework vs defining
	for kind, count := range subgraph.Kinds {
		if frameworkKinds[kind] {
			sig.FrameworkMembers[kind] = count
		} else {
			sig.DefiningMembers[kind] = count
		}
	}

	// Classify relationships into defining, framework, or provenance
	allMembers := append([]string{subgraph.Root}, subgraph.Members...)
	memberSet := make(map[string]bool)
	for _, m := range allMembers {
		memberSet[m] = true
	}

	for _, memberKey := range allMembers {
		memberKind := extractKind(memberKey)
		for _, edge := range g.OutgoingEdges(memberKey) {
			if !memberSet[edge.Target] {
				continue
			}
			targetKind := extractKind(edge.Target)

			// Provenance relationships — excluded from semantic fingerprint entirely
			if provenanceRelationTypes[edge.Type] {
				sig.ProvenanceRelationships[string(edge.Type)]++
				continue
			}

			// Framework relationships — controller→generated child
			if isFrameworkRelationship(memberKind, targetKind) {
				sig.FrameworkRelationships[string(edge.Type)]++
			} else {
				sig.DefiningRelationships[string(edge.Type)]++
				sig.DirectedEdges = append(sig.DirectedEdges, DirectedEdge{
					SourceKind: memberKind,
					Type:       string(edge.Type),
					TargetKind: targetKind,
					Frequency:  1.0,
				})
			}
		}
	}

	// Structural traits (contextual layer)
	sig.Traits["clusterScopedResources"] = hasClusterScopedKind(subgraph.Kinds)
	sig.Traits["dedicatedServiceAccount"] = subgraph.Kinds["ServiceAccount"] > 0
	sig.Traits["exposesService"] = subgraph.Kinds["Service"] > 0
	sig.Traits["hasRBAC"] = subgraph.Kinds["Role"] > 0 || subgraph.Kinds["ClusterRole"] > 0
	sig.Traits["hasStorage"] = subgraph.Kinds["PersistentVolumeClaim"] > 0
	sig.Traits["hasWebhook"] = subgraph.Kinds["ValidatingWebhookConfiguration"] > 0 || subgraph.Kinds["MutatingWebhookConfiguration"] > 0

	// Calculate dual fingerprints
	fp := DualFingerprint{
		Mechanical: calculateMechanicalFingerprint(sig),
		Semantic:   calculateSemanticFingerprint(sig),
	}

	return sig, fp
}

// calculateSemanticFingerprint uses only defining members and relationships.
// Excludes framework relationships and provenance (BelongsToRelease, ManagedBy).
func calculateSemanticFingerprint(sig AnonymousSignature) string {
	canonical := struct {
		RootKind      string         `json:"r"`
		Members       map[string]int `json:"m"`
		Relationships []string       `json:"e"`
		Traits        []string       `json:"t"`
	}{
		RootKind: sig.RootKind,
		Members:  sig.DefiningMembers,
	}

	for relType, count := range sig.DefiningRelationships {
		canonical.Relationships = append(canonical.Relationships, fmt.Sprintf("%s:%d", relType, count))
	}
	sort.Strings(canonical.Relationships)

	for trait, val := range sig.Traits {
		if val {
			canonical.Traits = append(canonical.Traits, trait)
		}
	}
	sort.Strings(canonical.Traits)

	data, _ := json.Marshal(canonical)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash[:12])
}

// calculateMechanicalFingerprint includes everything for graph verification
func calculateMechanicalFingerprint(sig AnonymousSignature) string {
	allMembers := make(map[string]int)
	for k, v := range sig.DefiningMembers {
		allMembers[k] = v
	}
	for k, v := range sig.FrameworkMembers {
		allMembers[k] = v
	}

	canonical := struct {
		RootKind      string         `json:"r"`
		Members       map[string]int `json:"m"`
		Relationships []string       `json:"e"`
	}{
		RootKind: sig.RootKind,
		Members:  allMembers,
	}

	var allRels []string
	for relType, count := range sig.DefiningRelationships {
		allRels = append(allRels, fmt.Sprintf("%s:%d", relType, count))
	}
	for relType, count := range sig.FrameworkRelationships {
		allRels = append(allRels, fmt.Sprintf("fw:%s:%d", relType, count))
	}
	for relType, count := range sig.ProvenanceRelationships {
		allRels = append(allRels, fmt.Sprintf("prov:%s:%d", relType, count))
	}
	sort.Strings(allRels)
	canonical.Relationships = allRels

	data, _ := json.Marshal(canonical)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash[:12])
}

func isFrameworkRelationship(sourceKind, targetKind string) bool {
	if targets, ok := frameworkRelationships[sourceKind]; ok {
		return targets[targetKind]
	}
	return false
}

func extractKind(key string) string {
	parts := splitKey(key)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func hasClusterScopedKind(kinds map[string]int) bool {
	clusterScoped := map[string]bool{
		"ClusterRole": true, "ClusterRoleBinding": true,
		"CustomResourceDefinition": true, "Namespace": true,
	}
	for kind := range kinds {
		if clusterScoped[kind] {
			return true
		}
	}
	return false
}
