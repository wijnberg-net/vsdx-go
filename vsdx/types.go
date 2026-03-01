package vsdx

// Point represents a 2D coordinate.
type Point struct {
	X, Y float64
}

// Rect represents a bounding rectangle with begin and end coordinates.
type Rect struct {
	BeginX, BeginY, EndX, EndY float64
}
