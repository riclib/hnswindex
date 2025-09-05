package hnswindex

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/viper"
	"go.etcd.io/bbolt"
)

// Config holds the configuration for the index manager
type Config struct {
	DataPath     string `mapstructure:"data_path"`
	OllamaURL    string `mapstructure:"ollama_url"`
	EmbedModel   string `mapstructure:"embed_model"`
	ChunkSize    int    `mapstructure:"chunk_size"`
	ChunkOverlap int    `mapstructure:"chunk_overlap"`
	MaxWorkers   int    `mapstructure:"max_workers"`
	AutoSave     bool   `mapstructure:"auto_save"`
}

// NewConfig returns a new configuration with default values
func NewConfig() *Config {
	return &Config{
		DataPath:     "./hnswdata",
		OllamaURL:    "http://localhost:11434",
		EmbedModel:   "nomic-embed-text",
		ChunkSize:    512,
		ChunkOverlap: 50,
		MaxWorkers:   8,
		AutoSave:     true,
	}
}

// LoadFromViper loads configuration from a viper instance with namespace
func (c *Config) LoadFromViper(v *viper.Viper, namespace string) error {
	if namespace != "" {
		sub := v.Sub(namespace)
		if sub != nil {
			return sub.Unmarshal(c)
		}
	}
	return v.Unmarshal(c)
}

// Document represents a document to be indexed
type Document struct {
	URI      string                 `json:"uri"`
	Title    string                 `json:"title"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SearchResult represents a search result
type SearchResult struct {
	Document  Document `json:"document"`
	Score     float64  `json:"score"`
	ChunkID   string   `json:"chunk_id"`
	ChunkText string   `json:"chunk_text"`
	IndexName string   `json:"index_name"`
}

// BatchResult represents the result of batch document processing
type BatchResult struct {
	TotalDocuments     int               `json:"total_documents"`
	NewDocuments       int               `json:"new_documents"`
	UpdatedDocuments   int               `json:"updated_documents"`
	UnchangedDocuments int               `json:"unchanged_documents"`
	ProcessedChunks    int               `json:"processed_chunks"`
	FailedURIs         map[string]string `json:"failed_uris,omitempty"`
}

// ProgressUpdate represents a progress update during batch processing
type ProgressUpdate struct {
	Stage   string  `json:"stage"`   // "checking", "processing", "embedding", "saving"
	Current int     `json:"current"` // Current item number
	Total   int     `json:"total"`   // Total items
	Message string  `json:"message"` // Human-readable message
	URI     string  `json:"uri,omitempty"` // Optional: current document URI
}

// IndexStats represents statistics for an index
type IndexStats struct {
	Name          string `json:"name"`
	DocumentCount int    `json:"document_count"`
	ChunkCount    int    `json:"chunk_count"`
	LastUpdated   string `json:"last_updated"`
	SizeBytes     int64  `json:"size_bytes"`
}

// IndexManager manages multiple indexes
type IndexManager struct {
	config   *Config
	db       *bbolt.DB
	indexes  map[string]*Index
	mu       sync.RWMutex
	embedder interface{} // Embedder interface
	impl     interface{} // Implementation reference
}

// Index represents a single document index
type Index struct {
	name    string
	manager *IndexManager
	mu      sync.RWMutex
}

// NewIndexManager creates a new index manager
func NewIndexManager(config *Config) (*IndexManager, error) {
	if config.DataPath == "" {
		return nil, errors.New("data path cannot be empty")
	}

	// Use the full implementation
	return NewIndexManagerImpl(config)
}

// GetIndex retrieves an existing index
func (im *IndexManager) GetIndex(name string) (*Index, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	index, exists := im.indexes[name]
	if !exists {
		return nil, fmt.Errorf("index '%s' not found", name)
	}
	return index, nil
}

// CreateIndex creates a new index
func (im *IndexManager) CreateIndex(name string) (*Index, error) {
	if impl := im.getImpl(); impl != nil {
		return impl.CreateIndex(name)
	}
	
	return nil, fmt.Errorf("implementation not available")
}

// DeleteIndex deletes an index
func (im *IndexManager) DeleteIndex(name string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if _, exists := im.indexes[name]; !exists {
		return fmt.Errorf("index '%s' not found", name)
	}

	// Delete from database
	err := im.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("_indexes"))
		return bucket.Delete([]byte(name))
	})
	if err != nil {
		return fmt.Errorf("failed to delete index: %w", err)
	}

	// Remove from memory
	delete(im.indexes, name)
	return nil
}

// ListIndexes returns a list of all index names
func (im *IndexManager) ListIndexes() ([]string, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	var names []string
	for name := range im.indexes {
		names = append(names, name)
	}
	return names, nil
}

// Close closes the index manager and all resources
func (im *IndexManager) Close() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.db != nil {
		return im.db.Close()
	}
	return nil
}

// loadIndexes loads existing indexes from the database
func (im *IndexManager) loadIndexes() error {
	return im.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("_indexes"))
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(k, v []byte) error {
			name := string(k)
			im.indexes[name] = &Index{
				name:    name,
				manager: im,
			}
			return nil
		})
	})
}

// Name returns the index name
func (i *Index) Name() string {
	return i.name
}

// AddDocumentBatch adds multiple documents to the index with context support
// The progress channel can be nil if progress updates are not needed.
// The context can be used to cancel long-running operations.
func (i *Index) AddDocumentBatch(ctx context.Context, docs []Document, progress chan<- ProgressUpdate) (*BatchResult, error) {
	if impl := i.getImpl(); impl != nil {
		return impl.AddDocumentBatch(ctx, docs, progress)
	}
	return &BatchResult{
		TotalDocuments: len(docs),
		FailedURIs:     make(map[string]string),
	}, fmt.Errorf("implementation not available")
}

// Search performs a semantic search on the index
func (i *Index) Search(query string, limit int) ([]SearchResult, error) {
	if impl := i.getImpl(); impl != nil {
		return impl.Search(query, limit)
	}
	return []SearchResult{}, fmt.Errorf("implementation not available")
}

// GetDocument retrieves a document by URI
func (i *Index) GetDocument(uri string) (*Document, error) {
	if impl := i.getImpl(); impl != nil {
		return impl.GetDocument(uri)
	}
	return nil, fmt.Errorf("implementation not available")
}

// DeleteDocument deletes a document from the index
func (i *Index) DeleteDocument(uri string) error {
	if impl := i.getImpl(); impl != nil {
		return impl.DeleteDocument(uri)
	}
	return fmt.Errorf("implementation not available")
}

// Stats returns statistics for the index
func (i *Index) Stats() (IndexStats, error) {
	if impl := i.getImpl(); impl != nil {
		return impl.Stats()
	}
	return IndexStats{
		Name: i.name,
	}, fmt.Errorf("implementation not available")
}

// Clear removes all documents from the index
func (i *Index) Clear() error {
	if impl := i.getImpl(); impl != nil {
		return impl.Clear()
	}
	return fmt.Errorf("implementation not available")
}