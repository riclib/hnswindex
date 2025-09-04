package chunker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// Chunk represents a text chunk with metadata
type Chunk struct {
	ID          string                 `json:"id"`
	DocumentURI string                 `json:"document_uri,omitempty"`
	Text        string                 `json:"text"`
	Position    int                    `json:"position"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Chunker handles text chunking with tiktoken
type Chunker struct {
	chunkSize   int
	overlapSize int
	encoder     *tiktoken.Tiktoken
}

// NewChunker creates a new chunker with specified chunk and overlap sizes
func NewChunker(chunkSize, overlapSize int) (*Chunker, error) {
	if chunkSize < 50 {
		return nil, errors.New("chunk size must be at least 50 tokens")
	}
	if overlapSize >= chunkSize {
		return nil, errors.New("overlap cannot be larger than or equal to chunk size")
	}
	if overlapSize < 0 {
		overlapSize = 0
	}

	// Use cl100k_base for GPT-4
	encoder, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("failed to get tiktoken encoder: %w", err)
	}

	return &Chunker{
		chunkSize:   chunkSize,
		overlapSize: overlapSize,
		encoder:     encoder,
	}, nil
}

// Chunk splits text into chunks with overlap
func (c *Chunker) Chunk(text string) ([]Chunk, error) {
	if text == "" {
		slog.Debug("Empty text provided to chunker")
		return []Chunk{}, nil
	}

	slog.Debug("Starting text chunking",
		"text_length", len(text),
		"chunk_size", c.chunkSize,
		"overlap_size", c.overlapSize,
	)

	// Encode the entire text
	tokens := c.encoder.Encode(text, nil, nil)
	tokenCount := len(tokens)
	
	slog.Debug("Text tokenized",
		"token_count", tokenCount,
	)
	
	if tokenCount <= c.chunkSize {
		// Text fits in a single chunk
		slog.Debug("Text fits in single chunk")
		return []Chunk{
			{
				ID:       generateChunkID(text, 0),
				Text:     text,
				Position: 0,
			},
		}, nil
	}

	chunks := []Chunk{}
	position := 0
	stride := c.chunkSize - c.overlapSize

	slog.Debug("Chunking with stride",
		"stride", stride,
		"expected_chunks", (tokenCount-c.overlapSize)/stride+1,
	)

	for i := 0; i < len(tokens); i += stride {
		end := i + c.chunkSize
		if end > len(tokens) {
			end = len(tokens)
		}

		// Decode the chunk tokens back to text
		chunkTokens := tokens[i:end]
		chunkText := c.encoder.Decode(chunkTokens)

		chunk := Chunk{
			ID:       generateChunkID(chunkText, position),
			Text:     chunkText,
			Position: position,
		}
		chunks = append(chunks, chunk)
		
		slog.Debug("Created chunk",
			"position", position,
			"token_start", i,
			"token_end", end,
			"chunk_length", len(chunkText),
			"chunk_id", chunk.ID[:8],
		)
		
		position++

		// If we've reached the end, break
		if end == len(tokens) {
			break
		}
	}

	slog.Info("Text chunked successfully",
		"input_tokens", tokenCount,
		"chunks_created", len(chunks),
		"text_length", len(text),
	)

	return chunks, nil
}

// ChunkWithMetadata chunks text and adds metadata to each chunk
func (c *Chunker) ChunkWithMetadata(text string, metadata map[string]interface{}) ([]Chunk, error) {
	chunks, err := c.Chunk(text)
	if err != nil {
		return nil, err
	}

	for i := range chunks {
		chunks[i].Metadata = metadata
	}

	return chunks, nil
}

// ChunkDocument chunks a document and adds the document URI to each chunk
func (c *Chunker) ChunkDocument(documentURI string, text string) ([]Chunk, error) {
	chunks, err := c.Chunk(text)
	if err != nil {
		return nil, err
	}

	for i := range chunks {
		chunks[i].DocumentURI = documentURI
		// Include document URI in chunk ID for uniqueness
		chunks[i].ID = fmt.Sprintf("%s_%s", documentURI, chunks[i].ID)
	}

	return chunks, nil
}

// CountTokens counts the number of tokens in a text
func (c *Chunker) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	tokens := c.encoder.Encode(text, nil, nil)
	return len(tokens)
}

// SplitIntoSentences splits text into sentences (simple implementation)
func SplitIntoSentences(text string) []string {
	// Simple sentence splitting on common delimiters
	// This is a basic implementation - could be improved with NLP libraries
	sentences := []string{}
	
	// Replace common abbreviations to avoid false splits
	text = strings.ReplaceAll(text, "Mr.", "Mr")
	text = strings.ReplaceAll(text, "Mrs.", "Mrs")
	text = strings.ReplaceAll(text, "Dr.", "Dr")
	text = strings.ReplaceAll(text, "Ms.", "Ms")
	text = strings.ReplaceAll(text, "Prof.", "Prof")
	
	// Split on sentence endings
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == '\n'
	})
	
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			sentences = append(sentences, trimmed)
		}
	}
	
	return sentences
}

// generateChunkID generates a unique ID for a chunk
func generateChunkID(text string, position int) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s_%d", text, position)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}