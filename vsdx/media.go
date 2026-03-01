package vsdx

import (
	_ "embed"
)

//go:embed media/media.vsdx
var mediaVsdxData []byte

// Media provides access to template shapes from the embedded media.vsdx file.
type Media struct {
	vis *VisioFile
}

// NewMedia opens the embedded media.vsdx and returns a Media instance.
func NewMedia() (*Media, error) {
	vis, err := OpenBytes(mediaVsdxData)
	if err != nil {
		return nil, err
	}
	return &Media{vis: vis}, nil
}

// Close releases resources.
func (m *Media) Close() {
	m.vis.Close()
}

// StraightConnector returns the straight connector template shape.
func (m *Media) StraightConnector() *Shape {
	return m.vis.Pages[0].FindShapeByText("STRAIGHT_CONNECTOR")
}

// CurvedConnector returns the curved connector template shape.
func (m *Media) CurvedConnector() *Shape {
	return m.vis.Pages[0].FindShapeByText("CURVED_CONNECTOR")
}

// Rectangle returns the rectangle template shape.
func (m *Media) Rectangle() *Shape {
	return m.vis.Pages[0].FindShapeByText("RECTANGLE")
}

// Circle returns the circle template shape.
func (m *Media) Circle() *Shape {
	return m.vis.Pages[0].FindShapeByText("CIRCLE")
}

// Line returns the line template shape.
func (m *Media) Line() *Shape {
	return m.vis.Pages[0].FindShapeByText("LINE")
}

// RelsXML returns the page rels XML from the media template (for master page setup).
func (m *Media) RelsXML() []byte {
	page := m.vis.Pages[0]
	if page.RelsXML != nil {
		data, _ := page.RelsXML.WriteToBytes()
		return data
	}
	return nil
}

// VisioFile returns the underlying VisioFile for advanced access.
func (m *Media) VisioFile() *VisioFile {
	return m.vis
}
