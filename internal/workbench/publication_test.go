package workbench

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPublicationDOCXEPUBPDFPreserveChineseAndValidPackages(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	for _, format := range []string{"docx", "epub", "pdf", "html"} {
		t.Run(format, func(t *testing.T) {
			v, e := s.Publish(ctx, c.ProjectID, format, PublicationOptions{Author: "原创作者", Language: "zh-CN"})
			if format == "pdf" && e != nil && !strings.Contains(e.Error(), "字体") {
				t.Fatal(e)
			}
			if e != nil {
				if format == "pdf" && os.Getenv("PDF_FONT_PATH") == "" {
					t.Skip("PDF font unavailable")
				}
				t.Fatal(e)
			}
			if len(v.Data) < 100 {
				t.Fatal("empty publication")
			}
			if format == "pdf" {
				if !bytes.HasPrefix(v.Data, []byte("%PDF")) {
					t.Fatal("invalid PDF")
				}
				return
			}
			if format == "html" {
				if !bytes.Contains(v.Data, []byte("林川左臂受伤")) {
					t.Fatal("Chinese manuscript omitted")
				}
				return
			}
			z, e := zip.NewReader(bytes.NewReader(v.Data), int64(len(v.Data)))
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, f := range z.File {
				r, e := f.Open()
				if e != nil {
					t.Fatal(e)
				}
				raw, _ := io.ReadAll(r)
				r.Close()
				if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") || strings.HasSuffix(f.Name, ".xhtml") || strings.HasSuffix(f.Name, ".opf") {
					d := xml.NewDecoder(bytes.NewReader(raw))
					for {
						_, e := d.Token()
						if e == io.EOF {
							break
						}
						if e != nil {
							t.Fatalf("%s invalid XML: %v", f.Name, e)
						}
					}
				}
				if bytes.Contains(raw, []byte("林川左臂受伤")) {
					found = true
				}
			}
			if !found {
				t.Fatal("publication dropped manuscript")
			}
			if format == "epub" && (z.File[0].Name != "mimetype" || z.File[0].Method != zip.Store) {
				t.Fatal("EPUB mimetype must be first and uncompressed")
			}
		})
	}
}
func TestPublicationFinalizedOnlyDoesNotIncludeDraft(t *testing.T) {
	s, c := runtimeFixture(t)
	if _, e := s.Publish(context.Background(), c.ProjectID, "docx", PublicationOptions{UseFinal: true}); e == nil {
		t.Fatal("draft exported as finalized")
	}
}

func TestPublicationBodyOnlyRemovesMatchingLeadingChapterHeading(t *testing.T) {
	if got := publicationBody("断桥下", "# 第一章：断桥下\n\n江水。\n正文。"); got != "江水。\n正文。" {
		t.Fatal(got)
	}
	for _, text := range []string{"# 不同标题\n正文。", "第一章：断桥下\n正文。", "断桥下发生了事故\n正文。"} {
		if publicationBody("断桥下", text) != text {
			t.Fatal("body was stripped")
		}
	}
}
