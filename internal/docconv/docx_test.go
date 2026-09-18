package docconv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMdToDocxRoundTrip(t *testing.T) {
	dir := t.TempDir()
	md := "# Title\n\nHello **bold** world.\n\n- item1\n- item2\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	out := filepath.Join(dir, "out.docx")
	if err := MdToDocx(md, out); err != nil {
		t.Fatalf("MdToDocx: %v", err)
	}
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Size() < 200 {
		t.Errorf("docx too small: %d bytes", fi.Size())
	}
	back, err := DocxToMd(out)
	if err != nil {
		t.Fatalf("DocxToMd: %v", err)
	}
	if !strings.Contains(back, "Title") {
		t.Errorf("roundtrip missing Title: %q", back)
	}
	if !strings.Contains(back, "item1") {
		t.Errorf("roundtrip missing item1: %q", back)
	}
	if !strings.Contains(back, "bold") {
		t.Errorf("roundtrip missing bold: %q", back)
	}
}

func TestMdToDocxParaOnly(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "simple.docx")
	if err := MdToDocx("plain paragraph text", out); err != nil {
		t.Fatalf("MdToDocx: %v", err)
	}
	back, err := DocxToMd(out)
	if err != nil {
		t.Fatalf("DocxToMd: %v", err)
	}
	if !strings.Contains(back, "plain paragraph text") {
		t.Errorf("roundtrip missing text: %q", back)
	}
}

func TestDocxToMdMissingFile(t *testing.T) {
	_, err := DocxToMd("nonexistent.docx")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestPdfToMdMissingFile(t *testing.T) {
	_, err := PdfToMd("nonexistent.pdf", "")
	if err == nil {
		t.Error("expected error for missing pdf")
	}
}

func TestParsePagesAll(t *testing.T) {
	pages, err := parsePages("", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 5 || pages[0] != 1 || pages[4] != 5 {
		t.Errorf("parsePages all: %v", pages)
	}
}

func TestParsePagesRange(t *testing.T) {
	pages, err := parsePages("2-4", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 || pages[0] != 2 || pages[2] != 4 {
		t.Errorf("parsePages 2-4: %v", pages)
	}
}

func TestParsePagesMixed(t *testing.T) {
	pages, err := parsePages("1-2,5", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 5}
	if len(pages) != len(want) {
		t.Fatalf("got %v want %v", pages, want)
	}
	for i, p := range pages {
		if p != want[i] {
			t.Errorf("idx %d: got %d want %d", i, p, want[i])
		}
	}
}

func TestParsePagesOutOfBounds(t *testing.T) {
	if _, err := parsePages("1-100", 5); err == nil {
		t.Error("expected out-of-bounds error")
	}
	if _, err := parsePages("6", 5); err == nil {
		t.Error("expected single-page out-of-bounds error")
	}
}

func TestParsePagesInvalid(t *testing.T) {
	if _, err := parsePages("abc", 5); err == nil {
		t.Error("expected parse error")
	}
	if _, err := parsePages("", 0); err != nil {
		t.Errorf("empty spec with 0 pages should still return empty slice, got %v", err)
	}
}

func TestComProbeStubNonWindows(t *testing.T) {
	// On non-windows this returns "". On windows it may return a progID.
	// We only assert no panic and type correctness.
	got := ComProbe("auto")
	if got != "" && !strings.Contains(got, ".Application") {
		t.Errorf("unexpected progID: %q", got)
	}
}
