package graph

// Reachable returns all nodes reachable from a source via BFS, with max depth
func (g *Graph) Reachable(source string, maxDepth int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	visited[source] = true

	type item struct {
		key   string
		depth int
	}

	queue := []item{{key: source, depth: 0}}
	var result []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth > 0 {
			result = append(result, current.key)
		}

		if current.depth >= maxDepth {
			continue
		}

		for _, edge := range g.outgoing[current.key] {
			if !visited[edge.Target] {
				visited[edge.Target] = true
				queue = append(queue, item{key: edge.Target, depth: current.depth + 1})
			}
		}
	}

	return result
}

// ReachableWithEdges returns all edges reachable from a source via BFS
func (g *Graph) ReachableWithEdges(source string, maxDepth int) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	visited[source] = true

	type item struct {
		key   string
		depth int
	}

	queue := []item{{key: source, depth: 0}}
	var result []Edge

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth >= maxDepth {
			continue
		}

		for _, edge := range g.outgoing[current.key] {
			result = append(result, edge)
			if !visited[edge.Target] {
				visited[edge.Target] = true
				queue = append(queue, item{key: edge.Target, depth: current.depth + 1})
			}
		}
	}

	return result
}

// Ancestors returns all nodes that can reach the target via incoming edges (reverse BFS)
func (g *Graph) Ancestors(target string, maxDepth int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[string]bool)
	visited[target] = true

	type item struct {
		key   string
		depth int
	}

	queue := []item{{key: target, depth: 0}}
	var result []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth > 0 {
			result = append(result, current.key)
		}

		if current.depth >= maxDepth {
			continue
		}

		for _, edge := range g.incoming[current.key] {
			if !visited[edge.Source] {
				visited[edge.Source] = true
				queue = append(queue, item{key: edge.Source, depth: current.depth + 1})
			}
		}
	}

	return result
}
