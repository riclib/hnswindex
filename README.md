# hnswindex

A Go package for semantic document indexing and retrieval using local embeddings via Ollama and vector similarity search with HNSW (Hierarchical Navigable Small World) graphs.

## Features

- 🔍 **Semantic Search**: Vector similarity search using HNSW algorithm for fast and accurate results
- 🎯 **Multiple Indexes**: Support for managing multiple independent document indexes
- 🚀 **Efficient Batch Processing**: Smart change detection to process only new or modified documents
- 🔧 **Local Embeddings**: Generate embeddings locally using Ollama (no external API dependencies)
- 💾 **Persistent Storage**: Built on bbolt for reliable, embedded database storage
- ⚡ **Progress Tracking**: Real-time progress updates during indexing operations
- 🛑 **Cancellation Support**: Context-based cancellation and timeout support
- 🔗 **Confluence Integration**: Index documents directly from Confluence spaces (demo application)

## Installation

```bash
go get github.com/riclib/hnswindex@v0.1.0
```

### Prerequisites

- Go 1.22 or higher
- [Ollama](https://ollama.ai/) installed and running locally
- An embedding model installed in Ollama (e.g., `ollama pull nomic-embed-text`)

## Quick Start

### As a Library

```go
package main

import (
    "context"
    "fmt"
    "github.com/riclib/hnswindex"
)

func main() {
    // Initialize the index manager
    config := hnswindex.NewConfig()
    config.DataPath = "./hnswdata"
    config.OllamaURL = "http://localhost:11434"
    config.EmbedModel = "nomic-embed-text"
    
    manager, err := hnswindex.NewIndexManager(config)
    if err != nil {
        panic(err)
    }
    defer manager.Close()

    // Create an index
    index, err := manager.CreateIndex("documents")
    if err != nil {
        panic(err)
    }

    // Add documents
    docs := []hnswindex.Document{
        {
            URI:     "doc1",
            Title:   "Introduction to Go",
            Content: "Go is a statically typed, compiled programming language...",
        },
        {
            URI:     "doc2",
            Title:   "Concurrency in Go",
            Content: "Go provides built-in support for concurrent programming...",
        },
    }

    // Option 1: Without progress tracking
    result, err := index.AddDocumentBatch(context.Background(), docs, nil)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Indexed %d new documents\n", result.NewDocuments)
    
    // Option 2: With progress tracking
    progress := make(chan hnswindex.ProgressUpdate, 100)
    go func() {
        for update := range progress {
            fmt.Printf("[%d/%d] %s\n", update.Current, update.Total, update.Message)
        }
    }()
    result, err = index.AddDocumentBatch(context.Background(), docs, progress)
    close(progress)

    // Search
    results, err := index.Search("concurrent programming", 5)
    if err != nil {
        panic(err)
    }

    for _, result := range results {
        fmt.Printf("Score: %.3f - %s\n", result.Score, result.Document.Title)
    }
}
```

### Demo CLI Application

The repository includes a demo CLI application showcasing the library's capabilities:

```bash
# Build the demo
go build -o demo ./cmd/demo

# Index local markdown files
./demo index --dir ./documents --index myindex

# Index Confluence space
./demo confluence --space SPACENAME --url https://company.atlassian.net --index confluence

# Search
./demo search --index myindex "your search query"

# Show index statistics
./demo stats --index myindex

# List all indexes
./demo list
```

## Architecture

### Core Components

- **IndexManager**: Manages multiple indexes and database connections
- **Index**: Individual document index with HNSW graph for vector search
- **Document Chunking**: Splits large documents into smaller, overlapping chunks
- **Embeddings**: Generated locally using Ollama
- **Storage**: bbolt for metadata and HNSW files for vector graphs

### Change Detection

The system uses content hashing to detect document changes, ensuring that only new or modified documents are processed during batch operations, significantly improving performance for incremental updates.

## Configuration

```go
config := hnswindex.NewConfig()  // Creates config with sensible defaults
config.DataPath = "./hnswdata"   // Directory for storage
config.OllamaURL = "http://localhost:11434"
config.EmbedModel = "nomic-embed-text"
config.ChunkSize = 512           // Token size for chunks
config.ChunkOverlap = 50         // Overlap between chunks
config.MaxWorkers = 8            // Concurrent processing workers
config.AutoSave = true           // Auto-save after batch operations
```

## Context Support and Cancellation

The library supports context-based cancellation and timeouts:

```go
// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()
result, err := index.AddDocumentBatch(ctx, docs, nil)

// With manual cancellation
ctx, cancel := context.WithCancel(context.Background())
// Cancel from another goroutine or on user action
go func() {
    if userClickedCancel {
        cancel()
    }
}()
result, err := index.AddDocumentBatch(ctx, docs, nil)
```

## Progress Tracking

Monitor indexing progress in real-time:

```go
progress := make(chan hnswindex.ProgressUpdate, 100)
go func() {
    for update := range progress {
        // update.Stage: "checking", "processing", "saving", "complete"
        // update.Current: current item number
        // update.Total: total items
        // update.Message: human-readable message
        // update.URI: current document URI (optional)
        
        fmt.Printf("[%d/%d] %s: %s\n", 
            update.Current, update.Total, update.Stage, update.Message)
    }
}()

result, err := index.AddDocumentBatch(ctx, docs, progress)
close(progress)
```

## API Reference

### IndexManager

- `NewIndexManager(config *Config) (*IndexManager, error)`
- `GetIndex(name string) (*Index, error)`
- `CreateIndex(name string) (*Index, error)`
- `DeleteIndex(name string) error`
- `ListIndexes() ([]string, error)`
- `Close() error`

### Index

- `AddDocumentBatch(ctx context.Context, docs []Document, progress chan<- ProgressUpdate) (*BatchResult, error)`
- `Search(query string, limit int) ([]SearchResult, error)`
- `GetDocument(uri string) (*Document, error)`
- `DeleteDocument(uri string) error`
- `Stats() (IndexStats, error)`
- `Clear() error`

## Development

### Building

```bash
go build ./...
```

### Testing

```bash
go test ./...
```

### Linting

```bash
go fmt ./...
go vet ./...
```

## Performance Considerations

- **Lazy Loading**: HNSW graphs are loaded only when needed
- **Batch Processing**: Efficient handling of multiple documents
- **Connection Pooling**: Reuses connections to Ollama
- **Memory Management**: Configurable chunk sizes for large document sets

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- [HNSW implementation](https://github.com/coder/hnsw) for vector similarity search
- [bbolt](https://github.com/etcd-io/bbolt) for embedded database
- [Ollama](https://ollama.ai/) for local LLM embeddings
- [templ](https://github.com/a-h/templ) for type-safe HTML templates