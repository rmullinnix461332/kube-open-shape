package graph

import "testing"

func TestGraph_AddAndQuery(t *testing.T) {
	g := New()

	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns, Evidence: "ownerRef"})
	g.AddEdge(Edge{Source: "A", Target: "C", Type: UsesServiceAccount, Evidence: "serviceAccountName"})
	g.AddEdge(Edge{Source: "B", Target: "D", Type: Mounts, Evidence: "volumeMount"})

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "outgoing edges",
			fn: func(t *testing.T) {
				edges := g.OutgoingEdges("A")
				if len(edges) != 2 {
					t.Errorf("got %d edges from A, want 2", len(edges))
				}
			},
		},
		{
			name: "incoming edges",
			fn: func(t *testing.T) {
				edges := g.IncomingEdges("B")
				if len(edges) != 1 {
					t.Errorf("got %d edges to B, want 1", len(edges))
				}
				if edges[0].Source != "A" {
					t.Errorf("source = %q, want %q", edges[0].Source, "A")
				}
			},
		},
		{
			name: "edges of type",
			fn: func(t *testing.T) {
				edges := g.EdgesOfType("A", Owns)
				if len(edges) != 1 {
					t.Errorf("got %d Owns edges from A, want 1", len(edges))
				}
			},
		},
		{
			name: "edge count",
			fn: func(t *testing.T) {
				if g.EdgeCount() != 3 {
					t.Errorf("edge count = %d, want 3", g.EdgeCount())
				}
			},
		},
		{
			name: "no duplicate edges",
			fn: func(t *testing.T) {
				g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns, Evidence: "ownerRef"})
				if g.EdgeCount() != 3 {
					t.Errorf("duplicate added, edge count = %d, want 3", g.EdgeCount())
				}
			},
		},
		{
			name: "remove node",
			fn: func(t *testing.T) {
				g2 := New()
				g2.AddEdge(Edge{Source: "X", Target: "Y", Type: Owns})
				g2.AddEdge(Edge{Source: "Y", Target: "Z", Type: UsesServiceAccount})
				g2.RemoveNode("Y")
				if len(g2.OutgoingEdges("X")) != 0 {
					t.Error("expected edge X->Y removed")
				}
				if len(g2.OutgoingEdges("Y")) != 0 {
					t.Error("expected edge Y->Z removed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestGraph_Reachable(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns})
	g.AddEdge(Edge{Source: "B", Target: "C", Type: UsesServiceAccount})
	g.AddEdge(Edge{Source: "B", Target: "D", Type: Mounts})
	g.AddEdge(Edge{Source: "C", Target: "E", Type: References})

	tests := []struct {
		name    string
		source  string
		depth   int
		wantLen int
	}{
		{name: "full depth", source: "A", depth: 10, wantLen: 4},
		{name: "depth 1", source: "A", depth: 1, wantLen: 1},
		{name: "depth 2", source: "A", depth: 2, wantLen: 3},
		{name: "leaf node", source: "E", depth: 10, wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reachable := g.Reachable(tt.source, tt.depth)
			if len(reachable) != tt.wantLen {
				t.Errorf("reachable from %s depth=%d: got %d, want %d (%v)", tt.source, tt.depth, len(reachable), tt.wantLen, reachable)
			}
		})
	}
}

func TestGraph_Ancestors(t *testing.T) {
	g := New()
	g.AddEdge(Edge{Source: "A", Target: "B", Type: Owns})
	g.AddEdge(Edge{Source: "B", Target: "C", Type: UsesServiceAccount})

	ancestors := g.Ancestors("C", 10)
	if len(ancestors) != 2 {
		t.Errorf("ancestors of C: got %d, want 2", len(ancestors))
	}
}
