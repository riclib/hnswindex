package hnswindex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_URIChangeDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	index, err := manager.CreateIndex("test-uri-changes")
	require.NoError(t, err)

	// Create initial document
	doc1 := Document{
		URI:     "http://example.com/doc1",
		Title:   "Test Document",
		Content: "This is test content that remains the same",
		Metadata: map[string]interface{}{
			"author": "test",
		},
	}

	// First indexing
	result1, err := index.AddDocumentBatch(context.Background(), []Document{doc1}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result1.NewDocuments)
	assert.Equal(t, 0, result1.UpdatedDocuments)
	assert.Equal(t, 0, result1.UnchangedDocuments)

	// Re-index with same URI and content - should be unchanged
	result2, err := index.AddDocumentBatch(context.Background(), []Document{doc1}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.NewDocuments)
	assert.Equal(t, 0, result2.UpdatedDocuments)
	assert.Equal(t, 1, result2.UnchangedDocuments)

	// Change URI but keep same content
	doc2 := doc1
	doc2.URI = "https://example.com/new/location/doc1" // Different URI

	// Re-index with different URI - should be detected as changed
	result3, err := index.AddDocumentBatch(context.Background(), []Document{doc2}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result3.NewDocuments, "Document with new URI should be treated as new")
	assert.Equal(t, 0, result3.UpdatedDocuments)
	assert.Equal(t, 0, result3.UnchangedDocuments)

	// Search should return the new URI
	results, err := index.Search("test content", 10)
	require.NoError(t, err)
	if len(results) > 0 {
		// Check that at least one result has the new URI
		hasNewURI := false
		for _, r := range results {
			if r.Document.URI == doc2.URI {
				hasNewURI = true
				break
			}
		}
		assert.True(t, hasNewURI, "Search results should contain document with new URI")
	}
}

func TestIntegration_ForceUpdateOption(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	index, err := manager.CreateIndex("test-force-update")
	require.NoError(t, err)

	// Create test document
	doc := Document{
		URI:     "http://example.com/doc",
		Title:   "Test Document",
		Content: "Test content for force update",
		Metadata: map[string]interface{}{
			"version": "1.0",
		},
	}

	// Initial indexing
	result1, err := index.AddDocumentBatch(context.Background(), []Document{doc}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result1.NewDocuments)

	// Re-index without force - should be unchanged
	result2, err := index.AddDocumentBatch(context.Background(), []Document{doc}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.NewDocuments)
	assert.Equal(t, 0, result2.UpdatedDocuments)
	assert.Equal(t, 1, result2.UnchangedDocuments)

	// Re-index with force flag - should be processed
	options := AddOptions{
		ForceUpdate: true,
	}
	result3, err := index.AddDocumentBatchWithOptions(context.Background(), []Document{doc}, nil, options)
	require.NoError(t, err)
	assert.Equal(t, 0, result3.NewDocuments)
	assert.Equal(t, 1, result3.UpdatedDocuments, "Force update should mark document as updated")
	assert.Equal(t, 0, result3.UnchangedDocuments)
}

func TestIntegration_ClearWithHashes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	index, err := manager.CreateIndex("test-clear-hashes")
	require.NoError(t, err)

	// Create and index documents
	docs := []Document{
		{
			URI:     "http://example.com/doc1",
			Title:   "Document 1",
			Content: "Content for document 1",
		},
		{
			URI:     "http://example.com/doc2",
			Title:   "Document 2",
			Content: "Content for document 2",
		},
	}

	// Initial indexing
	result1, err := index.AddDocumentBatch(context.Background(), docs, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result1.NewDocuments)

	// Clear the index
	err = index.Clear()
	require.NoError(t, err)

	// Re-index the same documents
	result2, err := index.AddDocumentBatch(context.Background(), docs, nil)
	require.NoError(t, err)
	// After clearing (which now clears hashes), documents should be treated as new
	assert.Equal(t, 2, result2.NewDocuments, "After Clear(), all documents should be treated as new")
	assert.Equal(t, 0, result2.UpdatedDocuments)
	assert.Equal(t, 0, result2.UnchangedDocuments)
}

func TestIntegration_ComplexURIScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	index, err := manager.CreateIndex("test-complex-uri")
	require.NoError(t, err)

	// Simulate the Confluence URI fix scenario
	oldDocs := []Document{
		{
			URI:     "confluence://SPACE_KEY/655361",
			Title:   "Confluence Page 1",
			Content: "This is a confluence page content",
		},
		{
			URI:     "confluence://SPACE_KEY/655362",
			Title:   "Confluence Page 2",
			Content: "Another confluence page content",
		},
	}

	// Index with old URIs
	result1, err := index.AddDocumentBatch(context.Background(), oldDocs, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result1.NewDocuments)

	// Clear the index (simulating a rebuild)
	err = index.Clear()
	require.NoError(t, err)

	// Create documents with fixed URIs but same content
	newDocs := []Document{
		{
			URI:     "https://confluence.example.com/wiki/spaces/SPACE_KEY/pages/655361",
			Title:   "Confluence Page 1",
			Content: "This is a confluence page content",
		},
		{
			URI:     "https://confluence.example.com/wiki/spaces/SPACE_KEY/pages/655362",
			Title:   "Confluence Page 2",
			Content: "Another confluence page content",
		},
	}

	// Re-index with proper URIs
	result2, err := index.AddDocumentBatch(context.Background(), newDocs, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result2.NewDocuments, "Documents with new URIs after Clear should be indexed")

	// Verify the new URIs are stored
	doc1, err := index.GetDocument("https://confluence.example.com/wiki/spaces/SPACE_KEY/pages/655361")
	require.NoError(t, err)
	assert.Equal(t, "https://confluence.example.com/wiki/spaces/SPACE_KEY/pages/655361", doc1.URI)

	// Old URIs should not exist
	_, err = index.GetDocument("confluence://SPACE_KEY/655361")
	assert.Error(t, err, "Old URI should not exist after rebuild")
}