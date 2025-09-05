package hnswindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_IndexManager(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)
	defer manager.Close()

	// Test create index
	index, err := manager.CreateIndex("test-index")
	require.NoError(t, err)
	assert.NotNil(t, index)
	assert.Equal(t, "test-index", index.Name())

	// Test get index
	retrieved, err := manager.GetIndex("test-index")
	require.NoError(t, err)
	assert.Equal(t, index.Name(), retrieved.Name())

	// Test list indexes
	indexes, err := manager.ListIndexes()
	require.NoError(t, err)
	assert.Contains(t, indexes, "test-index")

	// Test delete index
	err = manager.DeleteIndex("test-index")
	assert.NoError(t, err)

	_, err = manager.GetIndex("test-index")
	assert.Error(t, err)
}

func TestIntegration_DocumentProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewConfig()
	cfg.DataPath = t.TempDir()
	cfg.ChunkSize = 100
	cfg.ChunkOverlap = 20

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	index, err := manager.CreateIndex("docs")
	require.NoError(t, err)

	// Create test documents
	docs := []Document{
		{
			URI:     "doc://test/1",
			Title:   "Test Document 1",
			Content: "This is the first test document with some content about testing.",
			Metadata: map[string]interface{}{
				"author": "test",
				"type":   "test",
			},
		},
		{
			URI:     "doc://test/2",
			Title:   "Test Document 2",
			Content: "This is the second test document with different content about validation.",
			Metadata: map[string]interface{}{
				"author": "test",
				"type":   "validation",
			},
		},
	}

	// Process documents (this would normally generate embeddings)
	result, err := index.AddDocumentBatch(context.Background(), docs, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.TotalDocuments)
	
	// Check document retrieval
	doc, err := index.GetDocument("doc://test/1")
	if err == nil {
		assert.Equal(t, "Test Document 1", doc.Title)
	}

	// Test document deletion
	err = index.DeleteDocument("doc://test/1")
	assert.NoError(t, err)
}

func TestIntegration_ViperConfig(t *testing.T) {
	v := viper.New()
	v.Set("hnsw.data_path", t.TempDir())
	v.Set("hnsw.ollama_url", "http://localhost:11434")
	v.Set("hnsw.embed_model", "nomic-embed-text")
	v.Set("hnsw.chunk_size", 256)
	v.Set("hnsw.chunk_overlap", 30)
	v.Set("hnsw.max_workers", 4)
	v.Set("hnsw.auto_save", false)

	cfg := NewConfig()
	err := cfg.LoadFromViper(v, "hnsw")
	require.NoError(t, err)

	assert.Equal(t, 256, cfg.ChunkSize)
	assert.Equal(t, 30, cfg.ChunkOverlap)
	assert.Equal(t, 4, cfg.MaxWorkers)
	assert.False(t, cfg.AutoSave)

	// Test creating manager with viper config
	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	assert.NotNil(t, manager)
	manager.Close()
}

func TestIntegration_ConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := NewConfig()
	cfg.DataPath = t.TempDir()
	cfg.MaxWorkers = 4

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	// Create multiple indexes concurrently
	var wg sync.WaitGroup
	numIndexes := 5

	for i := 0; i < numIndexes; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			indexName := fmt.Sprintf("index-%d", idx)
			_, err := manager.CreateIndex(indexName)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// Verify all indexes were created
	indexes, err := manager.ListIndexes()
	require.NoError(t, err)
	assert.Len(t, indexes, numIndexes)
}

// MockEmbedder for testing without Ollama
type MockEmbedder struct {
	dimension int
	mu        sync.Mutex
	embeddings map[string][]float32
}

func NewMockEmbedder(dimension int) *MockEmbedder {
	return &MockEmbedder{
		dimension: dimension,
		embeddings: make(map[string][]float32),
	}
}

func (m *MockEmbedder) GenerateEmbedding(text string) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate deterministic fake embedding based on text hash
	hash := sha256.Sum256([]byte(text))
	embedding := make([]float32, m.dimension)
	for i := 0; i < m.dimension; i++ {
		embedding[i] = float32(hash[i%32]) / 255.0
	}
	
	m.embeddings[text] = embedding
	return embedding, nil
}

func (m *MockEmbedder) GenerateEmbeddings(texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := m.GenerateEmbedding(text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = emb
	}
	return embeddings, nil
}

func (m *MockEmbedder) Dimension() int {
	return m.dimension
}

// TestIntegration_BatchProcessingWithMock tests batch processing without Ollama
func TestIntegration_BatchProcessingWithMock(t *testing.T) {
	cfg := NewConfig()
	cfg.DataPath = t.TempDir()
	cfg.ChunkSize = 50
	cfg.ChunkOverlap = 10

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	// Override with mock embedder for testing
	manager.embedder = NewMockEmbedder(768)

	index, err := manager.CreateIndex("test")
	require.NoError(t, err)

	// Create test documents
	docs := []Document{
		{
			URI:     "doc1",
			Title:   "First Document",
			Content: generateLongText(200), // Generate text that will be chunked
		},
		{
			URI:     "doc2", 
			Title:   "Second Document",
			Content: generateLongText(300),
		},
		{
			URI:     "doc3",
			Title:   "Third Document",
			Content: "Short document that won't be chunked.",
		},
	}

	// Process batch
	result, err := index.AddDocumentBatch(context.Background(), docs, nil)
	require.NoError(t, err)

	assert.Equal(t, 3, result.TotalDocuments)
	assert.Equal(t, 3, result.NewDocuments)
	assert.Equal(t, 0, result.UpdatedDocuments)
	assert.Equal(t, 0, result.UnchangedDocuments)
	assert.Greater(t, result.ProcessedChunks, 3) // Should have multiple chunks

	// Process same documents again - should detect no changes
	result2, err := index.AddDocumentBatch(context.Background(), docs, nil)
	require.NoError(t, err)

	assert.Equal(t, 3, result2.TotalDocuments)
	assert.Equal(t, 0, result2.NewDocuments)
	assert.Equal(t, 0, result2.UpdatedDocuments)
	assert.Equal(t, 3, result2.UnchangedDocuments)
	assert.Equal(t, 0, result2.ProcessedChunks)

	// Update one document
	docs[1].Content = "Updated content for the second document"
	result3, err := index.AddDocumentBatch(context.Background(), docs, nil)
	require.NoError(t, err)

	assert.Equal(t, 3, result3.TotalDocuments)
	assert.Equal(t, 0, result3.NewDocuments)
	assert.Equal(t, 1, result3.UpdatedDocuments)
	assert.Equal(t, 2, result3.UnchangedDocuments)
	assert.Greater(t, result3.ProcessedChunks, 0)
}

func TestIntegration_SearchWithMock(t *testing.T) {
	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	// Override with mock embedder
	manager.embedder = NewMockEmbedder(768)

	index, err := manager.CreateIndex("search-test")
	require.NoError(t, err)

	// Add documents
	docs := []Document{
		{URI: "doc1", Title: "Go Programming", Content: "Go is a statically typed language"},
		{URI: "doc2", Title: "Python Tutorial", Content: "Python is dynamically typed"},
		{URI: "doc3", Title: "JavaScript Guide", Content: "JavaScript is also dynamically typed"},
	}

	_, err = index.AddDocumentBatch(context.Background(), docs, nil)
	require.NoError(t, err)

	// Search (with mock embedder, results won't be semantic)
	results, err := index.Search("programming languages", 2)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 2)

	// Verify result structure
	if len(results) > 0 {
		assert.NotEmpty(t, results[0].Document.URI)
		assert.NotEmpty(t, results[0].Document.Title)
		assert.Greater(t, results[0].Score, float64(0))
	}
}

// Helper to generate long text for chunking tests
func generateLongText(words int) string {
	text := ""
	lorem := []string{"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", 
		"adipiscing", "elit", "sed", "do", "eiusmod", "tempor", "incididunt", 
		"ut", "labore", "et", "dolore", "magna", "aliqua"}
	
	for i := 0; i < words; i++ {
		text += lorem[i%len(lorem)] + " "
	}
	return text
}

// TestIntegration_ReadMarkdownFiles tests reading actual markdown files
func TestIntegration_ReadMarkdownFiles(t *testing.T) {
	cfg := NewConfig()
	cfg.DataPath = t.TempDir()

	manager, err := NewIndexManager(cfg)
	require.NoError(t, err)
	defer manager.Close()

	// Override with mock embedder
	manager.embedder = NewMockEmbedder(768)

	index, err := manager.CreateIndex("markdown-test")
	require.NoError(t, err)

	// Read test markdown files
	testDataDir := "./testdata"
	files, err := os.ReadDir(testDataDir)
	require.NoError(t, err)

	var docs []Document
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".md" {
			path := filepath.Join(testDataDir, file.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			
			docs = append(docs, Document{
				URI:     fmt.Sprintf("file://%s", path),
				Title:   file.Name(),
				Content: string(content),
				Metadata: map[string]interface{}{
					"filename": file.Name(),
					"path":     path,
				},
			})
		}
	}

	require.NotEmpty(t, docs, "Should have found markdown files in testdata")

	// Process the documents
	result, err := index.AddDocumentBatch(context.Background(), docs, nil)
	require.NoError(t, err)
	
	assert.Equal(t, len(docs), result.TotalDocuments)
	assert.Equal(t, len(docs), result.NewDocuments)
	assert.Greater(t, result.ProcessedChunks, len(docs)) // Should have multiple chunks per doc

	// Test search
	results, err := index.Search("architecture", 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, results)
}

// computeHash computes SHA-256 hash of content
func computeHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}