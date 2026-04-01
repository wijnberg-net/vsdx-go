package vsdx

// CellName is a type alias for cell name strings used in VSDX XML.
// Using a type alias (not a defined type) keeps backward compatibility
// with existing code that passes raw strings.
type CellName = string

// Cell name constants for shape properties.
const (
	// Position cells
	CellPinX    CellName = "PinX"
	CellPinY    CellName = "PinY"
	CellLocPinX CellName = "LocPinX"
	CellLocPinY CellName = "LocPinY"
	CellBeginX  CellName = "BeginX"
	CellBeginY  CellName = "BeginY"
	CellEndX    CellName = "EndX"
	CellEndY    CellName = "EndY"

	// Size cells
	CellWidth  CellName = "Width"
	CellHeight CellName = "Height"
	CellAngle  CellName = "Angle"

	// Style cells
	CellLineWeight  CellName = "LineWeight"
	CellLineColor   CellName = "LineColor"
	CellFillForegnd CellName = "FillForegnd"
	CellEndArrow    CellName = "EndArrow"

	// Text pin cells
	CellTxtPinX CellName = "TxtPinX"
	CellTxtPinY CellName = "TxtPinY"

	// Trigger cells
	CellBegTrigger CellName = "BegTrigger"
	CellEndTrigger CellName = "EndTrigger"

	// Page dimension cells
	CellPageWidth  CellName = "PageWidth"
	CellPageHeight CellName = "PageHeight"
)

// Connect part constants used in FromPart/ToPart attributes.
const (
	PartWholeShape = "3"  // Connection to whole shape (PinX/PinY)
	PartBeginX     = "9"  // Connection to BeginX
	PartEndX       = "12" // Connection to EndX
)

// Connect cell name constants used in FromCell/ToCell attributes.
const (
	ConnCellBeginX = "BeginX"
	ConnCellEndX   = "EndX"
	ConnCellPinX   = "PinX"
)
