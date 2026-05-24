package vsdx

import (
	"fmt"
	"math"
	"strings"
)

// RenderIssue represents an issue found during render validation.
type RenderIssue struct {
	ShapeID  string
	Category string
	Message  string
	Severity string // "error", "warning", "info"
}

func (e RenderIssue) String() string {
	return fmt.Sprintf("[%s] Shape %s: %s - %s", e.Severity, e.ShapeID, e.Category, e.Message)
}

// RenderValidation holds all issues from a render validation run.
type RenderValidation struct {
	Errors   []RenderIssue
	Warnings []RenderIssue
	Info     []RenderIssue
}

func (r *RenderValidation) AddError(shapeID, category, message string) {
	r.Errors = append(r.Errors, RenderIssue{shapeID, category, message, "error"})
}

func (r *RenderValidation) AddWarning(shapeID, category, message string) {
	r.Warnings = append(r.Warnings, RenderIssue{shapeID, category, message, "warning"})
}

func (r *RenderValidation) AddInfo(shapeID, category, message string) {
	r.Info = append(r.Info, RenderIssue{shapeID, category, message, "info"})
}

func (r *RenderValidation) HasErrors() bool {
	return len(r.Errors) > 0
}

func (r *RenderValidation) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("RenderValidation: %d errors, %d warnings, %d info\n",
		len(r.Errors), len(r.Warnings), len(r.Info)))

	for _, e := range r.Errors {
		sb.WriteString("  " + e.String() + "\n")
	}
	for _, w := range r.Warnings {
		sb.WriteString("  " + w.String() + "\n")
	}
	return sb.String()
}

// ValidateConnectorEndpoints checks that connector endpoints are correctly positioned.
func ValidateConnectorEndpoints(page *Page) *RenderValidation {
	result := &RenderValidation{}

	for _, shape := range page.AllShapes() {
		// Check if this is a connector
		beginX := shape.CellValue("BeginX")
		endX := shape.CellValue("EndX")
		if beginX == "" || endX == "" {
			continue
		}

		// Check for connection references
		connects := page.Connects()
		var beginConnect, endConnect *Connect
		for _, c := range connects {
			if c.FromID == shape.ID {
				if c.FromRel == "BeginX" {
					beginConnect = c
				} else if c.FromRel == "EndX" {
					endConnect = c
				}
			}
		}

		// Validate BeginX connection
		if beginConnect != nil {
			targetShape := page.FindShapeByID(beginConnect.ToID)
			if targetShape == nil {
				result.AddError(shape.ID, "connector",
					fmt.Sprintf("BeginX connected to non-existent shape %s", beginConnect.ToID))
			} else {
				// Verify endpoint is within target bounds
				bx := toFloat(beginX)
				by := toFloat(shape.CellValue("BeginY"))
				if !isPointInShapeBounds(bx, by, targetShape) {
					result.AddWarning(shape.ID, "connector",
						fmt.Sprintf("BeginX (%.2f,%.2f) may be outside target shape %s bounds",
							bx, by, beginConnect.ToID))
				}
			}
		}

		// Validate EndX connection
		if endConnect != nil {
			targetShape := page.FindShapeByID(endConnect.ToID)
			if targetShape == nil {
				result.AddError(shape.ID, "connector",
					fmt.Sprintf("EndX connected to non-existent shape %s", endConnect.ToID))
			} else {
				ex := toFloat(endX)
				ey := toFloat(shape.CellValue("EndY"))
				if !isPointInShapeBounds(ex, ey, targetShape) {
					result.AddWarning(shape.ID, "connector",
						fmt.Sprintf("EndX (%.2f,%.2f) may be outside target shape %s bounds",
							ex, ey, endConnect.ToID))
				}
			}
		}

		// Check for dangling connectors
		if beginConnect == nil && endConnect == nil {
			result.AddInfo(shape.ID, "connector", "Connector has no connections")
		}
	}

	return result
}

func isPointInShapeBounds(x, y float64, shape *Shape) bool {
	// Get shape bounds in page coordinates
	pinX := shape.X()
	pinY := shape.Y()
	locPinX := shape.LocX()
	locPinY := shape.LocY()
	width := math.Abs(shape.Width())
	height := math.Abs(shape.Height())

	// Shape bounds
	left := pinX - locPinX
	right := left + width
	bottom := pinY - locPinY
	top := bottom + height

	// Allow some tolerance
	eps := 0.1
	return x >= left-eps && x <= right+eps && y >= bottom-eps && y <= top+eps
}

// ValidateZOrder checks that z-order is consistent.
func ValidateZOrder(page *Page) *RenderValidation {
	result := &RenderValidation{}

	shapes := page.AllShapes()
	if len(shapes) == 0 {
		return result
	}

	// Check for explicit z-order properties
	hasZOrder := false
	for _, shape := range shapes {
		if shape.CellValue("OrderIndex") != "" {
			hasZOrder = true
			break
		}
	}

	if !hasZOrder {
		result.AddInfo("", "z-order", "No explicit OrderIndex cells found; using document order")
	}

	// Check for z-order conflicts in same parent
	parentChildren := make(map[string][]*Shape)
	for _, shape := range shapes {
		parentID := ""
		if parent := shape.ParentShape(); parent != nil {
			parentID = parent.ID
		}
		parentChildren[parentID] = append(parentChildren[parentID], shape)
	}

	for parentID, children := range parentChildren {
		orders := make(map[int][]*Shape)
		for _, child := range children {
			orderStr := child.CellValue("OrderIndex")
			if orderStr != "" {
				order := int(toFloat(orderStr))
				orders[order] = append(orders[order], child)
			}
		}

		for order, shapes := range orders {
			if len(shapes) > 1 {
				ids := make([]string, len(shapes))
				for i, s := range shapes {
					ids[i] = s.ID
				}
				result.AddWarning(parentID, "z-order",
					fmt.Sprintf("Duplicate OrderIndex %d for shapes: %v", order, ids))
			}
		}
	}

	return result
}

// ValidateTransforms checks transform consistency.
func ValidateTransforms(shape *Shape) *RenderValidation {
	result := &RenderValidation{}
	transforms := ComputeGroupTransforms(shape)

	validateTransformRecursive(shape, transforms, result)

	return result
}

func validateTransformRecursive(shape *Shape, transforms map[string]*ShapeTransform, result *RenderValidation) {
	st := transforms[shape.ID]
	if st == nil {
		result.AddError(shape.ID, "transform", "No transform computed")
		return
	}

	// Check for degenerate transforms
	det := st.World.A*st.World.D - st.World.B*st.World.C
	if math.Abs(det) < 1e-10 {
		result.AddError(shape.ID, "transform", "Degenerate world transform (determinant near zero)")
	}

	// Check for very small bounds
	if st.WorldBounds.Width < 0.001 || st.WorldBounds.Height < 0.001 {
		result.AddWarning(shape.ID, "transform",
			fmt.Sprintf("Very small world bounds: %.4fx%.4f", st.WorldBounds.Width, st.WorldBounds.Height))
	}

	// Check for unreasonable scale
	scaleX := math.Sqrt(st.World.A*st.World.A + st.World.B*st.World.B)
	scaleY := math.Sqrt(st.World.C*st.World.C + st.World.D*st.World.D)
	if scaleX > 1000 || scaleY > 1000 {
		result.AddWarning(shape.ID, "transform",
			fmt.Sprintf("Very large scale factor: %.1fx%.1f", scaleX, scaleY))
	}
	if scaleX < 0.001 || scaleY < 0.001 {
		result.AddWarning(shape.ID, "transform",
			fmt.Sprintf("Very small scale factor: %.4fx%.4f", scaleX, scaleY))
	}

	// Recurse to children
	for _, child := range shape.ChildShapes() {
		validateTransformRecursive(child, transforms, result)
	}
}

// ValidateArrowSetbacks checks that arrow setbacks are reasonable.
func ValidateArrowSetbacks(page *Page) *RenderValidation {
	result := &RenderValidation{}

	for _, shape := range page.AllShapes() {
		es := shape.ComputeEffectiveStyle()

		if es.BeginArrow > 0 || es.EndArrow > 0 {
			// Check that setback doesn't exceed shape dimensions
			width := math.Abs(shape.Width())
			height := math.Abs(shape.Height())
			pathLen := math.Sqrt(width*width + height*height)

			if es.BeginArrowSetback > pathLen*0.5 {
				result.AddWarning(shape.ID, "arrow",
					fmt.Sprintf("BeginArrowSetback (%.2f) > 50%% of path length (%.2f)",
						es.BeginArrowSetback, pathLen))
			}
			if es.EndArrowSetback > pathLen*0.5 {
				result.AddWarning(shape.ID, "arrow",
					fmt.Sprintf("EndArrowSetback (%.2f) > 50%% of path length (%.2f)",
						es.EndArrowSetback, pathLen))
			}

			// Check that total setback doesn't exceed path
			totalSetback := es.BeginArrowSetback + es.EndArrowSetback
			if totalSetback > pathLen*0.8 {
				result.AddError(shape.ID, "arrow",
					fmt.Sprintf("Total arrow setback (%.2f) > 80%% of path length (%.2f)",
						totalSetback, pathLen))
			}
		}
	}

	return result
}

// ValidateRenderTree checks a render tree for issues.
func ValidateRenderTree(tree *RenderNode) *RenderValidation {
	result := &RenderValidation{}
	validateNodeRecursive(tree, result, make(map[int]bool))
	return result
}

func validateNodeRecursive(node *RenderNode, result *RenderValidation, usedZOrders map[int]bool) {
	// Check for z-order conflicts
	if usedZOrders[node.ZOrder] && node.Visible {
		result.AddWarning(node.Shape.ID, "render-tree",
			fmt.Sprintf("Duplicate z-order %d", node.ZOrder))
	}
	usedZOrders[node.ZOrder] = true

	// Check for missing style
	if node.Style == nil {
		result.AddError(node.Shape.ID, "render-tree", "No effective style computed")
	}

	// Check for missing transform
	if node.Transform == nil {
		result.AddError(node.Shape.ID, "render-tree", "No transform computed")
	}

	// Check visible nodes have geometry or text
	if node.Visible && len(node.Geometry) == 0 && node.Text == nil && len(node.Children) == 0 {
		result.AddInfo(node.Shape.ID, "render-tree", "Visible node has no geometry, text, or children")
	}

	// Recurse
	for _, child := range node.Children {
		validateNodeRecursive(child, result, usedZOrders)
	}
}
