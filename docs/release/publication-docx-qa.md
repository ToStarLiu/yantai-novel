# DOCX 原生 Word 渲染验收

日期：2026-11-12。对象为最终自产三章样例 `.local/publication-sample.docx`，本项只验证出版导出排版与文字保真，不评价小说质量。

## 渲染环境与方法

- 原生 Microsoft Word 16.0.20527.20150，使用现有安装的 COM 接口。
- 打开前设置 `Visible=false`、`DisplayAlerts=0`、`AutomationSecurity=3`、`UpdateLinksAtOpen=false`；以只读、不加入最近文件方式打开 DOCX。
- 使用 `ExportAsFixedFormat(..., 17)` 导出 PDF，随后通过 Codex 工作区依赖的 Python / pypdfium2 逐页渲染为 144 DPI PNG。
- DOCX SHA-256：`47d4406101d45c3a9e902b98bfb04c6b79a4ac8392117bb2f61f9e4d683545ed`。
- Word 结果为 5 页 A4；QA 文件位于 `.local/docx-render-word/publication-sample.pdf` 与 `page-1.png` 至 `page-5.png`。这些文件为本地验收中间产物，不作为最终出版交付物。

本机 documents 技能的 `render_docx.py` 依赖 LibreOffice，而 Windows 捆绑运行时不含 soffice。本次按授权使用原生 Word 替代渲染；未将其记录为 LibreOffice 兼容性通过。此前下载的 LibreOffice MSI 校验失败，未执行或安装；没有为本验收运行该文件。

## 逐页检查结果

| 页 | 内容 | 结果 |
|---|---|---|
| 1 | 作品标题、作者、简介与页码 | 标题居中，中文可读，无装饰线、裁切或重叠 |
| 2 | 第一卷、断桥下章节及正文 | 卷章层级明确，无重复 Markdown 章节标题；中文标点、缩进、边距正常 |
| 3 | 前章续文、只说王都章节及正文 | 页顶续文与新章标题正常，无孤立标题、缺字或文字重叠 |
| 4 | 第二章续文、启箱之前章节及正文 | 段落和章节间距清晰，行尾及引号可读，正文未侵入页脚 |
| 5 | 第三章结尾及页码 | 结尾完整，剩余留白符合末页；无空白尾页或截断 |

所有 5 张 PNG 已逐页实际打开检查。正文段落间留白来自样例本身的空段，不是丢失内容；没有为压缩页数删除作者段落。Word 的 5 页与独立 PDF 导出器的分页不同，不影响本项 DOCX 验收。

还对 DOCX `word/document.xml` 的全部文本与 Word 导出 PDF 提取文本进行比对：去除每页页码及排版空白后完全相同；标题、简介和三章正文没有丢失或重复。DOCX 保持只读，未被 Word 改写。

本结果覆盖该样例在本机原生 Word 的显示；其他办公套件、阅读器、复杂插图、注释、脚注与最终印刷规范需另行验证。
