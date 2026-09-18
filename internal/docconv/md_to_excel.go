package docconv

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// MdToExcel converts markdown table(s) to an xlsx file. Non-table content is
// ignored. Returns the number of tables written.
func MdToExcel(mdContent, outputPath, sheetName string) (int, error) {
	src := []byte(mdContent)
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	doc := md.Parser().Parse(text.NewReader(src))

	var tables []*extast.Table
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := n.(*extast.Table); ok {
				tables = append(tables, t)
			}
		}
		return ast.WalkContinue, nil
	})
	if len(tables) == 0 {
		return 0, fmt.Errorf("no markdown table found in content")
	}

	f := excelize.NewFile()
	defer f.Close()

	if len(tables) == 1 && sheetName != "" {
		f.SetSheetName("Sheet1", sheetName)
		writeTableSheet(f, sheetName, tables[0], src)
	} else {
		base := "Sheet1"
		if len(tables) == 1 {
			writeTableSheet(f, base, tables[0], src)
		} else {
			for i, t := range tables {
				name := fmt.Sprintf("Table%d", i+1)
				if i == 0 {
					f.SetSheetName(base, name)
				} else {
					f.NewSheet(name)
				}
				writeTableSheet(f, name, t, src)
			}
		}
	}

	if err := f.SaveAs(outputPath); err != nil {
		return 0, fmt.Errorf("save xlsx: %w", err)
	}
	return len(tables), nil
}

func writeTableSheet(f *excelize.File, sheet string, t *extast.Table, src []byte) {
	rowIdx := 1
	for n := t.FirstChild(); n != nil; n = n.NextSibling() {
		row := extractRowChildren(n, src)
		if len(row) == 0 {
			continue
		}
		for col, val := range row {
			axis, _ := excelize.CoordinatesToCellName(col+1, rowIdx)
			f.SetCellValue(sheet, axis, val)
		}
		rowIdx++
	}
}

// extractRowChildren pulls cells out of either a TableHeader or TableRow node.
func extractRowChildren(n ast.Node, src []byte) []string {
	var out []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		cell, ok := c.(*extast.TableCell)
		if !ok {
			continue
		}
		out = append(out, cellText(cell, src))
	}
	return out
}

func cellText(n ast.Node, src []byte) string {
	var buf strings.Builder
	ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if txt, ok := node.(*ast.Text); ok {
			buf.Write(txt.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}
