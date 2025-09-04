package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStorage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	
	store, err := NewStorage(dbPath)
	require.NoError(t, err)
	require.NotNil(t, store)
	defer store.Close()
	
	// Verify database file was created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestStorage_InvalidPath(t *testing.T) {
	// Try to create storage in an invalid path
	_, err := NewStorage("/invalid/path/that/does/not/exist/test.db")
	assert.Error(t, err)
}

func TestStorage_CreateIndex(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	// Create an index
	err = store.CreateIndex("test-index")
	assert.NoError(t, err)
	
	// Try to create the same index again
	err = store.CreateIndex("test-index")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	
	// Verify index exists
	exists, err := store.IndexExists("test-index")
	assert.NoError(t, err)
	assert.True(t, exists)
	
	// Verify non-existent index
	exists, err = store.IndexExists("non-existent")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestStorage_DeleteIndex(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	// Create and delete an index
	err = store.CreateIndex("temp-index")
	require.NoError(t, err)
	
	err = store.DeleteIndex("temp-index")
	assert.NoError(t, err)
	
	// Verify index is deleted
	exists, err := store.IndexExists("temp-index")
	assert.NoError(t, err)
	assert.False(t, exists)
	
	// Try to delete non-existent index
	err = store.DeleteIndex("non-existent")
	assert.Error(t, err)
}

func TestStorage_ListIndexes(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	// Initially no indexes
	indexes, err := store.ListIndexes()
	assert.NoError(t, err)
	assert.Empty(t, indexes)
	
	// Create multiple indexes
	store.CreateIndex("index1")
	store.CreateIndex("index2")
	store.CreateIndex("index3")
	
	indexes, err = store.ListIndexes()
	assert.NoError(t, err)
	assert.Len(t, indexes, 3)
	assert.Contains(t, indexes, "index1")
	assert.Contains(t, indexes, "index2")
	assert.Contains(t, indexes, "index3")
}

func TestStorage_StoreDocument(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	doc := Document{
		URI:     "doc://test/1",
		Title:   "Test Document",
		Content: "This is test content",
		Hash:    "hash123",
		Metadata: map[string]interface{}{
			"author": "tester",
		},
	}
	
	// Store document
	err = store.StoreDocument("test-index", doc)
	assert.NoError(t, err)
	
	// Retrieve document
	retrieved, err := store.GetDocument("test-index", "doc://test/1")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, doc.URI, retrieved.URI)
	assert.Equal(t, doc.Title, retrieved.Title)
	assert.Equal(t, doc.Content, retrieved.Content)
	assert.Equal(t, doc.Hash, retrieved.Hash)
	
	// Try to get non-existent document
	_, err = store.GetDocument("test-index", "non-existent")
	assert.Error(t, err)
}

func TestStorage_DeleteDocument(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	doc := Document{
		URI:     "doc://test/1",
		Title:   "Test Document",
		Content: "This is test content",
		Hash:    "hash123",
	}
	
	// Store and delete document
	err = store.StoreDocument("test-index", doc)
	require.NoError(t, err)
	
	err = store.DeleteDocument("test-index", "doc://test/1")
	assert.NoError(t, err)
	
	// Verify document is deleted
	_, err = store.GetDocument("test-index", "doc://test/1")
	assert.Error(t, err)
}

func TestStorage_StoreChunk(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	chunk := Chunk{
		ID:          "chunk123",
		HNSWId:      42,
		DocumentURI: "doc://test/1",
		Text:        "Chunk text",
		Embedding:   []float32{0.1, 0.2, 0.3},
		Position:    0,
	}
	
	// Store chunk
	err = store.StoreChunk("test-index", chunk)
	assert.NoError(t, err)
	
	// Retrieve chunk
	retrieved, err := store.GetChunk("test-index", "chunk123")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, chunk.ID, retrieved.ID)
	assert.Equal(t, chunk.HNSWId, retrieved.HNSWId)
	assert.Equal(t, chunk.Text, retrieved.Text)
	assert.Equal(t, chunk.Embedding, retrieved.Embedding)
}

func TestStorage_GetChunksByDocument(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	// Store multiple chunks for a document
	for i := 0; i < 3; i++ {
		chunk := Chunk{
			ID:          fmt.Sprintf("chunk%d", i),
			HNSWId:      uint64(i),
			DocumentURI: "doc://test/1",
			Text:        fmt.Sprintf("Chunk %d text", i),
			Position:    i,
		}
		err = store.StoreChunk("test-index", chunk)
		require.NoError(t, err)
	}
	
	// Store chunk for different document
	otherChunk := Chunk{
		ID:          "other-chunk",
		HNSWId:      99,
		DocumentURI: "doc://test/2",
		Text:        "Other document chunk",
	}
	store.StoreChunk("test-index", otherChunk)
	
	// Get chunks for first document
	chunks, err := store.GetChunksByDocument("test-index", "doc://test/1")
	assert.NoError(t, err)
	assert.Len(t, chunks, 3)
	
	// Verify chunks are in order
	for i, chunk := range chunks {
		assert.Equal(t, i, chunk.Position)
	}
}

func TestStorage_DeleteChunksByDocument(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	// Store chunks
	for i := 0; i < 3; i++ {
		chunk := Chunk{
			ID:          fmt.Sprintf("chunk%d", i),
			DocumentURI: "doc://test/1",
			Text:        fmt.Sprintf("Chunk %d", i),
		}
		store.StoreChunk("test-index", chunk)
	}
	
	// Delete all chunks for document
	err = store.DeleteChunksByDocument("test-index", "doc://test/1")
	assert.NoError(t, err)
	
	// Verify chunks are deleted
	chunks, err := store.GetChunksByDocument("test-index", "doc://test/1")
	assert.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestStorage_GetDocumentHash(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	// Store document with hash
	doc := Document{
		URI:  "doc://test/1",
		Hash: "hash123",
	}
	store.StoreDocument("test-index", doc)
	
	// Get hash
	hash, err := store.GetDocumentHash("test-index", "doc://test/1")
	assert.NoError(t, err)
	assert.Equal(t, "hash123", hash)
	
	// Non-existent document
	_, err = store.GetDocumentHash("test-index", "non-existent")
	assert.Error(t, err)
}

func TestStorage_GetIndexMetadata(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	// Set metadata
	metadata := IndexMetadata{
		NextHNSWId:    100,
		DocumentCount: 10,
		ChunkCount:    50,
		LastUpdated:   "2024-01-01T00:00:00Z",
	}
	err = store.SetIndexMetadata("test-index", metadata)
	assert.NoError(t, err)
	
	// Get metadata
	retrieved, err := store.GetIndexMetadata("test-index")
	assert.NoError(t, err)
	assert.Equal(t, metadata.NextHNSWId, retrieved.NextHNSWId)
	assert.Equal(t, metadata.DocumentCount, retrieved.DocumentCount)
	assert.Equal(t, metadata.ChunkCount, retrieved.ChunkCount)
}

func TestStorage_GetNextHNSWId(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	// Get next IDs
	id1, err := store.GetNextHNSWId("test-index")
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), id1)
	
	id2, err := store.GetNextHNSWId("test-index")
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), id2)
	
	id3, err := store.GetNextHNSWId("test-index")
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), id3)
}

func TestStorage_ListDocuments(t *testing.T) {
	store, err := NewStorage(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer store.Close()
	
	err = store.CreateIndex("test-index")
	require.NoError(t, err)
	
	// Store multiple documents
	for i := 0; i < 3; i++ {
		doc := Document{
			URI:   fmt.Sprintf("doc://test/%d", i),
			Title: fmt.Sprintf("Document %d", i),
		}
		store.StoreDocument("test-index", doc)
	}
	
	// List documents
	docs, err := store.ListDocuments("test-index")
	assert.NoError(t, err)
	assert.Len(t, docs, 3)
}