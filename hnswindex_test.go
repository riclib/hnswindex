package hnswindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocument(t *testing.T) {
	doc := Document{
		URI:     "test-uri",
		Title:   "Test Document",
		Content: "This is test content",
		Metadata: map[string]interface{}{
			"author": "test",
			"date":   "2024-01-01",
		},
	}

	assert.Equal(t, "test-uri", doc.URI)
	assert.Equal(t, "Test Document", doc.Title)
	assert.Equal(t, "This is test content", doc.Content)
	assert.Equal(t, "test", doc.Metadata["author"])
}

func TestConfig_Defaults(t *testing.T) {
	cfg := NewConfig()

	assert.Equal(t, "./hnswdata", cfg.DataPath)
	assert.Equal(t, "http://localhost:11434", cfg.OllamaURL)
	assert.Equal(t, "nomic-embed-text", cfg.EmbedModel)
	assert.Equal(t, 512, cfg.ChunkSize)
	assert.Equal(t, 50, cfg.ChunkOverlap)
	assert.Equal(t, 8, cfg.MaxWorkers)
	assert.True(t, cfg.AutoSave)
}

func TestConfig_FromViper(t *testing.T) {
	// This test will be expanded when we add viper integration
	t.Skip("Viper integration test - to be implemented")
}

func TestBatchResult(t *testing.T) {
	result := &BatchResult{
		TotalDocuments:     10,
		NewDocuments:       5,
		UpdatedDocuments:   3,
		UnchangedDocuments: 2,
		ProcessedChunks:    25,
		FailedURIs: map[string]string{
			"doc1": "connection error",
		},
	}

	assert.Equal(t, 10, result.TotalDocuments)
	assert.Equal(t, 5, result.NewDocuments)
	assert.Equal(t, 3, result.UpdatedDocuments)
	assert.Equal(t, 2, result.UnchangedDocuments)
	assert.Equal(t, 25, result.ProcessedChunks)
	assert.Contains(t, result.FailedURIs["doc1"], "connection error")
}

func TestSearchResult(t *testing.T) {
	sr := SearchResult{
		Document: Document{
			URI:   "doc1",
			Title: "Test Doc",
		},
		Score:     0.95,
		ChunkID:   "chunk1",
		ChunkText: "relevant text",
		IndexName: "test-index",
	}

	assert.Equal(t, "doc1", sr.Document.URI)
	assert.Equal(t, 0.95, sr.Score)
	assert.Equal(t, "chunk1", sr.ChunkID)
	assert.Equal(t, "test-index", sr.IndexName)
}

func TestIndexStats(t *testing.T) {
	stats := IndexStats{
		Name:          "test-index",
		DocumentCount: 100,
		ChunkCount:    500,
		LastUpdated:   "2024-01-01T00:00:00Z",
		SizeBytes:     1024000,
	}

	assert.Equal(t, "test-index", stats.Name)
	assert.Equal(t, 100, stats.DocumentCount)
	assert.Equal(t, 500, stats.ChunkCount)
	assert.Equal(t, int64(1024000), stats.SizeBytes)
}

func TestNewIndexManager_InvalidConfig(t *testing.T) {
	cfg := NewConfig()
	cfg.DataPath = ""

	_, err := NewIndexManager(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data path cannot be empty")
}

func TestNewIndexManager_ValidConfig(t *testing.T) {
	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)
	
	err = manager.Close()
	assert.NoError(t, err)
}