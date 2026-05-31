package vsdx

import (
	"container/heap"
	"math"
)

// Router handles automatic connector routing on a page.
type Router struct {
	page      *Page
	obstacles []Rect   // Bounding boxes of shapes to avoid
	gridSize  float64  // Grid cell size in inches
	padding   float64  // Minimum distance from obstacles
}

// RouteOptions configures the routing algorithm.
type RouteOptions struct {
	Orthogonal bool    // Only allow horizontal/vertical lines
	Padding    float64 // Minimum distance from obstacles (default: 0.1)
	MaxBends   int     // Maximum number of bends (0 = unlimited)
	GridSize   float64 // Grid cell size (default: 0.1)
}

// DefaultRouteOptions returns sensible default routing options.
func DefaultRouteOptions() RouteOptions {
	return RouteOptions{
		Orthogonal: true,
		Padding:    0.1,
		MaxBends:   10,
		GridSize:   0.1,
	}
}

// NewRouter creates a new router for the given page.
func NewRouter(page *Page) *Router {
	r := &Router{
		page:     page,
		gridSize: 0.1,
		padding:  0.1,
	}
	r.buildObstacles()
	return r
}

// buildObstacles collects bounding boxes of all shapes on the page.
func (r *Router) buildObstacles() {
	r.obstacles = nil
	for _, shape := range r.page.AllShapes() {
		// Skip connectors (1D shapes).
		if shape.CellValue("ObjType") == "2" {
			continue
		}
		bbox := shape.BoundingBox()
		if bbox.Width > 0 && bbox.Height > 0 {
			// Add padding to the bounding box.
			bbox.X -= r.padding
			bbox.Y -= r.padding
			bbox.Width += 2 * r.padding
			bbox.Height += 2 * r.padding
			r.obstacles = append(r.obstacles, bbox)
		}
	}
}

// Route calculates a path between two points that avoids obstacles.
// Returns a slice of waypoints including the start and end points.
func (r *Router) Route(from, to Point, opts RouteOptions) []Point {
	if opts.GridSize > 0 {
		r.gridSize = opts.GridSize
	}
	if opts.Padding > 0 {
		r.padding = opts.Padding
		r.buildObstacles() // Rebuild with new padding
	}

	if opts.Orthogonal {
		return r.routeOrthogonal(from, to, opts)
	}
	return r.routeAStar(from, to, opts)
}

// routeOrthogonal finds an orthogonal (right-angle only) path.
func (r *Router) routeOrthogonal(from, to Point, opts RouteOptions) []Point {
	// Simple orthogonal routing: try horizontal first, then vertical.
	// If blocked, try vertical first, then horizontal.

	// Direct line check.
	if !r.intersectsObstacle(from, to) {
		return []Point{from, to}
	}

	// Try L-shaped routes.
	mid1 := Point{X: to.X, Y: from.Y}
	if !r.intersectsObstacle(from, mid1) && !r.intersectsObstacle(mid1, to) {
		return []Point{from, mid1, to}
	}

	mid2 := Point{X: from.X, Y: to.Y}
	if !r.intersectsObstacle(from, mid2) && !r.intersectsObstacle(mid2, to) {
		return []Point{from, mid2, to}
	}

	// Try Z-shaped routes (3 segments).
	midX := (from.X + to.X) / 2
	midY := (from.Y + to.Y) / 2

	// Horizontal-Vertical-Horizontal.
	p1 := Point{X: midX, Y: from.Y}
	p2 := Point{X: midX, Y: to.Y}
	if !r.intersectsObstacle(from, p1) && !r.intersectsObstacle(p1, p2) && !r.intersectsObstacle(p2, to) {
		return r.simplifyPath([]Point{from, p1, p2, to})
	}

	// Vertical-Horizontal-Vertical.
	p1 = Point{X: from.X, Y: midY}
	p2 = Point{X: to.X, Y: midY}
	if !r.intersectsObstacle(from, p1) && !r.intersectsObstacle(p1, p2) && !r.intersectsObstacle(p2, to) {
		return r.simplifyPath([]Point{from, p1, p2, to})
	}

	// Fall back to A* pathfinding for complex cases.
	return r.routeAStar(from, to, opts)
}

// routeAStar uses A* pathfinding to find a path.
func (r *Router) routeAStar(from, to Point, opts RouteOptions) []Point {
	// Grid-based A* pathfinding.
	startNode := &routeNode{p: from, g: 0, h: r.heuristic(from, to)}
	openSet := &nodeHeap{startNode}
	heap.Init(openSet)

	closedSet := make(map[Point]bool)
	nodeMap := make(map[Point]*routeNode)
	nodeMap[from] = startNode

	directions := []Point{
		{X: r.gridSize, Y: 0},
		{X: -r.gridSize, Y: 0},
		{X: 0, Y: r.gridSize},
		{X: 0, Y: -r.gridSize},
	}
	if !opts.Orthogonal {
		// Add diagonal directions.
		diag := r.gridSize * 0.7071 // sqrt(2)/2
		directions = append(directions,
			Point{X: diag, Y: diag},
			Point{X: diag, Y: -diag},
			Point{X: -diag, Y: diag},
			Point{X: -diag, Y: -diag},
		)
	}

	maxIterations := 10000
	for openSet.Len() > 0 && maxIterations > 0 {
		maxIterations--

		current := heap.Pop(openSet).(*routeNode)

		// Check if we've reached the goal (within grid size).
		if r.distance(current.p, to) < r.gridSize {
			// Reconstruct path.
			path := []Point{to}
			for n := current; n != nil; n = n.parent {
				path = append([]Point{n.p}, path...)
			}
			return r.simplifyPath(path)
		}

		closedSet[current.p] = true

		for _, dir := range directions {
			neighbor := Point{X: current.p.X + dir.X, Y: current.p.Y + dir.Y}

			if closedSet[neighbor] {
				continue
			}

			// Check if this segment intersects an obstacle.
			if r.intersectsObstacle(current.p, neighbor) {
				continue
			}

			g := current.g + r.distance(current.p, neighbor)

			existingNode, exists := nodeMap[neighbor]
			if exists && g >= existingNode.g {
				continue
			}

			if exists {
				existingNode.g = g
				existingNode.parent = current
				heap.Fix(openSet, existingNode.index)
			} else {
				newNode := &routeNode{
					p:      neighbor,
					g:      g,
					h:      r.heuristic(neighbor, to),
					parent: current,
				}
				heap.Push(openSet, newNode)
				nodeMap[neighbor] = newNode
			}
		}
	}

	// No path found - return direct line.
	return []Point{from, to}
}

// heuristic calculates the estimated cost from a to b.
func (r *Router) heuristic(a, b Point) float64 {
	return math.Abs(b.X-a.X) + math.Abs(b.Y-a.Y) // Manhattan distance
}

// distance calculates the actual distance between two points.
func (r *Router) distance(a, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// intersectsObstacle checks if a line segment intersects any obstacle.
func (r *Router) intersectsObstacle(from, to Point) bool {
	for _, obs := range r.obstacles {
		if r.lineIntersectsRect(from, to, obs) {
			return true
		}
	}
	return false
}

// lineIntersectsRect checks if a line segment intersects a rectangle.
func (r *Router) lineIntersectsRect(p1, p2 Point, rect Rect) bool {
	// Check if either point is inside the rectangle.
	if r.pointInRect(p1, rect) || r.pointInRect(p2, rect) {
		return true
	}

	// Check intersection with each edge of the rectangle.
	topLeft := Point{X: rect.X, Y: rect.Y + rect.Height}
	topRight := Point{X: rect.X + rect.Width, Y: rect.Y + rect.Height}
	bottomLeft := Point{X: rect.X, Y: rect.Y}
	bottomRight := Point{X: rect.X + rect.Width, Y: rect.Y}

	return r.linesIntersect(p1, p2, topLeft, topRight) ||
		r.linesIntersect(p1, p2, topRight, bottomRight) ||
		r.linesIntersect(p1, p2, bottomRight, bottomLeft) ||
		r.linesIntersect(p1, p2, bottomLeft, topLeft)
}

// pointInRect checks if a point is inside a rectangle.
func (r *Router) pointInRect(p Point, rect Rect) bool {
	return p.X >= rect.X && p.X <= rect.X+rect.Width &&
		p.Y >= rect.Y && p.Y <= rect.Y+rect.Height
}

// linesIntersect checks if two line segments intersect.
func (r *Router) linesIntersect(p1, p2, p3, p4 Point) bool {
	d1 := r.direction(p3, p4, p1)
	d2 := r.direction(p3, p4, p2)
	d3 := r.direction(p1, p2, p3)
	d4 := r.direction(p1, p2, p4)

	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}

	if d1 == 0 && r.onSegment(p3, p4, p1) {
		return true
	}
	if d2 == 0 && r.onSegment(p3, p4, p2) {
		return true
	}
	if d3 == 0 && r.onSegment(p1, p2, p3) {
		return true
	}
	if d4 == 0 && r.onSegment(p1, p2, p4) {
		return true
	}

	return false
}

// direction calculates the cross product direction.
func (r *Router) direction(p1, p2, p3 Point) float64 {
	return (p3.X-p1.X)*(p2.Y-p1.Y) - (p2.X-p1.X)*(p3.Y-p1.Y)
}

// onSegment checks if point p is on the line segment p1-p2.
func (r *Router) onSegment(p1, p2, p Point) bool {
	return p.X <= math.Max(p1.X, p2.X) && p.X >= math.Min(p1.X, p2.X) &&
		p.Y <= math.Max(p1.Y, p2.Y) && p.Y >= math.Min(p1.Y, p2.Y)
}

// simplifyPath removes redundant waypoints from a path.
func (r *Router) simplifyPath(path []Point) []Point {
	if len(path) <= 2 {
		return path
	}

	simplified := []Point{path[0]}

	for i := 1; i < len(path)-1; i++ {
		prev := simplified[len(simplified)-1]
		curr := path[i]
		next := path[i+1]

		// Check if curr is collinear with prev and next.
		if !r.isCollinear(prev, curr, next) {
			simplified = append(simplified, curr)
		}
	}

	simplified = append(simplified, path[len(path)-1])
	return simplified
}

// isCollinear checks if three points are collinear (on the same line).
func (r *Router) isCollinear(p1, p2, p3 Point) bool {
	area := (p2.X-p1.X)*(p3.Y-p1.Y) - (p3.X-p1.X)*(p2.Y-p1.Y)
	return math.Abs(area) < 0.0001
}

// ApplyRoute applies a route to a connector shape by setting its geometry.
func (r *Router) ApplyRoute(connector *Shape, points []Point) {
	if len(points) < 2 {
		return
	}

	// Clear existing geometry.
	connector.Geometries = nil
	connector.Geometry = nil

	// Create new geometry section.
	geom := connector.AddGeometry()

	// Add MoveTo for start point.
	geom.AddMoveTo(points[0].X, points[0].Y)

	// Add LineTo for each subsequent point.
	for i := 1; i < len(points); i++ {
		geom.AddLineTo(points[i].X, points[i].Y)
	}
}

// AutoRouteConnectors routes all connectors on the page.
func (p *Page) AutoRouteConnectors() {
	router := NewRouter(p)
	opts := DefaultRouteOptions()

	for _, shape := range p.AllShapes() {
		// Check if this is a connector (1D shape).
		if shape.CellValue("ObjType") != "2" {
			continue
		}

		// Get begin and end points.
		beginX := toFloat(shape.CellValue("BeginX"))
		beginY := toFloat(shape.CellValue("BeginY"))
		endX := toFloat(shape.CellValue("EndX"))
		endY := toFloat(shape.CellValue("EndY"))

		from := Point{X: beginX, Y: beginY}
		to := Point{X: endX, Y: endY}

		// Calculate route.
		route := router.Route(from, to, opts)

		// Apply route to connector.
		router.ApplyRoute(shape, route)
	}
}

// routeNode represents a node in the A* pathfinding algorithm.
type routeNode struct {
	p      Point
	g, h   float64
	parent *routeNode
	index  int
}

// nodeHeap implements heap.Interface for A* pathfinding.
type nodeHeap []*routeNode

func (h nodeHeap) Len() int { return len(h) }

func (h nodeHeap) Less(i, j int) bool {
	return h[i].g+h[i].h < h[j].g+h[j].h
}

func (h nodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *nodeHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*routeNode)
	item.index = n
	*h = append(*h, item)
}

func (h *nodeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}
