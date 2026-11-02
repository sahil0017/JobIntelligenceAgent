package api

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseDOCXResumeExtractsText(t *testing.T) {
	var content bytes.Buffer
	archive := zip.NewWriter(&content)
	document, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.Write([]byte(`<document><body><p><t>Backend Engineer</t></p><p><t>Go and PostgreSQL</t></p></body></document>`)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := parseResume("resume.docx", content.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Backend Engineer") || !strings.Contains(text, "Go and PostgreSQL") {
		t.Fatalf("resume text was not extracted: %q", text)
	}
}

func TestParseResumeRejectsUnsupportedAndUnextractableFiles(t *testing.T) {
	if _, err := parseResume("resume.doc", []byte("contents")); err == nil {
		t.Fatal("expected legacy DOC format to be rejected")
	}
	if _, err := parseResume("resume.pdf", []byte("not a PDF")); err == nil {
		t.Fatal("expected malformed PDF to be rejected")
	}
}
