# Package Specification: github.com/riclib/hnswindex

## Overview

A self-contained Go package for semantic document indexing and retrieval using local embeddings via Ollama and vector similarity search with HNSW. The package supports multiple independent indexes and provides efficient batch processing with change detection.

## Project Structure

```
github.com/riclib/hnswindex/
├── go.mod
├── go.sum
├── README.md
├── hnswindex.go              # Main package interface
├── internal/
│   ├── chunker/
│   │   └── chunker.go        # Document chunking logic
│   ├── embedder/
│   │   └── ollama.go         # Ollama client for embeddings
│   ├── storage/
│   │   └── bbolt.go          # bbolt database operations
│   └── indexer/
│       └── hnsw.go           # HNSW index management
├── cmd/
│   └── demo/
│       ├── main.go           # CLI entry point
│       ├── confluence/
│       │   ├── client.go     # Confluence API client
│       │   ├── extractor.go  # HTML to plain text
│       │   └── auth.go       # Authentication
│       ├── commands/
│       │   ├── confluence.go # Confluence indexing
│       │   ├── files.go      # File indexing
│       │   ├── search.go     # Search commands
│       │   └── server.go     # Web server
│       ├── templates/
│       │   ├── layout.templ  # Base layout
│       │   ├── index.templ   # Search interface
│       │   ├── results.templ # Search results
│       │   ├── manage.templ  # Index management
│       │   └── stats.templ   # Statistics
│       └── static/
│           └── style.css     # DaisyUI + minimal Tailwind
└── testdata/
    └── sample_docs/          # Test documents
```

## Dependencies

```go
module github.com/riclib/hnswindex

go 1.22

require (
    go.etcd.io/bbolt v1.3.8
    github.com/coder/hnsw v0.1.0
    github.com/a-h/templ v0.2.543
    // Add suitable Confluence API client
    // Add suitable Ollama client
)
```

## Core Library API

### Main Types

```go
// hnswindex.go
package hnswindex

type Config struct {
    DBPath       string // Path to bbolt database file
    IndexDir     string // Directory for HNSW files (default: "./indexes")
    OllamaURL    string // Default: "http://localhost:11434"
    EmbedModel   string // Default: "nomic-embed-text"
    ChunkSize    int    // Default: 512 tokens
    ChunkOverlap int    // Default: 50 tokens
}

type IndexManager struct {
    // private implementation
}

type Index struct {
    // private implementation
}

type Document struct {
    URI      string                 // Unique identifier
    Title    string                 // Document title
    Content  string                 // Full text content
    Metadata map[string]interface{} // Additional metadata
}

type SearchResult struct {
    Document   Document `json:"document"`
    Score      float64  `json:"score"`
    ChunkID    string   `json:"chunk_id"`
    ChunkText  string   `json:"chunk_text"`
    IndexName  string   `json:"index_name"`
}

type BatchResult struct {
    TotalDocuments     int      `json:"total_documents"`
    NewDocuments       int      `json:"new_documents"`
    UpdatedDocuments   int      `json:"updated_documents"`
    UnchangedDocuments int      `json:"unchanged_documents"`
    ProcessedChunks    int      `json:"processed_chunks"`
    Errors            []string  `json:"errors,omitempty"`
}

type IndexStats struct {
    Name          string `json:"name"`
    DocumentCount int    `json:"document_count"`
    ChunkCount    int    `json:"chunk_count"`
    LastUpdated   string `json:"last_updated"`
    SizeBytes     int64  `json:"size_bytes"`
}
```

### Core Methods

```go
// IndexManager methods
func NewIndexManager(config Config) (*IndexManager, error)
func (im *IndexManager) GetIndex(name string) (*Index, error)
func (im *IndexManager) CreateIndex(name string) (*Index, error)
func (im *IndexManager) DeleteIndex(name string) error
func (im *IndexManager) ListIndexes() ([]string, error)
func (im *IndexManager) Close() error

// Index methods
func (i *Index) Name() string
func (i *Index) AddDocumentBatch(docs []Document) (*BatchResult, error)
func (i *Index) Search(query string, limit int) ([]SearchResult, error)
func (i *Index) GetDocument(uri string) (*Document, error)
func (i *Index) DeleteDocument(uri string) error
func (i *Index) Stats() (IndexStats, error)
func (i *Index) Clear() error
```

## Storage Architecture

### bbolt Database Schema

```
// Global buckets
_indexes                    // List of all index names
_config                     // Global configuration

// Per-index buckets (prefixed with index name)
{index_name}_documents      // URI -> Document JSON
{index_name}_chunks         // ChunkID -> Chunk JSON (with embedding)
{index_name}_doc_chunks     // URI -> []ChunkID mapping
{index_name}_hashes         // URI -> content hash (for change detection)
{index_name}_metadata       // Index stats, next_hnsw_id counter
```

### File System Layout

```
project_data/
├── indexes.db              // bbolt database
└── indexes/                // HNSW graph files
    ├── sales.hnsw
    ├── cs.hnsw
    └── engineering.hnsw
```

### Internal Chunk Structure

```go
type Chunk struct {
    ID          string    `json:"id"`           // Internal chunk ID
    HNSWId      uint64    `json:"hnsw_id"`      // HNSW graph ID
    DocumentURI string    `json:"document_uri"` // Parent document
    Text        string    `json:"text"`         // Chunk text
    Embedding   []float32 `json:"embedding"`    // Vector embedding
    Position    int       `json:"position"`     // Position in document
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

## Key Implementation Details

### Batch Processing Logic

```go
func (i *Index) AddDocumentBatch(docs []Document) (*BatchResult, error) {
    result := &BatchResult{TotalDocuments: len(docs)}
    
    // Phase 1: Analyze what needs updating
    var toProcess []Document
    for _, doc := range docs {
        if i.hasDocumentChanged(doc) {
            if i.documentExists(doc.URI) {
                result.UpdatedDocuments++
            } else {
                result.NewDocuments++
            }
            toProcess = append(toProcess, doc)
        } else {
            result.UnchangedDocuments++
        }
    }
    
    // Early return if nothing to do
    if len(toProcess) == 0 {
        return result, nil
    }
    
    // Phase 2: Process documents (chunking, embedding, HNSW insertion)
    for _, doc := range toProcess {
        if err := i.processDocument(doc); err != nil {
            result.Errors = append(result.Errors, 
                fmt.Sprintf("Error processing %s: %v", doc.URI, err))
            continue
        }
        result.ProcessedChunks += i.getDocumentChunkCount(doc.URI)
    }
    
    // Phase 3: Save HNSW index once at the end
    if err := i.saveGraph(); err != nil {
        return result, fmt.Errorf("failed to save index: %w", err)
    }
    
    return result, nil
}
```

### Document Change Detection

- Hash document content (title + content + relevant metadata)
- Store hash in bbolt
- Compare hash on subsequent indexing to detect changes
- Only process changed or new documents

### HNSW Index Management

- Use `Insert(vector []float32, id uint64)` for incremental additions
- Use `Delete(id uint64)` for removals
- Maintain mapping between internal chunk IDs and HNSW IDs
- Save graph to disk after batch operations using `Export(io.Writer)`
- Load graph on startup using `Import(io.Reader)`
- Lazy loading: only load HNSW graph when first search is performed

### Embedding Generation

- Batch requests to Ollama when possible for efficiency
- Handle connection pooling and error recovery
- Cache embeddings in bbolt to avoid regeneration
- Support configurable embedding models

### Chunking Strategy

- Split documents into configurable token-sized chunks (default 512)
- Maintain overlap between chunks (default 50 tokens)
- Preserve document structure and metadata
- Generate unique chunk IDs

## Demo Application

### CLI Commands

```bash
# Environment setup
export CONF_TOKEN=your_confluence_token
export CONF_URL=https://company.atlassian.net

# Index from Confluence space
./demo confluence --index "Sales" --space "SALES" --limit 100

# Index with filters
./demo confluence --index "Engineering" \
    --space "TECH" \
    --modified-since "2024-01-01" \
    --include-archived false

# Index local files
./demo files --index "Policies" --dir ./company_docs

# Search operations
./demo search --index "Sales" --query "pricing strategy"
./demo search --all --query "company policy"  # Search all indexes

# Management operations
./demo list-indexes
./demo stats --index "Sales"
./demo stats --all
./demo delete-index --index "OldIndex"

# Web interface
./demo server --port 8080
```

### Web Interface Features

Built using:
- **a-h/templ** for type-safe HTML templates
- **DaisyUI** for component styling
- **Minimal Tailwind** for utilities
- **data-star** for dynamic interactions (minimal JavaScript)

Features:
- Multi-index search interface with dropdown selection
- Real-time search results
- Document preview and metadata display
- Index management (create, delete, statistics)
- Batch indexing progress indication
- Clean, responsive design

### Confluence Integration

```go
// Demo-specific Confluence client
type ConfluenceClient struct {
    baseURL string
    token   string
    client  *http.Client
}

func (c *ConfluenceClient) GetSpacePages(spaceKey string, limit int) ([]Page, error)
func (c *ConfluenceClient) GetPageContent(pageID string) (*Page, error)
func extractPlainText(html string) string // Convert Confluence HTML to plain text
```

## Usage Examples

### Library Usage

```go
import "github.com/riclib/hnswindex"

// Initialize
manager, err := hnswindex.NewIndexManager(hnswindex.Config{
    DBPath:     "./indexes.db",
    IndexDir:   "./indexes",
    OllamaURL:  "http://localhost:11434",
    EmbedModel: "nomic-embed-text",
})
defer manager.Close()

// Create indexes
salesIndex, err := manager.CreateIndex("Sales")
csIndex, err := manager.CreateIndex("CS")

// Batch document processing
docs := []hnswindex.Document{
    {URI: "doc1", Title: "Sales Process", Content: "..."},
    {URI: "doc2", Title: "Pricing Guide", Content: "..."},
}

result, err := salesIndex.AddDocumentBatch(docs)
fmt.Printf("Processed: %d new, %d updated, %d unchanged\n", 
    result.NewDocuments, result.UpdatedDocuments, result.UnchangedDocuments)

// Search
results, err := salesIndex.Search("pricing strategy", 10)
for _, result := range results {
    fmt.Printf("Score: %.3f, Title: %s\n", result.Score, result.Document.Title)
}
```

### Demo Command Implementation

```go
func confluenceIndexCommand(indexName, space string, limit int) error {
    // Initialize library
    manager, err := hnswindex.NewIndexManager(config)
    if err != nil {
        return err
    }
    defer manager.Close()
    
    index, err := manager.GetIndex(indexName)
    if err != nil {
        return err
    }
    
    // Fetch from Confluence
    client := confluence.NewClient(os.Getenv("CONF_URL"), os.Getenv("CONF_TOKEN"))
    pages, err := client.GetSpacePages(space, limit)
    if err != nil {
        return err
    }
    
    // Convert to library format
    var docs []hnswindex.Document
    for _, page := range pages {
        docs = append(docs, hnswindex.Document{
            URI:     page.Links.WebUI,
            Title:   page.Title,
            Content: extractPlainText(page.Body.Storage.Value),
            Metadata: map[string]interface{}{
                "space":    page.Space.Key,
                "author":   page.Version.By.DisplayName,
                "created":  page.History.CreatedDate,
                "modified": page.Version.When,
            },
        })
    }
    
    // Process through library
    result, err := index.AddDocumentBatch(docs)
    if err != nil {
        return err
    }
    
    // Report results
    fmt.Printf("Indexing complete:\n")
    fmt.Printf("  Total: %d, New: %d, Updated: %d, Unchanged: %d\n", 
        result.TotalDocuments, result.NewDocuments, 
        result.UpdatedDocuments, result.UnchangedDocuments)
    fmt.Printf("  Chunks processed: %d\n", result.ProcessedChunks)
    
    return nil
}
```

## Error Handling

- Structured error types with context
- Graceful handling of partial failures in batch operations
- Comprehensive logging of operations
- Clear error messages for common issues:
  - Ollama service not running
  - Confluence authentication failures
  - Disk space issues
  - Invalid document formats

## Performance Considerations

- Lazy loading of HNSW graphs (load on first search)
- Connection pooling for Ollama requests
- Batch embedding generation when possible
- Efficient change detection using content hashing
- Atomic operations for data consistency
- Configurable chunk sizes and overlap
- Memory management for large document sets

## Testing Strategy

- Unit tests for each internal component
- Integration tests with real Ollama instance
- Mock Confluence API for testing
- Benchmark tests for search performance
- Test data in testdata/ directory
- Property-based testing for chunking logic

## Security Considerations

- Environment-based configuration for sensitive data
- No storage of authentication tokens in database
- Proper file permissions for index files
- Input validation and sanitization
- Rate limiting for external API calls

This specification provides a complete, implementable design for a semantic document indexing system with clean separation between the core library and integration-specific demo code.
