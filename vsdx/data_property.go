package vsdx

import "github.com/beevik/etree"

// DataProperty represents a single data property item associated with a Shape object.
type DataProperty struct {
	xml       *etree.Element
	shape     *Shape
	Name      string
	Label     string
	ValueType string
	Prompt    string
	SortKey   string
}

func newDataProperty(xml *etree.Element, shape *Shape) *DataProperty {
	dp := &DataProperty{
		xml:   xml,
		shape: shape,
		Name:  xml.SelectAttrValue("N", ""),
	}

	labelCell := xml.FindElement("Cell[@N='Label']")

	if labelCell != nil {
		dp.Label = labelCell.SelectAttrValue("V", "")

		if typeCell := xml.FindElement("Cell[@N='Type']"); typeCell != nil {
			dp.ValueType = typeCell.SelectAttrValue("V", "")
		}
		if promptCell := xml.FindElement("Cell[@N='Prompt']"); promptCell != nil {
			dp.Prompt = promptCell.SelectAttrValue("V", "")
		}
		if sortKeyCell := xml.FindElement("Cell[@N='SortKey']"); sortKeyCell != nil {
			dp.SortKey = sortKeyCell.SelectAttrValue("V", "")
		}
	} else {
		// Overridden master shape properties have no label - only a name and value.
		// Look up the master shape's properties to get label/type/prompt/sortkey.
		masterShape := shape.MasterShape()
		if masterShape != nil {
			masterProps := masterShape.DataProperties()
			for _, mp := range masterProps {
				if mp.Name == dp.Name {
					dp.Label = mp.Label
					dp.ValueType = mp.ValueType
					dp.Prompt = mp.Prompt
					dp.SortKey = mp.SortKey
					break
				}
			}
		}
	}

	return dp
}

// Value returns the value of the data property.
func (dp *DataProperty) Value() string {
	valueCell := dp.xml.FindElement("Cell[@N='Value']")
	if valueCell == nil {
		return ""
	}

	// Try V attribute first
	if v := valueCell.SelectAttrValue("V", ""); v != "" {
		return v
	}
	// Fall back to element inner text
	if text := valueCell.Text(); text != "" {
		return text
	}
	return ""
}

// SetValue sets the value of the data property.
func (dp *DataProperty) SetValue(value string) {
	valueCell := dp.xml.FindElement("Cell[@N='Value']")
	if valueCell == nil {
		return
	}

	if valueCell.SelectAttrValue("V", "") != "" {
		valueCell.CreateAttr("V", value)
	} else if valueCell.Text() != "" {
		valueCell.SetText(value)
	}
}
