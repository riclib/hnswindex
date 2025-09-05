package main

import (
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/riclib/hnswindex"
	"github.com/riclib/hnswindex/pkg/confluence"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile   string
	dataPath  string
	indexName string
	verbose   bool
	debug     bool
	logLevel  string
)

var rootCmd = &cobra.Command{
	Use:   "demo",
	Short: "HNSW Index Demo - Semantic document search",
	Long: `A demo application for the HNSW index library.
Index and search markdown documents using local embeddings.`,
}

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index markdown files from a directory",
	Long:  `Index all markdown files from a specified directory into a named index.`,
	RunE:  runIndex,
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search indexed documents",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSearch,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all indexes",
	RunE:  runList,
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	RunE:  runStats,
}

var confluenceCmd = &cobra.Command{
	Use:   "confluence",
	Short: "Index Confluence space pages",
	Long:  `Download and index all pages from a Confluence space or starting from a specific page.`,
	RunE:  runConfluence,
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().StringVar(&dataPath, "data", "./hnswdata", "data directory path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	// Index command flags
	indexCmd.Flags().StringVarP(&indexName, "index", "i", "default", "index name")
	indexCmd.Flags().StringP("dir", "d", "./", "directory containing markdown files")
	indexCmd.MarkFlagRequired("dir")

	// Search command flags
	searchCmd.Flags().StringVarP(&indexName, "index", "i", "default", "index name")
	searchCmd.Flags().IntP("limit", "l", 5, "number of results")

	// Stats command flags
	statsCmd.Flags().StringVarP(&indexName, "index", "i", "", "index name (empty for all)")

	// Confluence command flags
	confluenceCmd.Flags().StringP("space", "s", "", "Confluence space key (required)")
	confluenceCmd.Flags().StringP("url", "u", "", "Confluence base URL (required)")
	confluenceCmd.Flags().String("username", "", "Confluence username (or use CONFLUENCE_USERNAME env)")
	confluenceCmd.Flags().String("token", "", "Confluence API token (or use CONFLUENCE_API_TOKEN env)")
	confluenceCmd.Flags().StringVarP(&indexName, "index", "i", "confluence", "Index name")
	confluenceCmd.Flags().String("root-page", "", "Optional: Start from specific page ID and its children")
	confluenceCmd.MarkFlagRequired("space")
	confluenceCmd.MarkFlagRequired("url")

	// Add commands
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(confluenceCmd)

	// Bind flags to viper
	viper.BindPFlag("data_path", rootCmd.PersistentFlags().Lookup("data"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("HNSW")
	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("data_path", "./hnswdata")
	viper.SetDefault("ollama_url", "http://localhost:11434")
	viper.SetDefault("embed_model", "nomic-embed-text")
	viper.SetDefault("chunk_size", 512)
	viper.SetDefault("chunk_overlap", 50)
	viper.SetDefault("max_workers", 8)
	viper.SetDefault("auto_save", true)

	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	// Configure logging
	configureLogging()
}

func runIndex(cmd *cobra.Command, args []string) error {
	dir, _ := cmd.Flags().GetString("dir")
	
	// Create index manager
	config := hnswindex.NewConfig()
	config.DataPath = viper.GetString("data_path")
	config.OllamaURL = viper.GetString("ollama_url")
	config.EmbedModel = viper.GetString("embed_model")
	config.ChunkSize = viper.GetInt("chunk_size")
	config.ChunkOverlap = viper.GetInt("chunk_overlap")
	config.MaxWorkers = viper.GetInt("max_workers")
	config.AutoSave = viper.GetBool("auto_save")

	manager, err := hnswindex.NewIndexManager(config)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}
	defer manager.Close()

	// Get or create index
	index, err := manager.GetIndex(indexName)
	if err != nil {
		if verbose {
			fmt.Printf("Creating new index: %s\n", indexName)
		}
		index, err = manager.CreateIndex(indexName)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Find all markdown files
	var documents []hnswindex.Document
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-markdown files
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to read %s: %v\n", path, err)
			}
			return nil
		}

		// Create document
		relPath, _ := filepath.Rel(dir, path)
		doc := hnswindex.Document{
			URI:     fmt.Sprintf("file://%s", path),
			Title:   filepath.Base(path),
			Content: string(content),
			Metadata: map[string]interface{}{
				"path":     path,
				"rel_path": relPath,
				"size":     len(content),
			},
		}
		documents = append(documents, doc)

		if verbose {
			fmt.Printf("Found: %s (%d bytes)\n", relPath, len(content))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	if len(documents) == 0 {
		fmt.Println("No markdown files found")
		return nil
	}

	// Index documents
	fmt.Printf("Indexing %d documents...\n", len(documents))
	result, err := index.AddDocumentBatch(documents)
	if err != nil {
		return fmt.Errorf("failed to index documents: %w", err)
	}

	// Print results
	fmt.Printf("\nIndexing Results:\n")
	fmt.Printf("  Total documents: %d\n", result.TotalDocuments)
	fmt.Printf("  New documents: %d\n", result.NewDocuments)
	fmt.Printf("  Updated documents: %d\n", result.UpdatedDocuments)
	fmt.Printf("  Unchanged documents: %d\n", result.UnchangedDocuments)
	fmt.Printf("  Processed chunks: %d\n", result.ProcessedChunks)

	if len(result.FailedURIs) > 0 {
		fmt.Printf("\n  Failed documents:\n")
		for uri, err := range result.FailedURIs {
			fmt.Printf("    - %s: %s\n", uri, err)
		}
	}

	return nil
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")
	limit, _ := cmd.Flags().GetInt("limit")

	// Create index manager
	config := hnswindex.NewConfig()
	config.DataPath = viper.GetString("data_path")
	config.OllamaURL = viper.GetString("ollama_url")
	config.EmbedModel = viper.GetString("embed_model")

	manager, err := hnswindex.NewIndexManager(config)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}
	defer manager.Close()

	// Get index
	index, err := manager.GetIndex(indexName)
	if err != nil {
		return fmt.Errorf("index '%s' not found: %w", indexName, err)
	}

	// Search
	fmt.Printf("Searching for: %s\n\n", query)
	results, err := index.Search(query, limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No results found")
		return nil
	}

	// Display results
	for i, result := range results {
		fmt.Printf("%d. %s (Score: %.3f)\n", i+1, result.Document.Title, result.Score)
		if path, ok := result.Document.Metadata["path"].(string); ok {
			fmt.Printf("   Path: %s\n", path)
		}
		
		// Show chunk preview
		preview := result.ChunkText
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Printf("   Preview: %s\n\n", preview)
	}

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	config := hnswindex.NewConfig()
	config.DataPath = viper.GetString("data_path")

	manager, err := hnswindex.NewIndexManager(config)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}
	defer manager.Close()

	indexes, err := manager.ListIndexes()
	if err != nil {
		return fmt.Errorf("failed to list indexes: %w", err)
	}

	if len(indexes) == 0 {
		fmt.Println("No indexes found")
		return nil
	}

	fmt.Println("Available indexes:")
	for _, name := range indexes {
		fmt.Printf("  - %s\n", name)
	}

	return nil
}

func runStats(cmd *cobra.Command, args []string) error {
	config := hnswindex.NewConfig()
	config.DataPath = viper.GetString("data_path")

	manager, err := hnswindex.NewIndexManager(config)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}
	defer manager.Close()

	if indexName == "" {
		// Show stats for all indexes
		indexes, err := manager.ListIndexes()
		if err != nil {
			return fmt.Errorf("failed to list indexes: %w", err)
		}

		for _, name := range indexes {
			showIndexStats(manager, name)
			fmt.Println()
		}
	} else {
		// Show stats for specific index
		return showIndexStats(manager, indexName)
	}

	return nil
}

func showIndexStats(manager *hnswindex.IndexManager, name string) error {
	index, err := manager.GetIndex(name)
	if err != nil {
		return fmt.Errorf("index '%s' not found: %w", name, err)
	}

	stats, err := index.Stats()
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	fmt.Printf("Index: %s\n", stats.Name)
	fmt.Printf("  Documents: %d\n", stats.DocumentCount)
	fmt.Printf("  Chunks: %d\n", stats.ChunkCount)
	fmt.Printf("  Last updated: %s\n", stats.LastUpdated)
	if stats.SizeBytes > 0 {
		fmt.Printf("  Size: %.2f MB\n", float64(stats.SizeBytes)/(1024*1024))
	}

	return nil
}

func runConfluence(cmd *cobra.Command, args []string) error {
	spaceKey, _ := cmd.Flags().GetString("space")
	baseURL, _ := cmd.Flags().GetString("url")
	username, _ := cmd.Flags().GetString("username")
	apiToken, _ := cmd.Flags().GetString("token")
	rootPage, _ := cmd.Flags().GetString("root-page")
	
	// Get credentials from environment if not provided
	if username == "" {
		username = os.Getenv("CONFLUENCE_USERNAME")
		if username == "" {
			return fmt.Errorf("username required: provide via --username flag or CONFLUENCE_USERNAME environment variable")
		}
	}
	if apiToken == "" {
		apiToken = os.Getenv("CONFLUENCE_API_TOKEN")
		if apiToken == "" {
			return fmt.Errorf("API token required: provide via --token flag or CONFLUENCE_API_TOKEN environment variable")
		}
	}
	
	// Create downloader
	fmt.Printf("Connecting to Confluence at %s...\n", baseURL)
	downloader, err := confluence.NewConfluenceDownloader(baseURL, username, apiToken, spaceKey)
	if err != nil {
		return fmt.Errorf("failed to create Confluence downloader: %w", err)
	}
	
	// Download pages
	var documents []hnswindex.Document
	if rootPage != "" {
		fmt.Printf("Downloading page tree from page %s in space %s...\n", rootPage, spaceKey)
		documents, err = downloader.DownloadPageTree(rootPage)
	} else {
		fmt.Printf("Downloading all pages from space %s...\n", spaceKey)
		documents, err = downloader.DownloadSpace()
	}
	
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	
	if len(documents) == 0 {
		fmt.Println("No pages found to index")
		return nil
	}
	
	fmt.Printf("Downloaded %d pages\n", len(documents))
	
	// Create index manager
	config := hnswindex.NewConfig()
	config.DataPath = viper.GetString("data_path")
	config.OllamaURL = viper.GetString("ollama_url")
	config.EmbedModel = viper.GetString("embed_model")
	config.ChunkSize = viper.GetInt("chunk_size")
	config.ChunkOverlap = viper.GetInt("chunk_overlap")
	config.MaxWorkers = viper.GetInt("max_workers")
	config.AutoSave = viper.GetBool("auto_save")
	
	manager, err := hnswindex.NewIndexManager(config)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}
	defer manager.Close()
	
	// Get or create index
	index, err := manager.GetIndex(indexName)
	if err != nil {
		if verbose {
			fmt.Printf("Creating new index: %s\n", indexName)
		}
		index, err = manager.CreateIndex(indexName)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}
	
	// Index documents
	fmt.Printf("Indexing %d documents into '%s'...\n", len(documents), indexName)
	result, err := index.AddDocumentBatch(documents)
	if err != nil {
		return fmt.Errorf("failed to index documents: %w", err)
	}
	
	// Print results
	fmt.Printf("\nIndexing Results:\n")
	fmt.Printf("  Total documents: %d\n", result.TotalDocuments)
	fmt.Printf("  New documents: %d\n", result.NewDocuments)
	fmt.Printf("  Updated documents: %d\n", result.UpdatedDocuments)
	fmt.Printf("  Unchanged documents: %d\n", result.UnchangedDocuments)
	fmt.Printf("  Processed chunks: %d\n", result.ProcessedChunks)
	
	if len(result.FailedURIs) > 0 {
		fmt.Printf("\n  Failed documents:\n")
		for uri, err := range result.FailedURIs {
			fmt.Printf("    - %s: %s\n", uri, err)
		}
	}
	
	fmt.Printf("\nConfluence pages indexed successfully!\n")
	fmt.Printf("Use './demo search --index %s \"your query\"' to search\n", indexName)
	
	return nil
}

func configureLogging() {
	var level slog.Level

	// Debug flag overrides log-level
	if debug {
		level = slog.LevelDebug
	} else {
		switch strings.ToLower(logLevel) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			level = slog.LevelInfo
		}
	}

	// Create a text handler with the specified level
	opts := &slog.HandlerOptions{
		Level: level,
	}

	// If verbose or debug, include source information
	if verbose || debug {
		opts.AddSource = true
	}

	handler := slog.NewTextHandler(os.Stderr, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	if verbose || debug {
		slog.Info("Logging configured",
			"level", level.String(),
			"debug", debug,
			"verbose", verbose,
		)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}