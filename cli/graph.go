package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/grouping"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/shape"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	graphIncludeCandidates bool
	graphClusterID         string
)

func newGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Knowledge graph operations",
	}

	export := &cobra.Command{
		Use:   "export",
		Short: "Export the full knowledge graph as a JSON snapshot",
		RunE:  runGraphExport,
	}
	export.Flags().BoolVar(&graphIncludeCandidates, "include-candidates", false, "Include candidate discovery state")
	export.Flags().StringVar(&graphClusterID, "cluster-id", "", "Cluster identifier (default: from kubeconfig context)")

	cmd.AddCommand(export)
	return cmd
}

// --- Graph export types ---

type graphSnapshot struct {
	SchemaVersion string          `json:"schemaVersion"`
	Snapshot      snapshotMeta    `json:"snapshot"`
	Nodes         []graphNode     `json:"nodes"`
	Edges         []graphEdge     `json:"edges"`
	Classifiers   []graphEntity   `json:"classifiers,omitempty"`
	Shapes        []graphEntity   `json:"shapes,omitempty"`
	Candidates    []graphEntity   `json:"candidates,omitempty"`
	Summary       snapshotSummary `json:"summary"`
}

type snapshotMeta struct {
	ClusterID             string `json:"clusterId"`
	ObservedAt            string `json:"observedAt"`
	RelationshipModel     string `json:"relationshipModel"`
	CanonicalizationModel string `json:"canonicalizationModel"`
	TraitModel            string `json:"traitModel"`
}

type graphNode struct {
	ID        string        `json:"id"`
	Type      string        `json:"type"`
	Resource  resourceInfo  `json:"resource"`
	Ownership ownershipInfo `json:"ownership"`
}

type resourceInfo struct {
	ClusterID  string `json:"clusterId"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
	CreatedAt  string `json:"createdAt"`
}

type ownershipInfo struct {
	Classification string     `json:"classification"`
	Confidence     string     `json:"confidence"`
	Owner          *ownerInfo `json:"owner,omitempty"`
}

type ownerInfo struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type graphEdge struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	From     string       `json:"from"`
	To       string       `json:"to"`
	Evidence edgeEvidence `json:"evidence"`
	Semantic bool         `json:"semantic"`
	Model    string       `json:"model,omitempty"`
}

type edgeEvidence struct {
	Type          string `json:"type"`
	FieldPath     string `json:"fieldPath,omitempty"`
	ObservedValue string `json:"observedValue,omitempty"`
}

type graphEntity struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Role string `json:"role,omitempty"`
	Name string `json:"name,omitempty"`
}

type snapshotSummary struct {
	NodeCount       int `json:"nodeCount"`
	EdgeCount       int `json:"edgeCount"`
	ClassifierCount int `json:"classifierCount"`
	ShapeCount      int `json:"shapeCount"`
}

func runGraphExport(cmd *cobra.Command, args []string) error {
	index, err := collectOnce()
	if err != nil {
		return err
	}

	resolver := ownership.NewResolver()
	ownerResults := resolver.ResolveAll(index)
	g := graph.Build(index, ownerResults)

	clusterID := graphClusterID
	if clusterID == "" {
		clusterID = inferClusterID()
	}

	now := time.Now().UTC()

	// Build nodes
	records := index.List()
	nodes := make([]graphNode, 0, len(records))
	for _, rec := range records {
		nodeID := logicalID(clusterID, rec)
		node := graphNode{
			ID:   nodeID,
			Type: "KubernetesResource",
			Resource: resourceInfo{
				ClusterID:  clusterID,
				APIVersion: apiVersion(rec),
				Kind:       rec.Identity.GVK.Kind,
				Namespace:  rec.Identity.Namespace,
				Name:       rec.Identity.Name,
				UID:        string(rec.Identity.UID),
				CreatedAt:  rec.Identity.CreatedAt.Format(time.RFC3339),
			},
		}

		if result, ok := ownerResults[rec.Key()]; ok {
			node.Ownership = ownershipInfo{
				Classification: string(result.Classification),
				Confidence:     string(result.Confidence),
			}
			if result.Owner != nil {
				node.Ownership.Owner = &ownerInfo{
					Type:      result.Owner.Type,
					Name:      result.Owner.Name,
					Namespace: result.Owner.Namespace,
				}
			}
		}

		nodes = append(nodes, node)
	}

	// Build edges
	allEdges := g.AllEdges()
	edges := make([]graphEdge, 0, len(allEdges))
	for _, e := range allEdges {
		edge := graphEdge{
			ID:   edgeID(e),
			Type: string(e.Type),
			From: nodeIDFromKey(clusterID, e.Source, index),
			To:   nodeIDFromKey(clusterID, e.Target, index),
			Evidence: edgeEvidence{
				Type:          e.Confidence,
				FieldPath:     e.Evidence,
				ObservedValue: extractObservedValue(e.Evidence),
			},
			Semantic: isSemanticEdge(e.Type),
		}
		if edge.Semantic {
			edge.Model = "builtin:structural-composition-v1"
		}
		edges = append(edges, edge)
	}

	// Build classifiers
	var classifiers []graphEntity
	classifiers = append(classifiers, graphEntity{
		ID:   "classifier:kos-default-application",
		Type: "RoleClassifier",
		Role: "application",
	})
	classifiers = append(classifiers, graphEntity{
		ID:   "classifier:kos-default-node-system",
		Type: "RoleClassifier",
		Role: "node-system",
	})
	classifiers = append(classifiers, graphEntity{
		ID:   "classifier:kos-default-scheduled",
		Type: "RoleClassifier",
		Role: "scheduled-workload",
	})

	// Candidates (opt-in)
	var candidates []graphEntity
	if graphIncludeCandidates {
		classifiedRoots := make(map[string]bool)
		subgraphs := shape.SegmentUnclassified(index, g, classifiedRoots)
		groups := shape.GroupCandidates(subgraphs, g)
		for _, grp := range groups {
			candidates = append(candidates, graphEntity{
				ID:   "candidate:" + grp.ID,
				Type: "CandidateShapeGroup",
				Role: grp.RootKind,
				Name: grp.ID,
			})
		}
	}

	// Logical resource groups + MemberOf edges
	logicalGroups := grouping.BuildGroups(index, clusterID)
	logicalGroupCount := 0
	releaseGroupCount := 0
	for _, grp := range logicalGroups {
		nodes = append(nodes, graphNode{
			ID:   grp.ID,
			Type: "LogicalResourceGroup",
		})
		switch grp.GroupType {
		case grouping.GroupTypeRelease:
			releaseGroupCount++
		default:
			logicalGroupCount++
		}

		// MemberOf edges
		edgeType := "MemberOf"
		if grp.GroupType == grouping.GroupTypeRelease {
			edgeType = "MemberOfRelease"
		}
		for _, member := range grp.Members {
			fromID := nodeIDFromKey(clusterID, member.ResourceKey, index)
			raw := fmt.Sprintf("%s→%s→%s", fromID, edgeType, grp.ID)
			hash := sha256.Sum256([]byte(raw))
			ev := edgeEvidence{Type: "LabelAssociation"}
			if len(member.Evidence) > 0 {
				ev.FieldPath = member.Evidence[0].FieldPath
				ev.ObservedValue = member.Evidence[0].ObservedValue
			}
			edges = append(edges, graphEdge{
				ID:       fmt.Sprintf("edge:sha256:%x", hash[:8]),
				Type:     edgeType,
				From:     fromID,
				To:       grp.ID,
				Evidence: ev,
			})
		}
	}

	// ClassifiedAs edges (resource → classifier)
	for _, rec := range records {
		kind := rec.Identity.GVK.Kind
		var classifierID string
		switch kind {
		case "Deployment", "StatefulSet":
			classifierID = "classifier:kos-default-application"
		case "DaemonSet":
			classifierID = "classifier:kos-default-node-system"
		case "CronJob":
			classifierID = "classifier:kos-default-scheduled"
		default:
			continue
		}
		nodeID := logicalID(clusterID, rec)
		raw := fmt.Sprintf("%s→ClassifiedAs→%s", nodeID, classifierID)
		hash := sha256.Sum256([]byte(raw))
		edges = append(edges, graphEdge{
			ID:   fmt.Sprintf("edge:sha256:%x", hash[:8]),
			Type: "ClassifiedAs",
			From: nodeID,
			To:   classifierID,
			Evidence: edgeEvidence{
				Type:      "DefinitionMatch",
				FieldPath: classifierID[len("classifier:"):],
			},
		})
	}

	snapshot := graphSnapshot{
		SchemaVersion: "1",
		Snapshot: snapshotMeta{
			ClusterID:             clusterID,
			ObservedAt:            now.Format(time.RFC3339),
			RelationshipModel:     "builtin:structural-composition-v1",
			CanonicalizationModel: "generic-structural-v1@1",
			TraitModel:            "builtin:structural-v1",
		},
		Nodes:       nodes,
		Edges:       edges,
		Classifiers: classifiers,
		Shapes:      nil,
		Candidates:  candidates,
		Summary: snapshotSummary{
			NodeCount:       len(nodes),
			EdgeCount:       len(edges),
			ClassifierCount: len(classifiers),
			ShapeCount:      0,
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(snapshot)
}

// --- helpers ---

func logicalID(clusterID string, rec *knowledge.ResourceRecord) string {
	group := rec.Identity.GVK.Group
	if group == "" {
		group = "core"
	}
	if rec.Identity.Namespace != "" {
		return fmt.Sprintf("resource:%s/%s/%s/%s/%s",
			clusterID, group, rec.Identity.GVK.Kind, rec.Identity.Namespace, rec.Identity.Name)
	}
	return fmt.Sprintf("resource:%s/%s/%s/%s",
		clusterID, group, rec.Identity.GVK.Kind, rec.Identity.Name)
}

func nodeIDFromKey(clusterID, key string, index *knowledge.Index) string {
	rec, ok := index.Get(key)
	if ok {
		return logicalID(clusterID, rec)
	}
	return "resource:" + clusterID + "/" + key
}

func apiVersion(rec *knowledge.ResourceRecord) string {
	if rec.Identity.GVK.Group == "" {
		return "v1"
	}
	v := rec.Identity.GVK.Version
	if v == "" {
		v = "v1"
	}
	return rec.Identity.GVK.Group + "/" + v
}

func edgeID(e graph.Edge) string {
	raw := fmt.Sprintf("%s→%s→%s→%s", e.Source, e.Type, e.Target, e.Evidence)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("edge:sha256:%x", hash[:8])
}

func extractObservedValue(evidence string) string {
	// Evidence strings are "field=value" or just descriptions
	for i, c := range evidence {
		if c == '=' {
			return evidence[i+1:]
		}
	}
	return ""
}

func isSemanticEdge(relType graph.RelationType) bool {
	layer := graph.ClassifyRelationship(relType)
	return layer == graph.LayerDefining
}

func inferClusterID() string {
	// Try to get context name from kubeconfig
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).RawConfig()
	if err == nil && config.CurrentContext != "" {
		return config.CurrentContext
	}
	return "unknown"
}
