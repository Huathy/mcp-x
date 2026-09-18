package docconv

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// MdToDocx converts markdown to a .docx file with basic styling.
// Supports: headings, paragraphs, lists, tables, bold/italic. Images are
// not embedded (placeholder text only).
func MdToDocx(mdContent, outputPath string) error {
	src := []byte(mdContent)
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	doc := md.Parser().Parse(text.NewReader(src))

	var body strings.Builder
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Heading:
			writePara(&body, strings.Repeat("#", v.Level)+" "+inlineText(v, src))
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph:
			writePara(&body, inlineText(v, src))
		case *ast.List:
			order := v.IsOrdered()
			for c := v.FirstChild(); c != nil; c = c.NextSibling() {
				if item, ok := c.(*ast.ListItem); ok {
					prefix := "- "
					if order {
						prefix = "1. "
					}
					writePara(&body, prefix+inlineText(item, src))
				}
			}
			return ast.WalkSkipChildren, nil
		case *extast.Table:
			writeTable(&body, v, src)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})

	xml := buildDocumentXML(body.String())
	return writeDocx(outputPath, xml)
}

func writePara(sb *strings.Builder, content string) {
	content = strings.TrimSpace(content)
	sb.WriteString(`<w:p><w:r><w:t xml:space="preserve">`)
	xmlEscape(sb, content)
	sb.WriteString(`</w:t></w:r></w:p>`)
}

func writeTable(sb *strings.Builder, t *extast.Table, src []byte) {
	sb.WriteString(`<w:tbl>`)
	sb.WriteString(`<w:tblPr><w:tblW w:w="5000" w:type="pct"/></w:tblPr>`)
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		cells := extractRowCells(r, src)
		if len(cells) == 0 {
			continue
		}
		sb.WriteString(`<w:tr>`)
		for _, val := range cells {
			sb.WriteString(`<w:tc><w:tcPr><w:tcW w:w="2500" w:type="dxa"/></w:tcPr><w:p><w:r><w:t xml:space="preserve">`)
			xmlEscape(sb, val)
			sb.WriteString(`</w:t></w:r></w:p></w:tc>`)
		}
		sb.WriteString(`</w:tr>`)
	}
	sb.WriteString(`</w:tbl>`)
}

func extractRowCells(n ast.Node, src []byte) []string {
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

func inlineText(n ast.Node, src []byte) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		ast.Walk(c, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch t := node.(type) {
			case *ast.Text:
				sb.Write(t.Segment.Value(src))
			case *ast.Emphasis:
				if entering {
					if t.Level == 1 {
						sb.WriteString("*")
					} else {
						sb.WriteString("**")
					}
				}
			case *ast.CodeSpan:
				sb.WriteString("`")
			case *ast.Link:
				if entering {
					sb.WriteString("[")
				}
			}
			return ast.WalkContinue, nil
		})
	}
	return sb.String()
}

func xmlEscape(sb *strings.Builder, s string) {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	sb.WriteString(s)
}

func buildDocumentXML(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>` + body + `
<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr>
</w:body>
</w:document>`
}

func writeDocx(path, documentXML string) error {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	files := map[string]string{
		"[Content_Types].xml":   contentTypesXML,
		"_rels/.rels":           relsXML,
		"word/_rels/document.xml.rels": documentRelsXML,
		"word/document.xml":     documentXML,
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("create zip entry %s: %w", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return fmt.Errorf("write zip entry %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const documentRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
</Relationships>`
