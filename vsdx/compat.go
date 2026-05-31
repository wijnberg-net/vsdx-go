package vsdx

import (
	"strings"

	"github.com/beevik/etree"
)

// ProcessMarkupCompatibility processes mc:AlternateContent elements in an XML document,
// replacing them with the appropriate fallback content per MS-VSDX §2.2.10.
// This ensures compatibility with files that contain extension elements.
func ProcessMarkupCompatibility(doc *etree.Document) {
	if doc == nil || doc.Root() == nil {
		return
	}
	processElement(doc.Root())
}

// processElement recursively processes an element and its children for mc:AlternateContent.
func processElement(elem *etree.Element) {
	// Process children first (depth-first) since we may be modifying the tree
	children := elem.ChildElements()
	for _, child := range children {
		processElement(child)
	}

	// Process mc:AlternateContent elements at this level
	for _, child := range elem.ChildElements() {
		if isAlternateContent(child) {
			replacement := selectAlternateContent(child)
			if replacement != nil {
				// Find index of mc:AlternateContent
				insertIndex := findChildIndex(elem, child)
				elem.RemoveChild(child)
				// Insert replacement content's children at the same position
				for i, rc := range replacement.ChildElements() {
					elem.InsertChildAt(insertIndex+i, rc.Copy())
				}
			} else {
				// No suitable content found, just remove the element
				elem.RemoveChild(child)
			}
		}
	}
}

// findChildIndex returns the index of a child element within its parent.
func findChildIndex(parent, child *etree.Element) int {
	for i, c := range parent.ChildElements() {
		if c == child {
			return i
		}
	}
	return 0
}

// isAlternateContent returns true if the element is mc:AlternateContent.
func isAlternateContent(elem *etree.Element) bool {
	// Check both fully qualified and prefixed forms
	if elem.Tag == "AlternateContent" {
		if elem.Space == "mc" || elem.NamespaceURI() == McCompatNS {
			return true
		}
	}
	// Check for mc: prefix in tag name (some parsers don't separate namespace)
	if strings.HasPrefix(elem.Tag, "mc:AlternateContent") {
		return true
	}
	return false
}

// selectAlternateContent selects the appropriate content from mc:AlternateContent.
// Per spec: prefer mc:Choice if we understand the Requires namespace, else use mc:Fallback.
func selectAlternateContent(ac *etree.Element) *etree.Element {
	var fallback *etree.Element

	for _, child := range ac.ChildElements() {
		tag := child.Tag
		space := child.Space

		// Check for mc:Choice
		if (tag == "Choice" && (space == "mc" || child.NamespaceURI() == McCompatNS)) ||
			strings.HasPrefix(tag, "mc:Choice") {
			// Check if we support the required namespace
			requires := child.SelectAttrValue("Requires", "")
			if requires == "" || isKnownNamespacePrefix(requires, ac) {
				// We can handle this choice
				return child
			}
		}

		// Check for mc:Fallback
		if (tag == "Fallback" && (space == "mc" || child.NamespaceURI() == McCompatNS)) ||
			strings.HasPrefix(tag, "mc:Fallback") {
			fallback = child
		}
	}

	// Return fallback if no suitable Choice was found
	return fallback
}

// isKnownNamespacePrefix checks if we know how to handle the given namespace prefix.
// The prefix is resolved using namespace declarations on the element or ancestors.
func isKnownNamespacePrefix(prefix string, elem *etree.Element) bool {
	// Resolve the prefix to a namespace URI
	ns := resolveNamespacePrefix(prefix, elem)
	if ns == "" {
		return false
	}

	// Check if it's a namespace we understand
	knownNamespaces := map[string]bool{
		MainNS:     true,
		RelNS:      true,
		PkgRelNS:   true,
		ContentNS:  true,
		ExtPropNS:  true,
		VtNS:       true,
		CorePropNS: true,
		DcNS:       true,
		DcTermsNS:  true,
		McCompatNS: true,
	}

	return knownNamespaces[ns]
}

// resolveNamespacePrefix resolves a namespace prefix to its URI by walking up the tree.
func resolveNamespacePrefix(prefix string, elem *etree.Element) string {
	for e := elem; e != nil; e = e.Parent() {
		// Check xmlns:prefix attributes
		for _, attr := range e.Attr {
			if attr.Space == "xmlns" && attr.Key == prefix {
				return attr.Value
			}
			// Handle xmlns without space separation
			if attr.Key == "xmlns:"+prefix {
				return attr.Value
			}
		}
	}
	return ""
}

// GetIgnorableNamespaces returns the list of namespace prefixes from mc:Ignorable attribute.
func GetIgnorableNamespaces(elem *etree.Element) []string {
	ignorable := elem.SelectAttrValue("mc:Ignorable", "")
	if ignorable == "" {
		// Try without namespace prefix
		ignorable = elem.SelectAttrValue("Ignorable", "")
	}
	if ignorable == "" {
		return nil
	}
	return strings.Fields(ignorable)
}

// StripIgnorableElements removes elements from ignored namespaces per mc:Ignorable.
// This is called after ProcessMarkupCompatibility to clean up any remaining unknown elements.
func StripIgnorableElements(doc *etree.Document) {
	if doc == nil || doc.Root() == nil {
		return
	}

	// Get ignorable prefixes from root
	ignorable := GetIgnorableNamespaces(doc.Root())
	if len(ignorable) == 0 {
		return
	}

	// Build a set of ignorable prefixes
	ignoreSet := make(map[string]bool)
	for _, prefix := range ignorable {
		ignoreSet[prefix] = true
	}

	stripIgnorableFromElement(doc.Root(), ignoreSet)
}

// stripIgnorableFromElement recursively removes elements with ignored namespace prefixes.
func stripIgnorableFromElement(elem *etree.Element, ignoreSet map[string]bool) {
	// Process children (make a copy since we may modify)
	children := elem.ChildElements()
	for _, child := range children {
		// Check if this element should be ignored
		if ignoreSet[child.Space] {
			elem.RemoveChild(child)
			continue
		}
		// Check for prefix in tag name
		if idx := strings.Index(child.Tag, ":"); idx > 0 {
			prefix := child.Tag[:idx]
			if ignoreSet[prefix] {
				elem.RemoveChild(child)
				continue
			}
		}
		// Recurse
		stripIgnorableFromElement(child, ignoreSet)
	}

	// Also strip ignorable attributes
	var attrsToRemove []string
	for _, attr := range elem.Attr {
		if ignoreSet[attr.Space] {
			attrsToRemove = append(attrsToRemove, attr.Key)
		}
	}
	for _, key := range attrsToRemove {
		elem.RemoveAttr(key)
	}
}
