package docconv

import (
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PdfToMd extracts text from a text-layer PDF and returns markdown.
// Scanned (image-only) PDFs return ErrScanPDF.
func PdfToMd(path string, pagesSpec string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	total := r.NumPage()
	pages, err := parsePages(pagesSpec, total)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	anyText := false
	for _, p := range pages {
		page := r.Page(p)
		if page.V.IsNull() {
			sb.WriteString(fmt.Sprintf("\n## Page %d\n\n(page not found)\n", p))
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extract page %d: %w", p, err)
		}
		sb.WriteString(fmt.Sprintf("\n## Page %d\n\n", p))
		txt := strings.TrimSpace(text)
		if txt != "" {
			anyText = true
			sb.WriteString(txt + "\n")
		} else {
			sb.WriteString("(no text on this page)\n")
		}
	}

	if !anyText {
		return "", ErrScanPDF
	}
	return sb.String(), nil
}

func parsePages(spec string, total int) ([]int, error) {
	if spec == "" {
		out := make([]int, 0, total)
		for i := 1; i <= total; i++ {
			out = append(out, i)
		}
		return out, nil
	}
	var out []int
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if dash := strings.Index(part, "-"); dash >= 0 {
			var a, b int
			if _, err := fmt.Sscanf(part, "%d-%d", &a, &b); err != nil {
				return nil, fmt.Errorf("invalid page range %q: %w", part, err)
			}
			if a < 1 || b > total || a > b {
				return nil, fmt.Errorf("page range %q out of bounds (total %d)", part, total)
			}
			for i := a; i <= b; i++ {
				out = append(out, i)
			}
		} else {
			var n int
			if _, err := fmt.Sscanf(part, "%d", &n); err != nil {
				return nil, fmt.Errorf("invalid page %q: %w", part, err)
			}
			if n < 1 || n > total {
				return nil, fmt.Errorf("page %d out of bounds (total %d)", n, total)
			}
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no pages parsed from spec %q", spec)
	}
	return out, nil
}

// guard against unused import in some build tags
var _ = io.EOF
