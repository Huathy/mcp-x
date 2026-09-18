package docconv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/signintech/gopdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// MdToPdf converts markdown to a basic PDF. Layout is simple: no CSS, no
// headers/footers, no merged cells. Chinese requires a system TTF font; if
// none is found, Chinese chars will render as tofu/missing.
func MdToPdf(mdContent, outputPath string) error {
	src := []byte(mdContent)
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	doc := md.Parser().Parse(text.NewReader(src))

	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: *gopdf.PageSizeA4})
	pdf.SetMargins(40, 40, 40, 40)

	fontPath, fontOK := findCJKFont()
	if !fontOK {
		return fmt.Errorf("no system CJK font found; md_to_pdf requires a TTF/TTC font on the host")
	}
	family := "sans"
	if err := pdf.AddTTFFont(family, fontPath); err != nil {
		return fmt.Errorf("add font %s: %w", fontPath, err)
	}
	if err := pdf.SetFont(family, "", 12); err != nil {
		return fmt.Errorf("set font: %w", err)
	}

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Heading:
			size := 16 - float64(v.Level)
			if size < 10 {
				size = 10
			}
			pdf.SetFontSize(size)
			writePdfLine(&pdf, strings.Repeat("#", v.Level)+" "+inlineText(v, src), 40, 8)
			pdf.SetFontSize(12)
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph:
			writePdfParagraph(&pdf, inlineText(v, src))
		case *ast.List:
			order := v.IsOrdered()
			i := 1
			for c := v.FirstChild(); c != nil; c = c.NextSibling() {
				prefix := "- "
				if order {
					prefix = fmt.Sprintf("%d. ", i)
					i++
				}
				writePdfLine(&pdf, prefix+inlineText(c, src), 20, 6)
			}
			return ast.WalkSkipChildren, nil
		case *extast.Table:
			writePdfTable(&pdf, v, src)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})

	return pdf.WritePdf(outputPath)
}

func writePdfLine(pdf *gopdf.GoPdf, text string, indent, after float64) {
	w := pdf.MarginLeft() + indent
	pdf.SetX(w)
	pdf.MultiCell(&gopdf.Rect{W: gopdf.PageSizeA4.W - w - pdf.MarginRight(), H: 18}, text)
	pdf.Br(after)
}

func writePdfParagraph(pdf *gopdf.GoPdf, text string) {
	w := pdf.MarginLeft()
	pdf.SetX(w)
	pdf.MultiCell(&gopdf.Rect{W: gopdf.PageSizeA4.W - w - pdf.MarginRight(), H: 18}, text)
	pdf.Br(6)
}

func writePdfTable(pdf *gopdf.GoPdf, t *extast.Table, src []byte) {
	xStart := pdf.MarginLeft()
	pageW := gopdf.PageSizeA4.W
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		cells := extractRowCells(r, src)
		cellCount := len(cells)
		if cellCount == 0 {
			continue
		}
		colW := (pageW - xStart - pdf.MarginRight()) / float64(cellCount)
		x := xStart
		y := pdf.GetY()
		for _, val := range cells {
			pdf.RectFromUpperLeft(x, y, colW, 18)
			pdf.SetXY(x+2, y+2)
			pdf.MultiCell(&gopdf.Rect{W: colW - 4, H: 16}, val)
			x += colW
		}
		pdf.SetY(y + 18)
	}
	pdf.Br(4)
}

// findCJKFont probes common system CJK TTF/TTC paths across OSes.
func findCJKFont() (string, bool) {
	candidates := []string{
		`C:\Windows\Fonts\msyh.ttc`,
		`C:\Windows\Fonts\msyh.ttf`,
		`C:\Windows\Fonts\simsun.ttc`,
		`/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc`,
		`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc`,
		`/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc`,
		`/System/Library/Fonts/PingFang.ttc`,
		`/System/Library/Fonts/STHeiti Light.ttc`,
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	// fallback: glob for any CJK-looking font in Linux font dirs
	matches, _ := filepath.Glob(`/usr/share/fonts/**/*CJK*.ttc`)
	if len(matches) > 0 {
		return matches[0], true
	}
	matches, _ = filepath.Glob(`/usr/share/fonts/**/*CJK*.otf`)
	if len(matches) > 0 {
		return matches[0], true
	}
	return "", false
}
