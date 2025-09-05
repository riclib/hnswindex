package main

import (
	"context"
	"fmt"
	"time"
	"github.com/riclib/hnswindex"
)

func demonstrateProgress() {
	// Create sample documents
	docs := []hnswindex.Document{
		{URI: "doc1", Title: "First Document", Content: "This is the first test document"},
		{URI: "doc2", Title: "Second Document", Content: "This is the second test document"},
		{URI: "doc3", Title: "Third Document", Content: "This is the third test document"},
	}

	// Create index manager
	config := hnswindex.NewConfig()
	config.DataPath = "./testdata"
	
	manager, err := hnswindex.NewIndexManager(config)
	if err != nil {
		panic(err)
	}
	defer manager.Close()

	// Get or create index
	index, _ := manager.GetIndex("demo")
	if index == nil {
		index, _ = manager.CreateIndex("demo")
	}

	// Process documents with progress
	fmt.Println("Starting indexing with progress updates...")
	
	// Create progress channel
	progressChan := make(chan hnswindex.ProgressUpdate, 100)
	
	// Consume progress in a separate goroutine
	done := make(chan bool)
	go func() {
		for update := range progressChan {
			// Display progress with nice formatting
			switch update.Stage {
			case "checking":
				fmt.Printf("📋 [%d/%d] Checking: %s\n", 
					update.Current, update.Total, update.Message)
			case "processing":
				fmt.Printf("⚙️  [%d/%d] Processing: %s\n", 
					update.Current, update.Total, update.Message)
			case "embedding":
				fmt.Printf("🧮 [%d/%d] Embedding: %s\n", 
					update.Current, update.Total, update.Message)
			case "saving":
				fmt.Printf("💾 Saving: %s\n", update.Message)
			case "complete":
				fmt.Printf("✅ %s\n", update.Message)
			default:
				fmt.Printf("[%d/%d] %s: %s\n", 
					update.Current, update.Total, update.Stage, update.Message)
			}
			
			// Small delay to make progress visible
			time.Sleep(100 * time.Millisecond)
		}
		done <- true
	}()
	
	// Call AddDocumentBatch with context and progress channel
	ctx := context.Background()
	result, err := index.AddDocumentBatch(ctx, docs, progressChan)
	
	// Close channel and wait for completion
	close(progressChan)
	<-done
	
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	// Display results
	fmt.Println("\n📊 Indexing Results:")
	fmt.Printf("  Total: %d documents\n", result.TotalDocuments)
	fmt.Printf("  New: %d\n", result.NewDocuments)
	fmt.Printf("  Updated: %d\n", result.UpdatedDocuments)
	fmt.Printf("  Unchanged: %d\n", result.UnchangedDocuments)
	fmt.Printf("  Chunks: %d\n", result.ProcessedChunks)
}