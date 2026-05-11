package ingestion

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse_PlainText(t *testing.T) {
	p := NewParser()
	ctx := context.Background()

	t.Run("valid plain text", func(t *testing.T) {
		content := []byte("Hello world, this is a test document.")
		doc, err := p.Parse(ctx, "test.txt", content, "text/plain")

		require.NoError(t, err)
		assert.Equal(t, "Hello world, this is a test document.", doc.Content)
		assert.Equal(t, "text/markdown", doc.MimeType)
		assert.Equal(t, 0, doc.Pages)
	})

	t.Run("valid markdown", func(t *testing.T) {
		content := []byte("# Heading\n\nSome paragraph text here.")
		doc, err := p.Parse(ctx, "test.md", content, "text/markdown")

		require.NoError(t, err)
		assert.Equal(t, "# Heading\n\nSome paragraph text here.", doc.Content)
		assert.Equal(t, "text/markdown", doc.MimeType)
		assert.Equal(t, 0, doc.Pages)
	})

	t.Run("empty plain text returns error", func(t *testing.T) {
		doc, err := p.Parse(ctx, "empty.txt", []byte(""), "text/plain")

		assert.Error(t, err)
		assert.Nil(t, doc)
		assert.Contains(t, err.Error(), "no extractable text")
	})

	t.Run("whitespace-only plain text returns error", func(t *testing.T) {
		doc, err := p.Parse(ctx, "blank.txt", []byte("   \n\t  "), "text/plain")

		assert.Error(t, err)
		assert.Nil(t, doc)
	})

	t.Run("null bytes are stripped", func(t *testing.T) {
		content := []byte("Hello\x00World")
		doc, err := p.Parse(ctx, "test.txt", content, "text/plain")

		require.NoError(t, err)
		assert.Equal(t, "HelloWorld", doc.Content)
	})

	t.Run("leading and trailing whitespace is trimmed", func(t *testing.T) {
		content := []byte("   \n  actual content here  \n   ")
		doc, err := p.Parse(ctx, "test.txt", content, "text/plain")

		require.NoError(t, err)
		assert.Equal(t, "actual content here", doc.Content)
	})
}

func TestParser_Parse_UnsupportedMimeType(t *testing.T) {
	p := NewParser()
	ctx := context.Background()

	unsupportedTypes := []string{
		"application/json",
		"text/html",
		"image/png",
		"application/octet-stream",
	}

	for _, mimeType := range unsupportedTypes {
		t.Run(mimeType, func(t *testing.T) {
			doc, err := p.Parse(ctx, "file", []byte("some content"), mimeType)

			assert.Error(t, err)
			assert.Nil(t, doc)
			assert.Contains(t, err.Error(), "unsupported mime type")
		})
	}
}

func TestParser_Parse_ContextPropagation(t *testing.T) {
	p := NewParser()

	// cancelled context should not affect pure text parsing (no I/O involved)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	content := []byte("some valid text content")
	// parseMarkdown doesn't use ctx, so this should still succeed
	doc, err := p.Parse(ctx, "test.txt", content, "text/plain")

	require.NoError(t, err)
	assert.NotNil(t, doc)
}
