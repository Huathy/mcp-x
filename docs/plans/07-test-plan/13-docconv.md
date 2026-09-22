# 13 文档转换工具测试子计划

> 日期：2026-09-16
> 关联：[08-doc-conversion.md](../08-doc-conversion.md)
> 范围：8 个 docconv MCP 工具 + 公共层（路径校验/文件大小）

## 1. 工具清单

| ID | 工具 | 转换 | 平台 |
|----|------|------|------|
| M1 | excel_to_md | xlsx→md | 全 |
| M2 | md_to_excel | md→xlsx | 全 |
| M3 | pdf_to_md | pdf→md | 全 |
| M4 | docx_to_md | docx→md | 全 |
| M5 | md_to_docx | md→docx | 全 |
| M6 | md_to_pdf | md→pdf | 全 |
| M7 | word_to_pdf | docx→pdf | Windows+COM |
| M8 | pdf_to_word | pdf→docx | Windows+MS Word |

## 2. 用例

| ID | 工具 | 输入 | 期望 |
|----|------|------|------|
| T-1101 | excel_to_md | sample.xlsx(2 sheet) | 输出含多 sheet 分隔、表格行、分隔符 |
| T-1102 | excel_to_md | 单 sheet 名 | 仅该 sheet，无 `##` 标题 |
| T-1103 | excel_to_md | max_rows=2 | 第 3 行数据被丢弃 |
| T-1104 | excel_to_md | 单元格含 `\|` | 转义为 `\|` |
| T-1105 | md_to_excel | 单表格 md | xlsx 含表头+数据 |
| T-1106 | md_to_excel | 多表格 md | 多 sheet（Table1/Table2） |
| T-1107 | md_to_excel | 无表格 md | 报错 "no markdown table" |
| T-1108 | pdf_to_md | text.pdf | 文本提取正确 |
| T-1109 | pdf_to_md | scan.pdf | 报 ErrScanPDF |
| T-1110 | pdf_to_md | pages="1-2,5" | 仅指定页 |
| T-1111 | pdf_to_md | pages 越界 | 报错 |
| T-1112 | docx_to_md | sample.docx | 标题/段落/列表/表格/粗斜体转 md |
| T-1113 | md_to_docx | full.md | 生成 docx 非空 |
| T-1114 | 往返 | md→docx→md | 文本近似（标题/列表/表格数据保留） |
| T-1115 | md_to_pdf | full.md | 生成 pdf 非空（需系统 CJK 字体，否则报错可接受） |
| T-1116 | 路径校验 | `../escape` | ErrPathTraversal |
| T-1117 | 路径校验 | workdir 外路径 | ErrPathOutsideWorkdir |
| T-1118 | 文件大小 | 超 max_file_size_mb | ErrFileTooLarge |
| T-1119 | word_to_pdf | 非 Windows / 无 COM | 不注册或返回 ErrCOMNotAvailable |
| T-1120 | word_to_pdf | Windows+Word | 生成 pdf 可打开 |
| T-1121 | pdf_to_word | Windows+Word | 生成 docx（布局保真度低） |
| T-1122 | pdf_to_word | WPS progID | 报错 "only supported with MS Word" |

## 3. 测试数据

- `sample.xlsx`：代码内 `writeTestXlsx` 生成（2 sheet）
- `text.pdf` / `scan.pdf`：需外部准备（文本型 / 扫描型）
- `sample.docx`：由 `md_to_docx` 生成的 `full.md` 产出，再回读
- `full.md`：含标题/段落/列表/表格/粗斜体的 markdown

## 4. 已实现状态（2026-09-16）

- 单元测试覆盖 T-1101~T-1107, T-1110~T-1118（`internal/docconv/`）
- 往返测试 T-1114 覆盖（`TestMdToDocxRoundTrip`）
- T-1108/T-1109 需真实 PDF 文件，留作集成测试
- T-1119/T-1120/T-1121/T-1122 需 Windows+Office 环境，留作集成测试

## 5. 回归

docconv 为独立包，不触碰 driver 层。现有 66 条 SQL/Redis/DocStore/Object 用例不受影响。
