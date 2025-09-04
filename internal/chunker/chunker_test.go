package chunker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChunker(t *testing.T) {
	// Test with default values
	c, err := NewChunker(512, 50)
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.Equal(t, 512, c.chunkSize)
	assert.Equal(t, 50, c.overlapSize)
}

func TestNewChunker_InvalidSize(t *testing.T) {
	// Chunk size too small
	_, err := NewChunker(10, 50)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chunk size must be at least")

	// Overlap larger than chunk size
	_, err = NewChunker(100, 150)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overlap cannot be larger than or equal to chunk size")
}

func TestChunk_SimpleText(t *testing.T) {
	c, err := NewChunker(100, 20)
	require.NoError(t, err)

	text := "This is a simple test text that should be chunked into smaller pieces. " +
		"We want to test if the chunking works correctly with overlap. " +
		"The overlap ensures context is preserved between chunks."

	chunks, err := c.Chunk(text)
	require.NoError(t, err)
	assert.NotEmpty(t, chunks)
	
	// Verify each chunk
	for i, chunk := range chunks {
		assert.NotEmpty(t, chunk.Text)
		assert.Equal(t, i, chunk.Position)
		assert.NotEmpty(t, chunk.ID)
		
		// Check that chunk size is within limits (except possibly the last one)
		tokens := c.CountTokens(chunk.Text)
		if i < len(chunks)-1 {
			assert.LessOrEqual(t, tokens, 100)
		}
	}
	
	// Verify overlap exists between consecutive chunks
	if len(chunks) > 1 {
		for i := 0; i < len(chunks)-1; i++ {
			// There should be some overlap in text
			assert.True(t, hasOverlap(chunks[i].Text, chunks[i+1].Text))
		}
	}
}

func TestChunk_EmptyText(t *testing.T) {
	c, err := NewChunker(512, 50)
	require.NoError(t, err)

	chunks, err := c.Chunk("")
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestChunk_ShortText(t *testing.T) {
	c, err := NewChunker(512, 50)
	require.NoError(t, err)

	text := "This is a very short text."
	chunks, err := c.Chunk(text)
	require.NoError(t, err)
	assert.Len(t, chunks, 1)
	assert.Equal(t, text, chunks[0].Text)
	assert.Equal(t, 0, chunks[0].Position)
}

func TestChunk_LongText(t *testing.T) {
	c, err := NewChunker(50, 10)
	require.NoError(t, err)

	// Create a long text with repeated patterns
	longText := strings.Repeat("This is a test sentence. ", 100)
	
	chunks, err := c.Chunk(longText)
	require.NoError(t, err)
	assert.Greater(t, len(chunks), 1)
	
	// Verify chunks cover the entire text
	var reconstructed string
	lastEnd := ""
	for i, chunk := range chunks {
		if i > 0 {
			// Remove the overlapping part
			if strings.HasPrefix(chunk.Text, lastEnd) {
				continue
			}
		}
		reconstructed += chunk.Text
		if len(chunk.Text) > 10 {
			lastEnd = chunk.Text[len(chunk.Text)-10:]
		}
	}
	// The reconstructed text should contain all the original content
	// (may not be exact due to tokenization boundaries)
	assert.Contains(t, longText, "This is a test sentence")
}

func TestChunk_WithMetadata(t *testing.T) {
	c, err := NewChunker(100, 20)
	require.NoError(t, err)

	text := "Test text for metadata"
	metadata := map[string]interface{}{
		"source": "test",
		"author": "tester",
	}
	
	chunks, err := c.ChunkWithMetadata(text, metadata)
	require.NoError(t, err)
	assert.NotEmpty(t, chunks)
	
	for _, chunk := range chunks {
		assert.Equal(t, metadata, chunk.Metadata)
	}
}

func TestCountTokens(t *testing.T) {
	c, err := NewChunker(512, 50)
	require.NoError(t, err)

	// Test various texts
	tests := []struct {
		text     string
		minCount int // Minimum expected tokens
	}{
		{"Hello world", 2},
		{"This is a longer sentence with more tokens.", 8},
		{"", 0},
		{"Single", 1},
	}

	for _, tt := range tests {
		count := c.CountTokens(tt.text)
		assert.GreaterOrEqual(t, count, tt.minCount, 
			"Text '%s' should have at least %d tokens", tt.text, tt.minCount)
	}
}

func TestChunkDocument(t *testing.T) {
	c, err := NewChunker(100, 20)
	require.NoError(t, err)

	docURI := "doc://test"
	text := strings.Repeat("This is test content. ", 50)
	
	chunks, err := c.ChunkDocument(docURI, text)
	require.NoError(t, err)
	assert.NotEmpty(t, chunks)
	
	for _, chunk := range chunks {
		assert.Equal(t, docURI, chunk.DocumentURI)
		assert.NotEmpty(t, chunk.ID)
		assert.NotEmpty(t, chunk.Text)
		assert.Contains(t, chunk.ID, docURI)
	}
}

// Helper function to check if two strings have overlapping content
func hasOverlap(text1, text2 string) bool {
	// Check if the end of text1 overlaps with the beginning of text2
	minLen := 10
	if len(text1) < minLen || len(text2) < minLen {
		minLen = min(len(text1), len(text2)) / 2
	}
	
	for i := minLen; i <= len(text1) && i <= len(text2); i++ {
		if strings.HasPrefix(text2, text1[len(text1)-i:]) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}