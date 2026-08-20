package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kube-open-shape/kube-open-shape/api/v1alpha1"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/grouping"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/janitor"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/ownership/engine/setup"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/shape"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
)

// Server is the local HTTP API server
type Server struct {
	index     *knowledge.Index
	resolver  *ownership.Resolver
	store     *store.Store
	janitor   *janitor.Engine
	addr      string
	clusterID string
}

// NewServer creates a local API server
func NewServer(index *knowledge.Index, resolver *ownership.Resolver, st *store.Store, jan *janitor.Engine, addr string) *Server {
	return &Server{
		index:     index,
		resolver:  resolver,
		store:     st,
		janitor:   jan,
		addr:      addr,
		clusterID: "edge-local",
	}
}

// Start begins serving the API
func (s *Server) Start() error {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/api/v1/knowledge", s.handleKnowledge)
	r.Get("/api/v1/knowledge/{namespace}/{kind}/{name}", s.handleResource)
	r.Get("/api/v1/ownership/summary", s.handleOwnershipSummary)
	r.Get("/api/v1/shapes", s.handleShapes)
	r.Get("/api/v1/candidates", s.handleCandidates)
	r.Get("/api/v1/report", s.handleReport)
	r.Get("/api/v1/graph", s.handleGraph)
	r.Get("/api/v1/groups", s.handleGroups)
	r.Get("/api/v1/findings", s.handleFindings)
	r.Get("/api/v1/rules", s.handleRules)
	r.Get("/api/v1/health", s.handleHealth)

	return http.ListenAndServe(s.addr, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]any{
		"status":    "healthy",
		"resources": s.index.Count(),
	})
}

func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	kind := r.URL.Query().Get("kind")

	records := s.index.List()
	ownerResults := s.resolver.ResolveAll(s.index)

	var items []map[string]any
	for _, rec := range records {
		if ns != "" && rec.Identity.Namespace != ns {
			continue
		}
		if kind != "" && rec.Identity.GVK.Kind != kind {
			continue
		}
		item := map[string]any{
			"kind":      rec.Identity.GVK.Kind,
			"namespace": rec.Identity.Namespace,
			"name":      rec.Identity.Name,
			"uid":       rec.Identity.UID,
			"createdAt": rec.Identity.CreatedAt,
		}
		if result, ok := ownerResults[rec.Key()]; ok {
			item["ownership"] = map[string]any{
				"classification": result.Classification,
				"confidence":     result.Confidence,
			}
			if result.Owner != nil {
				item["owner"] = map[string]any{
					"type": result.Owner.Type,
					"name": result.Owner.Name,
				}
			}
		}
		items = append(items, item)
	}

	respondJSON(w, map[string]any{"resources": items, "total": len(items)})
}

func (s *Server) handleResource(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	kind := chi.URLParam(r, "kind")
	name := chi.URLParam(r, "name")

	key := fmt.Sprintf("%s/%s/%s", kind, ns, name)
	rec, ok := s.index.Get(key)
	if !ok {
		http.Error(w, "not found", 404)
		return
	}

	ownerResult := s.resolver.Resolve(rec, s.index)

	item := map[string]any{
		"kind":        rec.Identity.GVK.Kind,
		"apiVersion":  apiVersionStr(rec),
		"namespace":   rec.Identity.Namespace,
		"name":        rec.Identity.Name,
		"uid":         rec.Identity.UID,
		"createdAt":   rec.Identity.CreatedAt,
		"labels":      rec.Labels,
		"annotations": rec.Annotations,
		"ownership":   ownerResult,
	}

	if s.store != nil {
		clocks, _ := s.store.GetAllClocks(key)
		if len(clocks) > 0 {
			item["lifecycle"] = clocks
		}
	}

	respondJSON(w, item)
}

func (s *Server) handleOwnershipSummary(w http.ResponseWriter, r *http.Request) {
	eng, err := setup.DefaultEngine()
	if err != nil {
		http.Error(w, "engine init: "+err.Error(), 500)
		return
	}
	results := eng.EvaluateAll(s.index)

	// Build authority summary (same logic as CLI)
	type authSummary struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Resources int    `json:"resources"`
		Direct    int    `json:"direct"`
		Inherited int    `json:"inherited"`
	}
	byAuth := make(map[string]*authSummary)
	noAuthority := 0
	total := 0

	for _, result := range results {
		total++
		la := apiPrimaryAuthority(result)
		if la == nil {
			noAuthority++
			continue
		}
		if la.ResourceRole == "AuthorityRecord" {
			continue
		}
		key := la.Authority.Type + "/" + la.Authority.Name
		s, ok := byAuth[key]
		if !ok {
			s = &authSummary{Name: la.Authority.Name, Type: la.Authority.Type}
			byAuth[key] = s
		}
		s.Resources++
		if la.Attribution == engine.AttrDirect {
			s.Direct++
		} else {
			s.Inherited++
		}
	}

	var authorities []authSummary
	for _, s := range byAuth {
		authorities = append(authorities, *s)
	}

	respondJSON(w, map[string]any{
		"total":       total,
		"noAuthority": noAuthority,
		"authorities": authorities,
	})
}

func (s *Server) handleShapes(w http.ResponseWriter, r *http.Request) {
	ownerResults := s.resolver.ResolveAll(s.index)
	g := graph.Build(s.index, ownerResults)

	// Load default definitions and run matcher
	compiler := shape.NewCompiler()
	for _, def := range defaultShapeDefinitions() {
		compiler.Compile(def.Name, def.Spec, 1)
	}

	matcher := shape.NewMatcher(s.index, g)
	results := matcher.EvaluateAll(compiler.All())
	resolved := shape.ResolveConflicts(results)

	catalog := shape.NewCatalog()
	for _, result := range resolved {
		if result.Matched {
			def, _ := compiler.Get(result.Definition)
			if def != nil {
				catalog.AddInstance(&result, def)
			}
		}
	}

	var shapes []map[string]any
	for _, entry := range catalog.Shapes {
		var instances []map[string]any
		for _, inst := range entry.Instances {
			instances = append(instances, map[string]any{
				"rootKey":     inst.RootKey,
				"fingerprint": inst.Fingerprint,
			})
		}
		shapes = append(shapes, map[string]any{
			"shapeId":            entry.ShapeID,
			"role":               entry.Role,
			"fingerprint":        entry.Fingerprint,
			"definition":         entry.Definition,
			"classificationMode": entry.ClassificationMode,
			"instances":          instances,
		})
	}

	respondJSON(w, map[string]any{"shapes": shapes, "total": len(shapes)})
}

func (s *Server) handleCandidates(w http.ResponseWriter, r *http.Request) {
	ownerResults := s.resolver.ResolveAll(s.index)
	g := graph.Build(s.index, ownerResults)

	classifiedRoots := make(map[string]bool)
	subgraphs := shape.SegmentUnclassified(s.index, g, classifiedRoots)
	groups := shape.GroupCandidates(subgraphs, g)

	var items []map[string]any
	for _, grp := range groups {
		var instances []map[string]any
		for _, inst := range grp.Instances {
			instances = append(instances, map[string]any{
				"rootKey": inst.RootKey,
				"members": inst.Members,
			})
		}
		items = append(items, map[string]any{
			"id":         grp.ID,
			"semanticFP": grp.SemanticFP,
			"rootKind":   grp.RootKind,
			"instances":  instances,
			"evidence":   grp.Evidence,
			"commonCore": grp.CommonCore,
		})
	}
	respondJSON(w, map[string]any{"candidates": items, "total": len(items)})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	records := s.index.List()
	ownerResults := s.resolver.ResolveAll(s.index)
	g := graph.Build(s.index, ownerResults)

	ownerCounts := make(map[string]int)
	for _, result := range ownerResults {
		ownerCounts[string(result.Classification)]++
	}

	classifiedRoots := make(map[string]bool)
	subgraphs := shape.SegmentUnclassified(s.index, g, classifiedRoots)
	groups := shape.GroupCandidates(subgraphs, g)

	report := map[string]any{
		"resources": map[string]any{
			"total": len(records),
		},
		"ownership": map[string]any{
			"total":           len(ownerResults),
			"classifications": ownerCounts,
		},
		"candidates": map[string]any{
			"groups":    len(groups),
			"instances": countGroupInstances(groups),
		},
		"graph": map[string]any{
			"edges": g.EdgeCount(),
			"nodes": g.NodeCount(),
		},
	}

	respondJSON(w, report)
}

// handleGraph serves the full knowledge-graph snapshot with nodes, edges, and model metadata.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	ownerResults := s.resolver.ResolveAll(s.index)
	g := graph.Build(s.index, ownerResults)
	records := s.index.List()
	now := time.Now().UTC()

	includeCandidates := r.URL.Query().Get("include-candidates") == "true"

	// --- Nodes: resource nodes ---
	nodes := make([]map[string]any, 0, len(records)+5)
	for _, rec := range records {
		nodeID := s.logicalID(rec)
		node := map[string]any{
			"id":   nodeID,
			"type": "KubernetesResource",
			"resource": map[string]any{
				"clusterId":  s.clusterID,
				"apiVersion": apiVersionStr(rec),
				"kind":       rec.Identity.GVK.Kind,
				"namespace":  rec.Identity.Namespace,
				"name":       rec.Identity.Name,
				"uid":        string(rec.Identity.UID),
				"createdAt":  rec.Identity.CreatedAt.Format(time.RFC3339),
			},
		}
		if result, ok := ownerResults[rec.Key()]; ok {
			ow := map[string]any{
				"classification": string(result.Classification),
				"confidence":     string(result.Confidence),
			}
			if result.Owner != nil {
				ow["owner"] = map[string]any{
					"type":      result.Owner.Type,
					"name":      result.Owner.Name,
					"namespace": result.Owner.Namespace,
				}
			}
			node["ownership"] = ow
		}
		nodes = append(nodes, node)
	}

	// --- Nodes: classifier nodes ---
	classifierDefs := []struct {
		id   string
		role string
	}{
		{"classifier:kos-default-application", "application"},
		{"classifier:kos-default-node-system", "node-system"},
		{"classifier:kos-default-scheduled", "scheduled-workload"},
	}
	for _, c := range classifierDefs {
		nodes = append(nodes, map[string]any{
			"id":   c.id,
			"type": "RoleClassifier",
			"role": c.role,
		})
	}

	// --- Nodes: logical group nodes ---
	groups := grouping.BuildGroups(s.index, s.clusterID)
	logicalGroupCount := 0
	releaseGroupCount := 0
	for _, grp := range groups {
		nodes = append(nodes, map[string]any{
			"id":        grp.ID,
			"type":      "LogicalResourceGroup",
			"groupType": grp.GroupType,
			"name":      grp.Name,
			"scope": map[string]any{
				"clusterId": grp.Scope.ClusterID,
				"namespace": grp.Scope.HomeNamespace,
			},
			"identity": map[string]any{
				"strategy": grp.Identity.Strategy,
				"key":      grp.Identity.Key,
			},
			"confidence": grp.Confidence,
			"state":      grp.State,
		})
		switch grp.GroupType {
		case grouping.GroupTypeRelease:
			releaseGroupCount++
		default:
			logicalGroupCount++
		}
	}

	// --- Edges: relationship edges ---
	allEdges := g.AllEdges()
	edges := make([]map[string]any, 0, len(allEdges)+len(records))
	for _, e := range allEdges {
		fieldPath, observedValue := splitEvidence(e.Evidence)
		edge := map[string]any{
			"id":   edgeHashID(e),
			"type": string(e.Type),
			"from": s.nodeIDFromKey(e.Source),
			"to":   s.nodeIDFromKey(e.Target),
			"evidence": map[string]any{
				"type":          e.Confidence,
				"fieldPath":     fieldPath,
				"observedValue": observedValue,
			},
			"compositionRole": compositionRole(e.Type),
		}
		if graph.ClassifyRelationship(e.Type) == graph.LayerDefining {
			edge["model"] = "builtin:structural-composition-v1"
		}
		edges = append(edges, edge)
	}

	// --- Edges: ClassifiedAs (resource → classifier) ---
	// Run the RoleOnly matcher to determine which resources are classified
	classifiedAsEdges := s.buildClassifiedAsEdges(records)
	edges = append(edges, classifiedAsEdges...)

	// --- Edges: MemberOf (resource → logical group) ---
	memberOfEdges := s.buildMemberOfEdges(groups)
	edges = append(edges, memberOfEdges...)

	// --- Candidates (opt-in) ---
	if includeCandidates {
		classifiedRoots := make(map[string]bool)
		subgraphs := shape.SegmentUnclassified(s.index, g, classifiedRoots)
		groups := shape.GroupCandidates(subgraphs, g)
		for _, grp := range groups {
			nodes = append(nodes, map[string]any{
				"id":        "candidate:" + grp.ID,
				"type":      "CandidateShapeGroup",
				"rootKind":  grp.RootKind,
				"instances": len(grp.Instances),
				"evidence":  grp.Evidence,
			})
		}
	}

	snapshot := map[string]any{
		"schemaVersion": "1",
		"snapshot": map[string]any{
			"clusterId":             s.clusterID,
			"observedAt":            now.Format(time.RFC3339),
			"relationshipModel":     "builtin:structural-composition-v1",
			"canonicalizationModel": "generic-structural-v1@1",
			"traitModel":            "builtin:structural-v1",
		},
		"nodes": nodes,
		"edges": edges,
		"summary": map[string]any{
			"nodeCount":         len(nodes),
			"edgeCount":         len(edges),
			"resourceNodes":     len(records),
			"logicalGroupNodes": logicalGroupCount,
			"releaseGroupNodes": releaseGroupCount,
			"classifierNodes":   len(classifierDefs),
		},
	}

	respondJSON(w, snapshot)
}

func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		respondJSON(w, map[string]any{"findings": []any{}, "total": 0})
		return
	}

	ruleFilter := r.URL.Query().Get("rule")
	severityFilter := r.URL.Query().Get("severity")
	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		// Also accept the legacy "stage" parameter for backward compatibility
		statusFilter = r.URL.Query().Get("stage")
	}
	if statusFilter == "" {
		statusFilter = "Active"
	}

	findings, err := s.store.ListFindings(ruleFilter, severityFilter, statusFilter)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	respondJSON(w, map[string]any{"findings": findings, "total": len(findings)})
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if s.janitor == nil {
		respondJSON(w, map[string]any{"rules": []any{}, "total": 0})
		return
	}

	rules := s.janitor.Rules()
	var items []map[string]any
	for _, rule := range rules {
		item := map[string]any{
			"id":          rule.ID,
			"name":        rule.Name,
			"displayName": rule.DisplayName,
			"evaluator":   rule.Evaluator,
			"severity":    rule.Severity,
			"maxAction":   rule.MaxAction,
		}
		if rule.GracePeriod > 0 {
			item["gracePeriod"] = janitor.FormatDurationHuman(rule.GracePeriod)
		}
		if s.store != nil {
			active, _ := s.store.ActiveFindingCountByRule(rule.ID)
			resolved, _ := s.store.ResolvedFindingCountByRule(rule.ID)
			item["activeFindings"] = active
			item["resolvedFindings"] = resolved
		}
		items = append(items, item)
	}

	respondJSON(w, map[string]any{"rules": items, "total": len(items)})
}

// handleGroups serves the logical resource groups.
func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	groups := grouping.BuildGroups(s.index, s.clusterID)

	// Apply optional filters
	groupType := r.URL.Query().Get("type")
	namespace := r.URL.Query().Get("namespace")

	var filtered []*grouping.LogicalResourceGroup
	for _, g := range groups {
		if groupType != "" && g.GroupType != groupType {
			continue
		}
		if namespace != "" && g.Scope.HomeNamespace != namespace {
			continue
		}
		filtered = append(filtered, g)
	}

	respondJSON(w, map[string]any{"groups": filtered, "total": len(filtered)})
}

// buildMemberOfEdges creates MemberOf/MemberOfRelease edges from resources to logical groups.
func (s *Server) buildMemberOfEdges(groups []*grouping.LogicalResourceGroup) []map[string]any {
	var edges []map[string]any

	for _, grp := range groups {
		edgeType := "MemberOf"
		if grp.GroupType == grouping.GroupTypeRelease {
			edgeType = "MemberOfRelease"
		}

		for _, member := range grp.Members {
			fromID := s.nodeIDFromKey(member.ResourceKey)
			raw := fmt.Sprintf("%s→%s→%s", fromID, edgeType, grp.ID)
			hash := sha256.Sum256([]byte(raw))

			ev := map[string]any{
				"type": "LabelAssociation",
			}
			if len(member.Evidence) > 0 {
				ev["fieldPath"] = member.Evidence[0].FieldPath
				ev["observedValue"] = member.Evidence[0].ObservedValue
			}

			edges = append(edges, map[string]any{
				"id":              fmt.Sprintf("edge:sha256:%x", hash[:8]),
				"type":            edgeType,
				"from":            fromID,
				"to":              grp.ID,
				"evidence":        ev,
				"compositionRole": "Contextual",
				"model":           "builtin:logical-grouping-v1",
			})
		}
	}

	return edges
}

// buildClassifiedAsEdges creates ClassifiedAs edges from resources to their role classifiers.
func (s *Server) buildClassifiedAsEdges(records []*knowledge.ResourceRecord) []map[string]any {
	var edges []map[string]any

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

		nodeID := s.logicalID(rec)
		raw := fmt.Sprintf("%s→ClassifiedAs→%s", nodeID, classifierID)
		hash := sha256.Sum256([]byte(raw))

		edges = append(edges, map[string]any{
			"id":   fmt.Sprintf("edge:sha256:%x", hash[:8]),
			"type": "ClassifiedAs",
			"from": nodeID,
			"to":   classifierID,
			"evidence": map[string]any{
				"type":       "DefinitionMatch",
				"definition": classifierID[len("classifier:"):],
			},
			"compositionRole": "Taxonomic",
		})
	}

	return edges
}

// compositionRole returns the graph-consumer-friendly role category for an edge type.
func compositionRole(relType graph.RelationType) string {
	switch graph.ClassifyRelationship(relType) {
	case graph.LayerDefining:
		return "Defining"
	case graph.LayerFramework:
		return "Framework"
	case graph.LayerProvenance:
		return "Contextual"
	default:
		return "Defining"
	}
}

// compositionRoleForClassification returns the role for taxonomic edges.
func compositionRoleForClassification() string {
	return "Taxonomic"
}

// splitEvidence separates "fieldPath=observedValue" from evidence strings.
func splitEvidence(evidence string) (fieldPath, observedValue string) {
	for i, c := range evidence {
		if c == '=' {
			return evidence[:i], evidence[i+1:]
		}
	}
	return evidence, ""
}

// --- helpers ---

func (s *Server) logicalID(rec *knowledge.ResourceRecord) string {
	group := rec.Identity.GVK.Group
	if group == "" {
		group = "core"
	}
	if rec.Identity.Namespace != "" {
		return fmt.Sprintf("resource:%s/%s/%s/%s/%s",
			s.clusterID, group, rec.Identity.GVK.Kind, rec.Identity.Namespace, rec.Identity.Name)
	}
	return fmt.Sprintf("resource:%s/%s/%s/%s",
		s.clusterID, group, rec.Identity.GVK.Kind, rec.Identity.Name)
}

func (s *Server) nodeIDFromKey(key string) string {
	rec, ok := s.index.Get(key)
	if ok {
		return s.logicalID(rec)
	}
	return "resource:" + s.clusterID + "/" + key
}

func apiVersionStr(rec *knowledge.ResourceRecord) string {
	if rec.Identity.GVK.Group == "" {
		return "v1"
	}
	v := rec.Identity.GVK.Version
	if v == "" {
		v = "v1"
	}
	return rec.Identity.GVK.Group + "/" + v
}

func edgeHashID(e graph.Edge) string {
	raw := fmt.Sprintf("%s→%s→%s→%s", e.Source, e.Type, e.Target, e.Evidence)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("edge:sha256:%x", hash[:8])
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func countGroupInstances(groups []*shape.CandidateShapeGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Instances)
	}
	return total
}

// defaultShapeDefinitions returns built-in shape definitions for the API.
func defaultShapeDefinitions() []struct {
	Name string
	Spec v1alpha1.ShapeDefinitionSpec
} {
	return []struct {
		Name string
		Spec v1alpha1.ShapeDefinitionSpec
	}{
		{
			Name: "kos-default-application",
			Spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion: 1, DefinitionVersion: 1,
				DisplayName: "Application", Role: "application", Priority: 100,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "workload",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"Deployment", "StatefulSet"}},
				}},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
			},
		},
		{
			Name: "kos-default-node-system",
			Spec: v1alpha1.ShapeDefinitionSpec{
				SchemaVersion: 1, DefinitionVersion: 1,
				DisplayName: "Node System", Role: "node-system", Priority: 100,
				Roots: []v1alpha1.RootSpec{{
					Alias:    "agent",
					Resource: v1alpha1.ResourceSelector{APIGroups: []string{"apps"}, Kinds: []string{"DaemonSet"}},
				}},
				Composition: v1alpha1.CompositionSpec{UnmatchedResources: "Ignore"},
			},
		},
	}
}

// apiPrimaryAuthority returns the most relevant authority layer from an ownership result.
func apiPrimaryAuthority(r *engine.OwnershipResult) *engine.LayerResult {
	if r.LifecycleAuthority != nil {
		return r.LifecycleAuthority
	}
	if r.AuthorityRecord != nil {
		return r.AuthorityRecord
	}
	if r.HigherLevelReconciler != nil {
		return r.HigherLevelReconciler
	}
	if r.RuntimeController != nil {
		return r.RuntimeController
	}
	return nil
}
