package vsdx

// Point represents a 2D coordinate.
type Point struct {
	X, Y float64
}

// Rect represents a bounding rectangle with position and dimensions.
// X, Y is the lower-left corner; Width, Height are the dimensions.
type Rect struct {
	X, Y, Width, Height float64
}

// BeginX returns the left edge of the rectangle.
func (r Rect) BeginX() float64 { return r.X }

// BeginY returns the bottom edge of the rectangle.
func (r Rect) BeginY() float64 { return r.Y }

// EndX returns the right edge of the rectangle.
func (r Rect) EndX() float64 { return r.X + r.Width }

// EndY returns the top edge of the rectangle.
func (r Rect) EndY() float64 { return r.Y + r.Height }
