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

	// Line style cells
	CellLineWeight  CellName = "LineWeight"
	CellLineColor   CellName = "LineColor"
	CellLinePattern CellName = "LinePattern"
	CellLineCap     CellName = "LineCap"
	CellBeginArrow  CellName = "BeginArrow"
	CellEndArrow    CellName = "EndArrow"
	CellRounding    CellName = "Rounding"

	// Fill style cells
	CellFillForegnd      CellName = "FillForegnd"
	CellFillBkgnd        CellName = "FillBkgnd"
	CellFillPattern      CellName = "FillPattern"
	CellFillForegndTrans CellName = "FillForegndTrans"
	CellFillBkgndTrans   CellName = "FillBkgndTrans"

	// Layer cell
	CellLayerMember CellName = "LayerMember"

	// Text block cells
	CellTxtPinX    CellName = "TxtPinX"
	CellTxtPinY    CellName = "TxtPinY"
	CellTxtLocPinX CellName = "TxtLocPinX"
	CellTxtLocPinY CellName = "TxtLocPinY"
	CellTxtWidth   CellName = "TxtWidth"
	CellTxtHeight  CellName = "TxtHeight"
	CellTxtAngle   CellName = "TxtAngle"

	// Protection cells
	CellLockWidth  CellName = "LockWidth"
	CellLockHeight CellName = "LockHeight"
	CellLockMoveX  CellName = "LockMoveX"
	CellLockMoveY  CellName = "LockMoveY"
	CellLockDelete CellName = "LockDelete"
	CellLockRotate CellName = "LockRotate"
	CellLockAspect CellName = "LockAspect"

	// Trigger cells
	CellBegTrigger CellName = "BegTrigger"
	CellEndTrigger CellName = "EndTrigger"

	// Page dimension cells
	CellPageWidth  CellName = "PageWidth"
	CellPageHeight CellName = "PageHeight"

	// 3D/Bevel effect cells (MS-VSDX §2.2.7.3.2)
	CellBevelTopType      CellName = "BevelTopType"
	CellBevelTopWidth     CellName = "BevelTopWidth"
	CellBevelTopHeight    CellName = "BevelTopHeight"
	CellBevelBottomType   CellName = "BevelBottomType"
	CellBevelBottomWidth  CellName = "BevelBottomWidth"
	CellBevelBottomHeight CellName = "BevelBottomHeight"
	CellBevelDepthColor   CellName = "BevelDepthColor"
	CellBevelDepthSize    CellName = "BevelDepthSize"
	CellBevelContourColor CellName = "BevelContourColor"
	CellBevelContourSize  CellName = "BevelContourSize"
	CellBevelMaterialType CellName = "BevelMaterialType"
	CellBevelLightingType CellName = "BevelLightingType"
	CellBevelLightingAngle CellName = "BevelLightingAngle"

	// Glow effect cells (MS-VSDX §2.2.7.3.3)
	CellGlowColor      CellName = "GlowColor"
	CellGlowColorTrans CellName = "GlowColorTrans"
	CellGlowSize       CellName = "GlowSize"

	// Reflection effect cells (MS-VSDX §2.2.7.3.4)
	CellReflectionSize  CellName = "ReflectionSize"
	CellReflectionTrans CellName = "ReflectionTrans"
	CellReflectionDist  CellName = "ReflectionDist"
	CellReflectionBlur  CellName = "ReflectionBlur"

	// Soft edges effect cells (MS-VSDX §2.2.7.3.5)
	CellSoftEdgesSize CellName = "SoftEdgesSize"

	// Sketch effect cells (MS-VSDX §2.2.7.3.6)
	CellSketchEnabled    CellName = "SketchEnabled"
	CellSketchSeed       CellName = "SketchSeed"
	CellSketchAmount     CellName = "SketchAmount"
	CellSketchLineWeight CellName = "SketchLineWeight"
	CellSketchLineChange CellName = "SketchLineChange"
	CellSketchFillChange CellName = "SketchFillChange"

	// 3D Rotation effect cells (MS-VSDX §2.2.7.3.7)
	CellRotationXAngle     CellName = "RotationXAngle"
	CellRotationYAngle     CellName = "RotationYAngle"
	CellRotationZAngle     CellName = "RotationZAngle"
	CellRotationType       CellName = "RotationType"
	CellPerspective        CellName = "Perspective"
	CellDistanceFromGround CellName = "DistanceFromGround"
	CellKeepTextFlat       CellName = "KeepTextFlat"

	// QuickStyle cells (MS-VSDX §2.2.7.4.3, §2.2.7.4.4)
	CellQuickStyleType          CellName = "QuickStyleType"
	CellQuickStyleLineMatrix    CellName = "QuickStyleLineMatrix"
	CellQuickStyleFillMatrix    CellName = "QuickStyleFillMatrix"
	CellQuickStyleEffectsMatrix CellName = "QuickStyleEffectsMatrix"
	CellQuickStyleFontMatrix    CellName = "QuickStyleFontMatrix"
	CellQuickStyleLineColor     CellName = "QuickStyleLineColor"
	CellQuickStyleFillColor     CellName = "QuickStyleFillColor"
	CellQuickStyleShadowColor   CellName = "QuickStyleShadowColor"
	CellQuickStyleFontColor     CellName = "QuickStyleFontColor"
	CellQuickStyleVariation     CellName = "QuickStyleVariation"
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
