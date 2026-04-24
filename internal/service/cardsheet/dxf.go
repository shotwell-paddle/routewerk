package cardsheet

import (
	"fmt"
	"io"
	"strings"
)

// RenderCutlinesDXF writes an AutoCAD R12 DXF to w containing the full
// 8-up cut geometry for a single sheet: eight rounded-rectangle cut paths
// at the same grid positions the PDF uses. The same file works for every
// sheet in a batch; the cutter aligns to each sheet's printed registration
// marks.
//
// This is a *fallback* for Silhouette's "Cut by Color" — usually the
// magenta hairlines in the print-and-cut PDF are enough, but if Studio
// mis-assigns them (weird interactions with route-colour fills) you can
// import this DXF instead and cut from clean vector geometry.
//
// Units are millimetres. Each rounded rectangle is emitted as four LINE
// entities (the straight edges) plus four ARC entities (the corners) —
// explicit primitives that every DXF reader handles identically. An
// earlier implementation used LWPOLYLINE with bulge encoding; Silhouette
// Studio mis-rendered the closing bulge and produced shapes where one
// edge arced across the whole card. LINE+ARC sidesteps the ambiguity.
//
// For partial sheets (batches of fewer than 8 cards), all 8 cut paths are
// emitted — the user deletes the unwanted ones in Studio before cutting.
// That's simpler than stripping them here and keeps the DXF reusable
// across full and partial sheets in a multi-sheet batch.
func RenderCutlinesDXF(w io.Writer) error {
	var buf strings.Builder

	writeDXFHeader(&buf)
	writeDXFTables(&buf)
	buf.WriteString("0\nSECTION\n2\nENTITIES\n")

	// Emit the 8 card cut paths in grid order. Coordinates mirror sheet.go
	// exactly (including the cardGutterMM spacing between cards) — the PDF
	// cut path and DXF geometry describe the same rectangle in the same
	// place on the page.
	for row := 0; row < gridRows; row++ {
		for col := 0; col < gridCols; col++ {
			x := gridXMM + float64(col)*cardPitchXMM
			y := gridYMM + float64(row)*cardPitchYMM
			writeRoundedRect(&buf, x, y, cardWMM, cardHMM, cardCornerRadiusMM)
		}
	}

	buf.WriteString("0\nENDSEC\n0\nEOF\n")

	if _, err := io.WriteString(w, buf.String()); err != nil {
		return fmt.Errorf("cardsheet: write dxf: %w", err)
	}
	return nil
}

// writeDXFHeader writes an AC1009 (AutoCAD R12) header with $INSUNITS=4
// (millimetres). R12 is the oldest DXF revision and the one most
// conservatively supported by cutter software; LINE and ARC entities are
// native to R12 so no subclass markers are required.
func writeDXFHeader(b *strings.Builder) {
	b.WriteString("0\nSECTION\n2\nHEADER\n")
	b.WriteString("9\n$ACADVER\n1\nAC1009\n")
	b.WriteString("9\n$INSUNITS\n70\n4\n")
	b.WriteString("0\nENDSEC\n")
}

// writeDXFTables writes a minimal TABLES section: one LAYER table with the
// default layer "0" plus a CUT layer (AutoCAD color index 6 = magenta,
// matching the PDF cut-signal convention). Some DXF readers refuse files
// that omit TABLES entirely even when every entity could default; this
// keeps us on the well-behaved side.
func writeDXFTables(b *strings.Builder) {
	b.WriteString("0\nSECTION\n2\nTABLES\n")
	b.WriteString("0\nTABLE\n2\nLAYER\n70\n2\n")
	// Layer "0" — required to exist in every DXF file.
	b.WriteString("0\nLAYER\n2\n0\n70\n0\n62\n7\n6\nCONTINUOUS\n")
	// Layer "CUT" — colour index 6 = magenta, matches the PDF cut colour
	// so Studio's Cut-by-Color picks up the same hue either way.
	b.WriteString("0\nLAYER\n2\nCUT\n70\n0\n62\n6\n6\nCONTINUOUS\n")
	b.WriteString("0\nENDTAB\n")
	b.WriteString("0\nENDSEC\n")
}

// writeRoundedRect appends eight entities describing a rounded rectangle
// whose bounding box in the caller's y-DOWN screen space is (x, y) →
// (x+w, y+h) with corner radius r: four LINE entities for the straight
// edges and four ARC entities for the corners.
//
// DXF uses y-UP, so coordinates are flipped against pageHMM before
// emission — the resulting rectangle sits in the same physical position
// on the sheet as the PDF cut path at the same (x, y, w, h).
//
// ARC angles are measured CCW from the positive-x axis in DXF's y-up
// space. For a 90° corner arc that joins two tangent sides, the centre
// sits r-inside the corner and the sweep covers one of the cardinal
// quadrants (0-90°, 90-180°, 180-270°, or 270-360°).
//
// All entities go on layer "CUT" so a Silhouette Studio user can still
// cut-by-layer if colour filtering isn't convenient.
func writeRoundedRect(b *strings.Builder, x, y, w, h, r float64) {
	// Flip y. In screen space, y grows downward. DXF is y-up, so a shape
	// at screen-y `y` sits at DXF-y `pageHMM - (y + h)` (bottom edge) and
	// `pageHMM - y` (top edge).
	bottom := pageHMM - (y + h)
	top := pageHMM - y
	left := x
	right := x + w

	// Straight edges (go CCW around the rectangle, matching the corner
	// ordering below).
	writeLine(b, left+r, bottom, right-r, bottom) // bottom edge
	writeLine(b, right, bottom+r, right, top-r)   // right edge
	writeLine(b, right-r, top, left+r, top)       // top edge
	writeLine(b, left, top-r, left, bottom+r)     // left edge

	// Corner arcs. Centre is r-inside the corner; angles cover one
	// quadrant each, CCW from positive-x.
	writeArc(b, right-r, bottom+r, r, 270, 360) // bottom-right
	writeArc(b, right-r, top-r, r, 0, 90)       // top-right
	writeArc(b, left+r, top-r, r, 90, 180)      // top-left
	writeArc(b, left+r, bottom+r, r, 180, 270)  // bottom-left
}

func writeLine(b *strings.Builder, x1, y1, x2, y2 float64) {
	b.WriteString("0\nLINE\n8\nCUT\n")
	fmt.Fprintf(b, "10\n%.4f\n20\n%.4f\n", x1, y1)
	fmt.Fprintf(b, "11\n%.4f\n21\n%.4f\n", x2, y2)
}

func writeArc(b *strings.Builder, cx, cy, r, startDeg, endDeg float64) {
	b.WriteString("0\nARC\n8\nCUT\n")
	fmt.Fprintf(b, "10\n%.4f\n20\n%.4f\n", cx, cy)
	fmt.Fprintf(b, "40\n%.4f\n", r)
	fmt.Fprintf(b, "50\n%.4f\n51\n%.4f\n", startDeg, endDeg)
}
