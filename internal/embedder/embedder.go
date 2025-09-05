package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Embedder interface for generating text embeddings
type Embedder interface {
	GenerateEmbedding(text string) ([]float32, error)
	GenerateEmbeddings(texts []string) ([][]float32, error)
	Dimension() int
}

// embedRequest represents the request to Ollama's embed API
type embedRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"`
}

// embedResponse represents the response from Ollama's embed API
type embedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

// OllamaEmbedder implements Embedder using Ollama HTTP API
type OllamaEmbedder struct {
	baseURL   string
	client    *http.Client
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

	// Validate URL
	_, err := url.Parse(ollamaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Ollama URL: %w", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	embedder := &OllamaEmbedder{
		baseURL: ollamaURL,
		client:  client,
		model:   model,
		// Default dimensions for known models
		dimension: getDimensionForModel(model),
	}

	return embedder, nil
}

// GenerateEmbedding generates an embedding for a single text
func (o *OllamaEmbedder) GenerateEmbedding(text string) ([]float32, error) {
	start := time.Now()
	textPreview := text
	if len(textPreview) > 100 {
		textPreview = textPreview[:100] + "..."
	}
	
	slog.Debug("Generating embedding",
		"model", o.model,
		"text_length", len(text),
		"text_preview", textPreview,
	)
	
	// Create request
	req := embedRequest{
		Model: o.model,
		Input: text,
	}
	
	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(context.Background(), 
		"POST", o.baseURL+"/api/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	
	// Send request
	httpResp, err := o.client.Do(httpReq)
	if err != nil {
		slog.Error("Failed to send embedding request",
			"error", err,
			"model", o.model,
		)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer httpResp.Body.Close()
	
	// Check status code
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		slog.Error("Embedding request failed",
			"status", httpResp.StatusCode,
			"body", string(body),
			"model", o.model,
		)
		return nil, fmt.Errorf("embedding request failed with status %d: %s", 
			httpResp.StatusCode, string(body))
	}
	
	// Decode response
	var resp embedResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0]) == 0 {
		slog.Error("No embedding returned from Ollama",
			"model", o.model,
		)
		return nil, errors.New("no embedding returned from Ollama")
	}

	embedding := resp.Embeddings[0]

	// Update dimension if it was unknown
	o.mu.Lock()
	if o.dimension == 0 {
		o.dimension = len(embedding)
		slog.Info("Embedder dimension detected",
			"dimension", o.dimension,
			"model", o.model,
		)
	}
	o.mu.Unlock()

	slog.Debug("Embedding generated successfully",
		"dimension", len(embedding),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return embedding, nil
}

// GenerateEmbeddings generates embeddings for multiple texts
func (o *OllamaEmbedder) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	start := time.Now()
	slog.Info("Starting batch embedding generation",
		"count", len(texts),
		"model", o.model,
	)

	// For now, process sequentially
	// TODO: Add concurrent processing with worker pool
	embeddings := make([][]float32, len(texts))
	
	for i, text := range texts {
		embedding, err := o.GenerateEmbedding(text)
		if err != nil {
			slog.Error("Failed to generate embedding in batch",
				"index", i,
				"error", err,
			)
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}
		embeddings[i] = embedding
	}

	slog.Info("Batch embedding generation completed",
		"count", len(texts),
		"duration_ms", time.Since(start).Milliseconds(),
	)

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