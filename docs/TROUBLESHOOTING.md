# Troubleshooting Guide

## Common Issues and Solutions

### 1. Search Returns No Results

#### Problem
After indexing documents, searches return empty results.

#### Diagnosis
Enable debug logging to trace the issue:
```bash
./demo --debug search "your query" --index myindex
```

Check for these log messages:
- `"Index is empty, returning empty results"` - HNSW index not loaded
- `"Failed to load existing index"` - Index file corruption

#### Solutions

**Solution 1: Index Loading Issue**
```bash
# Check if index file exists
ls -la hnswdata/indexes/myindex/

# If file exists but won't load, rebuild index
./demo index --dir ./documents --index myindex --force
```

**Solution 2: Embedding Model Mismatch**
```bash
# Verify Ollama model is installed
ollama list

# Pull the correct model
ollama pull nomic-embed-text

# Re-index with correct model
./demo index --dir ./documents --index myindex
```

### 2. Index Loading Fails with ByteReader Error

#### Problem
Error: `"failed to import graph: reading *int at index 0: reader does not implement io.ByteReader"`

#### Cause
The HNSW library requires io.ByteReader interface for importing.

#### Solution
This has been fixed in the latest version. The fix wraps the file with bufio.Reader:

```go
// internal/indexer/indexer.go
reader := bufio.NewReader(file)
if err := h.graph.Import(reader); err != nil {
    return fmt.Errorf("failed to import graph: %w", err)
}
```

Update to the latest version:
```bash
go get -u github.com/riclib/hnswindex@latest
```

### 3. Ollama Connection Errors

#### Problem
Error: `"failed to generate embedding: connection refused"`

#### Diagnosis
```bash
# Check if Ollama is running
curl http://localhost:11434/api/tags

# Check Ollama logs
journalctl -u ollama -f  # Linux with systemd
```

#### Solutions

**Solution 1: Start Ollama**
```bash
# macOS/Linux
ollama serve

# Or run in background
nohup ollama serve > ollama.log 2>&1 &
```

**Solution 2: Configure Custom URL**
```go
config := hnswindex.NewConfig()
config.OllamaURL = "http://your-ollama-server:11434"
```

Or via CLI:
```bash
./demo --ollama-url http://remote:11434 search "query"
```

### 4. High Memory Usage

#### Problem
Application uses excessive memory during indexing.

#### Diagnosis
Monitor memory usage:
```bash
# During indexing
go tool pprof -http=:6060 http://localhost:8080/debug/pprof/heap
```

#### Solutions

**Solution 1: Reduce Batch Size**
```go
// Process in smaller batches
batchSize := 50  // Instead of 100+
for i := 0; i < len(docs); i += batchSize {
    // Process batch
}
```

**Solution 2: Adjust Worker Pool**
```go
config.MaxWorkers = 4  // Reduce from default 8
```

**Solution 3: Use Smaller Embedding Model**
```go
config.EmbedModel = "all-minilm"  // 384 dimensions vs 768
```

### 5. Slow Indexing Performance

#### Problem
Document indexing takes too long.

#### Diagnosis
Enable debug logging to identify bottlenecks:
```bash
./demo --debug index --dir ./documents --index myindex
```

Look for timing logs:
- `"Batch embedding generation completed" duration_ms=X`
- `"HNSW index saved successfully" duration_ms=X`

#### Solutions

**Solution 1: Increase Workers**
```go
config.MaxWorkers = 16  // For CPU with many cores
```

**Solution 2: Disable Auto-Save**
```go
config.AutoSave = false

// Save manually after batch
index.Save()
```

**Solution 3: Skip Unchanged Documents**
The system automatically skips unchanged documents. Ensure proper change detection:
```bash
# Check document stats
./demo stats --index myindex
```

### 6. Chunk Count Shows Zero

#### Problem
Stats show `ChunkCount: 0` despite successful indexing.

#### Diagnosis
```bash
# Check debug logs for chunking
./demo --debug index --dir ./docs --index test | grep -i chunk
```

#### Solutions

**Solution 1: Verify Chunk Storage**
Check if chunks are being stored:
```go
// Add logging to processChunks
slog.Debug("Storing chunk",
    "chunk_id", chunk.ID,
    "hnsw_id", chunk.HNSWId,
)
```

**Solution 2: Update Metadata**
Ensure metadata is updated after processing:
```go
metadata.ChunkCount = result.ProcessedChunks
storage.SetIndexMetadata(indexName, metadata)
```

### 7. Inconsistent Search Results

#### Problem
Search results vary significantly between queries.

#### Diagnosis
Check HNSW parameters:
```go
slog.Debug("HNSW parameters",
    "M", config.M,
    "EfSearch", config.Ef,
)
```

#### Solutions

**Solution 1: Increase Search Width**
```go
// In HNSWConfig
config.Ef = 50  // Increase from 20 for better accuracy
```

**Solution 2: Verify Distance Metric**
```go
// Ensure consistent distance metric
config.DistanceType = "cosine"  // Don't mix with "l2"
```

### 8. Storage Corruption

#### Problem
Error: `"database file appears corrupted"`

#### Diagnosis
```bash
# Check file integrity
file hnswdata/indexes.db

# Try to open with bbolt CLI
go install go.etcd.io/bbolt/cmd/bbolt@latest
bbolt check hnswdata/indexes.db
```

#### Solutions

**Solution 1: Restore from Backup**
```bash
cp hnswdata/indexes.db.backup hnswdata/indexes.db
```

**Solution 2: Rebuild Index**
```bash
# Move corrupted files
mv hnswdata hnswdata.old

# Rebuild
./demo index --dir ./documents --index myindex
```

### 9. Embedding Dimension Mismatch

#### Problem
Error: `"vector dimension 1024 does not match index dimension 768"`

#### Cause
Switching embedding models with different dimensions.

#### Solutions

**Solution 1: Rebuild Index**
```bash
# Delete old index
./demo delete --index myindex

# Create with new model
./demo index --dir ./documents --index myindex --model mxbai-embed-large
```

**Solution 2: Use Multiple Indexes**
```go
// Create separate indexes for different models
index768 := manager.CreateIndex("docs-768")  // nomic-embed-text
index1024 := manager.CreateIndex("docs-1024") // mxbai-embed-large
```

### 10. CLI Not Building

#### Problem
```bash
go build: cannot find module providing package github.com/riclib/hnswindex
```

#### Solutions

**Solution 1: Update Dependencies**
```bash
go mod tidy
go mod download
```

**Solution 2: Build from Source**
```bash
cd cmd/demo
go build -o demo main.go
```

## Debug Logging Reference

### Enable Debug Mode

#### Via CLI
```bash
./demo --debug [command]
./demo --log-level debug [command]
```

#### Via Code
```go
import "log/slog"

opts := &slog.HandlerOptions{
    Level: slog.LevelDebug,
    AddSource: true,  // Include file:line
}
handler := slog.NewTextHandler(os.Stderr, opts)
slog.SetDefault(slog.New(handler))
```

### Key Debug Messages

| Component | Message | Meaning |
|-----------|---------|---------|
| Chunker | `"Text chunked successfully"` | Document split into chunks |
| Embedder | `"Embedding generated successfully"` | Vector created |
| Storage | `"Document stored successfully"` | Saved to BBolt |
| HNSW | `"Vector added successfully"` | Added to graph |
| Index | `"Batch processing complete"` | Documents indexed |

### Performance Metrics in Logs

Look for `duration_ms` fields:
```
msg="Batch embedding generation completed" count=16 duration_ms=531
msg="HNSW index saved successfully" duration_ms=45
msg="Search results prepared" count=5 duration_ms=23
```

## Environment Variables

### Debugging Environment

```bash
# Ollama configuration
export OLLAMA_HOST=http://localhost:11434
export OLLAMA_MODELS_DIR=/usr/share/ollama/.ollama/models

# Application configuration
export HNSW_DATA_PATH=./hnswdata
export HNSW_OLLAMA_URL=http://localhost:11434
export HNSW_EMBED_MODEL=nomic-embed-text
export HNSW_LOG_LEVEL=debug

# Resource limits
export HNSW_MAX_WORKERS=8
export HNSW_CHUNK_SIZE=512
```

## Getting Help

### Diagnostic Information to Collect

When reporting issues, provide:

1. **Version Information**
```bash
go version
ollama --version
./demo --version
```

2. **Configuration**
```bash
cat config.yaml  # If using config file
env | grep HNSW  # Environment variables
```

3. **Debug Logs**
```bash
./demo --debug [failing command] 2>&1 | tee debug.log
```

4. **System Information**
```bash
uname -a  # OS version
free -h   # Memory
df -h     # Disk space
```

5. **Index Statistics**
```bash
./demo stats --index myindex
ls -la hnswdata/indexes/
```

### Common Log Patterns to Search

```bash
# Find errors
grep -i error debug.log

# Find slow operations
grep "duration_ms" debug.log | awk '$NF > 1000'

# Track document processing
grep "Document processed" debug.log

# Monitor memory issues
grep -E "(memory|heap|alloc)" debug.log
```

## Prevention Best Practices

1. **Regular Backups**
```bash
# Backup script
#!/bin/bash
cp -r hnswdata hnswdata.backup.$(date +%Y%m%d)
```

2. **Monitor Disk Space**
```bash
# Check index size
du -sh hnswdata/
```

3. **Test Configuration**
```go
// Validate before production
config := hnswindex.NewConfig()
if err := config.Validate(); err != nil {
    log.Fatal(err)
}
```

4. **Gradual Rollout**
- Test with small document sets first
- Monitor resource usage
- Scale up gradually

5. **Health Checks**
```go
// Implement health endpoint
func healthCheck(manager *IndexManager) error {
    indexes, err := manager.ListIndexes()
    if err != nil {
        return err
    }
    
    for _, name := range indexes {
        index, _ := manager.GetIndex(name)
        stats, _ := index.Stats()
        if stats.ChunkCount == 0 {
            return fmt.Errorf("index %s is empty", name)
        }
    }
    return nil
}
```