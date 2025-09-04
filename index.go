package hnswindex

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/riclib/hnswindex/internal/chunker"
	"github.com/riclib/hnswindex/internal/embedder"
	"github.com/riclib/hnswindex/internal/indexer"
	"github.com/riclib/hnswindex/internal/storage"
)

// Ensure IndexManager is properly implemented
type indexManagerImpl struct {
	config   *Config
	storage  *storage.Storage
	embedder embedder.Embedder
	chunker  *chunker.Chunker
	indexes  map[string]*indexImpl
	mu       sync.RWMutex
	wrapper  *IndexManager // Reference to wrapper for callbacks
}

// Ensure Index is properly implemented
type indexImpl struct {
	name     string
	manager  *indexManagerImpl
	hnswIndex *indexer.HNSWIndex
	mu       sync.RWMutex
}

// NewIndexManagerImpl creates the actual implementation
func NewIndexManagerImpl(config *Config) (*IndexManager, error) {
	// Create storage
	dbPath := filepath.Join(config.DataPath, "indexes.db")
	store, err := storage.NewStorage(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create embedder
	emb, err := embedder.NewOllamaEmbedder(config.OllamaURL, config.EmbedModel)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// Create chunker
	chunk, err := chunker.NewChunker(config.ChunkSize, config.ChunkOverlap)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create chunker: %w", err)
	}

	impl := &indexManagerImpl{
		config:   config,
		storage:  store,
		embedder: emb,
		chunker:  chunk,
		indexes:  make(map[string]*indexImpl),
	}

	// Create wrapper first
	manager := &IndexManager{
		config:   config,
		db:       nil,
		indexes:  make(map[string]*Index),
		embedder: emb,
		impl:     impl,
	}

	// Store wrapper reference in impl for callbacks
	impl.wrapper = manager

	// Load existing indexes
	if err := impl.loadIndexes(); err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to load indexes: %w", err)
	}

	// Wrap existing indexes
	for name := range impl.indexes {
		manager.indexes[name] = &Index{
			name:    name,
			manager: manager,
		}
	}

	return manager, nil
}

// loadIndexes loads all indexes from storage
func (im *indexManagerImpl) loadIndexes() error {
	indexNames, err := im.storage.ListIndexes()
	if err != nil {
		return err
	}

	for _, name := range indexNames {
		// Get embedding dimension from config or default
		dimension := 768 // Default for nomic-embed-text
		
		// Create HNSW index path
		indexPath := filepath.Join(im.config.DataPath, "indexes", name, "index.hnsw")
		
		// Ensure directory exists
		indexDir := filepath.Dir(indexPath)
		if err := ensureDir(indexDir); err != nil {
			return fmt.Errorf("failed to create index directory: %w", err)
		}

		// Load or create HNSW index
		hnswCfg := indexer.DefaultConfig()
		hnswIdx, err := indexer.NewHNSWIndex(indexPath, dimension, hnswCfg)
		if err != nil {
			return fmt.Errorf("failed to load HNSW index for %s: %w", name, err)
		}

		im.indexes[name] = &indexImpl{
			name:      name,
			manager:   im,
			hnswIndex: hnswIdx,
		}
	}

	return nil
}

// Extend the existing IndexManager methods to use the implementation

func (im *IndexManager) getImpl() *indexManagerImpl {
	if impl, ok := im.impl.(*indexManagerImpl); ok {
		return impl
	}
	return nil
}

// CreateIndex creates a new index
func (im *indexManagerImpl) CreateIndex(name string) (*Index, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Check if index already exists
	if _, exists := im.indexes[name]; exists {
		// Return wrapped Index
		return &Index{
			name:    name,
			manager: im.wrapperManager(),
		}, fmt.Errorf("index '%s' already exists", name)
	}

	// Create index in storage
	if err := im.storage.CreateIndex(name); err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	// Get embedding dimension
	dimension := 768 // Default for nomic-embed-text
	if im.embedder != nil {
		dimension = im.embedder.Dimension()
	}

	// Create HNSW index path
	indexPath := filepath.Join(im.config.DataPath, "indexes", name, "index.hnsw")
	
	// Ensure directory exists
	indexDir := filepath.Dir(indexPath)
	if err := ensureDir(indexDir); err != nil {
		return nil, fmt.Errorf("failed to create index directory: %w", err)
	}

	// Create HNSW index
	hnswCfg := indexer.DefaultConfig()
	hnswIdx, err := indexer.NewHNSWIndex(indexPath, dimension, hnswCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HNSW index: %w", err)
	}

	// Store implementation
	im.indexes[name] = &indexImpl{
		name:      name,
		manager:   im,
		hnswIndex: hnswIdx,
	}

	// Return wrapped Index
	return &Index{
		name:    name,
		manager: im.wrapperManager(),
	}, nil
}

// wrapperManager returns the wrapper IndexManager 
func (im *indexManagerImpl) wrapperManager() *IndexManager {
	return im.wrapper
}

// Index implementation methods

func (i *Index) getImpl() *indexImpl {
	// Get implementation from manager
	if mgr := i.manager.getImpl(); mgr != nil {
		if impl, ok := mgr.indexes[i.name]; ok {
			return impl
		}
	}
	return nil
}

// AddDocumentBatch implementation with full processing pipeline
func (i *indexImpl) AddDocumentBatch(docs []Document) (*BatchResult, error) {
	slog.Info("Starting batch document processing",
		"index", i.name,
		"document_count", len(docs),
	)

	result := &BatchResult{
		TotalDocuments: len(docs),
		FailedURIs:     make(map[string]string),
	}

	// Phase 1: Analyze what needs updating
	var toProcess []Document
	for _, doc := range docs {
		// Compute content hash
		hash := computeDocumentHash(doc)
		
		slog.Debug("Checking document",
			"uri", doc.URI,
			"title", doc.Title,
			"hash", hash[:16],
		)
		
		// Check if document has changed
		existingHash, err := i.manager.storage.GetDocumentHash(i.name, doc.URI)
		if err != nil {
			// Document doesn't exist
			slog.Debug("Document is new",
				"uri", doc.URI,
			)
			result.NewDocuments++
			toProcess = append(toProcess, doc)
		} else if existingHash != hash {
			// Document has changed
			slog.Debug("Document has changed",
				"uri", doc.URI,
				"old_hash", existingHash[:16],
				"new_hash", hash[:16],
			)
			result.UpdatedDocuments++
			toProcess = append(toProcess, doc)
		} else {
			// Document unchanged
			slog.Debug("Document unchanged",
				"uri", doc.URI,
			)
			result.UnchangedDocuments++
		}
	}

	slog.Info("Document analysis complete",
		"new", result.NewDocuments,
		"updated", result.UpdatedDocuments,
		"unchanged", result.UnchangedDocuments,
		"to_process", len(toProcess),
	)

	// Early return if nothing to process
	if len(toProcess) == 0 {
		slog.Info("No documents to process")
		return result, nil
	}

	// Phase 2: Process documents
	for _, doc := range toProcess {
		slog.Debug("Processing document",
			"uri", doc.URI,
			"content_length", len(doc.Content),
		)
		
		if err := i.processDocument(doc); err != nil {
			slog.Error("Failed to process document",
				"uri", doc.URI,
				"error", err,
			)
			result.FailedURIs[doc.URI] = err.Error()
			continue
		}
		
		// Count chunks for this document
		chunks, err := i.manager.storage.GetChunksByDocument(i.name, doc.URI)
		if err == nil {
			result.ProcessedChunks += len(chunks)
			slog.Debug("Document processed",
				"uri", doc.URI,
				"chunks", len(chunks),
			)
		}
	}

	// Phase 3: Save HNSW index if auto-save is enabled
	if i.manager.config.AutoSave {
		slog.Debug("Saving HNSW index")
		if err := i.hnswIndex.Save(); err != nil {
			slog.Error("Failed to save HNSW index",
				"error", err,
			)
			return result, fmt.Errorf("failed to save HNSW index: %w", err)
		}
		slog.Debug("HNSW index saved")
	}

	// Update index metadata
	metadata, _ := i.manager.storage.GetIndexMetadata(i.name)
	if metadata != nil {
		metadata.LastUpdated = time.Now().Format(time.RFC3339)
		metadata.DocumentCount = result.NewDocuments + result.UpdatedDocuments
		metadata.ChunkCount = result.ProcessedChunks
		i.manager.storage.SetIndexMetadata(i.name, *metadata)
	}

	slog.Info("Batch processing complete",
		"index", i.name,
		"processed_chunks", result.ProcessedChunks,
		"failed", len(result.FailedURIs),
	)

	return result, nil
}

// processDocument processes a single document
func (i *indexImpl) processDocument(doc Document) error {
	// Store document with hash
	hash := computeDocumentHash(doc)
	storageDoc := storage.Document{
		URI:      doc.URI,
		Title:    doc.Title,
		Content:  doc.Content,
		Hash:     hash,
		Metadata: doc.Metadata,
	}
	
	if err := i.manager.storage.StoreDocument(i.name, storageDoc); err != nil {
		return fmt.Errorf("failed to store document: %w", err)
	}

	// Delete existing chunks if updating
	if err := i.manager.storage.DeleteChunksByDocument(i.name, doc.URI); err != nil {
		// Ignore error if no chunks exist
	}

	// Chunk the document
	chunks, err := i.manager.chunker.ChunkDocument(doc.URI, doc.Content)
	if err != nil {
		return fmt.Errorf("failed to chunk document: %w", err)
	}

	// Process chunks with embeddings
	if err := i.processChunks(doc.URI, chunks, doc.Metadata); err != nil {
		return fmt.Errorf("failed to process chunks: %w", err)
	}

	return nil
}

// processChunks generates embeddings and stores chunks
func (i *indexImpl) processChunks(docURI string, chunks []chunker.Chunk, metadata map[string]interface{}) error {
	// Extract texts for embedding
	texts := make([]string, len(chunks))
	for idx, chunk := range chunks {
		texts[idx] = chunk.Text
	}

	// Generate embeddings (could be done concurrently)
	embeddings, err := i.manager.embedder.GenerateEmbeddings(texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Store chunks with embeddings
	for idx, chunk := range chunks {
		// Get next HNSW ID
		hnswID, err := i.manager.storage.GetNextHNSWId(i.name)
		if err != nil {
			return fmt.Errorf("failed to get HNSW ID: %w", err)
		}

		// Store chunk
		storageChunk := storage.Chunk{
			ID:          chunk.ID,
			HNSWId:      hnswID,
			DocumentURI: docURI,
			Text:        chunk.Text,
			Embedding:   embeddings[idx],
			Position:    chunk.Position,
			Metadata:    metadata,
		}

		if err := i.manager.storage.StoreChunk(i.name, storageChunk); err != nil {
			return fmt.Errorf("failed to store chunk: %w", err)
		}

		// Add to HNSW index
		if err := i.hnswIndex.Add(embeddings[idx], hnswID); err != nil {
			return fmt.Errorf("failed to add to HNSW index: %w", err)
		}
	}

	return nil
}

// Search implementation
func (i *indexImpl) Search(query string, limit int) ([]SearchResult, error) {
	// Generate query embedding
	embedding, err := i.manager.embedder.GenerateEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Search in HNSW index
	hnswResults, err := i.hnswIndex.Search(embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search HNSW index: %w", err)
	}

	// Convert results
	results := make([]SearchResult, 0, len(hnswResults))
	for _, hr := range hnswResults {
		// Find chunk by HNSW ID
		chunk, doc := i.findChunkAndDocument(hr.ID)
		if chunk == nil || doc == nil {
			continue
		}

		result := SearchResult{
			Document: Document{
				URI:      doc.URI,
				Title:    doc.Title,
				Content:  doc.Content,
				Metadata: doc.Metadata,
			},
			Score:     float64(hr.Score),
			ChunkID:   chunk.ID,
			ChunkText: chunk.Text,
			IndexName: i.name,
		}
		results = append(results, result)
	}

	return results, nil
}

// findChunkAndDocument finds chunk and document by HNSW ID
func (i *indexImpl) findChunkAndDocument(hnswID uint64) (*storage.Chunk, *storage.Document) {
	// This is inefficient - in production, we'd maintain a mapping
	// For now, scan all chunks to find the one with matching HNSW ID
	docs, err := i.manager.storage.ListDocuments(i.name)
	if err != nil {
		return nil, nil
	}

	for _, docURI := range docs {
		chunks, err := i.manager.storage.GetChunksByDocument(i.name, docURI)
		if err != nil {
			continue
		}

		for idx := range chunks {
			if chunks[idx].HNSWId == hnswID {
				doc, err := i.manager.storage.GetDocument(i.name, docURI)
				if err != nil {
					return nil, nil
				}
				return &chunks[idx], doc
			}
		}
	}

	return nil, nil
}

// GetDocument implementation
func (i *indexImpl) GetDocument(uri string) (*Document, error) {
	doc, err := i.manager.storage.GetDocument(i.name, uri)
	if err != nil {
		return nil, err
	}

	return &Document{
		URI:      doc.URI,
		Title:    doc.Title,
		Content:  doc.Content,
		Metadata: doc.Metadata,
	}, nil
}

// DeleteDocument implementation
func (i *indexImpl) DeleteDocument(uri string) error {
	// Get chunks to remove from HNSW
	chunks, err := i.manager.storage.GetChunksByDocument(i.name, uri)
	if err == nil {
		for _, chunk := range chunks {
			i.hnswIndex.Delete(chunk.HNSWId)
		}
	}

	// Delete from storage
	if err := i.manager.storage.DeleteDocument(i.name, uri); err != nil {
		return err
	}

	// Delete chunks
	if err := i.manager.storage.DeleteChunksByDocument(i.name, uri); err != nil {
		return err
	}

	// Save HNSW if auto-save
	if i.manager.config.AutoSave {
		i.hnswIndex.Save()
	}

	return nil
}

// Stats implementation
func (i *indexImpl) Stats() (IndexStats, error) {
	metadata, err := i.manager.storage.GetIndexMetadata(i.name)
	if err != nil {
		return IndexStats{Name: i.name}, err
	}

	// Get document count
	docs, _ := i.manager.storage.ListDocuments(i.name)
	
	return IndexStats{
		Name:          i.name,
		DocumentCount: len(docs),
		ChunkCount:    metadata.ChunkCount,
		LastUpdated:   metadata.LastUpdated,
		SizeBytes:     0, // Would need to calculate actual size
	}, nil
}

// Clear implementation
func (i *indexImpl) Clear() error {
	// Clear HNSW index
	if err := i.hnswIndex.Clear(); err != nil {
		return err
	}

	// Delete all documents
	docs, err := i.manager.storage.ListDocuments(i.name)
	if err != nil {
		return err
	}

	for _, uri := range docs {
		i.manager.storage.DeleteDocument(i.name, uri)
		i.manager.storage.DeleteChunksByDocument(i.name, uri)
	}

	// Reset metadata
	metadata := storage.IndexMetadata{
		NextHNSWId:    1,
		DocumentCount: 0,
		ChunkCount:    0,
		LastUpdated:   time.Now().Format(time.RFC3339),
	}
	i.manager.storage.SetIndexMetadata(i.name, metadata)

	return nil
}

// computeDocumentHash computes a hash of document content
func computeDocumentHash(doc Document) string {
	h := sha256.New()
	h.Write([]byte(doc.Title))
	h.Write([]byte(doc.Content))
	// Include relevant metadata in hash
	if doc.Metadata != nil {
		h.Write([]byte(fmt.Sprintf("%v", doc.Metadata)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ensureDir ensures a directory exists
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}