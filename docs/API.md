# API Documentation

## Core Types

### Document
Represents a document to be indexed.

```go
type Document struct {
    URI      string                 // Unique identifier for the document
    Title    string                 // Document title
    Content  string                 // Full text content
    Metadata map[string]interface{} // Optional metadata
}
```

### SearchResult
Result from a search query.

```go
type SearchResult struct {
    Document  Document // The matched document
    Score     float64  // Similarity score (0-1, higher is better)
    ChunkID   string   // ID of the matched chunk
    ChunkText string   // Text of the matched chunk
    IndexName string   // Name of the index
}
```

### BatchResult
Result from batch document processing.

```go
type BatchResult struct {
    TotalDocuments     int               // Total documents processed
    NewDocuments       int               // New documents added
    UpdatedDocuments   int               // Documents updated
    UnchangedDocuments int               // Documents skipped (unchanged)
    ProcessedChunks    int               // Total chunks processed
    FailedURIs         map[string]string // Failed documents with error messages
}
```

### IndexStats
Statistics for an index.

```go
type IndexStats struct {
    Name          string // Index name
    DocumentCount int    // Number of documents
    ChunkCount    int    // Number of chunks
    LastUpdated   string // Last update timestamp
    SizeBytes     int64  // Storage size in bytes
}
```

### Config
Configuration for the IndexManager.

```go
type Config struct {
    DataPath     string // Base directory for all data
    OllamaURL    string // Ollama server URL
    EmbedModel   string // Embedding model name
    ChunkSize    int    // Maximum tokens per chunk
    ChunkOverlap int    // Overlapping tokens between chunks
    MaxWorkers   int    // Worker pool size
    AutoSave     bool   // Auto-save HNSW index after modifications
}
```

## IndexManager API

### NewIndexManager
Creates a new index manager.

```go
func NewIndexManager(config *Config) (*IndexManager, error)
```

**Parameters:**
- `config`: Configuration for the manager

**Returns:**
- `*IndexManager`: The index manager instance
- `error`: Error if initialization fails

**Example:**
```go
config := hnswindex.NewConfig()
config.DataPath = "./data"
config.OllamaURL = "http://localhost:11434"

manager, err := hnswindex.NewIndexManager(config)
if err != nil {
    log.Fatal(err)
}
defer manager.Close()
```

### CreateIndex
Creates a new index.

```go
func (im *IndexManager) CreateIndex(name string) (*Index, error)
```

**Parameters:**
- `name`: Unique name for the index

**Returns:**
- `*Index`: The created index
- `error`: Error if index already exists or creation fails

### GetIndex
Retrieves an existing index.

```go
func (im *IndexManager) GetIndex(name string) (*Index, error)
```

**Parameters:**
- `name`: Name of the index to retrieve

**Returns:**
- `*Index`: The requested index
- `error`: Error if index doesn't exist

### DeleteIndex
Deletes an index and all its data.

```go
func (im *IndexManager) DeleteIndex(name string) error
```

**Parameters:**
- `name`: Name of the index to delete

**Returns:**
- `error`: Error if deletion fails

### ListIndexes
Lists all available indexes.

```go
func (im *IndexManager) ListIndexes() ([]string, error)
```

**Returns:**
- `[]string`: List of index names
- `error`: Error if listing fails

### Close
Closes the manager and releases resources.

```go
func (im *IndexManager) Close() error
```

## Index API

### AddDocument
Adds a single document to the index.

```go
func (i *Index) AddDocument(doc Document) error
```

**Parameters:**
- `doc`: Document to add

**Returns:**
- `error`: Error if indexing fails

### AddDocumentBatch
Adds multiple documents in batch.

```go
func (i *Index) AddDocumentBatch(docs []Document) (*BatchResult, error)
```

**Parameters:**
- `docs`: Slice of documents to add

**Returns:**
- `*BatchResult`: Processing results
- `error`: Critical error (partial failures are in BatchResult)

**Example:**
```go
docs := []hnswindex.Document{
    {
        URI:     "doc1",
        Title:   "First Document",
        Content: "This is the content...",
        Metadata: map[string]interface{}{
            "author": "John Doe",
            "date": "2024-01-01",
        },
    },
}

result, err := index.AddDocumentBatch(docs)
if err != nil {
    log.Fatal(err)
}

if len(result.FailedURIs) > 0 {
    for uri, errMsg := range result.FailedURIs {
        log.Printf("Failed: %s - %s\n", uri, errMsg)
    }
}
```

### Search
Searches for documents matching a query.

```go
func (i *Index) Search(query string, limit int) ([]SearchResult, error)
```

**Parameters:**
- `query`: Search query text
- `limit`: Maximum number of results

**Returns:**
- `[]SearchResult`: Ranked search results
- `error`: Error if search fails

**Example:**
```go
results, err := index.Search("machine learning algorithms", 10)
if err != nil {
    log.Fatal(err)
}

for _, result := range results {
    fmt.Printf("%.3f: %s\n", result.Score, result.Document.Title)
    fmt.Printf("  Chunk: %s\n", result.ChunkText[:100])
}
```

### GetDocument
Retrieves a specific document.

```go
func (i *Index) GetDocument(uri string) (*Document, error)
```

**Parameters:**
- `uri`: Document URI

**Returns:**
- `*Document`: The document
- `error`: Error if document not found

### DeleteDocument
Removes a document from the index.

```go
func (i *Index) DeleteDocument(uri string) error
```

**Parameters:**
- `uri`: Document URI to delete

**Returns:**
- `error`: Error if deletion fails

### Stats
Gets index statistics.

```go
func (i *Index) Stats() (IndexStats, error)
```

**Returns:**
- `IndexStats`: Index statistics
- `error`: Error if stats retrieval fails

### Clear
Removes all documents from the index.

```go
func (i *Index) Clear() error
```

**Returns:**
- `error`: Error if clearing fails

## Configuration API

### NewConfig
Creates a default configuration.

```go
func NewConfig() *Config
```

**Returns:**
- `*Config`: Configuration with default values

**Default values:**
- `DataPath`: "./hnswdata"
- `OllamaURL`: "http://localhost:11434"
- `EmbedModel`: "nomic-embed-text"
- `ChunkSize`: 512
- `ChunkOverlap`: 50
- `MaxWorkers`: 8
- `AutoSave`: true

### NewConfigFromViper
Creates configuration from Viper.

```go
func NewConfigFromViper(v *viper.Viper, prefix string) *Config
```

**Parameters:**
- `v`: Viper instance
- `prefix`: Configuration prefix (namespace)

**Returns:**
- `*Config`: Configuration from Viper

**Example:**
```go
v := viper.New()
v.SetConfigFile("config.yaml")
v.ReadInConfig()

config := hnswindex.NewConfigFromViper(v, "hnsw")
```

### LoadFromViper
Loads configuration from Viper into existing config.

```go
func (c *Config) LoadFromViper(v *viper.Viper, prefix string)
```

**Parameters:**
- `v`: Viper instance
- `prefix`: Configuration prefix

## Error Handling

The library uses wrapped errors for context. Use `errors.Is()` for error checking:

```go
err := index.AddDocument(doc)
if errors.Is(err, hnswindex.ErrDocumentExists) {
    // Handle duplicate document
}
```

Common errors:
- `ErrIndexNotFound`: Index doesn't exist
- `ErrIndexExists`: Index already exists
- `ErrDocumentNotFound`: Document not found
- `ErrInvalidConfig`: Invalid configuration

## Logging

The library uses `log/slog` for structured logging:

```go
import "log/slog"

// Set debug level
opts := &slog.HandlerOptions{
    Level: slog.LevelDebug,
}
handler := slog.NewTextHandler(os.Stderr, opts)
slog.SetDefault(slog.New(handler))
```

Log levels:
- `DEBUG`: Detailed operations (chunking, embedding, storage)
- `INFO`: High-level operations (indexing, searching)
- `WARN`: Potential issues
- `ERROR`: Operation failures

## Thread Safety

- `IndexManager` is thread-safe
- `Index` operations are thread-safe
- Multiple goroutines can safely search while indexing

## Performance Tips

1. **Batch Operations**: Use `AddDocumentBatch` instead of multiple `AddDocument` calls
2. **Worker Pool**: Adjust `MaxWorkers` based on CPU cores
3. **Chunk Size**: Larger chunks = fewer embeddings but less granular search
4. **Auto-save**: Disable for bulk operations, save manually at the end
5. **Memory**: Each vector uses ~3KB (768 dimensions × 4 bytes)

## Example: Advanced Usage

```go
// Custom configuration
config := &hnswindex.Config{
    DataPath:     "/var/lib/myapp/indexes",
    OllamaURL:    "http://ollama:11434",
    EmbedModel:   "mxbai-embed-large",
    ChunkSize:    1024,
    ChunkOverlap: 100,
    MaxWorkers:   16,
    AutoSave:     false, // Manual save for bulk operations
}

manager, err := hnswindex.NewIndexManager(config)
if err != nil {
    log.Fatal(err)
}
defer manager.Close()

// Create index with metadata
index, err := manager.CreateIndex("products")
if err != nil {
    log.Fatal(err)
}

// Bulk indexing
var documents []hnswindex.Document
for _, product := range products {
    documents = append(documents, hnswindex.Document{
        URI:     product.ID,
        Title:   product.Name,
        Content: product.Description,
        Metadata: map[string]interface{}{
            "category": product.Category,
            "price":    product.Price,
            "stock":    product.Stock,
        },
    })
}

// Index in batches for memory efficiency
batchSize := 100
for i := 0; i < len(documents); i += batchSize {
    end := i + batchSize
    if end > len(documents) {
        end = len(documents)
    }
    
    result, err := index.AddDocumentBatch(documents[i:end])
    if err != nil {
        log.Printf("Batch %d failed: %v", i/batchSize, err)
        continue
    }
    
    log.Printf("Batch %d: %d new, %d updated, %d failed",
        i/batchSize, result.NewDocuments, 
        result.UpdatedDocuments, len(result.FailedURIs))
}

// Manual save after bulk operation
if err := index.Save(); err != nil {
    log.Fatal(err)
}

// Semantic search with metadata filtering
results, err := index.Search("wireless headphones", 20)
if err != nil {
    log.Fatal(err)
}

// Filter results by metadata
for _, result := range results {
    if category, ok := result.Document.Metadata["category"].(string); ok {
        if category == "Electronics" {
            fmt.Printf("%.3f: %s ($%.2f)\n", 
                result.Score, 
                result.Document.Title,
                result.Document.Metadata["price"])
        }
    }
}
```