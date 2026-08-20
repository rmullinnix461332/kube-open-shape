package graph

import "sync"

// Graph is a thread-safe directed graph of resource relationships
type Graph struct {
	mu       sync.RWMutex
	outgoing map[string][]Edge // source -> edges
	incoming map[string][]Edge // target -> edges
}

// New creates an empty graph
func New() *Graph {
	return &Graph{
		outgoing: make(map[string][]Edge),
		incoming: make(map[string][]Edge),
	}
}

// AddEdge adds a directed edge to the graph
func (g *Graph) AddEdge(edge Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Avoid duplicates
	for _, e := range g.outgoing[edge.Source] {
		if e.Target == edge.Target && e.Type == edge.Type {
			return
		}
	}

	g.outgoing[edge.Source] = append(g.outgoing[edge.Source], edge)
	g.incoming[edge.Target] = append(g.incoming[edge.Target], edge)
}

// RemoveNode removes all edges involving a node
func (g *Graph) RemoveNode(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Remove outgoing edges and their incoming references
	for _, edge := range g.outgoing[key] {
		g.removeFromIncoming(edge.Target, key)
	}
	delete(g.outgoing, key)

	// Remove incoming edges and their outgoing references
	for _, edge := range g.incoming[key] {
		g.removeFromOutgoing(edge.Source, key)
	}
	delete(g.incoming, key)
}

// OutgoingEdges returns all edges from a source
func (g *Graph) OutgoingEdges(source string) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	edges := make([]Edge, len(g.outgoing[source]))
	copy(edges, g.outgoing[source])
	return edges
}

// IncomingEdges returns all edges to a target
func (g *Graph) IncomingEdges(target string) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	edges := make([]Edge, len(g.incoming[target]))
	copy(edges, g.incoming[target])
	return edges
}

// EdgesOfType returns all edges from a source of a given type
func (g *Graph) EdgesOfType(source string, relType RelationType) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []Edge
	for _, e := range g.outgoing[source] {
		if e.Type == relType {
			result = append(result, e)
		}
	}
	return result
}

// AllEdges returns all edges in the graph
func (g *Graph) AllEdges() []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var all []Edge
	for _, edges := range g.outgoing {
		all = append(all, edges...)
	}
	return all
}

// NodeCount returns the number of unique nodes
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	nodes := make(map[string]bool)
	for k := range g.outgoing {
		nodes[k] = true
	}
	for k := range g.incoming {
		nodes[k] = true
	}
	return len(nodes)
}

// EdgeCount returns the total number of edges
func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	count := 0
	for _, edges := range g.outgoing {
		count += len(edges)
	}
	return count
}

func (g *Graph) removeFromIncoming(target, source string) {
	edges := g.incoming[target]
	filtered := edges[:0]
	for _, e := range edges {
		if e.Source != source {
			filtered = append(filtered, e)
		}
	}
	g.incoming[target] = filtered
}

func (g *Graph) removeFromOutgoing(source, target string) {
	edges := g.outgoing[source]
	filtered := edges[:0]
	for _, e := range edges {
		if e.Target != target {
			filtered = append(filtered, e)
		}
	}
	g.outgoing[source] = filtered
}
