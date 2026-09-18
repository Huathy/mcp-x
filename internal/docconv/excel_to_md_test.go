package docconv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func writeTestXlsx(t *testing.T, path string) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", "Data")
	f.SetCellValue("Data", "A1", "id")
	f.SetCellValue("Data", "B1", "name")
	f.SetCellValue("Data", "A2", 1)
	f.SetCellValue("Data", "B2", "Alice")
	f.SetCellValue("Data", "A3", 2)
	f.SetCellValue("Data", "B3", "Bob")
	f.NewSheet("Notes")
	f.SetCellValue("Notes", "A1", "remark")
	f.SetCellValue("Notes", "B1", "value")
	f.SetCellValue("Notes", "A2", "x")
	f.SetCellValue("Notes", "B2", "y")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}
}

func TestExcelToMdMultiSheet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.xlsx")
	writeTestXlsx(t, p)
	out, err := ExcelToMd(p, "", 1000)
	if err != nil {
		t.Fatalf("ExcelToMd: %v", err)
	}
	if !strings.Contains(out, "## Data") {
		t.Errorf("missing Data sheet heading: %q", out)
	}
	if !strings.Contains(out, "## Notes") {
		t.Errorf("missing Notes sheet heading: %q", out)
	}
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Errorf("missing row data: %q", out)
	}
	if !strings.Contains(out, "---|") {
		t.Errorf("missing separator row: %q", out)
	}
}

func TestExcelToMdSingleSheet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.xlsx")
	writeTestXlsx(t, p)
	out, err := ExcelToMd(p, "Data", 1000)
	if err != nil {
		t.Fatalf("ExcelToMd: %v", err)
	}
	if strings.Contains(out, "## Data") {
		t.Errorf("single sheet should not have heading: %q", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("missing data: %q", out)
	}
}

func TestExcelToMdMaxRows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.xlsx")
	writeTestXlsx(t, p)
	out, err := ExcelToMd(p, "Data", 2)
	if err != nil {
		t.Fatalf("ExcelToMd: %v", err)
	}
	if strings.Contains(out, "Bob") {
		t.Errorf("max_rows=2 should drop 3rd data row (Bob): %q", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("max_rows=2 should keep 2nd row (Alice): %q", out)
	}
}

func TestExcelToMdPipeEscape(t *testing.T) {
	dir := t.TempDir()
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A1", "a|b")
	p := filepath.Join(dir, "pipe.xlsx")
	if err := f.SaveAs(p); err != nil {
		t.Fatal(err)
	}
	out, err := ExcelToMd(p, "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a\\|b") {
		t.Errorf("pipe not escaped: %q", out)
	}
}

func TestMdToExcelSingleTable(t *testing.T) {
	dir := t.TempDir()
	md := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	p := filepath.Join(dir, "out.xlsx")
	n, err := MdToExcel(md, p, "T")
	if err != nil {
		t.Fatalf("MdToExcel: %v", err)
	}
	if n != 1 {
		t.Errorf("table count = %d, want 1", n)
	}
	f, err := excelize.OpenFile(p)
	if err != nil {
		t.Fatalf("open result: %v", err)
	}
	defer f.Close()
	if v, _ := f.GetCellValue("T", "A1"); v != "a" {
		t.Errorf("A1 = %q, want a", v)
	}
	if v, _ := f.GetCellValue("T", "B2"); v != "2" {
		t.Errorf("B2 = %q, want 2", v)
	}
}

func TestMdToExcelNoTable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.xlsx")
	_, err := MdToExcel("hello world", p, "")
	if err == nil {
		t.Error("expected error for no table")
	}
	if !strings.Contains(err.Error(), "no markdown table") {
		t.Errorf("unexpected error: %v", err)
	}
	_ = os.Remove(p)
}

func TestMdToExcelMultiTable(t *testing.T) {
	dir := t.TempDir()
	md := "| a | b |\n|---|---|\n| 1 | 2 |\n\n| c |\n|---|\n| 3 |\n"
	p := filepath.Join(dir, "out.xlsx")
	n, err := MdToExcel(md, p, "")
	if err != nil {
		t.Fatalf("MdToExcel: %v", err)
	}
	if n != 2 {
		t.Errorf("table count = %d, want 2", n)
	}
	f, err := excelize.OpenFile(p)
	if err != nil {
		t.Fatalf("open result: %v", err)
	}
	defer f.Close()
	if v, _ := f.GetCellValue("Table1", "A2"); v != "1" {
		t.Errorf("Table1 A2 = %q, want 1", v)
	}
	if v, _ := f.GetCellValue("Table2", "A2"); v != "3" {
		t.Errorf("Table2 A2 = %q, want 3", v)
	}
}

func TestValidatePathTraversal(t *testing.T) {
	if err := ValidatePath("/tmp/workdir", "../escape.txt"); err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal, got %v", err)
	}
	if err := ValidatePath("/tmp/workdir", "sub/../ok.txt"); err != ErrPathTraversal {
		t.Errorf("expected ErrPathTraversal for mid .., got %v", err)
	}
}

func TestValidatePathOutside(t *testing.T) {
	err := ValidatePath("/tmp/workdir", "/etc/passwd")
	if err != ErrPathOutsideWorkdir {
		t.Errorf("expected ErrPathOutsideWorkdir, got %v", err)
	}
}

func TestValidatePathInside(t *testing.T) {
	if err := ValidatePath("/tmp/workdir", "/tmp/workdir/sub/a.txt"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckSize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(p, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckSize(p, 50); err != nil {
		t.Errorf("small file should pass: %v", err)
	}
	if err := CheckSize(p, 0); err != nil {
		t.Errorf("maxMB=0 should be no-op: %v", err)
	}
}

func TestCheckSizeTooLarge(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(p, make([]byte, 2*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckSize(p, 1); err == nil {
		t.Error("expected ErrFileTooLarge for 2MB > 1MB")
	}
}
