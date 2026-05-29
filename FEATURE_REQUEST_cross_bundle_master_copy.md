# Feature Request: Cross-Bundle Master Copy

**Requested by:** VisiGo (https://github.com/MichelW6667/VisiGo)
**Date:** 2026-05-29
**Priority:** High — blocks lossless stencil-library import
**Affects:** `vsdx.VisioFile`, `vsdx.Stencil`, master-rel + media handling

---

## Summary

vsdx-go currently lets callers:

- Open a `.vsdx`/`.vsdm`/`.vssx`/`.vssm` and enumerate `MasterPages`
- Add a master to a freshly created `Stencil` (`Stencil.AddMaster`)
- Instantiate a shape from a master *that already exists in the same bundle*
  (`Page.AddShape` + `Shape.SetMasterPageID`, used by VisiGo's
  `Service.AddShapeFromMaster`)

There is **no supported way to copy a master from one bundle into another**
— i.e. to take `*Page` `M` from stencil file `S.vssx` and embed it as a new
master inside diagram file `D.vsdx`, preserving its geometry, cells,
sub-shapes, theme references, embedded media, and connection points.

This is the exact operation Visio performs when a user drags a shape from a
stencil pane onto a drawing page: the master is *promoted* into the drawing
bundle on first use, and subsequent instances reference it locally so the
file remains self-contained and round-trips cleanly.

---

## Use case

VisiGo's repository hosts shared stencil libraries (Cisco, Microsoft
Network, custom company icons) as `.vssx`/`.vssm` files alongside diagrams.
Network engineers expect to drag a Cisco Router shape from the panel onto
any diagram and have it behave like a normal Visio master instance —
i.e. when the resulting `.vsdx` is opened in Microsoft Visio, the master
must already be embedded so the icon renders identically and the file
re-opens without repair prompts.

Today VisiGo works around the gap with a **lossy promote** in
`backend/internal/vsdx/service.go: AddShapeFromExternalMaster`:

```go
// We can't copy the master XML into the diagram bundle, so we read the
// master's natural size + name from the stencil and create a free-form
// shape in the diagram with those dimensions. The shape carries a
// User.MasterSource cell with "stencil:<sid>/<mid>" so VisiGo's renderer
// can overlay the master SVG; Visio sees a labelled rectangle.
shape := page.AddShape()
shape.ShapeName = masterName
shape.SetWidth(w); shape.SetHeight(h); shape.SetX(x); shape.SetY(y)
shape.SetText(masterName)
shape.SetCellValue("User.MasterSource", "stencil:"+stencilID+"/"+masterID)
```

This violates two principles in our CLAUDE.md priority order:

1. **Rendering fidelity** — Visio shows a plain rectangle, not the vendor icon
2. **Round-trip integrity** — round-tripping such a shape through Visio →
   VisiGo → Visio loses the back-reference unless the User cell is preserved
   verbatim (which Visio normally does, but we'd rather not rely on it)

---

## Proposed API

Add a public method on `*VisioFile` to import a master from another file,
returning the local master ID assigned in the receiver bundle:

```go
// ImportMaster copies the master shape `src` (from any open VisioFile or
// Stencil) into v's MasterPages and returns the master ID assigned in v.
//
// Side effects:
//   - master XML is added under visio/masters/master<n>.xml
//   - master rel is appended to visio/masters/masters.xml.rels
//   - masters.xml registers the new master with a fresh ID, NameU,
//     UniqueID (NameU/UniqueID copied from src unless they collide)
//   - any media files referenced by the master's rels (PNG/EMF/SVG) are
//     copied into visio/media/ with re-assigned filenames
//   - theme references that resolve in src.Theme but not v.Theme are
//     either inlined as concrete cells or recorded for caller follow-up
//   - if src is the dynamic-connector master, the receiver bundle's
//     existing dynamic-connector master is reused (no duplication)
//
// Returns ErrMasterAlreadyImported when src's UniqueID is already in v,
// in which case the existing local master ID is returned alongside.
func (v *VisioFile) ImportMaster(src *Page) (masterID string, err error)
```

Companion helper for the common case of "instantiate this external master
on a page in one call":

```go
// AddShapeFromExternalMaster combines ImportMaster + AddShape so callers
// don't have to know whether the master has been imported yet.
func (p *Page) AddShapeFromExternalMaster(src *Page, x, y float64) (*Shape, error)
```

VisiGo's `Service.AddShapeFromExternalMaster` would then collapse to:

```go
stencil, _ := vsdx.OpenBytes(stencilData)
src := stencil.GetMasterPageByID(masterID)
v, _ := vsdx.OpenBytes(diagramData)
shape, _ := v.Pages[pageIndex].AddShapeFromExternalMaster(src, x, y)
newData, _ := v.SaveVsdxBytes()
```

…with full Visio round-trip and no `User.MasterSource` hack.

---

## Edge cases the implementation must handle

1. **Master ID collisions** — receiver bundle already has a master with the
   same numeric ID; assign next free ID and rewrite cell references inside
   the copied master XML
2. **UniqueID-based deduplication** — Visio's `UniqueID` (GUID) attribute
   on Master should be preserved so re-imports of the same source master
   collapse into one entry rather than duplicating it
3. **Media re-numbering** — `image1.png` in the stencil may collide with
   `image1.png` in the diagram; renumber and rewrite all relationship
   targets that point at the copied media
4. **Theme cells** — masters often resolve fill/line via theme indices that
   only have meaning in the source bundle's `theme1.xml`. Recommended:
   inline the theme-resolved color as a concrete cell value during import
   so the imported master is theme-independent
5. **Connection points + connectors** — `Connections` section rows have
   absolute IDs; copy them verbatim and verify connector dynamic shapes
   keep their existing receiver-bundle master
6. **Foreign data masters** — Cisco stencils embed EMF / PNG via
   `<ForeignData>`; the referenced relationships + media bytes must be
   copied together
7. **Submasters / master-page links** — a master may reference a base
   master via `<Master MasterShape="x">`; cycle detection + recursive
   import required
8. **Content-types registration** — if the source bundle declares a
   content-type extension the receiver lacks (e.g. EMF), it must be added
   to `[Content_Types].xml`

---

## Round-trip requirement

CLAUDE.md is explicit:

> Round-trip integrity is non-negotiable. Never silently discard XML
> elements, attributes, namespaces, extensions, comments, or processing
> instructions.

Cross-bundle copy must preserve every element of the source master
verbatim — including unknown namespaces and extension blocks — and Visio
must re-open the resulting file without repair prompts, compatibility
warnings, or dropped data.

The acceptance test should be a round-trip pair:

1. Open `cisco-network.vssx`, pick one master `Router`
2. Open `blank.vsdx`, call `Pages[0].AddShapeFromExternalMaster(router, 4, 4)`
3. Save → open in Microsoft Visio → no warnings
4. Re-save from Visio → diff against our save → only the expected pin/size
   cells should differ, never any master-internal XML

---

## Suggested test fixtures

Add under `testdata/cross-bundle/`:

- `cisco-router.vssx` — single-master vendor stencil with EMF foreign data
- `theme-styled.vssx` — master that relies on theme indices
- `composite-master.vssx` — master that references a base master
- `dynamic-connector.vssx` — to verify the receiver's connector master is reused

…paired with a `blank.vsdx` and a `themed.vsdx` to import into, plus the
Visio-resaved expected outputs to byte-diff against.

---

## Why this belongs in vsdx-go, not in downstream callers

Cross-bundle master copy requires intimate knowledge of:

- The zip-level `[Content_Types].xml` invariants
- Master / page / connector rel files and how rIds are scoped
- Media file naming and rel-target rewriting
- Theme resolution semantics
- Visio's `UniqueID` deduplication contract

All of these are vsdx-go internals today (`ZipFileContents`, `addImage`'s
rel-counter logic, `loadMasterPages`, `Theme`). A downstream consumer
cannot do this safely without either re-implementing those internals
(brittle) or grabbing private state (fragile).

This is also a feature every Visio-compatible editor needs the moment it
supports stencil libraries — Microsoft Office implements it natively, so
the spec coverage already exists in MS-VSDX.pdf §2.1.2 (Master Document)
and §2.2.3 (Master Page).

---

## Workaround in VisiGo until this lands

VisiGo's `Service.AddShapeFromExternalMaster` instantiates a free-form
shape with the master's natural dimensions and stamps it with
`User.MasterSource = "stencil:<stencilID>/<masterID>"`. The frontend
overlays the master SVG using that back-reference. This is good enough for
"the user sees the right icon on the canvas," but is **not** Visio
round-trip clean and will be removed the moment `ImportMaster` is
available.

---

## Out of scope (not requested here)

- Stencil-to-stencil master copy (`Stencil.AddMaster` already covers this)
- Live master updates (when the source stencil changes, propagate to
  imported instances) — that's a future "linked stencil" feature
- UI / drag semantics (lives in the consumer)

---

## Contact

Open to design discussion. Happy to contribute test fixtures or a
draft implementation pull request once the API shape is agreed.

— michel.wijnberg@gmail.com (VisiGo)
