package workbench

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/go-pdf/fpdf"
	"html"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type PublicationOptions struct {
	Author          string `json:"author"`
	Language        string `json:"language"`
	UseFinal        bool   `json:"useFinal"`
	IncludeSynopsis bool   `json:"includeSynopsis"`
}
type Publication struct {
	Filename    string
	ContentType string
	Data        []byte
}
type bookChapter struct{ ID, Title, Volume, Text string }

var publicationChapterPrefix = regexp.MustCompile(`^第[0-9零一二三四五六七八九十百千万两]+[章节卷][：:\s]*`)

func publicationBody(title, text string) string {
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) < 2 {
		return text
	}
	first := strings.TrimSpace(lines[0])
	plain := strings.TrimSpace(strings.TrimLeft(first, "#"))
	if plain == strings.TrimSpace(title) || (strings.HasPrefix(first, "#") && publicationChapterPrefix.ReplaceAllString(plain, "") == publicationChapterPrefix.ReplaceAllString(strings.TrimSpace(title), "")) {
		return strings.TrimLeft(lines[1], "\r\n")
	}
	return text
}

func (s *Service) Publish(ctx context.Context, pid, format string, options PublicationOptions) (Publication, error) {
	pkg, err := s.ExportProject(ctx, pid)
	if err != nil {
		return Publication{}, err
	}
	revs := map[string]Revision{}
	for _, r := range pkg.Revisions {
		revs[r.ID] = r
	}
	sort.SliceStable(pkg.Chapters, func(i, j int) bool { return pkg.Chapters[i].Order < pkg.Chapters[j].Order })
	chapters := []bookChapter{}
	for _, c := range pkg.Chapters {
		if c.Deleted {
			continue
		}
		id := c.CurrentRevisionID
		if options.UseFinal {
			id = c.FinalRevisionID
			if id == "" {
				continue
			}
		}
		text := publicationBody(c.Title, revs[id].PlainText)
		chapters = append(chapters, bookChapter{c.ID, c.Title, c.Volume, text})
	}
	if len(chapters) == 0 {
		return Publication{}, fmt.Errorf("没有可导出的章节")
	}
	if options.Language == "" {
		options.Language = "zh-CN"
	}
	if len(options.Author) > 200 {
		return Publication{}, fmt.Errorf("作者名过长")
	}
	switch format {
	case "docx":
		data, err := bookDOCX(pkg.Project, chapters, options)
		return Publication{"novel.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data}, err
	case "epub":
		data, err := bookEPUB(pkg.Project, chapters, options)
		return Publication{"novel.epub", "application/epub+zip", data}, err
	case "pdf":
		data, err := bookPDF(pkg.Project, chapters, options)
		return Publication{"novel.pdf", "application/pdf", data}, err
	case "html":
		return Publication{"novel.html", "text/html; charset=utf-8", []byte(bookHTML(pkg.Project, chapters, options))}, nil
	default:
		return Publication{}, fmt.Errorf("出版格式支持docx、epub、pdf、html")
	}
}
func escapeXML(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
func zipFile(w *zip.Writer, name, content string) error {
	f, err := w.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(content))
	return err
}

func bookDOCX(p Project, chapters []bookChapter, o PublicationOptions) ([]byte, error) {
	var body strings.Builder
	para := func(style, text string) {
		fmt.Fprintf(&body, `<w:p><w:pPr><w:pStyle w:val="%s"/></w:pPr><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, style, escapeXML(text))
	}
	para("Title", p.Title)
	if o.Author != "" {
		para("Subtitle", o.Author)
	}
	if o.IncludeSynopsis && p.Description != "" {
		para("Normal", p.Description)
	}
	lastVolume := ""
	for _, c := range chapters {
		if c.Volume != "" && c.Volume != lastVolume {
			para("Heading1", c.Volume)
			lastVolume = c.Volume
		}
		para("Heading2", c.Title)
		for _, line := range strings.Split(c.Text, "\n") {
			para("Normal", line)
		}
	}
	body.WriteString(`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/><w:footerReference w:type="default" r:id="rFooter"/></w:sectPr>`)
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>` + body.String() + `</w:body></w:document>`
	styles := `<?xml version="1.0" encoding="UTF-8"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:eastAsia="SimSun" w:hAnsi="Calibri"/><w:sz w:val="24"/><w:color w:val="000000"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="360" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:pPr><w:ind w:firstLine="480"/><w:widowControl/></w:pPr></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:pPr><w:jc w:val="center"/><w:spacing w:before="1600" w:after="360"/></w:pPr><w:rPr><w:b/><w:sz w:val="44"/><w:color w:val="000000"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:pPr><w:jc w:val="center"/><w:spacing w:after="480"/></w:pPr><w:rPr><w:sz w:val="24"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:pPr><w:pageBreakBefore/><w:keepNext/><w:spacing w:before="240" w:after="240"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:pPr><w:keepNext/><w:spacing w:before="320" w:after="240"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/><w:sz w:val="28"/></w:rPr></w:style></w:styles>`
	files := map[string]string{
		"[Content_Types].xml":          `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`,
		"_rels/.rels":                  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rDoc" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rProps" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/></Relationships>`,
		"word/document.xml":            document, "word/styles.xml": styles,
		"word/footer1.xml":  `<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p></w:ftr>`,
		"docProps/core.xml": `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + escapeXML(p.Title) + `</dc:title><dc:creator>` + escapeXML(o.Author) + `</dc:creator><dc:language>` + escapeXML(o.Language) + `</dc:language></cp:coreProperties>`}
	var result bytes.Buffer
	z := zip.NewWriter(&result)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml", "word/_rels/document.xml.rels", "word/footer1.xml", "docProps/core.xml"} {
		if err := zipFile(z, name, files[name]); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func bookEPUB(p Project, chapters []bookChapter, o PublicationOptions) ([]byte, error) {
	var result bytes.Buffer
	z := zip.NewWriter(&result)
	mime, err := z.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	_, _ = mime.Write([]byte("application/epub+zip"))
	if err = zipFile(z, "META-INF/container.xml", `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`); err != nil {
		return nil, err
	}
	var manifest, spine, nav strings.Builder
	manifest.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="css" href="book.css" media-type="text/css"/>`)
	for i, c := range chapters {
		name := fmt.Sprintf("chapter-%03d.xhtml", i+1)
		id := fmt.Sprintf("c%d", i+1)
		fmt.Fprintf(&manifest, `<item id="%s" href="%s" media-type="application/xhtml+xml"/>`, id, name)
		fmt.Fprintf(&spine, `<itemref idref="%s"/>`, id)
		fmt.Fprintf(&nav, `<li><a href="%s">%s</a></li>`, name, escapeXML(c.Title))
		var content strings.Builder
		fmt.Fprintf(&content, `<h1>%s</h1>`, escapeXML(c.Title))
		for _, line := range strings.Split(c.Text, "\n") {
			if line != "" {
				fmt.Fprintf(&content, `<p>%s</p>`, escapeXML(line))
			}
		}
		page := `<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xml:lang="` + escapeXML(o.Language) + `"><head><title>` + escapeXML(c.Title) + `</title><link rel="stylesheet" href="book.css"/></head><body>` + content.String() + `</body></html>`
		if err = zipFile(z, "EPUB/"+name, page); err != nil {
			return nil, err
		}
	}
	navPage := `<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>目录</title></head><body><nav epub:type="toc"><h1>` + escapeXML(p.Title) + `</h1><ol>` + nav.String() + `</ol></nav></body></html>`
	opf := `<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="bookid">urn:novel:` + escapeXML(p.ID) + `</dc:identifier><dc:title>` + escapeXML(p.Title) + `</dc:title><dc:creator>` + escapeXML(o.Author) + `</dc:creator><dc:language>` + escapeXML(o.Language) + `</dc:language><meta property="dcterms:modified">` + time.Now().UTC().Format("2006-01-02T15:04:05Z") + `</meta></metadata><manifest>` + manifest.String() + `</manifest><spine>` + spine.String() + `</spine></package>`
	for name, content := range map[string]string{"EPUB/package.opf": opf, "EPUB/nav.xhtml": navPage, "EPUB/book.css": "body{font-family:serif;line-height:1.8;margin:5%;}h1{font-size:1.6em;page-break-after:avoid;}p{text-indent:2em;margin:0 0 .8em;}"} {
		if err = zipFile(z, name, content); err != nil {
			return nil, err
		}
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}
func bookHTML(p Project, chapters []bookChapter, o PublicationOptions) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>` + html.EscapeString(p.Title) + `</title><style>@page{size:A4;margin:24mm;}body{font-family:"Noto Serif CJK SC",SimSun,serif;max-width:42em;margin:3em auto;line-height:1.9;color:#171917}h1{font-size:2em}h2{break-before:page;font-size:1.5em}p{text-indent:2em;orphans:3;widows:3}header{text-align:center;margin:10em 0}small{color:#555}</style><header><h1>` + html.EscapeString(p.Title) + `</h1><p>` + html.EscapeString(o.Author) + `</p></header>`)
	if o.IncludeSynopsis {
		b.WriteString(`<p>` + html.EscapeString(p.Description) + `</p>`)
	}
	for _, c := range chapters {
		b.WriteString(`<section><h2>` + html.EscapeString(c.Title) + `</h2>`)
		for _, line := range strings.Split(c.Text, "\n") {
			if line != "" {
				b.WriteString(`<p>` + html.EscapeString(line) + `</p>`)
			}
		}
		b.WriteString(`</section>`)
	}
	b.WriteString(`</html>`)
	return b.String()
}

func bookPDF(p Project, chapters []bookChapter, o PublicationOptions) ([]byte, error) {
	font := os.Getenv("PDF_FONT_PATH")
	if font == "" {
		for _, path := range []string{"assets/fonts/NotoSansSC-Regular.ttf", `C:\Windows\Fonts\simhei.ttf`, "/usr/share/fonts/truetype/wqy/wqy-zenhei.ttf"} {
			if _, err := os.Stat(path); err == nil {
				font = path
				break
			}
		}
	}
	if font == "" {
		return nil, fmt.Errorf("PDF中文字体未找到，请配置PDF_FONT_PATH指向TTF字体")
	}
	fontData, err := os.ReadFile(font)
	if err != nil {
		return nil, fmt.Errorf("PDF字体不可用：%w", err)
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(23, 24, 23)
	pdf.SetAutoPageBreak(true, 22)
	// Resolve OS paths ourselves: fpdf treats filename arguments as relative to
	// its font directory, including configured absolute Unix paths.
	pdf.AddUTF8FontFromBytes("body", "", fontData)
	pdf.SetFont("body", "", 11)
	if pdf.Error() != nil {
		return nil, fmt.Errorf("PDF字体不可用：%w", pdf.Error())
	}
	pdf.SetTitle(p.Title, true)
	pdf.SetAuthor(o.Author, true)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("body", "", 9)
		pdf.CellFormat(0, 6, fmt.Sprint(pdf.PageNo()), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	pdf.SetY(65)
	pdf.SetFont("body", "", 24)
	pdf.MultiCell(0, 13, p.Title, "", "C", false)
	pdf.Ln(8)
	pdf.SetFont("body", "", 12)
	pdf.MultiCell(0, 8, o.Author, "", "C", false)
	if o.IncludeSynopsis && p.Description != "" {
		pdf.Ln(18)
		pdf.SetFont("body", "", 11)
		pdf.MultiCell(0, 8, p.Description, "", "L", false)
	}
	for _, c := range chapters {
		pdf.AddPage()
		pdf.SetFont("body", "", 17)
		pdf.MultiCell(0, 10, c.Title, "", "L", false)
		pdf.Ln(6)
		pdf.SetFont("body", "", 11)
		for _, line := range strings.Split(c.Text, "\n") {
			if strings.TrimSpace(line) == "" {
				pdf.Ln(3)
				continue
			}
			for _, row := range pdfParagraphLines(pdf, "　　"+line, 162) {
				pdf.CellFormat(0, 7.5, row, "", 1, "L", false, 0, "")
			}
			pdf.Ln(2)
		}
	}
	var result bytes.Buffer
	if err := pdf.Output(&result); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

// Keep closing punctuation off the beginning of a line and opening punctuation
// off its end without changing manuscript content or glyph size.
func pdfParagraphLines(pdf *fpdf.Fpdf, text string, width float64) []string {
	rows := pdf.SplitText(text, width)
	closing := "，。、；：！？）］】》〉”’」』…—"
	opening := "（［【《〈“‘「『"
	for i := 1; i < len(rows); i++ {
		before, next := []rune(rows[i-1]), []rune(rows[i])
		if len(before) > 1 && len(next) > 0 && (strings.ContainsRune(closing, next[0]) || strings.ContainsRune(opening, before[len(before)-1])) {
			move := len(before) - 1
			for move > 1 && strings.ContainsRune(closing, before[move]) {
				move--
			}
			rows[i-1] = string(before[:move])
			rows[i] = string(before[move:]) + string(next)
		}
	}
	return rows
}
