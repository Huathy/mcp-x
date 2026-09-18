package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yourname/mcp-x/internal/docconv"
)

func (s *Server) registerDocconvTools() {
	if !s.cfg.Docconv.Enabled {
		return
	}
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "excel_to_md",
		Description: "Convert Excel(xlsx) to Markdown tables. One sheet per section.",
	}, s.handleExcelToMd)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "md_to_excel",
		Description: "Convert Markdown tables to Excel(xlsx). Only tables are written; other content ignored.",
	}, s.handleMdToExcel)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "pdf_to_md",
		Description: "Extract text from a text-layer PDF as Markdown. Scanned/image-only PDFs return an error.",
	}, s.handlePdfToMd)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "docx_to_md",
		Description: "Convert Word(docx) to Markdown. Basic styles only; images ignored.",
	}, s.handleDocxToMd)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "md_to_docx",
		Description: "Convert Markdown to Word(docx) with basic styling. Supports headings/paragraphs/lists/tables/bold/italic.",
	}, s.handleMdToDocx)
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "md_to_pdf",
		Description: "Convert Markdown to PDF. Simple layout; Chinese requires a system CJK font.",
	}, s.handleMdToPdf)

	if runtime.GOOS == "windows" {
		if progID := docconv.ComProbe(s.cfg.Docconv.Com.ProgID); progID != "" {
			s.comProgID = progID
			mcp.AddTool(s.server, &mcp.Tool{
				Name:        "word_to_pdf",
				Description: "Convert Word(docx) to PDF via Windows COM (MS Word or WPS). High quality.",
			}, s.handleWordToPdf)
			mcp.AddTool(s.server, &mcp.Tool{
				Name:        "pdf_to_word",
				Description: "Convert PDF to Word(docx) via MS Word COM. Experimental; layout fidelity low.",
			}, s.handlePdfToWord)
		}
	}
}

// ComProbe is a thin wrapper to allow tests to call the package-level probe
// without a running server. Exposed for parity, not used directly elsewhere.
func ComProbe(progIDHint string) string { return docconv.ComProbe(progIDHint) }

func (s *Server) docconvCfg() docconv.Config {
	return docconv.Config{
		Workdir:       s.cfg.Docconv.Workdir,
		MaxFileSizeMB: s.cfg.Docconv.MaxFileSizeMB,
		ComProgID:     s.comProgID,
		ComTimeoutSec: int(s.cfg.Docconv.Com.Timeout.Std().Seconds()),
	}
}

func (s *Server) validateDocPath(p string) error {
	wd := s.cfg.Docconv.Workdir
	if wd == "" {
		wd = "./data/docconv"
	}
	if err := docconv.ValidatePath(wd, p); err != nil {
		return err
	}
	return nil
}

func resolveInputContent(content, path string) (string, []byte, error) {
	if content != "" {
		return content, nil, nil
	}
	if path == "" {
		return "", nil, fmt.Errorf("either content or path must be provided")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), b, nil
}

func ensureOutputDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return nil
}

type excelToMdInput struct {
	Path    string `json:"path" jsonschema:"xlsx file path"`
	Sheet   string `json:"sheet,omitempty" jsonschema:"sheet name (default all)"`
	MaxRows int    `json:"max_rows,omitempty" jsonschema:"max rows per sheet (default 1000)"`
}

type excelToMdOutput struct {
	Text string `json:"text" jsonschema:"markdown text"`
}

func (s *Server) handleExcelToMd(ctx context.Context, req *mcp.CallToolRequest, in excelToMdInput) (*mcp.CallToolResult, excelToMdOutput, error) {
	if err := s.validateDocPath(in.Path); err != nil {
		return nil, excelToMdOutput{}, err
	}
	if err := docconv.CheckSize(in.Path, s.cfg.Docconv.MaxFileSizeMB); err != nil {
		return nil, excelToMdOutput{}, err
	}
	maxRows := in.MaxRows
	if maxRows <= 0 {
		maxRows = 1000
	}
	text, err := docconv.ExcelToMd(in.Path, in.Sheet, maxRows)
	if err != nil {
		return nil, excelToMdOutput{}, err
	}
	return nil, excelToMdOutput{Text: text}, nil
}

type mdToExcelInput struct {
	Content    string `json:"content,omitempty" jsonschema:"markdown text"`
	Path       string `json:"path,omitempty" jsonschema:"markdown file path"`
	OutputPath string `json:"output_path" jsonschema:"xlsx output path"`
	SheetName  string `json:"sheet_name,omitempty" jsonschema:"sheet name for single-table input"`
}

type mdToExcelOutput struct {
	Text string `json:"text" jsonschema:"result text"`
}

func (s *Server) handleMdToExcel(ctx context.Context, req *mcp.CallToolRequest, in mdToExcelInput) (*mcp.CallToolResult, mdToExcelOutput, error) {
	if err := s.validateDocPath(in.OutputPath); err != nil {
		return nil, mdToExcelOutput{}, err
	}
	content, _, err := resolveInputContent(in.Content, in.Path)
	if err != nil {
		return nil, mdToExcelOutput{}, err
	}
	if err := ensureOutputDir(in.OutputPath); err != nil {
		return nil, mdToExcelOutput{}, err
	}
	n, err := docconv.MdToExcel(content, in.OutputPath, in.SheetName)
	if err != nil {
		return nil, mdToExcelOutput{}, err
	}
	abs, _ := filepath.Abs(in.OutputPath)
	return nil, mdToExcelOutput{Text: fmt.Sprintf("Wrote %d table(s) to %s", n, abs)}, nil
}

type pdfToMdInput struct {
	Path  string `json:"path" jsonschema:"pdf file path"`
	Pages string `json:"pages,omitempty" jsonschema:"page spec e.g. 1-5,8 (default all)"`
}

type pdfToMdOutput struct {
	Text string `json:"text" jsonschema:"markdown text"`
}

func (s *Server) handlePdfToMd(ctx context.Context, req *mcp.CallToolRequest, in pdfToMdInput) (*mcp.CallToolResult, pdfToMdOutput, error) {
	if err := s.validateDocPath(in.Path); err != nil {
		return nil, pdfToMdOutput{}, err
	}
	if err := docconv.CheckSize(in.Path, s.cfg.Docconv.MaxFileSizeMB); err != nil {
		return nil, pdfToMdOutput{}, err
	}
	text, err := docconv.PdfToMd(in.Path, in.Pages)
	if err != nil {
		return nil, pdfToMdOutput{}, err
	}
	return nil, pdfToMdOutput{Text: text}, nil
}

type docxToMdInput struct {
	Path string `json:"path" jsonschema:"docx file path"`
}

type docxToMdOutput struct {
	Text string `json:"text" jsonschema:"markdown text"`
}

func (s *Server) handleDocxToMd(ctx context.Context, req *mcp.CallToolRequest, in docxToMdInput) (*mcp.CallToolResult, docxToMdOutput, error) {
	if err := s.validateDocPath(in.Path); err != nil {
		return nil, docxToMdOutput{}, err
	}
	if err := docconv.CheckSize(in.Path, s.cfg.Docconv.MaxFileSizeMB); err != nil {
		return nil, docxToMdOutput{}, err
	}
	text, err := docconv.DocxToMd(in.Path)
	if err != nil {
		return nil, docxToMdOutput{}, err
	}
	return nil, docxToMdOutput{Text: text}, nil
}

type mdToDocxInput struct {
	Content    string `json:"content,omitempty" jsonschema:"markdown text"`
	Path       string `json:"path,omitempty" jsonschema:"markdown file path"`
	OutputPath string `json:"output_path" jsonschema:"docx output path"`
}

type mdToDocxOutput struct {
	Text string `json:"text" jsonschema:"result text"`
}

func (s *Server) handleMdToDocx(ctx context.Context, req *mcp.CallToolRequest, in mdToDocxInput) (*mcp.CallToolResult, mdToDocxOutput, error) {
	if err := s.validateDocPath(in.OutputPath); err != nil {
		return nil, mdToDocxOutput{}, err
	}
	content, _, err := resolveInputContent(in.Content, in.Path)
	if err != nil {
		return nil, mdToDocxOutput{}, err
	}
	if err := ensureOutputDir(in.OutputPath); err != nil {
		return nil, mdToDocxOutput{}, err
	}
	if err := docconv.MdToDocx(content, in.OutputPath); err != nil {
		return nil, mdToDocxOutput{}, err
	}
	abs, _ := filepath.Abs(in.OutputPath)
	return nil, mdToDocxOutput{Text: fmt.Sprintf("Wrote docx to %s", abs)}, nil
}

type mdToPdfInput struct {
	Content    string `json:"content,omitempty" jsonschema:"markdown text"`
	Path       string `json:"path,omitempty" jsonschema:"markdown file path"`
	OutputPath string `json:"output_path" jsonschema:"pdf output path"`
}

type mdToPdfOutput struct {
	Text string `json:"text" jsonschema:"result text"`
}

func (s *Server) handleMdToPdf(ctx context.Context, req *mcp.CallToolRequest, in mdToPdfInput) (*mcp.CallToolResult, mdToPdfOutput, error) {
	if err := s.validateDocPath(in.OutputPath); err != nil {
		return nil, mdToPdfOutput{}, err
	}
	content, _, err := resolveInputContent(in.Content, in.Path)
	if err != nil {
		return nil, mdToPdfOutput{}, err
	}
	if err := ensureOutputDir(in.OutputPath); err != nil {
		return nil, mdToPdfOutput{}, err
	}
	if err := docconv.MdToPdf(content, in.OutputPath); err != nil {
		return nil, mdToPdfOutput{}, err
	}
	abs, _ := filepath.Abs(in.OutputPath)
	return nil, mdToPdfOutput{Text: fmt.Sprintf("Wrote pdf to %s", abs)}, nil
}

type wordToPdfInput struct {
	Path       string `json:"path" jsonschema:"docx file path"`
	OutputPath string `json:"output_path" jsonschema:"pdf output path"`
}

type wordToPdfOutput struct {
	Text string `json:"text" jsonschema:"result text"`
}

func (s *Server) handleWordToPdf(ctx context.Context, req *mcp.CallToolRequest, in wordToPdfInput) (*mcp.CallToolResult, wordToPdfOutput, error) {
	if s.comProgID == "" {
		return nil, wordToPdfOutput{}, docconv.ErrCOMNotAvailable
	}
	if err := s.validateDocPath(in.Path); err != nil {
		return nil, wordToPdfOutput{}, err
	}
	if err := s.validateDocPath(in.OutputPath); err != nil {
		return nil, wordToPdfOutput{}, err
	}
	if err := ensureOutputDir(in.OutputPath); err != nil {
		return nil, wordToPdfOutput{}, err
	}
	timeout := int(s.cfg.Docconv.Com.Timeout.Std().Seconds())
	if err := docconv.WordToPdfCOM(in.Path, in.OutputPath, s.comProgID, timeout); err != nil {
		return nil, wordToPdfOutput{}, err
	}
	return nil, wordToPdfOutput{Text: fmt.Sprintf("Wrote pdf to %s", in.OutputPath)}, nil
}

type pdfToWordInput struct {
	Path       string `json:"path" jsonschema:"pdf file path"`
	OutputPath string `json:"output_path" jsonschema:"docx output path"`
}

type pdfToWordOutput struct {
	Text string `json:"text" jsonschema:"result text"`
}

func (s *Server) handlePdfToWord(ctx context.Context, req *mcp.CallToolRequest, in pdfToWordInput) (*mcp.CallToolResult, pdfToWordOutput, error) {
	if s.comProgID == "" {
		return nil, pdfToWordOutput{}, docconv.ErrCOMNotAvailable
	}
	if err := s.validateDocPath(in.Path); err != nil {
		return nil, pdfToWordOutput{}, err
	}
	if err := s.validateDocPath(in.OutputPath); err != nil {
		return nil, pdfToWordOutput{}, err
	}
	if err := ensureOutputDir(in.OutputPath); err != nil {
		return nil, pdfToWordOutput{}, err
	}
	timeout := int(s.cfg.Docconv.Com.Timeout.Std().Seconds())
	if err := docconv.PdfToWordCOM(in.Path, in.OutputPath, s.comProgID, timeout); err != nil {
		return nil, pdfToWordOutput{}, err
	}
	return nil, pdfToWordOutput{Text: fmt.Sprintf("Wrote docx to %s", in.OutputPath)}, nil
}

func init() {
	_ = strings.TrimSpace
}
