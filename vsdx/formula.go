package vsdx

import "math"

// CalcValue evaluates a formula string in the context of a shape.
// Returns the calculated value and true if successful, or 0 and false if no matching formula.
func CalcValue(shape *Shape, funcText string) (float64, bool) {
	switch funcText {
	case "Width*1":
		return shape.Width(), true
	case "Width*0":
		return 0, true
	case "(BeginX+EndX)/2", "GUARD((BeginX+EndX)/2)":
		return (shape.BeginX() + shape.EndX()) / 2, true
	case "(BeginY+EndY)/2", "GUARD((BeginY+EndY)/2)":
		return (shape.BeginY() + shape.EndY()) / 2, true
	case "Width*0.5", "GUARD(Width*0.5)":
		return shape.Width() * 0.5, true
	case "Height*0.5", "GUARD(Height*0.5)":
		return shape.Height() * 0.5, true
	case "SQRT((EndX-BeginX)^2+(EndY-BeginY)^2)":
		w := shape.EndX() - shape.BeginX()
		h := shape.EndY() - shape.BeginY()
		return math.Sqrt(w*w + h*h), true
	case "ATAN2(EndY-BeginY,EndX-BeginX)":
		w := shape.EndX() - shape.BeginX()
		h := shape.EndY() - shape.BeginY()
		return math.Atan2(w, h), true
	case "GUARD(EndX-BeginX)":
		return shape.EndX() - shape.BeginX(), true
	case "GUARD(EndY-BeginY)":
		return shape.EndY() - shape.BeginY(), true
	}
	return 0, false
}
