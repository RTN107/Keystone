package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// MaxUploadBytes is the largest file accepted. The cap is enforced by the HTTP
// layer before extraction is attempted.
const MaxUploadBytes = 5 << 20

// These errors carry the sentence shown to the admin, so each says what to do.
var (
	ErrUnsupportedType = errors.New("Keystone can read PDF, plain text and Markdown files. Upload one of those.")
	ErrEmptyDocument   = errors.New("That file has no readable text. Check that it is not empty and try again.")
	ErrNoPDFText       = errors.New("That PDF has no selectable text, which usually means it is a scan. Upload a text-based PDF, or paste the text into a .txt file.")
	ErrUnreadablePDF   = errors.New("That PDF could not be read. Re-export it from the source application and try again.")
)

// Extract returns the normalized text of an uploaded file, chosen by extension.
func Extract(filename string, data []byte) (string, error) {
	var raw string
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt", ".md", ".markdown":
		raw = string(data)
	case ".pdf":
		var err error
		if raw, err = pdfText(data); err != nil {
			return "", err
		}
	default:
		return "", ErrUnsupportedType
	}

	text := Normalize(raw)
	if text == "" {
		if strings.EqualFold(filepath.Ext(filename), ".pdf") {
			return "", ErrNoPDFText
		}
		return "", ErrEmptyDocument
	}
	return text, nil
}

// pdfText reads the selectable text of every page. The library can panic on
// malformed files, so a panic is reported as an unreadable PDF.
func pdfText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", ErrUnreadablePDF
		}
	}()
	if !bytes.HasPrefix(bytes.TrimLeft(data, "\x00\r\n\t "), []byte("%PDF-")) {
		return "", ErrUnreadablePDF
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("%w", ErrUnreadablePDF)
	}
	plain, err := r.GetPlainText()
	if err != nil {
		return "", ErrUnreadablePDF
	}
	b, err := io.ReadAll(plain)
	if err != nil {
		return "", ErrUnreadablePDF
	}
	return string(b), nil
}
