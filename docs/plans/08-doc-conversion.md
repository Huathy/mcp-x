# 08 文档格式转换工具集

> 数据版本：2026-09-16
> 状态：设计草案
> 关联：[06-tools-extension.md](06-tools-extension.md) 第 1 节工具 #3/#4/#8 的细化落地

## 1. 目标与约束

### 1.1 目标
为 mcp-x 新增 8 个文档转换 MCP 工具，覆盖 Office 文档与 Markdown 互转：

| # | 工具 | 转换 | 技术路线 | 质量 | 分层 |
|---|------|------|---------|------|------|
| M1 | `excel_to_md` | xlsx→md | xuri/excelize 读 + 手写表格→md | 高 | Tier1 内置 |
| M2 | `md_to_excel` | md→xlsx | excelize 写 + 解析 md 表格 | 高 | Tier1 内置 |
| M3 | `pdf_to_md` | pdf→md | ledongthuc/pdf 提文本 | 中 | Tier1 内置 |
| M4 | `docx_to_md` | docx→md | archive/zip+encoding/xml 解 OOXML | 中-高 | Tier1 内置 |
| M5 | `md_to_docx` | md→docx | 生成 OOXML XML 打包 zip | 中 | Tier1 内置 |
| M6 | `md_to_pdf` | md→pdf | signintech/gopdf + yuin/goldmark | 低-中 | Tier1 内置(简陋版) |
| M7 | `word_to_pdf` | docx→pdf | Windows COM (MS Word/WPS) | 高 | Tier2 可选 |
| M8 | `pdf_to_word` | pdf→docx | Windows COM (MS Word) | 低-中 | Tier2 实验性 |

### 1.2 硬约束
1. **零外部运行时**：终端用户安装 mcp-x 后，Tier1 工具不依赖 python/node/LibreOffice/pandoc。全部纯 Go 静态编译进单一二进制。
2. **跨平台**：Tier1 六工具 Linux/macOS/Windows 全可用。Tier2 (M7/M8) 仅 Windows + 用户机器装了 MS Office 或 WPS。
3. **不假装支持**：Tier2 探测失败时（非 Windows 或无 Office/WPS），对应工具不注册或调用返回明确错误，不静默降级到低质量输出。
4. **扫描型 PDF 无解**：M3/M8 对图片型 PDF 无效，需 OCR（纯 Go 无成熟方案），工具描述标注限制。

## 2. 技术选型

### 2.1 Tier1 纯 Go 依赖

| 依赖 | 用途 | 授权 | CGO |
|------|------|------|-----|
| `github.com/xuri/excelize/v2` | xlsx 读写 | BSD-3 | 无 |
| `github.com/ledongthuc/pdf` | pdf 文本提取 | MIT | 无 |
| `github.com/signintech/gopdf` | pdf 生成 | MIT | 无 |
| `github.com/yuin/goldmark` | md AST 解析 | MIT | 无 |
| `archive/zip` `encoding/xml` (stdlib) | OOXML docx 解析/生成 | - | 无 |

### 2.2 Tier2 COM 依赖（仅 Windows）

| 依赖 | 用途 | 授权 | CGO |
|------|------|------|-----|
| `github.com/go-ole/go-ole` | Windows COM 调用 | MIT | 无 |

go-ole 纯 Go，无 cgo，静态编译进二进制。非 Windows 平台编译时该包不启用（见 2.3）。

### 2.3 平台隔离

```go
// internal/docconv/docx_com.go
//go:build windows

package docconv

import "github.com/go-ole/go-ole"
// MS Word / WPS COM 实现仅 windows 编译
```

```go
// internal/docconv/docx_com_stub.go
//go:build !windows

package docconv

func wordToPdfCOM(src, dst string) error {
    return ErrCOMNotAvailable // "Word/PDF 互转需 Windows + MS Office/WPS"
}
func pdfToWordCOM(src, dst string) error {
    return ErrCOMNotAvailable
}
```

M7/M8 的 MCP 工具注册逻辑：
```go
if runtime.GOOS == "windows" && comProbeAvailable() {
    registerTool("word_to_pdf", ...)
    registerTool("pdf_to_word", ...)
}
```
`comProbeAvailable()` 启动时依次试 `ole.CreateObject("Word.Application")` / `ole.CreateObject("kwps.Application")` / `ole.CreateObject("wps.Application")`，记录可用 ProgID。无则不注册。

## 3. 架构设计

### 3.1 目录结构

```
internal/docconv/
├── docconv.go            # 公共类型、错误、注册入口
├── excel_to_md.go        # M1
├── md_to_excel.go        # M2
├── pdf_to_md.go          # M3
├── docx_to_md.go         # M4
├── md_to_docx.go         # M5
├── md_to_pdf.go          # M6
├── docx_com.go           # M7/M8 (//go:build windows)
├── docx_com_stub.go      # M7/M8 stub (//go:build !windows)
└── *_test.go             # 每工具对应测试

internal/mcp/
└── tools_docconv.go      # MCP 工具注册 + handler
```

### 3.2 文件路径安全（复用现有 safety 思路）

所有转换工具涉及文件读写，必须校验路径：
- 输入/输出路径 workdir 限制（config `docconv.workdir`）
- 拒绝 `..` 路径穿越
- 输出路径不得覆盖 workdir 外文件

### 3.3 配置扩展

```yaml
docconv:
  enabled: true
  workdir: "./data/docconv"    # 工作目录，限制读写范围
  max_file_size_mb: 50         # 输入文件大小上限
  com:                         # Tier2 配置
    prog_id: "auto"            # auto = 自动探测 Word.Application/kwps/wps
                              # 或指定 "Word.Application" / "kwps.Application"
    timeout: 60s               # COM 调用超时（拉起 Word 进程慢）
```

`Config` 结构体新增 `Docconv DocconvConfig`。

## 4. 各工具规格

### M1 excel_to_md

**输入**：`path`(xlsx 路径)、`sheet`(可选，默认第一个或全部)、`max_rows`(可选，默认 1000)

**逻辑**：
1. `excelize.OpenFile(path)`
2. 遍历指定 sheet（或全部）
3. 每行单元格 → md 表格行，`|` 分隔，首行做表头
4. 多 sheet 用 `## SheetName` 分隔
5. 空单元格输出空字符串

**输出**：md 文本（返回给 MCP）或写文件（若 `output_path` 指定）

**限制**：合并单元格降级为重复值；公式输出计算结果值；图表/图片忽略。

### M2 md_to_excel

**输入**：`content`(md 文本) 或 `path`(md 文件)、`output_path`(xlsx 输出)、`sheet_name`(可选)

**逻辑**：
1. goldmark 解析 md
2. 遍历 AST，提取 `ast.Table` 节点
3. 每个表格 → excelize sheet（多表用 sheet 名或 Table1/Table2）
4. 行列映射到单元格

**限制**：只转换 md 表格语法，其余文本（标题/列表/段落）忽略或写入单独 "Text" sheet。

### M3 pdf_to_md

**输入**：`path`(pdf 路径)、`pages`(可选，默认全部，格式 "1-5,8")

**逻辑**：
1. `pdf.Open(path)`
2. 遍历页，`page.GetPlainText(nil)`
3. 按页拼接，`## Page N` 分隔
4. 尝试保留段落空行

**限制**：扫描型 PDF 返回空文本 → 报错 "PDF 无文本层（可能为扫描件），需 OCR"；表格/复杂布局不保留结构。

### M4 docx_to_md

**输入**：`path`(docx 路径)

**逻辑**：
1. `zip.OpenReader(path)` 解压
2. 读 `word/document.xml`
3. `encoding/xml` 解析，遍历元素：
   - `w:p` 段落 → 换行
   - `w:pStyle` 标题样式 → `#`/`##`/`###`
   - `w:t` 文本运行 → 累加
   - `w:b`/`w:i` → `**`/`*`
   - `w:tbl` 表格 → md 表格
   - `w:numPr` 列表 → `-` 或 `1.`
4. 输出 md 文本

**限制**：复杂样式（颜色/字号/对齐）不保留；页眉页脚忽略；图片忽略（提取 alt 文本如有）。

### M5 md_to_docx

**输入**：`content`(md 文本) 或 `path`、`output_path`

**逻辑**：
1. goldmark 解析 md AST
2. 遍历节点生成 OOXML XML：
   - Heading → `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr>`
   - Paragraph → `<w:p>`
   - List → `<w:numPr>` 或简化为 `•` 前缀
   - Table → `<w:tbl>` 基本 grid
   - Strong → `<w:rPr><w:b/></w:rPr>`
   - Emphasis → `<w:rPr><w:i/></w:rPr>`
3. 套固定模板（document.xml + [Content_Types].xml + _rels）打包 zip

**限制**：基本样式 OK，复杂排版（多栏/分页/字体颜色）不支持；图片不嵌入（仅留占位文本）。

### M6 md_to_pdf

**输入**：`content` 或 `path`、`output_path`

**逻辑**：
1. goldmark 解析
2. gopdf 创建 PDF，逐元素排版：
   - Heading → 加粗大字号
   - Paragraph → 自动换行
   - Table → 简单格子绘制
   - List → `•`/`1.` 前缀
3. 中文字体：gopdf 需 `AddFont` 加载 ttf。策略：
   - Windows: `C:\Windows\Fonts\msyh.ttc`（微软雅黑）
   - Linux: `/usr/share/fonts/...` 探测
   - macOS: `/System/Library/Fonts/...` 探测
   - 全部找不到 → 报错或回退 Latin 字体（中文乱码，工具描述标注）

**限制**：无 CSS，排版简陋，无页眉页脚，表格无合并单元格支持。质量低-中，标注"简陋版，高质量需外部 pandoc"。

### M7 word_to_pdf

**输入**：`path`(docx)、`output_path`(pdf)

**逻辑**（Windows + COM 可用时）：
```go
func wordToPdfCOM(src, dst string) error {
    ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
    defer ole.CoUninitialize()
    progID := cfg.ComProgID // "Word.Application" 或 "kwps.Application"
    unk, err := ole.CreateObject(progID)
    if err != nil { return fmt.Errorf("COM 创建失败(%s): %w", progID, err) }
    defer unk.Release()
    app, _ := unk.QueryInterface(ole.IID_IDispatch)
    docs, _ := app.GetProperty("Documents")
    doc, _ := docs.Call("Open", src)
    defer doc.Call("Close", false)
    doc.Call("SaveAs2", dst, 17) // wdFormatPDF = 17
    app.Call("Quit")
    return nil
}
```

**探测顺序**：`Word.Application` → `kwps.Application` → `wps.Application`，首个成功的 progID 记入 config 缓存。

**限制**：仅 Windows；首启慢（拉起 Word 进程 1-3s）；需用户机器装 Office 或 WPS。

### M8 pdf_to_word

**输入**：`path`(pdf)、`output_path`(docx)

**逻辑**（Windows + MS Word COM）：
```go
func pdfToWordCOM(src, dst string) error {
    // ... 同 M7 初始化
    doc, _ := docs.Call("Open", src)        // Word 2013+ 可开 PDF
    defer doc.Call("Close", false)
    doc.Call("SaveAs2", dst, 12)            // wdFormatXMLDocument = 12
    app.Call("Quit")
    return nil
}
```

**限制（诚实标注）**：
- 仅 MS Word（WPS 的 PDF→Word 是独立功能，COM 暴露性不确定，**不保证 WPS 可用**，探测时优先 Word.Application）
- Word 重排 PDF，**复杂布局/表格/公式常错乱**，仅适合文本型 PDF
- 工具描述标注："(实验性，布局保真度低，建议先用 pdf_to_md 提文本)"

## 5. MCP 工具注册

`internal/mcp/tools_docconv.go`：

```go
func (s *Server) registerDocconvTools() {
    if !s.cfg.Docconv.Enabled {
        return
    }
    // Tier1 全平台
    s.registerTool("excel_to_md", "Excel(xlsx) → Markdown 表格", s.handleExcelToMd)
    s.registerTool("md_to_excel", "Markdown 表格 → Excel(xlsx)", s.handleMdToExcel)
    s.registerTool("pdf_to_md", "PDF 文本提取 → Markdown(仅文本型PDF)", s.handlePdfToMd)
    s.registerTool("docx_to_md", "Word(docx) → Markdown", s.handleDocxToMd)
    s.registerTool("md_to_docx", "Markdown → Word(docx,基本样式)", s.handleMdToDocx)
    s.registerTool("md_to_pdf", "Markdown → PDF(简陋排版,中文需系统字体)", s.handleMdToPdf)

    // Tier2 仅 Windows + COM 可用
    if runtime.GOOS == "windows" {
        if progID := comProbe(s.cfg.Docconv.Com.ProgID); progID != "" {
            s.comProgID = progID
            s.registerTool("word_to_pdf", "Word(docx) → PDF(Windows COM,高质量)", s.handleWordToPdf)
            s.registerTool("pdf_to_word", "PDF → Word(docx,实验性,布局保真度低)", s.handlePdfToWord)
        }
    }
}
```

`server.go` 的 `registerTools()` 调用 `registerDocconvTools()`。

## 6. 安全设计

### 6.1 路径校验
```go
func validatePath(workdir, p string) error {
    abs, err := filepath.Abs(p)
    if err != nil { return err }
    wd, _ := filepath.Abs(workdir)
    if !strings.HasPrefix(abs, wd) {
        return ErrPathOutsideWorkdir
    }
    if strings.Contains(filepath.ToSlash(p), "..") {
        return ErrPathTraversal
    }
    return nil
}
```
所有工具 handler 入口校验 `path`/`output_path`。

### 6.2 文件大小
读取前 `os.Stat` 检查 `max_file_size_mb`，防 OOM。

### 6.3 COM 超时
Tier2 调用用 `context.WithTimeout`，超时强制 `app.Call("Quit")` 释放 Word 进程。

## 7. 测试计划

### 7.1 单元测试（每工具）

| 工具 | 测试文件 | 断言 |
|------|---------|------|
| excel_to_md |testdata/sample.xlsx(含2 sheet,合并单元格)|输出含表格,多sheet分隔,合并单元格降级正确|
| md_to_excel |testdata/tables.md(含3表格)|输出 xlsx,3 sheet,数据对齐|
| pdf_to_md |testdata/text.pdf / scan.pdf|文本型提取正确;扫描型报错"无文本层"|
| docx_to_md |testdata/sample.docx(标题/段落/列表/表格/粗斜体)|结构转 md 正确|
| md_to_docx |testdata/full.md|生成 docx 可被 docx_to_md 回读(往返测试)|
| md_to_pdf |testdata/full.md|生成 pdf 非空,可被 pdf_to_md 提回文本|

### 7.2 集成测试（MCP 端到端）

复用 `07-test-plan/` 模式，新建 `07-test-plan/13-docconv.md`：
- T-1101~T-1108 每工具一条
- T-1109 往返测试：md→docx→md，比对文本近似度
- T-1110 往返测试：md→xlsx→md，比对表格
- T-1111 COM 不可用环境：word_to_pdf 不注册或返回明确错误
- T-1112 COM 可用环境（Windows+Word）：word_to_pdf 生成 pdf 可打开

### 7.3 回归
现有 66 条 SQL/Redis/DocStore 用例不受影响（docconv 是独立包，不碰 driver 层）。

## 8. 交付物清单

- [ ] `internal/docconv/docconv.go` — 公共类型、路径校验、错误
- [ ] `internal/docconv/excel_to_md.go` + 测试
- [ ] `internal/docconv/md_to_excel.go` + 测试
- [ ] `internal/docconv/pdf_to_md.go` + 测试
- [ ] `internal/docconv/docx_to_md.go` + 测试
- [ ] `internal/docconv/md_to_docx.go` + 测试
- [ ] `internal/docconv/md_to_pdf.go` + 测试
- [ ] `internal/docconv/docx_com.go` (windows) + `docx_com_stub.go` (非windows)
- [ ] `internal/mcp/tools_docconv.go` — MCP 工具注册
- [ ] `internal/config/config.go` — 新增 `DocconvConfig`
- [ ] `examples/mcp-x.yaml.example` — docconv 配置示例
- [ ] `docs/plans/07-test-plan/13-docconv.md` — 测试子计划
- [ ] `go.mod` 新增依赖（excelize/ledongthuc/pdf/gopdf/goldmark/go-ole）
- [ ] `cmd/mcp-x/main.go` — import docconv 驱动注册（如需 build tag）

## 9. 实施顺序与工时

| 阶段 | 内容 | 工时 | 前置 |
|------|------|------|------|
| S1 | docconv.go 公共 + 路径校验 + config 扩展 | 0.5天 | - |
| S2 | excel_to_md + md_to_excel(excelize 链路验证) | 1天 | S1 |
| S3 | docx_to_md + md_to_docx(OOXML 链路,往返测试) | 1.5天 | S1 |
| S4 | pdf_to_md(ledongthuc/pdf) | 0.5天 | S1 |
| S5 | md_to_pdf(gopdf+goldmark,中文字体探测) | 1天 | S1 |
| S6 | word_to_pdf + pdf_to_word(COM,Windows) | 1天 | S1 |
| S7 | MCP 工具注册 + 集成测试 | 0.5天 | S2-S6 |
| **合计** | | **6天** | |

S2 起步：excelize 最成熟，先验证纯 Go 链路无坑，确立 docconv.go 公共层后再推其余。

## 10. 已知限制与降级策略（全局）

1. **扫描型 PDF**：M3/M8 无解，报错提示需 OCR。纯 Go 无成熟 OCR。
2. **md_to_pdf 简陋**：无 CSS，排版粗糙。工具描述标注。高质量需外部 pandoc（可选 Tier3 探测二进制，不在本期）。
3. **pdf_to_word 实验性**：Word 重排 PDF，复杂布局错乱。工具描述标注"(实验性)"，推荐用户先用 `pdf_to_md`。
4. **WPS pdf_to_word 不保证**：WPS 的 PDF 转换是独立功能，COM 暴露性未确认。仅 M7(word_to_pdf) 承诺 WPS 可用，M8 仅 MS Word。
5. **docx 复杂排版**：M5 md_to_docx 只支持基本样式（标题/段落/列表/表格/粗斜体），颜色/字号/多栏不支持。
6. **中文字体**：M6 md_to_pdf 依赖系统字体文件，全平台探测失败则中文乱码，工具描述标注限制。

## 11. 不实现项（YAGNI）

- OCR（扫描型 PDF）：纯 Go 无成熟方案，不做
- docx 图片嵌入：本期仅留占位文本
- md_to_pdf CSS 排版：简陋版够用，高质量走外部 pandoc（未来可选）
- LibreOffice 集成：体积大（~500MB），word_to_pdf 已由 COM 覆盖 Windows，Linux/macOS 用户暂无 word_to_pdf（未来可选打包 pandoc）
- pptx 转换：office-md 原生支持，但方向是 pptx→md 单向，与本期双向目标不符，暂不做
