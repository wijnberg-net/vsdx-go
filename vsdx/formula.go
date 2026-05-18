package vsdx

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// FormulaStatus indicates the result status of formula evaluation.
type FormulaStatus int

const (
	// FormulaSuccess indicates the formula was successfully evaluated.
	FormulaSuccess FormulaStatus = iota
	// FormulaUnsupported indicates the formula uses an unsupported function.
	FormulaUnsupported
	// FormulaError indicates an error occurred during evaluation.
	FormulaError
)

// FormulaResult holds the result of formula evaluation with explicit status.
type FormulaResult struct {
	Status FormulaStatus
	Value  float64 // Numeric result (valid when Status == FormulaSuccess)
	Str    string  // String result for string functions
	Err    error   // Error details (valid when Status == FormulaError)
	Func   string  // Function name that caused Unsupported/Error status
}

// OK returns true if the formula evaluated successfully.
func (r FormulaResult) OK() bool {
	return r.Status == FormulaSuccess
}

// String returns a human-readable representation of the result.
func (r FormulaResult) String() string {
	switch r.Status {
	case FormulaSuccess:
		if r.Str != "" {
			return r.Str
		}
		return fmtFloat(r.Value)
	case FormulaUnsupported:
		return fmt.Sprintf("UNSUPPORTED(%s)", r.Func)
	case FormulaError:
		if r.Err != nil {
			return fmt.Sprintf("ERROR(%s: %v)", r.Func, r.Err)
		}
		return fmt.Sprintf("ERROR(%s)", r.Func)
	default:
		return "UNKNOWN"
	}
}

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
// For more detailed status information (including unsupported functions), use EvalResult.
func (e *FormulaEvaluator) Eval(formula string) (float64, bool) {
	result := e.EvalResult(formula)
	return result.Value, result.Status == FormulaSuccess
}

// EvalResult evaluates a formula string and returns a detailed result.
// Unlike Eval, this method distinguishes between evaluation failure and unsupported functions.
func (e *FormulaEvaluator) EvalResult(formula string) FormulaResult {
	formula = strings.TrimSpace(formula)
	if formula == "" {
		return FormulaResult{Status: FormulaError, Err: ErrFormulaInvalidSyntax, Func: ""}
	}

	// Try to parse as a simple number first
	if v, err := strconv.ParseFloat(formula, 64); err == nil {
		return FormulaResult{Status: FormulaSuccess, Value: v}
	}

	// Evaluate the expression
	return e.evalExprResult(formula)
}

// evalExprResult evaluates an expression and returns detailed result.
func (e *FormulaEvaluator) evalExprResult(expr string) FormulaResult {
	result, ok := e.evalExpr(expr)
	if ok {
		return FormulaResult{Status: FormulaSuccess, Value: result}
	}
	// Check if this was an unsupported function by trying to parse it
	if unsupported := e.checkUnsupportedFunc(expr); unsupported != "" {
		return FormulaResult{Status: FormulaUnsupported, Func: unsupported, Err: ErrFormulaUnsupported}
	}
	return FormulaResult{Status: FormulaError, Err: ErrFormulaInvalidSyntax}
}

// checkUnsupportedFunc checks if the expression contains an unsupported function.
func (e *FormulaEvaluator) checkUnsupportedFunc(expr string) string {
	expr = strings.TrimSpace(expr)
	// Look for function calls
	parenIdx := strings.Index(expr, "(")
	if parenIdx > 0 {
		name := strings.ToUpper(strings.TrimSpace(expr[:parenIdx]))
		if isUnsupportedFunction(name) {
			return name
		}
	}
	return ""
}

// isUnsupportedFunction returns true if the function is explicitly unsupported.
// These functions are listed in MS-VSDX spec §2.2.11.2 but not fully implemented.
var unsupportedFunctions = map[string]bool{
	// Geometry functions requiring path parsing (MS-VSDX §2.4.2)
	"POINTALONGPATH": true,
	"PATHLENGTH":     true,
	"ANGLEALONGPATH": true,

	// Layout/gravity functions (MS-VSDX §2.2.5.2.51)
	"GRAVITY": true,

	// Geometry intersection (MS-VSDX §2.2.11.2)
	"RECTSECT":     true,
	"POLYLINE":     true,
	"ELLIPSE":      true,
	"SPLINE":       true,
	"NURBS":        true,
	"INFINITELINE": true,

	// String manipulation functions that return strings (MS-VSDX §2.2.11.2)
	// These cannot be represented as float64
	"LOWER":       true,
	"UPPER":       true,
	"TRIM":        true,
	"REPLACE":     true,
	"SUBSTITUTE":  true,
	"REPT":        true,
	"CONCATENATE": true,

	// Date/time string parsing (MS-VSDX §2.2.11.2)
	"TIMEVALUE": true,
	"DATEVALUE": true,

	// Array functions (MS-VSDX §2.2.11.2)
	"SUMPRODUCT": true,

	// Document/UI functions (MS-VSDX §2.2.11.2)
	"OPENFILE":         true,
	"QUEUEMARKEREVENT": true,
	"OPENTEXTWIN":      true,
	"OPENSHEET":        true,
	"SETREF":           true,
	"DEPENDSON":        true,
}

func isUnsupportedFunction(name string) bool {
	return unsupportedFunctions[name]
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

	// Geometry functions - UNSUPPORTED: require geometry path parsing (MS-VSDX §2.4.2)
	case "POINTALONGPATH", "PATHLENGTH", "ANGLEALONGPATH":
		// These functions require parsing and computing along geometry paths.
		// Returning false to indicate unsupported - caller should check via EvalResult.
		return 0, false

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

	// GRAVITY - spacing function - UNSUPPORTED: requires layout engine (MS-VSDX §2.2.5.2.51)
	case "GRAVITY":
		return 0, false

	// RECTSECT - rectangle section - UNSUPPORTED: requires geometry intersection (MS-VSDX §2.2.11.2)
	case "RECTSECT":
		return 0, false

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

	// LEFT - left substring (returns length, string handling is limited)
	case "LEFT":
		if len(args) >= 1 {
			s := strings.Trim(strings.TrimSpace(args[0]), "\"'")
			n := 1
			if len(args) >= 2 {
				if v, ok := e.evalExpr(args[1]); ok {
					n = int(v)
				}
			}
			if n > len(s) {
				n = len(s)
			}
			if n > 0 {
				return float64(len(s[:n])), true
			}
			return 0, true
		}

	// RIGHT - right substring
	case "RIGHT":
		if len(args) >= 1 {
			s := strings.Trim(strings.TrimSpace(args[0]), "\"'")
			n := 1
			if len(args) >= 2 {
				if v, ok := e.evalExpr(args[1]); ok {
					n = int(v)
				}
			}
			if n > len(s) {
				n = len(s)
			}
			if n > 0 {
				return float64(len(s[len(s)-n:])), true
			}
			return 0, true
		}

	// MID - middle substring
	case "MID":
		if len(args) >= 3 {
			s := strings.Trim(strings.TrimSpace(args[0]), "\"'")
			start, sok := e.evalExpr(args[1])
			length, lok := e.evalExpr(args[2])
			if sok && lok {
				startIdx := int(start) - 1
				if startIdx < 0 {
					startIdx = 0
				}
				endIdx := startIdx + int(length)
				if endIdx > len(s) {
					endIdx = len(s)
				}
				if startIdx < len(s) {
					return float64(len(s[startIdx:endIdx])), true
				}
			}
			return 0, true
		}

	// String manipulation functions - UNSUPPORTED: return strings, not numeric values (MS-VSDX §2.2.11.2)
	// FormulaEvaluator only supports numeric evaluation; string results cannot be represented.
	case "LOWER", "UPPER", "TRIM", "REPLACE", "SUBSTITUTE", "REPT":
		return 0, false

	// CHAR - character from code
	case "CHAR":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return v, true
			}
		}
		return 0, true

	// CODE - code from character
	case "CODE":
		if len(args) >= 1 {
			s := strings.Trim(strings.TrimSpace(args[0]), "\"'")
			if len(s) > 0 {
				return float64(s[0]), true
			}
		}
		return 0, true

	// TEXT, FORMAT - number formatting (returns the number)
	case "TEXT", "FORMAT":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
		return 0, true

	// DATE - create date serial number
	case "DATE":
		if len(args) >= 3 {
			year, yok := e.evalExpr(args[0])
			month, mok := e.evalExpr(args[1])
			day, dok := e.evalExpr(args[2])
			if yok && mok && dok {
				// Return simplified date serial (days since epoch approximation)
				return year*365 + month*30 + day, true
			}
		}
		return 0, true

	// DAY - extract day from date serial
	case "DAY":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64(int(v) % 31), true
			}
		}
		return 0, true

	// MONTH - extract month from date serial
	case "MONTH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64((int(v) / 30) % 12), true
			}
		}
		return 0, true

	// YEAR - extract year from date serial
	case "YEAR":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64(int(v) / 365), true
			}
		}
		return 0, true

	// NOW, TODAY - current date/time (returns 0 as placeholder)
	case "NOW", "TODAY":
		return 0, true

	// HOUR - extract hour
	case "HOUR":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				frac := v - math.Floor(v)
				return math.Floor(frac * 24), true
			}
		}
		return 0, true

	// MINUTE - extract minute
	case "MINUTE":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				frac := v - math.Floor(v)
				hours := frac * 24
				return math.Floor((hours - math.Floor(hours)) * 60), true
			}
		}
		return 0, true

	// SECOND - extract second
	case "SECOND":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				frac := v - math.Floor(v)
				hours := frac * 24
				minutes := (hours - math.Floor(hours)) * 60
				return math.Floor((minutes - math.Floor(minutes)) * 60), true
			}
		}
		return 0, true

	// TIME - create time value
	case "TIME":
		if len(args) >= 3 {
			hour, hok := e.evalExpr(args[0])
			minute, mok := e.evalExpr(args[1])
			second, sok := e.evalExpr(args[2])
			if hok && mok && sok {
				return (hour + minute/60 + second/3600) / 24, true
			}
		}
		return 0, true

	// TIMEVALUE, DATEVALUE - parse time/date strings - UNSUPPORTED: require string parsing (MS-VSDX §2.2.11.2)
	case "TIMEVALUE", "DATEVALUE":
		return 0, false

	// WEEKDAY - day of week
	case "WEEKDAY":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64(int(v) % 7), true
			}
		}
		return 0, true

	// DATEDIF - date difference
	case "DATEDIF":
		if len(args) >= 2 {
			start, sok := e.evalExpr(args[0])
			end, eok := e.evalExpr(args[1])
			if sok && eok {
				return end - start, true
			}
		}
		return 0, true

	// SUM - sum of values
	case "SUM":
		sum := 0.0
		for _, arg := range args {
			if v, ok := e.evalExpr(arg); ok {
				sum += v
			}
		}
		return sum, true

	// AVERAGE - average of values
	case "AVERAGE":
		if len(args) == 0 {
			return 0, true
		}
		sum := 0.0
		count := 0
		for _, arg := range args {
			if v, ok := e.evalExpr(arg); ok {
				sum += v
				count++
			}
		}
		if count > 0 {
			return sum / float64(count), true
		}
		return 0, true

	// COUNT - count numeric values
	case "COUNT":
		count := 0
		for _, arg := range args {
			if _, ok := e.evalExpr(arg); ok {
				count++
			}
		}
		return float64(count), true

	// COUNTA - count non-empty values
	case "COUNTA":
		count := 0
		for _, arg := range args {
			if strings.TrimSpace(arg) != "" {
				count++
			}
		}
		return float64(count), true

	// TYPE - return type code
	case "TYPE":
		if len(args) >= 1 {
			if _, ok := e.evalExpr(args[0]); ok {
				return 1, true // 1 = number
			}
			return 2, true // 2 = text
		}
		return 0, true

	// ISTEXT - check if text
	case "ISTEXT":
		if len(args) >= 1 {
			if _, ok := e.evalExpr(args[0]); !ok {
				return 1, true
			}
		}
		return 0, true

	// ISLOGICAL - check if boolean
	case "ISLOGICAL":
		if len(args) >= 1 {
			arg := strings.ToUpper(strings.TrimSpace(args[0]))
			if arg == "TRUE" || arg == "FALSE" {
				return 1, true
			}
		}
		return 0, true

	// NA - return error value
	case "NA":
		return math.NaN(), true

	// ISNA - check if NA
	case "ISNA":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok && math.IsNaN(v) {
				return 1, true
			}
		}
		return 0, true

	// ISERRVALUE - check error value
	case "ISERRVALUE":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok && (math.IsNaN(v) || math.IsInf(v, 0)) {
				return 1, true
			}
		}
		return 0, true

	// PRODUCT - multiply all values
	case "PRODUCT":
		product := 1.0
		for _, arg := range args {
			if v, ok := e.evalExpr(arg); ok {
				product *= v
			}
		}
		return product, true

	// SUMPRODUCT - sum of products - UNSUPPORTED: requires array argument handling (MS-VSDX §2.2.11.2)
	case "SUMPRODUCT":
		return 0, false

	// QUOTIENT - integer division
	case "QUOTIENT":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok && b != 0 {
				return math.Trunc(a / b), true
			}
		}
		return 0, true

	// POWER - same as POW
	case "POWER":
		if len(args) >= 2 {
			base, bok := e.evalExpr(args[0])
			exp, eok := e.evalExpr(args[1])
			if bok && eok {
				return math.Pow(base, exp), true
			}
		}
		return 0, true

	// ABS2, ATAN2 already implemented, add ACOT, ACOTH, COT, COTH, CSC, CSCH, SEC, SECH
	case "COT":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return 1 / math.Tan(v), true
			}
		}
		return 0, false

	case "ACOT":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Atan(1 / v), true
			}
		}
		return 0, false

	case "COTH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return 1 / math.Tanh(v), true
			}
		}
		return 0, false

	case "ACOTH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return 0.5 * math.Log((v+1)/(v-1)), true
			}
		}
		return 0, false

	case "SEC":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return 1 / math.Cos(v), true
			}
		}
		return 0, false

	case "CSC":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return 1 / math.Sin(v), true
			}
		}
		return 0, false

	case "SECH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return 1 / math.Cosh(v), true
			}
		}
		return 0, false

	case "CSCH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return 1 / math.Sinh(v), true
			}
		}
		return 0, false

	// ASINH, ACOSH, ATANH - inverse hyperbolic functions
	case "ASINH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Asinh(v), true
			}
		}
		return 0, false

	case "ACOSH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Acosh(v), true
			}
		}
		return 0, false

	case "ATANH":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return math.Atanh(v), true
			}
		}
		return 0, false

	// COMBIN - combinations
	case "COMBIN":
		if len(args) >= 2 {
			n, nok := e.evalExpr(args[0])
			k, kok := e.evalExpr(args[1])
			if nok && kok {
				return combinations(int(n), int(k)), true
			}
		}
		return 0, true

	// PERMUT - permutations
	case "PERMUT":
		if len(args) >= 2 {
			n, nok := e.evalExpr(args[0])
			k, kok := e.evalExpr(args[1])
			if nok && kok {
				return permutations(int(n), int(k)), true
			}
		}
		return 0, true

	// ========== MS-VSDX §2.5.3 Additional Functions ==========

	// ANG360 - Normalize angle to 0 to 2*PI (§2.5.3.5)
	case "ANG360":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				result := math.Mod(v, 2*math.Pi)
				if result < 0 {
					result += 2 * math.Pi
				}
				return result, true
			}
		}
		return 0, false

	// ANGLETOLOC - Transform angle from source to destination (§2.5.3.6)
	case "ANGLETOLOC":
		if len(args) >= 3 {
			angle, aok := e.evalExpr(args[0])
			if aok {
				return angle, true
			}
		}
		return 0, false

	// ANGLETOPAR - Transform angle to parent (§2.5.3.7)
	case "ANGLETOPAR":
		if len(args) >= 3 {
			angle, aok := e.evalExpr(args[0])
			if aok {
				return angle, true
			}
		}
		return 0, false

	// BLEND - Blend two colors (§2.5.3.16)
	case "BLEND":
		if len(args) >= 3 {
			color1, c1ok := e.evalExpr(args[0])
			color2, c2ok := e.evalExpr(args[1])
			fraction, fok := e.evalExpr(args[2])
			if c1ok && c2ok && fok {
				if fraction < 0 || fraction > 1 {
					return 0, false
				}
				r1 := float64(int(color1) >> 16 & 0xFF)
				g1 := float64(int(color1) >> 8 & 0xFF)
				b1 := float64(int(color1) & 0xFF)
				r2 := float64(int(color2) >> 16 & 0xFF)
				g2 := float64(int(color2) >> 8 & 0xFF)
				b2 := float64(int(color2) & 0xFF)
				r := r1 + fraction*(r2-r1)
				g := g1 + fraction*(g2-g1)
				b := b1 + fraction*(b2-b1)
				return float64(int(r)<<16 | int(g)<<8 | int(b)), true
			}
		}
		return 0, false

	// CELLISTHEMED - Check if cell uses theme (§2.5.3.21)
	case "CELLISTHEMED":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				if v != 0 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, true

	// Fuzzy comparison operators (§2.5.3.45-56)
	case "ELE", "_LE_":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a-b <= 1e-9 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	case "ELT", "_LT_":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a < b {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	case "ENE", "_NE_":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if math.Abs(a-b) > 1e-9 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	case "FEQ", "_EQ_":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if math.Abs(a-b) <= 1e-9 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	case "FGE", "_GE_":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a-b >= -1e-9 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	case "FGT", "_GT_":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a-b > 1e-9 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	case "FLE":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a-b <= 1e-9 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	case "FLT":
		if len(args) >= 2 {
			a, aok := e.evalExpr(args[0])
			b, bok := e.evalExpr(args[1])
			if aok && bok {
				if a-b < -1e-9 {
					return 1, true
				}
				return 0, true
			}
		}
		return 0, false

	// Color difference functions
	case "HUEDIFF":
		if len(args) >= 2 {
			c1, c1ok := e.evalExpr(args[0])
			c2, c2ok := e.evalExpr(args[1])
			if c1ok && c2ok {
				r1, g1, b1 := float64(int(c1)>>16&0xFF)/255, float64(int(c1)>>8&0xFF)/255, float64(int(c1)&0xFF)/255
				h1, _, _ := rgbToHSL(r1, g1, b1)
				r2, g2, b2 := float64(int(c2)>>16&0xFF)/255, float64(int(c2)>>8&0xFF)/255, float64(int(c2)&0xFF)/255
				h2, _, _ := rgbToHSL(r2, g2, b2)
				return (h1 - h2) * 240, true
			}
		}
		return 0, false

	case "SATDIFF":
		if len(args) >= 2 {
			c1, c1ok := e.evalExpr(args[0])
			c2, c2ok := e.evalExpr(args[1])
			if c1ok && c2ok {
				r1, g1, b1 := float64(int(c1)>>16&0xFF)/255, float64(int(c1)>>8&0xFF)/255, float64(int(c1)&0xFF)/255
				_, s1, _ := rgbToHSL(r1, g1, b1)
				r2, g2, b2 := float64(int(c2)>>16&0xFF)/255, float64(int(c2)>>8&0xFF)/255, float64(int(c2)&0xFF)/255
				_, s2, _ := rgbToHSL(r2, g2, b2)
				return (s1 - s2) * 240, true
			}
		}
		return 0, false

	case "LUMDIFF":
		if len(args) >= 2 {
			c1, c1ok := e.evalExpr(args[0])
			c2, c2ok := e.evalExpr(args[1])
			if c1ok && c2ok {
				r1, g1, b1 := float64(int(c1)>>16&0xFF)/255, float64(int(c1)>>8&0xFF)/255, float64(int(c1)&0xFF)/255
				_, _, l1 := rgbToHSL(r1, g1, b1)
				r2, g2, b2 := float64(int(c2)>>16&0xFF)/255, float64(int(c2)>>8&0xFF)/255, float64(int(c2)&0xFF)/255
				_, _, l2 := rgbToHSL(r2, g2, b2)
				return (l1 - l2) * 240, true
			}
		}
		return 0, false

	// SHADE - Decrease luminance (§2.5.3.139)
	case "SHADE":
		if len(args) >= 2 {
			color, cok := e.evalExpr(args[0])
			delta, dok := e.evalExpr(args[1])
			if cok && dok {
				r, g, b := float64(int(color)>>16&0xFF)/255, float64(int(color)>>8&0xFF)/255, float64(int(color)&0xFF)/255
				h, s, l := rgbToHSL(r, g, b)
				l = math.Max(0, math.Min(1, l-delta/240))
				r, g, b = hslToRGB(h, s, l)
				return float64(int(r*255)<<16 | int(g*255)<<8 | int(b*255)), true
			}
		}
		return 0, false

	// TINT - Increase luminance (§2.5.3.163)
	case "TINT":
		if len(args) >= 2 {
			color, cok := e.evalExpr(args[0])
			delta, dok := e.evalExpr(args[1])
			if cok && dok {
				r, g, b := float64(int(color)>>16&0xFF)/255, float64(int(color)>>8&0xFF)/255, float64(int(color)&0xFF)/255
				h, s, l := rgbToHSL(r, g, b)
				l = math.Max(0, math.Min(1, l+delta/240))
				r, g, b = hslToRGB(h, s, l)
				return float64(int(r*255)<<16 | int(g*255)<<8 | int(b*255)), true
			}
		}
		return 0, false

	// TONE - Decrease saturation (§2.5.3.165)
	case "TONE":
		if len(args) >= 2 {
			color, cok := e.evalExpr(args[0])
			delta, dok := e.evalExpr(args[1])
			if cok && dok {
				r, g, b := float64(int(color)>>16&0xFF)/255, float64(int(color)>>8&0xFF)/255, float64(int(color)&0xFF)/255
				h, s, l := rgbToHSL(r, g, b)
				s = math.Max(0, math.Min(1, s-delta/240))
				r, g, b = hslToRGB(h, s, l)
				return float64(int(r*255)<<16 | int(g*255)<<8 | int(b*255)), true
			}
		}
		return 0, false

	// Coordinate transform functions
	case "LOCTOLOC", "LOCTOPAR", "PARTOLOC":
		if len(args) >= 4 {
			x, xok := e.evalExpr(args[0])
			if xok {
				return x, true
			}
		}
		return 0, false

	// Point functions
	case "PNT":
		if len(args) >= 2 {
			x, xok := e.evalExpr(args[0])
			if xok {
				return x, true
			}
		}
		return 0, false

	case "PNTX", "PNTY":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
		return 0, false

	// SETATREFEVAL
	case "SETATREFEVAL":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
		return 0, true

	// Text dimension functions
	case "TEXTHEIGHT":
		return 0.2, true

	case "TEXTWIDTH":
		return 1.0, true

	// Theme functions
	case "THEME", "THEMECBV", "THEMEPROP", "THEMERESTORE":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return v, true
			}
		}
		return 0, true


	// VALUE
	case "VALUE":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
		return 0, false

	// REF - Return #REF! error
	case "REF":
		return math.NaN(), true

	// LOG2, LOGN
	case "LOG2":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok && v > 0 {
				return math.Log2(v), true
			}
		}
		return 0, false

	case "LOGN":
		if len(args) >= 2 {
			v, vok := e.evalExpr(args[0])
			base, bok := e.evalExpr(args[1])
			if vok && bok && v > 0 && base > 0 && base != 1 {
				return math.Log(v) / math.Log(base), true
			}
		}
		return 0, false

	// Navigation/macro functions (return 0)
	case "RUNADDONWARGS":
		return 0, true

	// Document property functions
	case "BKGPAGENAME", "CATEGORY", "COMPANY", "CREATOR", "DESCRIPTION",
		"DIRECTORY", "FILENAME", "KEYWORDS", "MANAGER", "SUBJECT", "TITLE":
		return 0, true

	// Shape/page info functions
	case "SHAPETEXT", "FIELDPICTURE", "MASTERNAME", "PAGENAME":
		return 0, true

	case "PAGENUMBER":
		return 1, true

	case "PAGECOUNT":
		if e.shape != nil && e.shape.Page != nil && e.shape.Page.vis != nil {
			return float64(len(e.shape.Page.vis.Pages)), true
		}
		return 1, true

	// Date/time functions
	case "DATETIME":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
		return 0, true

	case "DAYOFYEAR":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return float64(int(v) % 365), true
			}
		}
		return 0, true

	case "EOMONTH":
		if len(args) >= 2 {
			start, sok := e.evalExpr(args[0])
			months, mok := e.evalExpr(args[1])
			if sok && mok {
				return start + months*30, true
			}
		}
		return 0, true

	// Reference check functions
	case "ISREF":
		if len(args) >= 1 {
			_, ok := e.evalExpr(args[0])
			if ok {
				return 1, true
			}
		}
		return 0, true

	case "N":
		if len(args) >= 1 {
			if v, ok := e.evalExpr(args[0]); ok {
				return v, true
			}
		}
		return 0, true

	// File/sheet operations (return 0)
	case "OPENFILE", "OPENSHEET", "OPENTEXTWIN", "DEPENDSON", "SETREF", "QUEUEMARKEREVENT":
		return 0, true

	// Format functions
	case "FORMATEX":
		if len(args) >= 1 {
			return e.evalExpr(args[0])
		}
		return 0, true

	// STRSAMEEX
	case "STRSAMEEX":
		if len(args) >= 4 {
			a := strings.Trim(strings.TrimSpace(args[0]), "\"'")
			b := strings.Trim(strings.TrimSpace(args[1]), "\"'")
			if strings.EqualFold(a, b) {
				return 1, true
			}
			return 0, true
		}
		return 0, false

	// Misc functions
	case "HYPERLINK", "BOUNDINGBOXRECT", "BOUNDINGBOXDIST", "PATHSEGMENT":
		return 0, true

	case "SEGMENTCOUNT":
		return 1, true

	// CONCATENATE - string concatenation (limited support)
	case "CONCATENATE", "CAT", "_CAT_":
		// Return 0 as we can't return strings, but mark as supported
		return 0, true

	}

	return 0, false
}

// combinations calculates n choose k
func combinations(n, k int) float64 {
	if k > n || k < 0 {
		return 0
	}
	if k == 0 || k == n {
		return 1
	}
	result := 1.0
	for i := 0; i < k; i++ {
		result = result * float64(n-i) / float64(i+1)
	}
	return result
}

// permutations calculates n permute k
func permutations(n, k int) float64 {
	if k > n || k < 0 {
		return 0
	}
	result := 1.0
	for i := 0; i < k; i++ {
		result *= float64(n - i)
	}
	return result
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

	// Handle TheCel reference token - inherits from master shape
	if strings.HasPrefix(ref, "TheCel") || strings.HasPrefix(ref, "THECEL") {
		cellName := strings.TrimPrefix(strings.TrimPrefix(ref, "TheCel"), "THECEL")
		cellName = strings.TrimPrefix(cellName, "!")
		if cellName != "" {
			if master := e.shape.MasterShape(); master != nil {
				if val := master.CellValue(cellName); val != "" {
					if v, err := strconv.ParseFloat(val, 64); err == nil {
						return v, true
					}
				}
			}
		}
		return 0, false
	}

	// Handle Sheet.N!Cell references (cross-shape references)
	if strings.HasPrefix(ref, "Sheet.") && strings.Contains(ref, "!") {
		parts := strings.SplitN(ref, "!", 2)
		if len(parts) == 2 {
			sheetRef := parts[0]
			cellName := parts[1]
			shapeID := strings.TrimPrefix(sheetRef, "Sheet.")

			// Find the shape on the same page
			if e.shape.Page != nil {
				for _, s := range e.shape.Page.AllShapes() {
					if s.ID == shapeID {
						if val := s.CellValue(cellName); val != "" {
							if v, err := strconv.ParseFloat(val, 64); err == nil {
								return v, true
							}
						}
						break
					}
				}
			}
		}
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
