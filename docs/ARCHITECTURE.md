# Architecture Documentation

## System Overview

The HNSW Index library is designed as a modular, efficient system for semantic document search using local embeddings and graph-based similarity search.

```
┌─────────────────────────────────────────────────────────┐
│                     Application Layer                     │
├─────────────────────────────────────────────────────────┤
│                      IndexManager                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │  Index 1  │  │  Index 2  │  │  Index N  │              │
│  └──────────┘  └──────────┘  └──────────┘              │
├─────────────────────────────────────────────────────────┤
│                    Processing Layer                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │ Chunker  │  │ Embedder │  │  Worker   │              │
│  │          │  │ (Ollama) │  │   Pool    │              │
│  └──────────┘  └──────────┘  └──────────┘              │
├─────────────────────────────────────────────────────────┤
│                     Storage Layer                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │  BBolt   │  │   HNSW   │  │   File    │              │
│  │ Database │  │  Graphs  │  │  System   │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
```

## Core Components

### 1. IndexManager

**Purpose**: Central orchestrator for managing multiple indexes.

**Responsibilities**:
- Lifecycle management of indexes
- Resource allocation and cleanup
- Configuration propagation
- Storage coordination

**Key Design Decisions**:
- Thread-safe operations using sync.RWMutex
- Lazy loading of indexes (loaded on first access)
- Centralized configuration management

### 2. Index

**Purpose**: Represents a searchable collection of documents.

**Responsibilities**:
- Document CRUD operations
- Search query processing
- Change detection and incremental updates
- Statistics and metadata management

**Implementation Details**:
```go
type indexImpl struct {
    name      string
    manager   *indexManagerImpl
    hnswIndex *indexer.HNSWIndex
    mu        sync.RWMutex
}
```

### 3. Document Processing Pipeline

#### 3.1 Chunker

**Purpose**: Splits documents into overlapping chunks for better search granularity.

**Algorithm**:
1. Tokenize using tiktoken (GPT-4 compatible)
2. Create chunks of `ChunkSize` tokens
3. Overlap chunks by `ChunkOverlap` tokens
4. Generate unique IDs using content hash

**Example**:
```
Document: "The quick brown fox jumps over the lazy dog. The dog was sleeping."
ChunkSize: 10 tokens
Overlap: 3 tokens

Chunk 1: "The quick brown fox jumps over the lazy dog."
Chunk 2: "lazy dog. The dog was sleeping."
         └── 3 token overlap ──┘
```

#### 3.2 Embedder

**Purpose**: Generates vector embeddings for text chunks.

**Implementation**:
- Uses Ollama API for local embedding generation
- Supports batch processing for efficiency
- Configurable embedding models
- Dimension detection and validation

**Supported Models**:
| Model | Dimensions | Use Case |
|-------|------------|----------|
| nomic-embed-text | 768 | General purpose (recommended) |
| mxbai-embed-large | 1024 | Higher accuracy, more memory |
| all-minilm | 384 | Faster, less memory |

### 4. Storage Architecture

#### 4.1 BBolt Database

**Structure**:
```
indexes.db
├── _indexes (bucket)          # Index registry
├── _config (bucket)           # Global configuration
├── {index}_documents          # Document storage
├── {index}_chunks             # Chunk storage
├── {index}_doc_chunks         # Document-chunk mapping
├── {index}_hashes             # Document hashes for change detection
└── {index}_metadata           # Index metadata
```

**Key-Value Schema**:
- Documents: `URI -> JSON(Document)`
- Chunks: `ChunkID -> JSON(Chunk)`
- Hashes: `URI -> SHA256(content)`
- Mappings: `URI -> []ChunkID`

#### 4.2 HNSW Graph Storage

**File Layout**:
```
hnswdata/
├── indexes.db                 # BBolt database
└── indexes/
    ├── index1/
    │   └── index.hnsw         # Binary HNSW graph
    └── index2/
        └── index.hnsw
```

**HNSW Parameters**:
- **M**: 16 (bi-directional links per node)
- **EfConstruction**: 200 (build-time search width)
- **EfSearch**: 20 (query-time search width)
- **Distance**: Cosine similarity

### 5. Search Algorithm

**Process Flow**:
1. Query text → Tokenization
2. Generate query embedding
3. HNSW approximate nearest neighbor search
4. Retrieve top-K candidates
5. Load full documents and chunks
6. Return ranked results with metadata

**Optimization Strategies**:
- Graph-based search (O(log N) complexity)
- Early termination for efficiency
- Score normalization for consistency
- Parallel candidate retrieval

## Data Flow

### Indexing Flow

```
Document Input
     ↓
Content Hashing ─────→ Change Detection
     ↓                      ↓
     ↓              [Skip if unchanged]
     ↓                      
Chunking (Tiktoken)
     ↓
Batch Embedding (Ollama)
     ↓
Concurrent Storage
     ├→ BBolt (metadata)
     └→ HNSW (vectors)
```

### Search Flow

```
Query Text
     ↓
Generate Embedding (Ollama)
     ↓
HNSW Search (k-NN)
     ↓
Retrieve Candidates
     ↓
Load Documents/Chunks (BBolt)
     ↓
Rank and Return Results
```

## Concurrency Model

### Worker Pool Architecture

```go
type WorkerPool struct {
    workers   int
    jobQueue  chan Job
    results   chan Result
    waitGroup sync.WaitGroup
}
```

**Benefits**:
- Controlled resource usage
- Prevents Ollama overload
- Efficient batch processing
- Configurable parallelism

### Thread Safety

**Synchronization Points**:
1. Index operations (RWMutex)
2. Storage transactions (BBolt)
3. HNSW graph updates (internal locks)
4. Worker pool coordination

**Lock Hierarchy** (to prevent deadlocks):
1. IndexManager lock
2. Index lock
3. Storage lock
4. HNSW lock

## Memory Management

### Memory Usage Estimates

| Component | Memory Usage | Formula |
|-----------|--------------|---------|
| Vector | 3KB | dimensions × 4 bytes |
| HNSW Node | ~256 bytes | M × 2 × 8 bytes + overhead |
| Document | Variable | len(content) + metadata |
| Chunk | ~2KB | text + embedding reference |

### Optimization Strategies

1. **Lazy Loading**: Indexes loaded only when accessed
2. **Streaming**: Large documents processed in chunks
3. **Batch Processing**: Amortize overhead costs
4. **Index Unloading**: Configurable TTL for unused indexes

## Error Handling

### Error Categories

1. **Configuration Errors**: Invalid settings, missing dependencies
2. **Storage Errors**: Disk I/O, corruption, space issues
3. **Processing Errors**: Embedding failures, tokenization issues
4. **Network Errors**: Ollama connectivity problems

### Error Propagation

```
Component Error
     ↓
Wrapped with Context (fmt.Errorf)
     ↓
Logged with Structure (slog)
     ↓
Returned to Caller
     ↓
Batch Result (partial failures)
```

## Performance Characteristics

### Time Complexity

| Operation | Complexity | Notes |
|-----------|------------|-------|
| Add Document | O(C × E) | C=chunks, E=embedding time |
| Search | O(log N × k) | N=vectors, k=results |
| Delete Document | O(C) | C=chunks |
| Update Document | O(C × E) | Same as add |

### Space Complexity

| Component | Complexity | Notes |
|-----------|------------|-------|
| Storage | O(N × D) | N=documents, D=avg size |
| HNSW Graph | O(V × M) | V=vectors, M=connections |
| Memory | O(I × V) | I=loaded indexes, V=vectors |

## Monitoring and Observability

### Structured Logging

```go
slog.Info("Document processed",
    "index", indexName,
    "uri", doc.URI,
    "chunks", chunkCount,
    "duration_ms", duration,
)
```

### Metrics Points

1. **Indexing Metrics**:
   - Documents/second
   - Chunks/second
   - Embedding generation time
   - Storage write time

2. **Search Metrics**:
   - Query latency (P50, P95, P99)
   - Result count distribution
   - Cache hit rate

3. **System Metrics**:
   - Memory usage
   - Goroutine count
   - Storage size
   - Index statistics

## Security Considerations

### Input Validation

- Document size limits
- URI format validation
- Metadata sanitization
- Query length limits

### Resource Limits

- Maximum indexes per manager
- Maximum documents per index
- Maximum chunk size
- Worker pool size

### Access Control

- File system permissions
- Network isolation (Ollama)
- Database encryption (optional)

## Future Enhancements

### Planned Features

1. **Distributed Mode**: Multi-node index sharding
2. **Caching Layer**: Query result caching
3. **Index Compression**: Reduce storage footprint
4. **GPU Acceleration**: CUDA-based HNSW search
5. **Incremental Learning**: Online index updates

### Extension Points

1. **Custom Embedders**: Interface for different embedding providers
2. **Alternative Storage**: S3, PostgreSQL backends
3. **Custom Chunkers**: Domain-specific text splitting
4. **Ranking Algorithms**: BM25 hybrid search
5. **Middleware**: Metrics, tracing, authentication

## Testing Strategy

### Unit Tests

```go
// Test individual components
func TestChunker_ChunkDocument(t *testing.T)
func TestEmbedder_GenerateEmbedding(t *testing.T)
func TestStorage_StoreDocument(t *testing.T)
```

### Integration Tests

```go
// Test component interactions
func TestIndex_AddAndSearch(t *testing.T)
func TestIndexManager_MultiIndex(t *testing.T)
```

### Benchmarks

```go
// Performance measurements
func BenchmarkHNSW_Search(b *testing.B)
func BenchmarkChunker_LargeDocument(b *testing.B)
```

### Load Tests

- Concurrent indexing stress test
- Search latency under load
- Memory usage profiling
- Storage growth analysis