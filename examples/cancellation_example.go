package main

import (
	"context"
	"fmt"
	"time"
	
	"github.com/riclib/hnswindex"
)

func main() {
	// Example showing how to use context for cancellation and timeout
	
	// Create index manager
	config := hnswindex.NewConfig()
	config.DataPath = "./hnswdata"
	
	manager, err := hnswindex.NewIndexManager(config)
	if err != nil {
		panic(err)
	}
	defer manager.Close()
	
	// Get or create index
	index, _ := manager.GetIndex("example")
	if index == nil {
		index, _ = manager.CreateIndex("example")
	}
	
	// Create some documents
	docs := []hnswindex.Document{
		{URI: "doc1", Title: "Document 1", Content: "Some content here..."},
		{URI: "doc2", Title: "Document 2", Content: "More content here..."},
		// ... many more documents
	}
	
	// Example 1: With timeout
	fmt.Println("Example 1: Indexing with 30-second timeout...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	progress := make(chan hnswindex.ProgressUpdate, 100)
	go func() {
		for update := range progress {
			fmt.Printf("[%d/%d] %s\n", update.Current, update.Total, update.Message)
		}
	}()
	
	result, err := index.AddDocumentBatch(ctx, docs, progress)
	close(progress)
	
	if err != nil {
		if err == context.DeadlineExceeded {
			fmt.Println("Indexing timed out!")
		} else {
			fmt.Printf("Error: %v\n", err)
		}
	} else {
		fmt.Printf("Indexed %d documents\n", result.NewDocuments)
	}
	
	// Example 2: Manual cancellation
	fmt.Println("\nExample 2: Indexing with manual cancellation...")
	ctx2, cancel2 := context.WithCancel(context.Background())
	
	// Simulate cancelling after 5 seconds
	go func() {
		time.Sleep(5 * time.Second)
		fmt.Println("Cancelling operation...")
		cancel2()
	}()
	
	result2, err2 := index.AddDocumentBatch(ctx2, docs, nil) // No progress tracking
	if err2 != nil {
		if err2 == context.Canceled {
			fmt.Println("Indexing was cancelled!")
		} else {
			fmt.Printf("Error: %v\n", err2)
		}
	} else {
		fmt.Printf("Indexed %d documents\n", result2.NewDocuments)
	}
}