package api

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

const (
	maxResumeSize = 10 << 20
	maxResumeText = 2 << 20
)

func parseResume(filename string, content []byte) (string, error) {
	if len(content) == 0 || len(content) > maxResumeSize {
		return "", fmt.Errorf("resume file must be between 1 byte and 10 MB")
	}
	var (
		text string
		err  error
	)
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		text, err = parsePDF(content)
	case ".docx":
		text, err = parseDOCX(content)
	default:
		return "", fmt.Errorf("resume must be a PDF or DOCX file")
	}
	if err != nil {
		return "", err
	}
	text = strings.Join(strings.Fields(text), " ")
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no selectable text was found; scanned resumes need OCR before they can be used")
	}
	if len(text) > maxResumeText {
		text = text[:maxResumeText]
	}
	return text, nil
}

func parsePDF(content []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("read PDF resume: %w", err)
	}
	textReader, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract PDF resume text: %w", err)
	}
	text, err := io.ReadAll(io.LimitReader(textReader, maxResumeText+1))
	if err != nil {
		return "", fmt.Errorf("read extracted PDF text: %w", err)
	}
	if len(text) > maxResumeText {
		text = text[:maxResumeText]
	}
	return string(text), nil
}

func parseDOCX(content []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("read DOCX resume: %w", err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		if file.UncompressedSize64 > maxResumeText*4 {
			return "", fmt.Errorf("DOCX resume content exceeds the 2 MB text limit")
		}
		reader, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("open DOCX resume text: %w", err)
		}
		decoder := xml.NewDecoder(io.LimitReader(reader, maxResumeText*4))
		var text strings.Builder
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = reader.Close()
				return "", fmt.Errorf("parse DOCX resume text: %w", err)
			}
			if content, ok := token.(xml.CharData); ok {
				text.Write([]byte(content))
				text.WriteByte(' ')
				if text.Len() > maxResumeText {
					break
				}
			}
		}
		if err := reader.Close(); err != nil {
			return "", fmt.Errorf("close DOCX resume text: %w", err)
		}
		return text.String(), nil
	}
	return "", fmt.Errorf("DOCX resume does not contain document text")
}
