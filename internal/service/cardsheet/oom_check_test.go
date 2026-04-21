package cardsheet

import (
	"bytes"
	"runtime"
	"testing"
)

// TestRenderLargeBatchDoesNotBlowUp smoke-tests the "post-vector-QR"
// memory behavior at the cap. It renders MaxBatchCards cards in one go and
// checks that peak allocations stay in the low-double-digit megabytes — well
// under the 256MB Fly VM budget with headroom for Go runtime + concurrent
// requests.
//
// Guardrail, not a precision benchmark: it's here so if someone reintroduces
// per-card image registration the test fails loudly instead of waiting for
// the next prod OOM.
func TestRenderLargeBatchDoesNotBlowUp(t *testing.T) {
	const n = 200 // matches cardbatch.MaxBatchCards

	// Warm up + baseline alloc read.
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	comp := NewComposer(nil) // card renderer is only needed for the
	// raster sheet-face path, not the vector render; drawCardVector goes
	// through gofpdf directly.
	var buf bytes.Buffer
	if err := comp.Render(&buf, testCards(n), SheetConfig{}); err != nil {
		t.Fatalf("Render(%d cards): %v", n, err)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	alloc := after.Alloc - before.Alloc
	totalAlloc := after.TotalAlloc - before.TotalAlloc
	pdfSize := buf.Len()

	t.Logf("n=%d cards: pdf=%d bytes, retained=%d bytes, total_alloc=%d bytes",
		n, pdfSize, alloc, totalAlloc)

	// Ceiling: 50 MB retained after Output() is already generous. Previously
	// (raster path) this scaled with N× the decoded QR pixel data; vector
	// path should be flat in Alloc after the buffer is released to buf.
	const ceiling = 50 << 20
	if alloc > ceiling {
		t.Errorf("retained memory after render = %d bytes, want < %d", alloc, ceiling)
	}
	if pdfSize == 0 {
		t.Fatalf("empty PDF")
	}
}
