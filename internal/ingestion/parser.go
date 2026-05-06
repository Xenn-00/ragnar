package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ParsedDocument is a result from parsing - a clean text that ready for chunked
type ParsedDocument struct {
	Content  string
	MimeType string
	Pages    int // relevant for PDF, 0 for plain text
}

// Parser is responsible for extracting a clean text from raw file bytes
type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

// Parse detect mime type and dispatch it to the suitable parser.
func (p *Parser) Parse(ctx context.Context, filename string, data []byte, mimeType string) (*ParsedDocument, error) {
	switch mimeType {
	case "application/pdf":
		return p.parsePDF(data)
	case "text/markdown", "text/plain":
		return p.parseMarkdown(data)
	default:
		return nil, fmt.Errorf("unsupported mime type: %s", mimeType)
	}
}

func (p *Parser) parsePDF(data []byte) (*ParsedDocument, error) {
	reader := bytes.NewReader(data)

	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}

	numPages := pdfReader.NumPage()
	var sb strings.Builder

	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// skip pages, which are failed to be extracted, don't abort it all
			continue
		}

		sb.WriteString(text)
		sb.WriteString("\n")
	}

	content := strings.TrimSpace(sb.String())
	if content == "" {
		return nil, fmt.Errorf("pdf contains no extractable text (might be scanned/image-based)")
	}

	return &ParsedDocument{
		Content:  content,
		MimeType: "application/pdf",
		Pages:    numPages,
	}, nil
}

func (p *Parser) parseMarkdown(data []byte) (*ParsedDocument, error) {
	// Strip null bytes
	cleaned := bytes.ReplaceAll(data, []byte{0x00}, []byte{})

	content := strings.TrimSpace(string(cleaned))

	if content == "" {
		return nil, fmt.Errorf("markdown contains no extractable text")
	}

	return &ParsedDocument{
		Content:  content,
		MimeType: "text/markdown",
		Pages:    0,
	}, nil
}

// make sure parser implement io.Closer

var _ io.Closer = (*parserCloser)(nil)

type parserCloser struct{ *Parser }

func (parserCloser) Close() error { return nil }
