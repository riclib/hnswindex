package embedder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/ollama/ollama/api"
)

// Embedder interface for generating text embeddings
type Embedder interface {
	GenerateEmbedding(text string) ([]float32, error)
	GenerateEmbeddings(texts []string) ([][]float32, error)
	Dimension() int
}

// OllamaEmbedder implements Embedder using Ollama
type OllamaEmbedder struct {
	client    *api.Client
	model     string
	dimension int
	mu        sync.RWMutex
}

// NewOllamaEmbedder creates a new Ollama embedder
func NewOllamaEmbedder(ollamaURL string, model string) (*OllamaEmbedder, error) {
	if ollamaURL == "" {
		return nil, errors.New("Ollama URL cannot be empty")
	}
	if model == "" {
		return nil, errors.New("model cannot be empty")
	}

	parsedURL, err := url.Parse(ollamaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Ollama URL: %w", err)
	}

	client := api.NewClient(parsedURL, http.DefaultClient)

	embedder := &OllamaEmbedder{
		client: client,
		model:  model,
		// Default dimensions for known models
		dimension: getDimensionForModel(model),
	}

	return embedder, nil
}

// GenerateEmbedding generates an embedding for a single text
func (o *OllamaEmbedder) GenerateEmbedding(text string) ([]float32, error) {
	ctx := context.Background()
	
	req := &api.EmbedRequest{
		Model: o.model,
		Input: text,
	}

	resp, err := o.client.Embed(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0]) == 0 {
		return nil, errors.New("no embedding returned from Ollama")
	}

	embedding := resp.Embeddings[0]

	// Update dimension if it was unknown
	o.mu.Lock()
	if o.dimension == 0 {
		o.dimension = len(embedding)
	}
	o.mu.Unlock()

	return embedding, nil
}

// GenerateEmbeddings generates embeddings for multiple texts
func (o *OllamaEmbedder) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// For now, process sequentially
	// TODO: Add concurrent processing with worker pool
	embeddings := make([][]float32, len(texts))
	
	for i, text := range texts {
		embedding, err := o.GenerateEmbedding(text)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// GenerateEmbeddingsConcurrent generates embeddings with concurrent processing
func (o *OllamaEmbedder) GenerateEmbeddingsConcurrent(texts []string, workers int) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	
	if workers <= 0 {
		workers = 8
	}

	type result struct {
		index     int
		embedding []float32
		err       error
	}

	// Create channels
	jobs := make(chan struct{ idx int; text string }, len(texts))
	results := make(chan result, len(texts))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				embedding, err := o.GenerateEmbedding(job.text)
				results <- result{
					index:     job.idx,
					embedding: embedding,
					err:       err,
				}
			}
		}()
	}

	// Send jobs
	for i, text := range texts {
		jobs <- struct{ idx int; text string }{idx: i, text: text}
	}
	close(jobs)

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	embeddings := make([][]float32, len(texts))
	for r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", r.index, r.err)
		}
		embeddings[r.index] = r.embedding
	}

	return embeddings, nil
}

// Dimension returns the embedding dimension
func (o *OllamaEmbedder) Dimension() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.dimension
}

// getDimensionForModel returns known dimensions for models
func getDimensionForModel(model string) int {
	dimensions := map[string]int{
		"nomic-embed-text":     768,
		"nomic-embed-text-v1":  768,
		"nomic-embed-text-v1.5": 768,
		"mxbai-embed-large":    1024,
		"all-minilm":          384,
	}
	
	if dim, ok := dimensions[model]; ok {
		return dim
	}
	return 0 // Unknown, will be set on first embedding
}