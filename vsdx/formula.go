package vsdx

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// FormulaEvaluator evaluates Visio formulas in the context of a shape.
type FormulaEvaluator struct {
	shape *Shape
}

// NewFormulaEvaluator creates a new evaluator for the given shape.
func NewFormulaEvaluator(shape *Shape) *FormulaEvaluator {
	return &FormulaEvaluator{shape: shape}
}

// Eval evaluates a formula string and returns the result.
// Returns the value and true if successful, or 0 and false if evaluation fails.
func (e *FormulaEvaluator) Eval(formula string) (float64, bool) {
	formula = strings.TrimSpace(formula)
	if formula == "" {
		return 0, false
	}

	// Try to parse as a simple number first
	if v, err := strconv.ParseFloat(formula, 64); err == nil {
		return v, true
	}

	// Evaluate the expression
	result, ok := e.evalExpr(formula)
	return result, ok
}

// evalExpr evaluates an expression, handling operators with proper precedence.
func (e *FormulaEvaluator) evalExpr(expr string) (float64, bool) {
	expr = strings.TrimSpace(expr)

	// Handle addition and subtraction (lowest precedence)
	// Find the rightmost + or - not inside parentheses
	depth := 0
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == ')' {
			depth++
		} else if c == '(' {
			depth--
		} else if depth == 0 && (c == '+' || c == '-') {
			// Check if this is a binary operator (not unary minus)
			if i > 0 {
				prev := expr[i-1]
				if prev != '+' && prev != '-' && prev != '*' && prev != '/' && prev != '^' && prev != '(' && prev != ',' {
					left, lok := e.evalExpr(expr[:i])
					right, rok := e.evalExpr(expr[i+1:])
					if lok && rok {
						if c == '+' {
							return left + right, true
						}
						return left - right, true
					}
					return 0, false
				}
			}
		}
	}

	// Handle multiplication and division
	depth = 0
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == ')' {
			depth++
		} else if c == '(' {
			depth--
		} else if depth == 0 && (c == '*' || c == '/') {
			left, lok := e.evalExpr(expr[:i])
			right, rok := e.evalExpr(expr[i+1:])
			if lok && rok {
				if c == '*' {
					return left * right, true
				}
				if right != 0 {
					return left / right, true
				}
				return 0, false
			}
			return 0, false
		}
	}

	// Handle exponentiation (right-to-left associative)
	depth = 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
		} else if depth == 0 && c == '^' {
			left, lok := e.evalExpr(expr[:i])
			right, rok := e.evalExpr(expr[i+1:])
			if lok && rok {
				return math.Pow(left, right), true
			}
			return 0, false
		}
	}

	// Handle unary minus
	if strings.HasPrefix(expr, "-") {
		val, ok := e.evalExpr(expr[1:])
		if ok {
			return -val, true
		}
	}

	// Handle parentheses
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		// Check if the parens are balanced and wrap the whole expression
		depth := 0
		balanced := true
		for i, c := range expr {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 && i != len(expr)-1 {
					balanced = false
					break
				}
			}
		}
		if balanced {
			return e.evalExpr(expr[1 : len(expr)-1])
		}
	}

	// Handle function calls
	if idx := strings.Index(expr, "("); idx > 0 && strings.HasSuffix(expr, ")") {
		funcName := strings.ToUpper(expr[:idx])
		argsStr := expr[idx+1 : len(expr)-1]
		return e.evalFunc(funcName, argsStr)
	}

	// Handle cell references
	return e.evalCellRef(expr)
}

// evalFunc evaluates a function call.
func (e *FormulaEvaluator) evalFunc(name, argsStr string) (float64, bool) {
	args := e.parseArgs(argsStr)

	switch name {
	// Protection functions (just return inner value)
	case "GUARD":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}

	// Math functions - single argument
	case "ABS":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Abs(v), true
			}
		}
	case "SQRT":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v >= 0 {
					return math.Sqrt(v), true
				}
			}
		}
	case "SIN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Sin(v), true
			}
		}
	case "COS":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Cos(v), true
			}
		}
	case "TAN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Tan(v), true
			}
		}
	case "ASIN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v >= -1 && v <= 1 {
					return math.Asin(v), true
				}
			}
		}
	case "ACOS":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v >= -1 && v <= 1 {
					return math.Acos(v), true
				}
			}
		}
	case "ATAN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Atan(v), true
			}
		}
	case "ATAN2":
		if len(args) >= 2 {
			y, yok := e.evalExpr(args[0])
			x, xok := e.evalExpr(args[1])
			if yok && xok {
				return math.Atan2(y, x), true
			}
		}
	case "EXP":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Exp(v), true
			}
		}
	case "LN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v > 0 {
					return math.Log(v), true
				}
			}
		}
	case "LOG", "LOG10":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v > 0 {
					return math.Log10(v), true
				}
			}
		}
	case "SINH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Sinh(v), true
			}
		}
	case "COSH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Cosh(v), true
			}
		}
	case "TANH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Tanh(v), true
			}
		}

	// Rounding functions
	case "INT", "TRUNC":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Trunc(v), true
			}
		}
	case "FLOOR":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Floor(v), true
			}
		}
	case "CEILING":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Ceil(v), true
			}
		}
	case "ROUND":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Round(v), true
			}
		}
	case "SIGN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v > 0 {
					return 1, true
				} else if v < 0 {
					return -1, true
				}
				return 0, true
			}
		}

	// Min/Max functions
	case "MAX":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				return math.Max(a, b), true
			}
		}
	case "MIN":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				return math.Min(a, b), true
			}
		}

	// Conditional
	case "IF":
		if len(args) >= 2 {
			cond, ok := e.evalExpr(args[0])
			if ok {
				if cond != 0 { // true
					return e.evalExpr(args[1])
				} else if len(args) >= 3 {
					return e.evalExpr(args[2])
				}
				return 0, true
			}
		}

	// Power
	case "POW":
		if len(args) >= 2 {
			base, bok := e.evalExpr(args[0])
			exp, eok := e.evalExpr(args[1])
			if bok && eok {
				return math.Pow(base, exp), true
			}
		}

	// Modulo
	case "MOD", "MODULUS":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok && b != 0 {
				return math.Mod(a, b), true
			}
		}

	// Conversion
	case "DEG":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return v * 180 / math.Pi, true
			}
		}
	case "RAD":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return v * math.Pi / 180, true
			}
		}

	// PI constant
	case "PI":
		return math.Pi, true

	// Boolean (return 1 or 0)
	case "NOT":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v == 0 {
					return 1, true
				}
				return 0, true
			}
		}
	case "AND":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a != 0 && b != 0 {
					return 1, true
				}
				return 0, true
			}
		}
	case "OR":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a != 0 || b != 0 {
					return 1, true
				}
				return 0, true
			}
		}

	// Bitwise
	case "BITAND":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				return float64(int64(a) & int64(b)), true
			}
		}
	case "BITOR":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				return float64(int64(a) | int64(b)), true
			}
		}
	case "BITXOR":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				return float64(int64(a) ^ int64(b)), true
			}
		}
	case "BITNOT":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64(^int64(v)), true
			}
		}

	// Bound/Clamp
	case "BOUND":
		if len(args) >= 4 {
			val, vok := e.evalExpr(args[0])
			// args[1] is bound type (ignored for simple implementation)
			min, minok := e.evalExpr(args[2])
			max, maxok := e.evalExpr(args[3])
			if vok && minok && maxok {
				if val < min {
					return min, true
				}
				if val > max {
					return max, true
				}
				return val, true
			}
		}
	}

	return 0, false
}

// parseArgs splits a comma-separated argument string, respecting parentheses.
func (e *FormulaEvaluator) parseArgs(argsStr string) []string {
	var args []string
	depth := 0
	start := 0

	for i, c := range argsStr {
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
		} else if c == ',' && depth == 0 {
			args = append(args, strings.TrimSpace(argsStr[start:i]))
			start = i + 1
		}
	}

	if start < len(argsStr) {
		args = append(args, strings.TrimSpace(argsStr[start:]))
	}

	return args
}

// evalCellRef evaluates a cell reference.
func (e *FormulaEvaluator) evalCellRef(ref string) (float64, bool) {
	ref = strings.TrimSpace(ref)

	// Try to parse as number first
	if v, err := strconv.ParseFloat(ref, 64); err == nil {
		return v, true
	}

	if e.shape == nil {
		return 0, false
	}

	// Common cell references
	switch strings.ToLower(ref) {
	case "width":
		return e.shape.Width(), true
	case "height":
		return e.shape.Height(), true
	case "pinx":
		return e.shape.X(), true
	case "piny":
		return e.shape.Y(), true
	case "locpinx":
		return toFloat(e.shape.CellValue("LocPinX")), true
	case "locpiny":
		return toFloat(e.shape.CellValue("LocPinY")), true
	case "beginx":
		return e.shape.BeginX(), true
	case "beginy":
		return e.shape.BeginY(), true
	case "endx":
		return e.shape.EndX(), true
	case "endy":
		return e.shape.EndY(), true
	case "angle":
		return toFloat(e.shape.CellValue("Angle")), true
	case "flipx":
		return toFloat(e.shape.CellValue("FlipX")), true
	case "flipy":
		return toFloat(e.shape.CellValue("FlipY")), true
	case "txtwidth":
		return toFloat(e.shape.CellValue("TxtWidth")), true
	case "txtheight":
		return toFloat(e.shape.CellValue("TxtHeight")), true
	case "txtpinx":
		return toFloat(e.shape.CellValue("TxtPinX")), true
	case "txtpiny":
		return toFloat(e.shape.CellValue("TxtPinY")), true
	case "txtlocpinx":
		return toFloat(e.shape.CellValue("TxtLocPinX")), true
	case "txtlocpiny":
		return toFloat(e.shape.CellValue("TxtLocPinY")), true
	case "lineweight":
		return e.shape.LineWeight(), true
	}

	// Try to look up from shape cells
	if val := e.shape.CellValue(ref); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			return v, true
		}
	}

	return 0, false
}

// CalcValue evaluates a formula string in the context of a shape.
// Returns the calculated value and true if successful, or 0 and false if no matching formula.
// This is a convenience function that creates a FormulaEvaluator.
func CalcValue(shape *Shape, formula string) (float64, bool) {
	eval := NewFormulaEvaluator(shape)
	return eval.Eval(formula)
}

// EvalFormula evaluates a formula and returns the result as a string.
// If evaluation fails, returns the original formula.
func EvalFormula(shape *Shape, formula string) string {
	if formula == "" {
		return ""
	}
	eval := NewFormulaEvaluator(shape)
	if v, ok := eval.Eval(formula); ok {
		return fmtFloat(v)
	}
	return formula
}

// formulaRefPattern matches cell references like "Width", "Height*0.5", etc.
var formulaRefPattern = regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9]*)\b`)

// SubstituteRefs replaces cell references in a formula with their values.
// Useful for debugging or displaying evaluated formulas.
func SubstituteRefs(shape *Shape, formula string) string {
	if shape == nil {
		return formula
	}
	eval := NewFormulaEvaluator(shape)

	return formulaRefPattern.ReplaceAllStringFunc(formula, func(ref string) string {
		// Skip function names
		upper := strings.ToUpper(ref)
		switch upper {
		case "GUARD", "ABS", "SQRT", "SIN", "COS", "TAN", "ASIN", "ACOS", "ATAN", "ATAN2",
			"EXP", "LN", "LOG", "LOG10", "SINH", "COSH", "TANH", "INT", "TRUNC", "FLOOR",
			"CEILING", "ROUND", "SIGN", "MAX", "MIN", "IF", "POW", "MOD", "MODULUS",
			"DEG", "RAD", "PI", "NOT", "AND", "OR", "BITAND", "BITOR", "BITXOR", "BITNOT", "BOUND":
			return ref
		}

		if v, ok := eval.evalCellRef(ref); ok {
			return fmtFloat(v)
		}
		return ref
	})
}
