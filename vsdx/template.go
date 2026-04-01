package vsdx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// expression evaluation regexes
var (
	reForLoop     = regexp.MustCompile(`{%\s*for\s+(\w+)\s+in\s+(\w+)\s*%}`)
	reShowIf      = regexp.MustCompile(`{%\s*showif\s+(.*?)\s*%}`)
	reSetSelf     = regexp.MustCompile(`{%\s*set\s+self\.(\w+)\s*=\s*(.*?)\s*%}`)
	rePlaceholder = regexp.MustCompile(`\{\{\s*(.*?)\s*\}\}`)
)

// RenderTemplate processes Jinja2-style template directives in all pages.
//
// Supported directives in shape text:
//   - {{key}} - Replace with context value
//   - {% for item in list %} - Duplicate shape for each item in list
//   - {% showif condition %} - Conditionally show/hide shape
//   - {% set self.x = value %} - Set shape property from expression
//
// Supported directives in page names:
//   - {% showif condition %} - Conditionally show/hide page
//
// Context values can be string, int, float64, bool, or []any (for loops).
func (v *VisioFile) RenderTemplate(context map[string]any) {
	// Process pages in reverse order so removal indices stay valid
	pagesToRemove := []int{}

	for i, page := range v.Pages {
		// Check page-level showif
		if !v.renderPageShowIf(page, context) {
			pagesToRemove = append(pagesToRemove, i)
			continue
		}

		// Process shapes
		for _, shapes := range page.shapes() {
			v.renderShapeSetSelfs(shapes, context)
			v.renderShapeForLoops(shapes, page, context)
			v.renderShapeShowIfs(shapes, context)
			v.renderShapeText(shapes, context)
		}
	}

	// Remove pages in reverse order
	for i := len(pagesToRemove) - 1; i >= 0; i-- {
		v.RemovePageByIndex(pagesToRemove[i])
	}
}

// renderPageShowIf checks if a page name contains {% showif %} and evaluates it.
// Returns true if page should be kept, false if it should be removed.
func (v *VisioFile) renderPageShowIf(page *Page, context map[string]any) bool {
	name := page.Name()
	matches := reShowIf.FindStringSubmatch(name)
	if matches == nil {
		return true // no showif, keep page
	}

	condition := matches[1]
	result := evaluateCondition(condition, context)

	if !result {
		return false // remove page
	}

	// Keep page but clean showif from name
	cleanName := reShowIf.ReplaceAllString(name, "")
	page.SetName(strings.TrimSpace(cleanName))
	return true
}

// renderShapeSetSelfs processes {% set self.prop = value %} directives in shapes.
func (v *VisioFile) renderShapeSetSelfs(shape *Shape, context map[string]any) {
	for _, child := range shape.ChildShapes() {
		v.renderShapeSetSelfs(child, context)
	}

	text := shape.Text()
	allMatches := reSetSelf.FindAllStringSubmatch(text, -1)
	if len(allMatches) == 0 {
		return
	}

	for _, match := range allMatches {
		propName := match[1]
		valueExpr := match[2]

		// Create extended context with self references
		extContext := make(map[string]any)
		for k, v := range context {
			extContext[k] = v
		}
		extContext["self.x"] = shape.X()
		extContext["self.y"] = shape.Y()
		extContext["self.width"] = shape.Width()
		extContext["self.height"] = shape.Height()

		value := evaluateNumericExpression(valueExpr, extContext)

		switch propName {
		case "x":
			shape.SetX(value)
		case "y":
			shape.SetY(value)
		case "width":
			shape.SetWidth(value)
		case "height":
			shape.SetHeight(value)
		}
	}

	// Remove set self directives from text
	cleanText := reSetSelf.ReplaceAllString(text, "")
	shape.SetText(cleanText)
}

// renderShapeForLoops processes {% for item in list %} directives.
func (v *VisioFile) renderShapeForLoops(shape *Shape, page *Page, context map[string]any) {
	children := shape.ChildShapes()
	for _, child := range children {
		v.renderShapeForLoops(child, page, context)
	}

	text := shape.Text()
	matches := reForLoop.FindStringSubmatch(text)
	if matches == nil {
		return
	}

	itemVar := matches[1] // e.g., "o"
	listVar := matches[2] // e.g., "test_list"

	listVal, ok := context[listVar]
	if !ok {
		return
	}

	items := toSlice(listVal)
	if len(items) == 0 {
		return
	}

	// Clear the for loop directive from the original shape text
	cleanText := reForLoop.ReplaceAllString(text, "")
	shape.SetText(cleanText)

	// Save a deep copy of the shape XML before applying first item's text,
	// so that copies for subsequent items retain {{ }} template placeholders.
	templateXML := shape.XML().Copy()

	// First item uses the original shape
	itemContext := mergeContext(context, map[string]any{itemVar: items[0]})
	applyTextContext(shape, itemContext)

	// Process showifs on child shapes of the original
	v.renderShapeShowIfs(shape, itemContext)

	// Duplicate shape for remaining items
	page.SetMaxIDs()
	var delta float64

	for idx := 1; idx < len(items); idx++ {
		item := items[idx]
		newShapeElem := v.CopyShape(templateXML, page)
		newShape := newShape(newShapeElem, shape.Parent, page)

		// Apply item context
		loopContext := mergeContext(context, map[string]any{itemVar: item})
		applyTextContext(newShape, loopContext)

		// Process showifs on children of the copy
		v.renderShapeShowIfs(newShape, loopContext)

		// Move copy down
		delta += newShape.Height()
		newShape.Move(0, -delta)
	}
}

// renderShapeShowIfs processes {% showif condition %} directives on shapes.
func (v *VisioFile) renderShapeShowIfs(shape *Shape, context map[string]any) {
	// Process children first (in reverse to handle removals)
	children := shape.ChildShapes()
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		v.renderShapeShowIfs(child, context)

		childText := child.Text()
		showIfMatches := reShowIf.FindAllStringSubmatch(childText, -1)
		for _, match := range showIfMatches {
			condition := match[1]
			if !evaluateCondition(condition, context) {
				child.Remove()
				break
			}
			// Keep shape but remove showif from text
			cleanText := reShowIf.ReplaceAllString(childText, "")
			child.SetText(cleanText)
		}
	}
}

// renderShapeText replaces {{key}} placeholders in shape text.
func (v *VisioFile) renderShapeText(shape *Shape, context map[string]any) {
	for _, child := range shape.ChildShapes() {
		v.renderShapeText(child, context)
	}

	text := shape.Text()
	if !strings.Contains(text, "{{") {
		return
	}

	newText := rePlaceholder.ReplaceAllStringFunc(text, func(match string) string {
		inner := rePlaceholder.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		expr := strings.TrimSpace(inner[1])
		return evaluateTextExpression(expr, context)
	})

	if newText != text {
		shape.SetText(newText)
	}
}

// --- Expression evaluation ---

// evaluateCondition evaluates a condition string to a boolean.
// Supports: variable truthiness, "not var", "var > num", "var < num", "var == num"
func evaluateCondition(expr string, context map[string]any) bool {
	expr = strings.TrimSpace(expr)

	// Handle "not variable"
	if strings.HasPrefix(expr, "not ") {
		inner := strings.TrimSpace(expr[4:])
		return !evaluateCondition(inner, context)
	}

	// Handle comparison operators
	for _, op := range []string{">=", "<=", "!=", "==", ">", "<"} {
		if parts := strings.SplitN(expr, op, 2); len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			leftVal := resolveNumeric(left, context)
			rightVal := resolveNumeric(right, context)
			switch op {
			case ">":
				return leftVal > rightVal
			case "<":
				return leftVal < rightVal
			case ">=":
				return leftVal >= rightVal
			case "<=":
				return leftVal <= rightVal
			case "==":
				return leftVal == rightVal
			case "!=":
				return leftVal != rightVal
			}
		}
	}

	// Simple truthiness check
	val, ok := context[expr]
	if !ok {
		return false
	}
	return isTruthy(val)
}

// evaluateNumericExpression evaluates a numeric expression.
// Supports: numbers, variables, "a*b", "a+b", "a-b", "a/b", "a if b else c"
func evaluateNumericExpression(expr string, context map[string]any) float64 {
	expr = strings.TrimSpace(expr)

	// Handle ternary: "value_if_true if condition else value_if_false"
	if parts := strings.SplitN(expr, " if ", 2); len(parts) == 2 {
		trueVal := strings.TrimSpace(parts[0])
		if elseParts := strings.SplitN(parts[1], " else ", 2); len(elseParts) == 2 {
			condStr := strings.TrimSpace(elseParts[0])
			falseVal := strings.TrimSpace(elseParts[1])
			if evaluateCondition(condStr, context) {
				return evaluateNumericExpression(trueVal, context)
			}
			return evaluateNumericExpression(falseVal, context)
		}
	}

	// Handle binary operators (order: +, -, *, /)
	// Note: handle + and - first (lower precedence), but avoid splitting negative numbers
	for _, op := range []string{"+", "-"} {
		// Find operator not at start of string
		idx := findOperator(expr, op)
		if idx > 0 {
			left := strings.TrimSpace(expr[:idx])
			right := strings.TrimSpace(expr[idx+1:])
			leftVal := resolveNumeric(left, context)
			rightVal := resolveNumeric(right, context)
			if op == "+" {
				return leftVal + rightVal
			}
			return leftVal - rightVal
		}
	}

	for _, op := range []string{"*", "/"} {
		if idx := strings.LastIndex(expr, op); idx > 0 {
			left := strings.TrimSpace(expr[:idx])
			right := strings.TrimSpace(expr[idx+1:])
			leftVal := resolveNumeric(left, context)
			rightVal := resolveNumeric(right, context)
			if op == "*" {
				return leftVal * rightVal
			}
			if rightVal != 0 {
				return leftVal / rightVal
			}
			return 0
		}
	}

	return resolveNumeric(expr, context)
}

// evaluateTextExpression evaluates a text expression for {{}} replacement.
func evaluateTextExpression(expr string, context map[string]any) string {
	expr = strings.TrimSpace(expr)

	// Try as arithmetic expression first (e.g., "x*y")
	for _, op := range []string{"*", "+", "-", "/"} {
		if strings.Contains(expr, op) {
			result := evaluateNumericExpression(expr, context)
			// Return clean integer if possible
			if result == float64(int(result)) {
				return strconv.Itoa(int(result))
			}
			return strconv.FormatFloat(result, 'f', -1, 64)
		}
	}

	// Simple variable lookup
	if val, ok := context[expr]; ok {
		return fmt.Sprintf("%v", val)
	}

	return "{{" + expr + "}}" // leave unreplaced if not found
}

// resolveNumeric resolves a string to a float64 (either a literal or context variable).
func resolveNumeric(s string, context map[string]any) float64 {
	s = strings.TrimSpace(s)

	// Try as literal number
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Try as context variable (including "self.x" etc.)
	if val, ok := context[s]; ok {
		return toInterfaceFloat(val)
	}

	return 0
}

// findOperator finds the index of an operator, skipping the start of the string
// (to avoid treating negative numbers as subtraction).
func findOperator(expr, op string) int {
	for i := len(expr) - 1; i > 0; i-- {
		if string(expr[i]) == op {
			// Check that left side is not empty
			left := strings.TrimSpace(expr[:i])
			if left != "" {
				return i
			}
		}
	}
	return -1
}

// isTruthy returns whether a value is truthy (non-zero, non-empty, non-false).
func isTruthy(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != "" && v != "0" && v != "false" && v != "False"
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	case []int:
		return len(v) > 0
	case nil:
		return false
	default:
		return true
	}
}

// toInterfaceFloat converts an any value to float64.
func toInterfaceFloat(val any) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case bool:
		if v {
			return 1
		}
		return 0
	}
	return 0
}

// toSlice converts a context value to a []any slice.
func toSlice(val any) []any {
	switch v := val.(type) {
	case []any:
		return v
	case []string:
		result := make([]any, len(v))
		for i, s := range v {
			result[i] = s
		}
		return result
	case []int:
		result := make([]any, len(v))
		for i, n := range v {
			result[i] = n
		}
		return result
	case []float64:
		result := make([]any, len(v))
		for i, f := range v {
			result[i] = f
		}
		return result
	}
	return nil
}

// mergeContext creates a new context with additional values merged in.
func mergeContext(base, extra map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

// applyTextContext replaces {{key}} placeholders in a shape and its children.
func applyTextContext(shape *Shape, context map[string]any) {
	text := shape.Text()
	if strings.Contains(text, "{{") {
		newText := rePlaceholder.ReplaceAllStringFunc(text, func(match string) string {
			inner := rePlaceholder.FindStringSubmatch(match)
			if len(inner) < 2 {
				return match
			}
			expr := strings.TrimSpace(inner[1])
			return evaluateTextExpression(expr, context)
		})
		if newText != text {
			shape.SetText(newText)
		}
	}
	for _, child := range shape.ChildShapes() {
		applyTextContext(child, context)
	}
}
