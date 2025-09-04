package indexer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/coder/hnsw"
)

// HNSWConfig contains configuration for HNSW index
type HNSWConfig struct {
	M              int    // Number of connections
	EfConstruction int    // Size of dynamic candidate list (not used in this implementation)
	Ef             int    // Size of search candidate list  
	DistanceType   string // "cosine" or "l2"
	Seed           int64  // Random seed for reproducibility
}

// DefaultConfig returns default HNSW configuration
func DefaultConfig() HNSWConfig {
	return HNSWConfig{
		M:              16,    // Good default for embeddings
		EfConstruction: 200,   // Not directly used but kept for compatibility
		Ef:             20,    // EfSearch in the library
		DistanceType:   "cosine",
		Seed:           42,
	}
}

// SearchResult represents a search result
type SearchResult struct {
	ID    uint64
	Score float32
}

// HNSWIndex wraps the HNSW graph
type HNSWIndex struct {
	graph      *hnsw.Graph[uint64]
	dimension  int
	config     HNSWConfig
	path       string
	mu         sync.RWMutex
	isModified bool
}

// NewHNSWIndex creates a new HNSW index
func NewHNSWIndex(path string, dimension int, config HNSWConfig) (*HNSWIndex, error) {
	slog.Info("Creating HNSW index",
		"path", path,
		"dimension", dimension,
		"M", config.M,
		"EfSearch", config.Ef,
		"distance_type", config.DistanceType,
	)
	
	if dimension <= 0 {
		return nil, errors.New("dimension must be positive")
	}

	// Create HNSW graph
	graph := hnsw.NewGraph[uint64]()
	
	// Configure graph parameters
	switch config.DistanceType {
	case "cosine":
		graph.Distance = hnsw.CosineDistance
	case "l2":
		graph.Distance = hnsw.EuclideanDistance
	default:
		return nil, fmt.Errorf("unsupported distance type: %s", config.DistanceType)
	}
	
	graph.M = config.M
	graph.EfSearch = config.Ef
	graph.Ml = 0.25 // Default level generation factor
	
	// Set deterministic random seed for reproducibility
	graph.Rng = rand.New(rand.NewSource(config.Seed))

	index := &HNSWIndex{
		graph:     graph,
		dimension: dimension,
		config:    config,
		path:      path,
	}

	// If path is specified and file exists, try to load it
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			slog.Debug("Attempting to load existing HNSW index",
				"path", path,
			)
			if err := index.load(); err != nil {
				// If loading fails, just start with empty index
				// (file might be from different version)
				slog.Debug("Failed to load existing index, starting fresh",
					"error", err,
				)
				index.graph = graph
			} else {
				slog.Debug("Successfully loaded existing HNSW index",
					"size", index.graph.Len(),
				)
			}
		} else {
			slog.Debug("No existing HNSW index file found",
				"path", path,
			)
		}
	}

	return index, nil
}

// LoadHNSWIndex loads an existing HNSW index from file
func LoadHNSWIndex(path string, dimension int, config HNSWConfig) (*HNSWIndex, error) {
	if path == "" {
		return nil, errors.New("path is required for loading index")
	}

	index, err := NewHNSWIndex(path, dimension, config)
	if err != nil {
		return nil, err
	}

	// Try to load, but don't fail if file doesn't exist
	if _, err := os.Stat(path); err == nil {
		if err := index.load(); err != nil {
			// Return empty index if load fails
			return index, nil
		}
	}

	return index, nil
}

// Add adds a vector to the index
func (h *HNSWIndex) Add(vector []float32, id uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(vector) != h.dimension {
		return fmt.Errorf("vector dimension %d does not match index dimension %d", 
			len(vector), h.dimension)
	}

	slog.Debug("Adding vector to HNSW index",
		"id", id,
		"dimension", len(vector),
		"current_size", h.graph.Len(),
	)

	node := hnsw.MakeNode(id, vector)
	h.graph.Add(node)
	h.isModified = true
	
	slog.Debug("Vector added successfully",
		"id", id,
		"new_size", h.graph.Len(),
	)
	
	return nil
}

// AddBatch adds multiple vectors to the index
func (h *HNSWIndex) AddBatch(vectors [][]float32, ids []uint64) error {
	if len(vectors) != len(ids) {
		return errors.New("vectors and ids must have the same length")
	}

	start := time.Now()
	slog.Info("Adding batch of vectors to HNSW index",
		"count", len(vectors),
		"current_size", h.Size(),
	)

	h.mu.Lock()
	defer h.mu.Unlock()

	nodes := make([]hnsw.Node[uint64], 0, len(vectors))
	for i, vector := range vectors {
		if len(vector) != h.dimension {
			return fmt.Errorf("vector %d dimension %d does not match index dimension %d", 
				i, len(vector), h.dimension)
		}
		nodes = append(nodes, hnsw.MakeNode(ids[i], vector))
	}
	
	h.graph.Add(nodes...)
	h.isModified = true
	
	slog.Info("Batch added to HNSW index successfully",
		"count", len(nodes),
		"new_size", h.graph.Len(),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	
	return nil
}

// Search searches for nearest neighbors
func (h *HNSWIndex) Search(query []float32, k int) ([]SearchResult, error) {
	start := time.Now()
	slog.Debug("Searching HNSW index",
		"k", k,
		"query_dimension", len(query),
		"index_size", h.Size(),
	)
	
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(query) != h.dimension {
		return nil, fmt.Errorf("query dimension %d does not match index dimension %d", 
			len(query), h.dimension)
	}

	if h.graph.Len() == 0 {
		slog.Debug("Index is empty, returning empty results")
		return []SearchResult{}, nil
	}

	// Search for k nearest neighbors
	neighbors := h.graph.Search(query, k)
	
	slog.Debug("HNSW search completed",
		"neighbors_found", len(neighbors),
		"requested_k", k,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	
	results := make([]SearchResult, len(neighbors))
	for i, n := range neighbors {
		// Calculate similarity score based on distance
		dist := h.graph.Distance(query, n.Value)
		score := float32(1.0) / (1.0 + dist)
		
		if h.config.DistanceType == "cosine" {
			// For cosine, convert distance to similarity (1 - distance)
			// Cosine distance is already normalized between 0 and 2
			score = 1.0 - (dist / 2.0)
		}
		
		results[i] = SearchResult{
			ID:    n.Key,
			Score: score,
		}
		
		slog.Debug("Search result",
			"rank", i+1,
			"id", n.Key,
			"distance", dist,
			"score", score,
		)
	}

	slog.Debug("Search results prepared",
		"count", len(results),
		"total_duration_ms", time.Since(start).Milliseconds(),
	)

	return results, nil
}

// Delete removes a vector from the index
func (h *HNSWIndex) Delete(id uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.graph.Delete(id)
	h.isModified = true
	return nil
}

// Size returns the number of vectors in the index
func (h *HNSWIndex) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.graph.Len()
}

// Clear removes all vectors from the index
func (h *HNSWIndex) Clear() error {
	slog.Info("Clearing HNSW index",
		"current_size", h.Size(),
	)
	
	h.mu.Lock()
	defer h.mu.Unlock()

	// Create a new graph with same configuration
	graph := hnsw.NewGraph[uint64]()
	
	switch h.config.DistanceType {
	case "cosine":
		graph.Distance = hnsw.CosineDistance
	case "l2":
		graph.Distance = hnsw.EuclideanDistance
	default:
		graph.Distance = hnsw.CosineDistance
	}
	
	graph.M = h.config.M
	graph.EfSearch = h.config.Ef
	graph.Ml = 0.25
	graph.Rng = rand.New(rand.NewSource(h.config.Seed))
	
	h.graph = graph
	h.isModified = true
	
	slog.Info("HNSW index cleared successfully")
	
	return nil
}

// Save saves the index to disk
func (h *HNSWIndex) Save() error {
	if h.path == "" {
		return errors.New("no path specified for saving")
	}

	start := time.Now()
	slog.Info("Saving HNSW index to disk",
		"path", h.path,
		"size", h.Size(),
		"modified", h.isModified,
	)

	h.mu.RLock()
	defer h.mu.RUnlock()

	file, err := os.Create(h.path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if err := h.graph.Export(file); err != nil {
		return fmt.Errorf("failed to export graph: %w", err)
	}

	h.isModified = false
	
	slog.Info("HNSW index saved successfully",
		"path", h.path,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	
	return nil
}

// load loads the index from disk
func (h *HNSWIndex) load() error {
	if h.path == "" {
		return errors.New("no path specified for loading")
	}
	
	start := time.Now()
	slog.Debug("Loading HNSW index from disk",
		"path", h.path,
	)

	file, err := os.Open(h.path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Wrap with bufio.Reader to provide ByteReader interface
	reader := bufio.NewReader(file)
	if err := h.graph.Import(reader); err != nil {
		return fmt.Errorf("failed to import graph: %w", err)
	}

	h.isModified = false
	
	slog.Debug("HNSW index loaded successfully",
		"path", h.path,
		"size", h.graph.Len(),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	
	return nil
}

// Close closes the index, saving if modified
func (h *HNSWIndex) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.isModified && h.path != "" {
		file, err := os.Create(h.path)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		defer file.Close()

		if err := h.graph.Export(file); err != nil {
			return fmt.Errorf("failed to export graph: %w", err)
		}
	}

	return nil
}

// IsModified returns whether the index has unsaved changes
func (h *HNSWIndex) IsModified() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.isModified
}

// Ensure interfaces are satisfied (compile-time check)
var (
	_ io.WriterTo   = (*writerAdapter)(nil)
	_ io.ReaderFrom = (*readerAdapter)(nil)
)

// writerAdapter and readerAdapter would be needed if Graph didn't implement these
type writerAdapter struct{}
type readerAdapter struct{}

func (w *writerAdapter) WriteTo(io.Writer) (int64, error) { return 0, nil }
func (r *readerAdapter) ReadFrom(io.Reader) (int64, error) { return 0, nil }