# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go package for semantic document indexing and retrieval using:
- **HNSW (Hierarchical Navigable Small World)** graphs for vector similarity search
- **Ollama** for local embeddings generation
- **bbolt** for persistent storage
- **templ** for type-safe HTML templates (demo app)

The package supports multiple independent indexes with efficient batch processing and change detection.

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
# Set environment variables for Confluence integration
export CONF_TOKEN=your_confluence_token
export CONF_URL=https://company.atlassian.net

# Run the demo
go run ./cmd/demo [command] [flags]
```

## Architecture

### Core Library Structure
- **hnswindex.go**: Main package interface with IndexManager and Index types
- **internal/chunker**: Document chunking with configurable size/overlap
- **internal/embedder**: Ollama client for generating embeddings
- **internal/storage**: bbolt database operations for persistent storage
- **internal/indexer**: HNSW graph management for vector search

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

## Common Tasks

### Create a New Index Type
1. Define the index in IndexManager
2. Add bucket schema in storage/bbolt.go
3. Implement HNSW graph initialization in indexer/hnsw.go
4. Add corresponding CLI command in cmd/demo/commands/

### Add Document Source
1. Create client in cmd/demo/[source]/
2. Implement document fetching and conversion
3. Add command in cmd/demo/commands/
4. Convert to hnswindex.Document format

### Modify Chunking Strategy
1. Update internal/chunker/chunker.go
2. Adjust Config defaults in hnswindex.go
3. Update tests for new chunking behavior