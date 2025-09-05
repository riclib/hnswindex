# hnswindex

A Go package for semantic document indexing and retrieval using local embeddings via Ollama and vector similarity search with HNSW (Hierarchical Navigable Small World) graphs.

## Features

- 🔍 **Semantic Search**: Vector similarity search using HNSW algorithm for fast and accurate results
- 🎯 **Multiple Indexes**: Support for managing multiple independent document indexes
- 🚀 **Efficient Batch Processing**: Smart change detection to process only new or modified documents
- 🔧 **Local Embeddings**: Generate embeddings locally using Ollama (no external API dependencies)
- 💾 **Persistent Storage**: Built on bbolt for reliable, embedded database storage
- 🌐 **Web Interface**: Clean web UI for search and index management (demo application)
- 🔗 **Confluence Integration**: Index documents directly from Confluence spaces (demo application)

## Installation

```bash
go get github.com/riclib/hnswindex
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
    manager, err := hnswindex.NewIndexManager(hnswindex.Config{
        DBPath:     "./indexes.db",
        OllamaURL:  "http://localhost:11434",
        EmbedModel: "nomic-embed-text",
    })
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

    result, err := index.AddDocumentBatch(context.Background(), docs, nil)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Indexed %d new documents\n", result.NewDocuments)

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

# Index local files
./demo files --index "docs" --dir ./documents

# Search
./demo search --index "docs" --query "your search query"

# Run web interface
./demo server --port 8080
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
type Config struct {
    DBPath       string // Path to bbolt database file
    IndexDir     string // Directory for HNSW files
    OllamaURL    string // Ollama service URL (default: "http://localhost:11434")
    EmbedModel   string // Embedding model name (default: "nomic-embed-text")
    ChunkSize    int    // Document chunk size in tokens (default: 512)
    ChunkOverlap int    // Overlap between chunks (default: 50)
}
```

## API Reference

### IndexManager

- `NewIndexManager(config Config) (*IndexManager, error)`
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