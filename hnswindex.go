package hnswindex

import (
	"errors"
	"fmt"
	"path/filepath"
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
	config  *Config
	db      *bbolt.DB
	indexes map[string]*Index
	mu      sync.RWMutex
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

	// Create data directory if it doesn't exist
	dbPath := filepath.Join(config.DataPath, "indexes.db")
	db, err := bbolt.Open(dbPath, 0644, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize global buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("_indexes"))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte("_config"))
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	manager := &IndexManager{
		config:  config,
		db:      db,
		indexes: make(map[string]*Index),
	}

	// Load existing indexes
	err = manager.loadIndexes()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load indexes: %w", err)
	}

	return manager, nil
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
	im.mu.Lock()
	defer im.mu.Unlock()

	// Check if index already exists
	if _, exists := im.indexes[name]; exists {
		return nil, fmt.Errorf("index '%s' already exists", name)
	}

	// Create index in database
	err := im.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("_indexes"))
		return bucket.Put([]byte(name), []byte("active"))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	// Create index instance
	index := &Index{
		name:    name,
		manager: im,
	}
	im.indexes[name] = index

	return index, nil
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

// AddDocumentBatch adds multiple documents to the index
func (i *Index) AddDocumentBatch(docs []Document) (*BatchResult, error) {
	// TODO: Implement batch document processing
	return &BatchResult{
		TotalDocuments: len(docs),
		FailedURIs:     make(map[string]string),
	}, nil
}

// Search performs a semantic search on the index
func (i *Index) Search(query string, limit int) ([]SearchResult, error) {
	// TODO: Implement search functionality
	return []SearchResult{}, nil
}

// GetDocument retrieves a document by URI
func (i *Index) GetDocument(uri string) (*Document, error) {
	// TODO: Implement document retrieval
	return nil, fmt.Errorf("not implemented")
}

// DeleteDocument deletes a document from the index
func (i *Index) DeleteDocument(uri string) error {
	// TODO: Implement document deletion
	return fmt.Errorf("not implemented")
}

// Stats returns statistics for the index
func (i *Index) Stats() (IndexStats, error) {
	// TODO: Implement statistics
	return IndexStats{
		Name: i.name,
	}, nil
}

// Clear removes all documents from the index
func (i *Index) Clear() error {
	// TODO: Implement index clearing
	return fmt.Errorf("not implemented")
}