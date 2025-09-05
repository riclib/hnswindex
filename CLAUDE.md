# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go package for semantic document indexing and retrieval using:
- **HNSW (Hierarchical Navigable Small World)** graphs for vector similarity search
- **Ollama** for local embeddings generation (nomic-embed-text model)
- **BBolt** for persistent storage
- **Tiktoken-go** for GPT-4 compatible tokenization (cl100k_base)
- **Viper** for configuration management with namespace support
- **slog** for structured logging and debugging

The package supports multiple independent indexes with efficient batch processing, change detection, and concurrent processing with configurable worker pools.

## Development Commands

### Building
```bash
go build ./...
go build -o demo ./cmd/demo
```

### Testing
```bash
go test ./...
go test -v ./...                    # Verbose output
go test ./internal/chunker -v       # Test specific package
go test -run TestSpecificFunc ./... # Run specific test
go test -bench=. ./...              # Run benchmarks
go test -race ./...                 # Race detection
go test -cover ./...                # Coverage report
```

### Linting & Formatting
```bash
go fmt ./...
gofmt -w .
go vet ./...
golangci-lint run  # If installed
```

### Dependencies
```bash
go mod download
go mod tidy
go mod verify
```

### Running the Demo Application
```bash
# Build the demo CLI
cd cmd/demo
go build -o demo

# Index markdown files
./demo index --dir ./docs --index mydocs

# Search with different log levels
./demo search "query" --index mydocs --limit 5
./demo --debug search "query" --index mydocs      # Debug logging
./demo --log-level warn search "query"            # Warning level

# View statistics
./demo stats --index mydocs
./demo list
```

## Architecture

### Core Library Structure
- **hnswindex.go**: Main package interface with Config, IndexManager, Index types
- **index.go**: Full integration implementation connecting all components
- **internal/chunker**: Tiktoken-based document chunking with overlap (cl100k_base encoding)
- **internal/embedder**: Ollama client for generating embeddings with batch support
- **internal/storage**: BBolt database operations for documents, chunks, and metadata
- **internal/indexer**: HNSW graph management with cosine similarity search

### Storage Design
- **bbolt database**: Stores documents, chunks, embeddings, and metadata
- **HNSW files**: Separate `.hnsw` files for each index's vector graph
- **Change detection**: Content hashing to skip unchanged documents

### Key Implementation Patterns
1. **Batch Processing**: Process multiple documents efficiently with change detection
2. **Lazy Loading**: HNSW graphs loaded only on first search
3. **Atomic Operations**: All-or-nothing updates for data consistency
4. **Error Recovery**: Graceful handling of partial failures

## Working with the Codebase

### Adding New Features
- Follow existing patterns in internal packages
- Maintain separation between core library and demo application
- Use structured error types with context
- Add appropriate unit tests in `*_test.go` files

### Testing Approach
- Unit tests for each internal component
- Integration tests require running Ollama instance
- Mock external services (Confluence API)
- Use testdata/ directory for test documents

### Performance Considerations
- Batch embedding requests to Ollama when possible
- Use connection pooling for HTTP clients
- Implement efficient change detection with content hashing
- Consider memory usage for large document sets

## External Dependencies

### Ollama
- Must be running locally (default: http://localhost:11434)
- Requires embedding model installed (default: nomic-embed-text)
- Install: `ollama pull nomic-embed-text`

### Demo Web UI
- Uses templ for type-safe templates
- DaisyUI for component styling
- Minimal Tailwind CSS
- data-star for dynamic interactions

## Recent Updates

### Progress Updates Feature (Latest - UPDATED API)
- `AddDocumentBatch` now accepts context for cancellation and optional progress channel
- Developer provides the progress channel (can be nil to skip)
- Context support enables timeout and cancellation of long operations
- Real-time updates during document checking, processing, and saving
- Non-blocking updates that check for context cancellation
- Demo CLI shows progress with terminal control sequences

Example usage:
```go
// Option 1: With progress tracking
progress := make(chan hnswindex.ProgressUpdate, 100)
go func() {
    for update := range progress {
        fmt.Printf("[%d/%d] %s: %s\n", 
            update.Current, update.Total, update.Stage, update.Message)
    }
}()
result, err := index.AddDocumentBatch(ctx, docs, progress)
close(progress)

// Option 2: Without progress (simpler)
result, err := index.AddDocumentBatch(ctx, docs, nil)

// Option 3: With cancellation
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()
result, err := index.AddDocumentBatch(ctx, docs, nil)
```

## Common Tasks

### Debug Issues
```bash
# Enable comprehensive debug logging
./demo --debug index --dir ./docs --index test

# Check specific components
./demo --debug search "test" | grep -i embedding  # Embedding issues
./demo --debug search "test" | grep -i chunk      # Chunking issues
./demo --debug search "test" | grep -i hnsw       # HNSW issues
```

### Fix HNSW Loading Issues
The HNSW index requires `io.ByteReader` for importing. Fix implemented:
```go
// Wrap file with bufio.Reader
reader := bufio.NewReader(file)
if err := h.graph.Import(reader); err != nil {
    return fmt.Errorf("failed to import graph: %w", err)
}
```

### Configuration with Viper
```go
// Use with namespace
v := viper.New()
v.SetDefault("hnsw.chunk_size", 512)
v.SetDefault("hnsw.max_workers", 8)
config := hnswindex.NewConfigFromViper(v, "hnsw")
```

### Monitor Performance
Key metrics to watch in logs:
- `duration_ms` fields for operation timing
- `chunks_created` for document processing
- `dimension` for embedding validation
- `index_size` for HNSW graph growth

## Recent Changes (2025-09-04)

### Added Comprehensive Logging
- All components now use `log/slog` for structured logging
- Debug level shows detailed operation tracking
- Info level shows high-level operations
- CLI supports `--debug` and `--log-level` flags

### Fixed HNSW Loading Bug
- Resolved "ByteReader" error when loading saved indexes
- Wrapped file operations with `bufio.Reader`
- Indexes now persist and load correctly between sessions

### Configuration Updates
- Added Viper support with namespace configuration
- Environment variable support with HNSW_ prefix
- Configurable worker pools for concurrent processing

### Testing Improvements
- Added 51 passing tests covering all components
- TDD approach maintained throughout development
- Race condition testing enabled