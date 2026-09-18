package docconv

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ExcelToMd converts an xlsx file to markdown text. If sheet is empty, all
// sheets are converted; otherwise only the named sheet.
func ExcelToMd(path, sheet string, maxRows int) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	var sheets []string
	if sheet != "" {
		sheets = []string{sheet}
	} else {
		sheets = f.GetSheetList()
	}

	var sb strings.Builder
	for _, name := range sheets {
		if len(sheets) > 1 {
			sb.WriteString("## " + name + "\n\n")
		}
		rows, err := f.GetRows(name)
		if err != nil {
			return "", fmt.Errorf("read sheet %s: %w", name, err)
		}
		if len(rows) == 0 {
			continue
		}
		if maxRows > 0 && len(rows) > maxRows {
			rows = rows[:maxRows]
		}
		writeMdTable(&sb, rows)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func writeMdTable(sb *strings.Builder, rows [][]string) {
	colCount := 0
	for _, r := range rows {
		if len(r) > colCount {
			colCount = len(r)
		}
	}
	if colCount == 0 {
		return
	}
	for i, row := range rows {
		cells := make([]string, colCount)
		for j := 0; j < colCount; j++ {
			if j < len(row) {
				cells[j] = escapeCell(row[j])
			}
		}
		sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			sb.WriteString("|" + strings.Repeat("---|", colCount) + "\n")
		}
	}
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimRight(s, "\r\n")
}
