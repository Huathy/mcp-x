package docconv

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// DocxToMd extracts text from a .docx file and returns markdown.
// Complex styling (colors/fonts/alignment) is not preserved; images are
// ignored (alt text used if present).
func DocxToMd(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open docx: %w", err)
	}
	defer zr.Close()

	var docXML io.ReadCloser
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("read document.xml: %w", err)
			}
			docXML = rc
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("docx missing word/document.xml")
	}
	defer docXML.Close()

	data, err := io.ReadAll(docXML)
	if err != nil {
		return "", fmt.Errorf("read document.xml body: %w", err)
	}

	return parseDocxXML(data), nil
}

func parseDocxXML(data []byte) string {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var sb strings.Builder
	var inPara bool
	var inCell bool
	var runBold, runItalic bool
	var listLevel int
	var listOrdered bool
	var curPStyle string

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inPara = true
				curPStyle = ""
				runBold, runItalic = false, false
			case "pStyle":
				for _, a := range t.Attr {
					if a.Name.Local == "val" {
						curPStyle = a.Value
					}
				}
			case "r":
				// run start
			case "b":
				runBold = true
			case "i":
				runItalic = true
			case "bdr":
				// ignore
			case "t":
				if !inPara {
					continue
				}
				var txt string
				if err := dec.DecodeElement(&txt, &t); err == nil {
					writeRun(&sb, txt, runBold, runItalic)
				}
			case "tab":
				sb.WriteString("\t")
			case "br":
				sb.WriteString("\n")
			case "tbl":
				sb.WriteString("\n")
			case "tr":
				// row start
			case "tc":
				inCell = true
				sb.WriteString("| ")
			case "numPr":
				listLevel, listOrdered = parseNumPr(dec)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if inCell {
					sb.WriteString(" ")
					inCell = false
				} else {
					style := headingFor(curPStyle)
					if style != "" {
						sb.WriteString(style + " ")
					} else if listLevel >= 0 {
						if listOrdered {
							sb.WriteString("1. ")
						} else {
							sb.WriteString("- ")
						}
					}
				}
				_ = listLevel
				sb.WriteString("\n")
				inPara = false
				listLevel = -1
				listOrdered = false
				curPStyle = ""
			case "r":
				runBold, runItalic = false, false
			case "tc":
				sb.WriteString("|")
			case "tr":
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

func writeRun(sb *strings.Builder, txt string, bold, italic bool) {
	if bold {
		txt = "**" + txt + "**"
	}
	if italic {
		txt = "*" + txt + "*"
	}
	sb.WriteString(txt)
}

func headingFor(style string) string {
	low := strings.ToLower(style)
	if strings.HasPrefix(low, "heading") || strings.HasPrefix(low, "标题") {
		for _, n := range []string{"1", "2", "3", "4", "5", "6"} {
			if strings.Contains(low, n) {
				return strings.Repeat("#", int(n[0]-'0'))
			}
		}
	}
	return ""
}

func parseNumPr(dec *xml.Decoder) (level int, ordered bool) {
	level = -1
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "numFmt" {
				for _, a := range t.Attr {
					if a.Name.Local == "val" {
						ordered = a.Value == "decimal"
					}
				}
			}
			if t.Name.Local == "ilvl" {
				for _, a := range t.Attr {
					if a.Name.Local == "val" {
						fmt.Sscanf(a.Value, "%d", &level)
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "numPr" {
				return
			}
		}
	}
}
