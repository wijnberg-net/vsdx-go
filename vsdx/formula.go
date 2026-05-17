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

	// Additional math functions
	case "EVEN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				n := math.Ceil(math.Abs(v))
				if int(n)%2 != 0 {
					n++
				}
				if v < 0 {
					return -n, true
				}
				return n, true
			}
		}
	case "ODD":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				n := math.Ceil(math.Abs(v))
				if int(n)%2 == 0 {
					n++
				}
				if v < 0 {
					return -n, true
				}
				return n, true
			}
		}
	case "FACT":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				n := int(v)
				if n < 0 {
					return 0, false
				}
				result := 1.0
				for i := 2; i <= n; i++ {
					result *= float64(i)
				}
				return result, true
			}
		}
	case "GCD":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				return float64(gcd(int(math.Abs(a)), int(math.Abs(b)))), true
			}
		}
	case "LCM":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				ai, bi := int(math.Abs(a)), int(math.Abs(b))
				if ai == 0 || bi == 0 {
					return 0, true
				}
				return float64(ai * bi / gcd(ai, bi)), true
			}
		}
	case "RAND":
		return randFloat(), true
	case "RANDBETWEEN":
		if len(args) >= 2 {
			lo, lok := e.evalExpr(args[0])
			hi, hok := e.evalExpr(args[1])
			if lok && hok {
				return lo + randFloat()*(hi-lo), true
			}
		}

	// Logic functions
	case "TRUE":
		return 1, true
	case "FALSE":
		return 0, true
	case "XOR":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if (a != 0) != (b != 0) {
					return 1, true
				}
				return 0, true
			}
		}
	case "ISERROR", "ISERR":
		if len(args) >= 1 {
			_, ok := e.evalExpr(args[0])
			if !ok {
				return 1, true
			}
			return 0, true
		}
	case "ISBLANK":
		if len(args) >= 1 {
			val := strings.TrimSpace(args[0])
			if val == "" {
				return 1, true
			}
			return 0, true
		}
	case "ISNUMBER":
		if len(args) >= 1 {
			_, ok := e.evalExpr(args[0])
			if ok {
				return 1, true
			}
			return 0, true
		}

	// Color functions (return components as 0-255)
	case "RED":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				// Assuming RGB packed integer
				return float64(int(v) >> 16 & 0xFF), true
			}
		}
	case "GREEN":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64(int(v) >> 8 & 0xFF), true
			}
		}
	case "BLUE":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64(int(v) & 0xFF), true
			}
		}
	case "RGB":
		if len(args) >= 3 {
			r, rok := e.evalExpr(args[0])
			g, gok := e.evalExpr(args[1])
			b, bok := e.evalExpr(args[2])
			if rok && gok && bok {
				ri := int(math.Max(0, math.Min(255, r)))
				gi := int(math.Max(0, math.Min(255, g)))
				bi := int(math.Max(0, math.Min(255, b)))
				return float64(ri<<16 | gi<<8 | bi), true
			}
		}
	case "HUE":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				r := float64(int(v)>>16&0xFF) / 255
				g := float64(int(v)>>8&0xFF) / 255
				b := float64(int(v)&0xFF) / 255
				h, _, _ := rgbToHSL(r, g, b)
				return h * 360, true
			}
		}
	case "SAT":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				r := float64(int(v)>>16&0xFF) / 255
				g := float64(int(v)>>8&0xFF) / 255
				b := float64(int(v)&0xFF) / 255
				_, s, _ := rgbToHSL(r, g, b)
				return s * 240, true
			}
		}
	case "LUM":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				r := float64(int(v)>>16&0xFF) / 255
				g := float64(int(v)>>8&0xFF) / 255
				b := float64(int(v)&0xFF) / 255
				_, _, l := rgbToHSL(r, g, b)
				return l * 240, true
			}
		}
	case "HSL":
		if len(args) >= 3 {
			h, hok := e.evalExpr(args[0])
			s, sok := e.evalExpr(args[1])
			l, lok := e.evalExpr(args[2])
			if hok && sok && lok {
				r, g, b := hslToRGB(h/360, s/240, l/240)
				ri := int(r * 255)
				gi := int(g * 255)
				bi := int(b * 255)
				return float64(ri<<16 | gi<<8 | bi), true
			}
		}

	// Geometry functions
	case "POINTALONGPATH":
		// Returns distance along path - simplified to return first arg
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
	case "PATHLENGTH":
		// Returns total path length - requires geometry parsing
		return 0, true
	case "ANGLEALONGPATH":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}

	// Unit conversion
	case "CY":
		// Convert to internal units (inches) - simplified
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
	case "DL", "IN", "FT", "MM", "CM", "M", "PT", "P", "PW", "PH":
		// Unit conversion - return value in internal units
		if len(args) >= 1 {
			v, ok := e.evalExpr(args[0])
			if ok {
				switch name {
				case "MM":
					return v / 25.4, true
				case "CM":
					return v / 2.54, true
				case "M":
					return v / 0.0254, true
				case "PT":
					return v / 72, true
				case "FT":
					return v * 12, true
				default:
					return v, true
				}
			}
		}

	// SETATREF/SETATREFEXPR - for indirect references
	case "SETATREF":
		if len(args) >= 2 {
			return e.evalExpr(args[1])
		}
	case "SETATREFEXPR":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}

	// SETF - set formula
	case "SETF":
		if len(args) >= 2 {
			return e.evalExpr(args[1])
		}

	// LOOKUP functions
	case "INDEX":
		if len(args) >= 2 {
			idx, iok := e.evalExpr(args[0])
			if iok {
				// Parse the list from remaining args
				listIdx := int(idx)
				if listIdx >= 1 && listIdx <= len(args)-1 {
					return e.evalExpr(args[listIdx])
				}
			}
		}
	case "LOOKUP":
		if len(args) >= 3 {
			key, kok := e.evalExpr(args[0])
			if kok {
				// Simple lookup - find key in list, return corresponding value
				for i := 1; i < len(args)-1; i += 2 {
					k, ok := e.evalExpr(args[i])
					if ok && k == key && i+1 < len(args) {
						return e.evalExpr(args[i+1])
					}
				}
			}
		}

	// THEMEVAL - theme value lookup (simplified)
	case "THEMEVAL":
		// Returns default value if theme not available
		if len(args) >= 2 {
			return e.evalExpr(args[1]) // Return default
		}
		return 0, true
	case "THEMEGUARD":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}

	// Navigation functions (return 0 as they affect UI, not values)
	case "GOTOPAGE", "RUNADDON", "CALLTHIS", "DOCMD", "RUNMACRO":
		return 0, true

	// IFERROR - return alternate value on error
	case "IFERROR":
		if len(args) >= 2 {
			v, ok := e.evalExpr(args[0])
			if ok {
				return v, true
			}
			return e.evalExpr(args[1])
		}

	// GRAVITY - spacing function
	case "GRAVITY":
		if len(args) >= 3 {
			return e.evalExpr(args[0]) // Return first value
		}

	// RECTSECT - rectangle section
	case "RECTSECT":
		if len(args) >= 5 {
			return e.evalExpr(args[4]) // Return last param
		}

	// STRSAME - string comparison
	case "STRSAME":
		if len(args) >= 2 {
			a := strings.TrimSpace(args[0])
			b := strings.TrimSpace(args[1])
			// Remove quotes
			a = strings.Trim(a, "\"'")
			b = strings.Trim(b, "\"'")
			ignoreCase := true
			if len(args) >= 3 {
				ic, _ := e.evalExpr(args[2])
				ignoreCase = ic != 0
			}
			if ignoreCase {
				if strings.EqualFold(a, b) {
					return 1, true
				}
			} else {
				if a == b {
					return 1, true
				}
			}
			return 0, true
		}

	// LEN - string length
	case "LEN":
		if len(args) >= 1 {
			s := strings.Trim(strings.TrimSpace(args[0]), "\"'")
			return float64(len(s)), true
		}

	// FIND - find substring
	case "FIND":
		if len(args) >= 2 {
			needle := strings.Trim(strings.TrimSpace(args[0]), "\"'")
			haystack := strings.Trim(strings.TrimSpace(args[1]), "\"'")
			start := 0
			if len(args) >= 3 {
				if s, ok := e.evalExpr(args[2]); ok {
					start = int(s) - 1
				}
			}
			if start < 0 {
				start = 0
			}
			if start < len(haystack) {
				idx := strings.Index(haystack[start:], needle)
				if idx >= 0 {
					return float64(idx + start + 1), true
				}
			}
			return 0, true
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

// gcd computes the greatest common divisor of two integers.
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// randFloat returns a random float64 in [0, 1).
func randFloat() float64 {
	// Use a simple LCG for deterministic pseudo-random numbers.
	// For formula evaluation, we don't need cryptographic randomness.
	return float64(randState.next()) / float64(1<<31)
}

// randState is a simple linear congruential generator.
var randState = &lcg{seed: 12345}

type lcg struct {
	seed uint32
}

func (l *lcg) next() uint32 {
	l.seed = l.seed*1103515245 + 12345
	return l.seed >> 1
}

// rgbToHSL converts RGB values (0-1) to HSL values (0-1).
func rgbToHSL(r, g, b float64) (h, s, l float64) {
	maxC := math.Max(r, math.Max(g, b))
	minC := math.Min(r, math.Min(g, b))
	l = (maxC + minC) / 2

	if maxC == minC {
		return 0, 0, l
	}

	d := maxC - minC
	if l > 0.5 {
		s = d / (2 - maxC - minC)
	} else {
		s = d / (maxC + minC)
	}

	switch maxC {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	case b:
		h = (r-g)/d + 4
	}
	h /= 6

	return h, s, l
}

// hslToRGB converts HSL values (0-1) to RGB values (0-1).
func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}

	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q

	r = hueToRGB(p, q, h+1.0/3.0)
	g = hueToRGB(p, q, h)
	b = hueToRGB(p, q, h-1.0/3.0)

	return r, g, b
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}
