package vsdx

import (
	"fmt"
	"math"
	"strings"
)

// WorldTransform represents a 2D affine transformation matrix.
// The matrix is stored as [a, b, c, d, e, f] representing:
//
//	| a  c  e |
//	| b  d  f |
//	| 0  0  1 |
//
// This transforms point (x, y) to (a*x + c*y + e, b*x + d*y + f).
type WorldTransform struct {
	A, B, C, D, E, F float64
}

// Identity returns the identity transform.
func Identity() WorldTransform {
	return WorldTransform{A: 1, D: 1}
}

// Translate returns a translation transform.
func Translate(dx, dy float64) WorldTransform {
	return WorldTransform{A: 1, D: 1, E: dx, F: dy}
}

// Scale returns a scaling transform.
func Scale(sx, sy float64) WorldTransform {
	return WorldTransform{A: sx, D: sy}
}

// Rotate returns a rotation transform (angle in radians, counter-clockwise).
func Rotate(angle float64) WorldTransform {
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	return WorldTransform{A: cos, B: sin, C: -sin, D: cos}
}

// RotateAround returns a rotation transform around a center point.
func RotateAround(angle, cx, cy float64) WorldTransform {
	// Translate to origin, rotate, translate back
	t1 := Translate(-cx, -cy)
	r := Rotate(angle)
	t2 := Translate(cx, cy)
	return t2.Multiply(r.Multiply(t1))
}

// Multiply returns this transform multiplied by another: this * other.
// This applies 'other' first, then 'this'.
func (t WorldTransform) Multiply(other WorldTransform) WorldTransform {
	return WorldTransform{
		A: t.A*other.A + t.C*other.B,
		B: t.B*other.A + t.D*other.B,
		C: t.A*other.C + t.C*other.D,
		D: t.B*other.C + t.D*other.D,
		E: t.A*other.E + t.C*other.F + t.E,
		F: t.B*other.E + t.D*other.F + t.F,
	}
}

// Apply transforms a point.
func (t WorldTransform) Apply(x, y float64) (float64, float64) {
	return t.A*x + t.C*y + t.E, t.B*x + t.D*y + t.F
}

// ApplyToRect transforms a rectangle, returning axis-aligned bounding box.
func (t WorldTransform) ApplyToRect(r Rect) Rect {
	// Transform all four corners
	corners := [][2]float64{
		{r.X, r.Y},
		{r.X + r.Width, r.Y},
		{r.X, r.Y + r.Height},
		{r.X + r.Width, r.Y + r.Height},
	}

	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64

	for _, c := range corners {
		tx, ty := t.Apply(c[0], c[1])
		if tx < minX {
			minX = tx
		}
		if tx > maxX {
			maxX = tx
		}
		if ty < minY {
			minY = ty
		}
		if ty > maxY {
			maxY = ty
		}
	}

	return Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

// Inverse returns the inverse transform.
func (t WorldTransform) Inverse() WorldTransform {
	det := t.A*t.D - t.B*t.C
	if math.Abs(det) < 1e-10 {
		return Identity()
	}
	return WorldTransform{
		A: t.D / det,
		B: -t.B / det,
		C: -t.C / det,
		D: t.A / det,
		E: (t.C*t.F - t.D*t.E) / det,
		F: (t.B*t.E - t.A*t.F) / det,
	}
}

// IsIdentity returns true if this is approximately the identity transform.
func (t WorldTransform) IsIdentity() bool {
	eps := 1e-10
	return math.Abs(t.A-1) < eps && math.Abs(t.B) < eps &&
		math.Abs(t.C) < eps && math.Abs(t.D-1) < eps &&
		math.Abs(t.E) < eps && math.Abs(t.F) < eps
}

// ToSVG returns the transform as an SVG transform attribute value.
func (t WorldTransform) ToSVG() string {
	if t.IsIdentity() {
		return ""
	}
	// Check for simple transforms
	if math.Abs(t.B) < 1e-10 && math.Abs(t.C) < 1e-10 {
		// No rotation
		if math.Abs(t.A-1) < 1e-10 && math.Abs(t.D-1) < 1e-10 {
			// Just translation
			return fmt.Sprintf("translate(%.4g,%.4g)", t.E, t.F)
		}
		if math.Abs(t.E) < 1e-10 && math.Abs(t.F) < 1e-10 {
			// Just scale
			return fmt.Sprintf("scale(%.4g,%.4g)", t.A, t.D)
		}
	}
	// General matrix
	return fmt.Sprintf("matrix(%.4g,%.4g,%.4g,%.4g,%.4g,%.4g)", t.A, t.B, t.C, t.D, t.E, t.F)
}

// String returns a debug representation.
func (t WorldTransform) String() string {
	return fmt.Sprintf("[%.4f %.4f %.4f; %.4f %.4f %.4f]", t.A, t.C, t.E, t.B, t.D, t.F)
}

// ShapeTransform holds the complete transform information for a shape.
type ShapeTransform struct {
	Local       WorldTransform // shape's local transform (from its properties)
	Parent      WorldTransform // accumulated parent transforms
	World       WorldTransform // final world transform (Parent * Local)
	LocalBounds Rect           // bounds in shape's local coordinate system
	WorldBounds Rect           // bounds in world coordinates
}

// ComputeShapeTransform computes the complete transform for a shape.
// parentTransform is the accumulated transform from all parent shapes/groups.
func (s *Shape) ComputeShapeTransform(parentTransform WorldTransform) *ShapeTransform {
	st := &ShapeTransform{
		Parent: parentTransform,
	}

	// Get shape dimensions (handle negative dimensions for connectors)
	width := s.Width()
	height := s.Height()
	absWidth := math.Abs(width)
	absHeight := math.Abs(height)
	if absWidth == 0 {
		absWidth = 1
	}
	if absHeight == 0 {
		absHeight = 1
	}

	// Local bounds in shape's coordinate system
	st.LocalBounds = Rect{X: 0, Y: 0, Width: absWidth, Height: absHeight}

	// Build local transform from shape properties
	// 1. Shape position (PinX, PinY is center position in parent coords)
	pinX := s.X()
	pinY := s.Y()
	// 2. Local pin (LocPinX, LocPinY is center within shape)
	locPinX := s.LocX()
	locPinY := s.LocY()
	// 3. Rotation angle
	angle := toFloat(s.CellValue("Angle"))

	// Build transform: translate to position, rotate around pin, offset by local pin
	// The transform order is: offset by -LocPin, rotate, translate to Pin
	local := Identity()

	// Start with translation to Pin position
	if pinX != 0 || pinY != 0 {
		local = Translate(pinX, pinY)
	}

	// Apply rotation around the pin point
	if angle != 0 {
		local = local.Multiply(Rotate(angle))
	}

	// Offset by negative local pin (to position shape correctly)
	if locPinX != 0 || locPinY != 0 {
		local = local.Multiply(Translate(-locPinX, -locPinY))
	}

	st.Local = local

	// Compute world transform
	st.World = parentTransform.Multiply(local)

	// Compute world bounds
	st.WorldBounds = st.World.ApplyToRect(st.LocalBounds)

	return st
}

// ComputeGroupTransforms computes transforms for all shapes in a group hierarchy.
// Returns a map from shape ID to ShapeTransform.
func ComputeGroupTransforms(shape *Shape) map[string]*ShapeTransform {
	result := make(map[string]*ShapeTransform)
	computeTransformsRecursive(shape, Identity(), result)
	return result
}

func computeTransformsRecursive(shape *Shape, parentTransform WorldTransform, result map[string]*ShapeTransform) {
	st := shape.ComputeShapeTransform(parentTransform)
	result[shape.ID] = st

	// Process children with this shape's world transform as their parent
	for _, child := range shape.ChildShapes() {
		computeTransformsRecursive(child, st.World, result)
	}
}

// DumpTransforms returns a debug string showing all transforms in a shape hierarchy.
func DumpTransforms(shape *Shape) string {
	transforms := ComputeGroupTransforms(shape)
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Shape: %s\n", shape.ID))
	dumpTransformRecursive(shape, transforms, &sb, 0)

	return sb.String()
}

func dumpTransformRecursive(shape *Shape, transforms map[string]*ShapeTransform, sb *strings.Builder, depth int) {
	indent := strings.Repeat("  ", depth)
	st := transforms[shape.ID]
	if st == nil {
		return
	}

	sb.WriteString(fmt.Sprintf("%sID: %s\n", indent, shape.ID))
	sb.WriteString(fmt.Sprintf("%s  Local: %s\n", indent, st.Local.String()))
	sb.WriteString(fmt.Sprintf("%s  World: %s\n", indent, st.World.String()))
	sb.WriteString(fmt.Sprintf("%s  LocalBounds: (%.2f,%.2f) %.2fx%.2f\n", indent,
		st.LocalBounds.X, st.LocalBounds.Y, st.LocalBounds.Width, st.LocalBounds.Height))
	sb.WriteString(fmt.Sprintf("%s  WorldBounds: (%.2f,%.2f) %.2fx%.2f\n", indent,
		st.WorldBounds.X, st.WorldBounds.Y, st.WorldBounds.Width, st.WorldBounds.Height))

	for _, child := range shape.ChildShapes() {
		dumpTransformRecursive(child, transforms, sb, depth+1)
	}
}
