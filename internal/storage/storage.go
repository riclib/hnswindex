package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"go.etcd.io/bbolt"
)

// Document represents a stored document
type Document struct {
	URI      string                 `json:"uri"`
	Title    string                 `json:"title"`
	Content  string                 `json:"content"`
	Hash     string                 `json:"hash"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Chunk represents a stored chunk with embedding
type Chunk struct {
	ID          string                 `json:"id"`
	HNSWId      uint64                 `json:"hnsw_id"`
	DocumentURI string                 `json:"document_uri"`
	Text        string                 `json:"text"`
	Embedding   []float32              `json:"embedding"`
	Position    int                    `json:"position"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// IndexMetadata stores metadata about an index
type IndexMetadata struct {
	NextHNSWId    uint64 `json:"next_hnsw_id"`
	DocumentCount int    `json:"document_count"`
	ChunkCount    int    `json:"chunk_count"`
	LastUpdated   string `json:"last_updated"`
}

// Storage manages bbolt database operations
type Storage struct {
	db *bbolt.DB
	mu sync.RWMutex
}

// NewStorage creates a new storage instance
func NewStorage(dbPath string) (*Storage, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := ensureDir(dir); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

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

	return &Storage{db: db}, nil
}

// Close closes the database
func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// CreateIndex creates a new index with its buckets
func (s *Storage) CreateIndex(name string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		// Check if index already exists
		indexBucket := tx.Bucket([]byte("_indexes"))
		if indexBucket.Get([]byte(name)) != nil {
			return fmt.Errorf("index '%s' already exists", name)
		}

		// Create index entry
		if err := indexBucket.Put([]byte(name), []byte("active")); err != nil {
			return err
		}

		// Create index-specific buckets
		bucketNames := []string{
			fmt.Sprintf("%s_documents", name),
			fmt.Sprintf("%s_chunks", name),
			fmt.Sprintf("%s_doc_chunks", name),
			fmt.Sprintf("%s_hashes", name),
			fmt.Sprintf("%s_metadata", name),
		}

		for _, bucketName := range bucketNames {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
			}
		}

		// Initialize metadata
		metadataBucket := tx.Bucket([]byte(fmt.Sprintf("%s_metadata", name)))
		metadata := IndexMetadata{
			NextHNSWId:    1,
			DocumentCount: 0,
			ChunkCount:    0,
		}
		data, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		return metadataBucket.Put([]byte("metadata"), data)
	})
}

// DeleteIndex deletes an index and all its data
func (s *Storage) DeleteIndex(name string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		// Check if index exists
		indexBucket := tx.Bucket([]byte("_indexes"))
		if indexBucket.Get([]byte(name)) == nil {
			return fmt.Errorf("index '%s' not found", name)
		}

		// Delete index entry
		if err := indexBucket.Delete([]byte(name)); err != nil {
			return err
		}

		// Delete index-specific buckets
		bucketNames := []string{
			fmt.Sprintf("%s_documents", name),
			fmt.Sprintf("%s_chunks", name),
			fmt.Sprintf("%s_doc_chunks", name),
			fmt.Sprintf("%s_hashes", name),
			fmt.Sprintf("%s_metadata", name),
		}

		for _, bucketName := range bucketNames {
			if err := tx.DeleteBucket([]byte(bucketName)); err != nil && err != bbolt.ErrBucketNotFound {
				return fmt.Errorf("failed to delete bucket %s: %w", bucketName, err)
			}
		}

		return nil
	})
}

// IndexExists checks if an index exists
func (s *Storage) IndexExists(name string) (bool, error) {
	var exists bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		indexBucket := tx.Bucket([]byte("_indexes"))
		if indexBucket != nil && indexBucket.Get([]byte(name)) != nil {
			exists = true
		}
		return nil
	})
	return exists, err
}

// ListIndexes returns all index names
func (s *Storage) ListIndexes() ([]string, error) {
	var indexes []string
	err := s.db.View(func(tx *bbolt.Tx) error {
		indexBucket := tx.Bucket([]byte("_indexes"))
		if indexBucket == nil {
			return nil
		}
		return indexBucket.ForEach(func(k, v []byte) error {
			indexes = append(indexes, string(k))
			return nil
		})
	})
	return indexes, err
}

// StoreDocument stores a document in the index
func (s *Storage) StoreDocument(indexName string, doc Document) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		// Store document
		docBucket := tx.Bucket([]byte(fmt.Sprintf("%s_documents", indexName)))
		if docBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}

		if err := docBucket.Put([]byte(doc.URI), data); err != nil {
			return err
		}

		// Store hash if present
		if doc.Hash != "" {
			hashBucket := tx.Bucket([]byte(fmt.Sprintf("%s_hashes", indexName)))
			if err := hashBucket.Put([]byte(doc.URI), []byte(doc.Hash)); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetDocument retrieves a document from the index
func (s *Storage) GetDocument(indexName, uri string) (*Document, error) {
	var doc *Document
	err := s.db.View(func(tx *bbolt.Tx) error {
		docBucket := tx.Bucket([]byte(fmt.Sprintf("%s_documents", indexName)))
		if docBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		data := docBucket.Get([]byte(uri))
		if data == nil {
			return fmt.Errorf("document '%s' not found", uri)
		}

		var d Document
		if err := json.Unmarshal(data, &d); err != nil {
			return err
		}
		doc = &d
		return nil
	})
	return doc, err
}

// DeleteDocument deletes a document from the index
func (s *Storage) DeleteDocument(indexName, uri string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		// Delete from documents bucket
		docBucket := tx.Bucket([]byte(fmt.Sprintf("%s_documents", indexName)))
		if docBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}
		if err := docBucket.Delete([]byte(uri)); err != nil {
			return err
		}

		// Delete from hashes bucket
		hashBucket := tx.Bucket([]byte(fmt.Sprintf("%s_hashes", indexName)))
		if hashBucket != nil {
			hashBucket.Delete([]byte(uri))
		}

		// Delete document-chunk mappings
		docChunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_doc_chunks", indexName)))
		if docChunkBucket != nil {
			docChunkBucket.Delete([]byte(uri))
		}

		return nil
	})
}

// StoreChunk stores a chunk in the index
func (s *Storage) StoreChunk(indexName string, chunk Chunk) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		// Store chunk
		chunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_chunks", indexName)))
		if chunkBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}

		if err := chunkBucket.Put([]byte(chunk.ID), data); err != nil {
			return err
		}

		// Update document-chunk mapping
		if chunk.DocumentURI != "" {
			docChunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_doc_chunks", indexName)))
			
			// Get existing chunk IDs for this document
			var chunkIDs []string
			existing := docChunkBucket.Get([]byte(chunk.DocumentURI))
			if existing != nil {
				json.Unmarshal(existing, &chunkIDs)
			}

			// Add new chunk ID if not already present
			found := false
			for _, id := range chunkIDs {
				if id == chunk.ID {
					found = true
					break
				}
			}
			if !found {
				chunkIDs = append(chunkIDs, chunk.ID)
			}

			// Store updated mapping
			data, err := json.Marshal(chunkIDs)
			if err != nil {
				return err
			}
			if err := docChunkBucket.Put([]byte(chunk.DocumentURI), data); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetChunk retrieves a chunk from the index
func (s *Storage) GetChunk(indexName, chunkID string) (*Chunk, error) {
	var chunk *Chunk
	err := s.db.View(func(tx *bbolt.Tx) error {
		chunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_chunks", indexName)))
		if chunkBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		data := chunkBucket.Get([]byte(chunkID))
		if data == nil {
			return fmt.Errorf("chunk '%s' not found", chunkID)
		}

		var c Chunk
		if err := json.Unmarshal(data, &c); err != nil {
			return err
		}
		chunk = &c
		return nil
	})
	return chunk, err
}

// GetChunksByDocument retrieves all chunks for a document
func (s *Storage) GetChunksByDocument(indexName, documentURI string) ([]Chunk, error) {
	var chunks []Chunk
	err := s.db.View(func(tx *bbolt.Tx) error {
		// Get chunk IDs for document
		docChunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_doc_chunks", indexName)))
		if docChunkBucket == nil {
			return nil
		}

		chunkIDsData := docChunkBucket.Get([]byte(documentURI))
		if chunkIDsData == nil {
			return nil
		}

		var chunkIDs []string
		if err := json.Unmarshal(chunkIDsData, &chunkIDs); err != nil {
			return err
		}

		// Get chunks
		chunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_chunks", indexName)))
		if chunkBucket == nil {
			return nil
		}

		for _, id := range chunkIDs {
			data := chunkBucket.Get([]byte(id))
			if data != nil {
				var chunk Chunk
				if err := json.Unmarshal(data, &chunk); err != nil {
					continue
				}
				chunks = append(chunks, chunk)
			}
		}

		return nil
	})

	// Sort chunks by position
	for i := 0; i < len(chunks)-1; i++ {
		for j := i + 1; j < len(chunks); j++ {
			if chunks[j].Position < chunks[i].Position {
				chunks[i], chunks[j] = chunks[j], chunks[i]
			}
		}
	}

	return chunks, err
}

// DeleteChunksByDocument deletes all chunks for a document
func (s *Storage) DeleteChunksByDocument(indexName, documentURI string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		// Get chunk IDs for document
		docChunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_doc_chunks", indexName)))
		if docChunkBucket == nil {
			return nil
		}

		chunkIDsData := docChunkBucket.Get([]byte(documentURI))
		if chunkIDsData == nil {
			return nil
		}

		var chunkIDs []string
		if err := json.Unmarshal(chunkIDsData, &chunkIDs); err != nil {
			return err
		}

		// Delete chunks
		chunkBucket := tx.Bucket([]byte(fmt.Sprintf("%s_chunks", indexName)))
		if chunkBucket != nil {
			for _, id := range chunkIDs {
				chunkBucket.Delete([]byte(id))
			}
		}

		// Delete document-chunk mapping
		docChunkBucket.Delete([]byte(documentURI))

		return nil
	})
}

// GetDocumentHash retrieves the hash for a document
func (s *Storage) GetDocumentHash(indexName, uri string) (string, error) {
	var hash string
	err := s.db.View(func(tx *bbolt.Tx) error {
		hashBucket := tx.Bucket([]byte(fmt.Sprintf("%s_hashes", indexName)))
		if hashBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		data := hashBucket.Get([]byte(uri))
		if data == nil {
			return fmt.Errorf("hash for document '%s' not found", uri)
		}

		hash = string(data)
		return nil
	})
	return hash, err
}

// ClearHashes removes all document hashes for an index
func (s *Storage) ClearHashes(indexName string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		hashBucket := tx.Bucket([]byte(fmt.Sprintf("%s_hashes", indexName)))
		if hashBucket == nil {
			// Bucket doesn't exist, nothing to clear
			return nil
		}
		
		// Delete all keys in the hash bucket
		c := hashBucket.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if err := hashBucket.Delete(k); err != nil {
				return err
			}
		}
		
		return nil
	})
}

// GetIndexMetadata retrieves metadata for an index
func (s *Storage) GetIndexMetadata(indexName string) (*IndexMetadata, error) {
	var metadata *IndexMetadata
	err := s.db.View(func(tx *bbolt.Tx) error {
		metadataBucket := tx.Bucket([]byte(fmt.Sprintf("%s_metadata", indexName)))
		if metadataBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		data := metadataBucket.Get([]byte("metadata"))
		if data == nil {
			return errors.New("metadata not found")
		}

		var m IndexMetadata
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		metadata = &m
		return nil
	})
	return metadata, err
}

// SetIndexMetadata updates metadata for an index
func (s *Storage) SetIndexMetadata(indexName string, metadata IndexMetadata) error {
	slog.Debug("Updating index metadata",
		"index", indexName,
		"document_count", metadata.DocumentCount,
		"chunk_count", metadata.ChunkCount,
		"next_hnsw_id", metadata.NextHNSWId,
		"last_updated", metadata.LastUpdated,
	)
	
	return s.db.Update(func(tx *bbolt.Tx) error {
		metadataBucket := tx.Bucket([]byte(fmt.Sprintf("%s_metadata", indexName)))
		if metadataBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		data, err := json.Marshal(metadata)
		if err != nil {
			return err
		}

		return metadataBucket.Put([]byte("metadata"), data)
	})
}

// GetNextHNSWId gets the next available HNSW ID for an index
func (s *Storage) GetNextHNSWId(indexName string) (uint64, error) {
	var nextID uint64
	err := s.db.Update(func(tx *bbolt.Tx) error {
		metadataBucket := tx.Bucket([]byte(fmt.Sprintf("%s_metadata", indexName)))
		if metadataBucket == nil {
			return fmt.Errorf("index '%s' not found", indexName)
		}

		// Get current metadata
		data := metadataBucket.Get([]byte("metadata"))
		if data == nil {
			return errors.New("metadata not found")
		}

		var metadata IndexMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			return err
		}

		// Get next ID
		nextID = metadata.NextHNSWId
		metadata.NextHNSWId++

		// Save updated metadata
		data, err := json.Marshal(metadata)
		if err != nil {
			return err
		}

		return metadataBucket.Put([]byte("metadata"), data)
	})
	return nextID, err
}

// ListDocuments returns all document URIs in an index
func (s *Storage) ListDocuments(indexName string) ([]string, error) {
	var uris []string
	err := s.db.View(func(tx *bbolt.Tx) error {
		docBucket := tx.Bucket([]byte(fmt.Sprintf("%s_documents", indexName)))
		if docBucket == nil {
			return nil
		}

		return docBucket.ForEach(func(k, v []byte) error {
			uris = append(uris, string(k))
			return nil
		})
	})
	return uris, err
}

// ensureDir ensures a directory exists
func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}