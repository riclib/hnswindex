package confluence

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	goconfluence "github.com/virtomize/confluence-go-api"
	"github.com/riclib/hnswindex"
)

// ConfluenceDownloader downloads and converts Confluence pages
type ConfluenceDownloader struct {
	client   *goconfluence.API
	spaceKey string
	baseURL  string
	converter *md.Converter
}

// NewConfluenceDownloader creates a new Confluence downloader
func NewConfluenceDownloader(baseURL, username, apiToken, spaceKey string) (*ConfluenceDownloader, error) {
	// Initialize the Confluence API client
	// The library expects the REST API endpoint path
	apiURL := strings.TrimSuffix(baseURL, "/") + "/wiki/rest/api"
	
	client, err := goconfluence.NewAPI(apiURL, username, apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Confluence client: %w", err)
	}
	
	// Create markdown converter with options
	converter := md.NewConverter("", true, nil)
	
	return &ConfluenceDownloader{
		client:    client,
		spaceKey:  spaceKey,
		baseURL:   baseURL,
		converter: converter,
	}, nil
}

// DownloadSpace downloads all pages from a Confluence space
func (cd *ConfluenceDownloader) DownloadSpace() ([]hnswindex.Document, error) {
	slog.Info("Starting Confluence space download",
		"space", cd.spaceKey,
		"url", cd.baseURL,
	)
	
	var documents []hnswindex.Document
	
	// Get all content from the space using pagination
	query := goconfluence.ContentQuery{
		SpaceKey: cd.spaceKey,
		Type:     "page",
		Expand:   []string{"body.storage", "metadata.labels", "version", "ancestors"},
		Limit:    50, // Reasonable batch size
		Start:    0,
	}
	
	totalPages := 0
	for {
		slog.Debug("Fetching pages batch",
			"start", query.Start,
			"limit", query.Limit,
		)
		
		content, err := cd.client.GetContent(query)
		if err != nil {
			return nil, fmt.Errorf("failed to get content: %w", err)
		}
		
		// Convert each page to document
		for _, page := range content.Results {
			doc := cd.convertToDocument(&page)
			documents = append(documents, doc)
			totalPages++
			
			slog.Debug("Downloaded page",
				"title", page.Title,
				"id", page.ID,
				"space", cd.spaceKey,
			)
		}
		
		// Check if there are more pages
		if len(content.Results) < query.Limit {
			break // No more pages
		}
		
		query.Start += query.Limit
		
		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}
	
	slog.Info("Confluence space download complete",
		"space", cd.spaceKey,
		"total_pages", totalPages,
	)
	
	return documents, nil
}

// DownloadPageTree downloads a page and all its children recursively
func (cd *ConfluenceDownloader) DownloadPageTree(rootPageID string) ([]hnswindex.Document, error) {
	slog.Info("Starting Confluence page tree download",
		"root_page", rootPageID,
		"space", cd.spaceKey,
	)
	
	var documents []hnswindex.Document
	
	// Get the root page with full content
	rootQuery := goconfluence.ContentQuery{
		Expand: []string{"body.storage", "metadata.labels", "version", "ancestors"},
	}
	
	rootPage, err := cd.client.GetContentByID(rootPageID, rootQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get root page: %w", err)
	}
	
	// Convert and add root page
	documents = append(documents, cd.convertToDocument(rootPage))
	
	slog.Debug("Downloaded root page",
		"title", rootPage.Title,
		"id", rootPage.ID,
	)
	
	// Recursively get child pages
	childDocs, err := cd.getChildPagesRecursive(rootPageID, 0)
	if err != nil {
		return nil, err
	}
	
	documents = append(documents, childDocs...)
	
	slog.Info("Page tree download complete",
		"root_page", rootPageID,
		"total_pages", len(documents),
	)
	
	return documents, nil
}

// getChildPagesRecursive recursively downloads child pages
func (cd *ConfluenceDownloader) getChildPagesRecursive(pageID string, depth int) ([]hnswindex.Document, error) {
	var documents []hnswindex.Document
	
	// Limit recursion depth to prevent infinite loops
	if depth > 10 {
		slog.Warn("Maximum recursion depth reached",
			"parent_id", pageID,
			"depth", depth,
		)
		return documents, nil
	}
	
	// Get child pages
	children, err := cd.client.GetChildPages(pageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get child pages for %s: %w", pageID, err)
	}
	
	if len(children.Results) == 0 {
		return documents, nil
	}
	
	slog.Debug("Found child pages",
		"parent_id", pageID,
		"count", len(children.Results),
		"depth", depth,
	)
	
	for _, child := range children.Results {
		// Get full content with body
		query := goconfluence.ContentQuery{
			Expand: []string{"body.storage", "metadata.labels", "version", "ancestors"},
		}
		
		page, err := cd.client.GetContentByID(child.ID, query)
		if err != nil {
			slog.Warn("Failed to get child page content",
				"id", child.ID,
				"title", child.Title,
				"error", err,
			)
			continue
		}
		
		documents = append(documents, cd.convertToDocument(page))
		
		// Rate limiting
		time.Sleep(100 * time.Millisecond)
		
		// Recursively get children of this page
		childDocs, err := cd.getChildPagesRecursive(child.ID, depth+1)
		if err != nil {
			slog.Warn("Failed to get nested children",
				"parent_id", child.ID,
				"error", err,
			)
			continue
		}
		
		documents = append(documents, childDocs...)
	}
	
	return documents, nil
}

// convertToDocument converts Confluence content to hnswindex.Document
func (cd *ConfluenceDownloader) convertToDocument(content *goconfluence.Content) hnswindex.Document {
	// Convert HTML storage format to markdown
	var bodyContent string
	if content.Body.Storage.Value != "" {
		bodyContent = cd.htmlToMarkdown(content.Body.Storage.Value)
	} else {
		slog.Warn("Page has no storage body",
			"id", content.ID,
			"title", content.Title,
		)
		bodyContent = ""
	}
	
	// Build metadata
	metadata := map[string]interface{}{
		"space_key": cd.spaceKey,
		"page_id":   content.ID,
		"type":      content.Type,
		"status":    content.Status,
	}
	
	// Add version info if available
	if content.Version != nil {
		metadata["version"] = content.Version.Number
		if content.Version.By != nil {
			metadata["modified_by"] = content.Version.By.Username
		}
		metadata["modified_date"] = content.Version.When
	}
	
	// Note: Labels need to be fetched separately using GetLabels API
	// This could be added as an enhancement if needed
	
	// Add ancestors for hierarchy context
	if content.Ancestors != nil && len(content.Ancestors) > 0 {
		// Ancestors only contain IDs in this API
		var ancestorIDs []string
		for _, ancestor := range content.Ancestors {
			ancestorIDs = append(ancestorIDs, ancestor.ID)
		}
		metadata["ancestor_ids"] = ancestorIDs
		metadata["parent_id"] = content.Ancestors[len(content.Ancestors)-1].ID
	}
	
	// Build confluence URL
	pageURL := fmt.Sprintf("%s/wiki/spaces/%s/pages/%s", 
		strings.TrimSuffix(cd.baseURL, "/"), cd.spaceKey, content.ID)
	metadata["url"] = pageURL
	
	// Create document with title and content
	fullContent := fmt.Sprintf("# %s\n\n%s", content.Title, bodyContent)
	
	return hnswindex.Document{
		URI:      fmt.Sprintf("confluence://%s/%s", cd.spaceKey, content.ID),
		Title:    content.Title,
		Content:  fullContent,
		Metadata: metadata,
	}
}

// htmlToMarkdown converts Confluence HTML storage format to markdown
func (cd *ConfluenceDownloader) htmlToMarkdown(html string) string {
	if html == "" {
		return ""
	}
	
	// Pre-process: Clean Confluence-specific HTML
	html = cd.cleanConfluenceHTML(html)
	
	// Convert to markdown
	markdown, err := cd.converter.ConvertString(html)
	if err != nil {
		slog.Warn("Failed to convert HTML to markdown, falling back to plain text",
			"error", err,
		)
		return cd.htmlToPlainText(html)
	}
	
	// Post-process: Clean up the markdown
	markdown = cd.cleanMarkdown(markdown)
	
	return markdown
}

// cleanConfluenceHTML removes Confluence-specific markup
func (cd *ConfluenceDownloader) cleanConfluenceHTML(html string) string {
	// Remove Confluence structured macros
	reStructuredMacro := regexp.MustCompile(`(?s)<ac:structured-macro[^>]*>.*?</ac:structured-macro>`)
	html = reStructuredMacro.ReplaceAllString(html, "")
	
	// Remove other ac: tags
	reAcTags := regexp.MustCompile(`</?ac:[^>]+>`)
	html = reAcTags.ReplaceAllString(html, "")
	
	// Remove ri: (resource identifier) tags
	reRiTags := regexp.MustCompile(`<ri:[^>]+/>`)
	html = reRiTags.ReplaceAllString(html, "")
	
	// Convert Confluence code blocks to standard HTML
	html = strings.ReplaceAll(html, `<ac:plain-text-body><![CDATA[`, "<pre><code>")
	html = strings.ReplaceAll(html, `]]></ac:plain-text-body>`, "</code></pre>")
	
	// Handle Confluence-specific line breaks
	html = strings.ReplaceAll(html, `<br />`, "\n")
	html = strings.ReplaceAll(html, `<br/>`, "\n")
	
	// Remove empty paragraphs that Confluence sometimes creates
	reEmptyP := regexp.MustCompile(`<p>\s*</p>`)
	html = reEmptyP.ReplaceAllString(html, "")
	
	return html
}

// cleanMarkdown cleans up converted markdown
func (cd *ConfluenceDownloader) cleanMarkdown(markdown string) string {
	// Remove excessive newlines (more than 2 in a row)
	reMultiNewline := regexp.MustCompile(`\n{3,}`)
	markdown = reMultiNewline.ReplaceAllString(markdown, "\n\n")
	
	// Remove trailing spaces from lines
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	markdown = strings.Join(lines, "\n")
	
	// Remove leading/trailing whitespace
	markdown = strings.TrimSpace(markdown)
	
	// Fix common markdown conversion artifacts
	// Remove escaped underscores in URLs
	reEscapedUnderscore := regexp.MustCompile(`\\_`)
	markdown = reEscapedUnderscore.ReplaceAllString(markdown, "_")
	
	// Fix double-escaped characters
	markdown = strings.ReplaceAll(markdown, `\\`, `\`)
	
	return markdown
}

// htmlToPlainText fallback converter for when markdown conversion fails
func (cd *ConfluenceDownloader) htmlToPlainText(html string) string {
	// Remove script and style tags with content
	reScript := regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	html = reScript.ReplaceAllString(html, "")
	
	reStyle := regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	html = reStyle.ReplaceAllString(html, "")
	
	// Replace common tags with text equivalents
	html = strings.ReplaceAll(html, "</p>", "\n\n")
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")
	html = strings.ReplaceAll(html, "</div>", "\n")
	html = strings.ReplaceAll(html, "</li>", "\n")
	
	// Add spacing around headers
	reHeaders := regexp.MustCompile(`<h[1-6][^>]*>`)
	html = reHeaders.ReplaceAllString(html, "\n\n")
	reHeadersEnd := regexp.MustCompile(`</h[1-6]>`)
	html = reHeadersEnd.ReplaceAllString(html, "\n\n")
	
	// Remove remaining HTML tags
	reTags := regexp.MustCompile(`<[^>]+>`)
	html = reTags.ReplaceAllString(html, "")
	
	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&mdash;", "—")
	html = strings.ReplaceAll(html, "&ndash;", "–")
	
	// Clean up whitespace
	reMultiSpace := regexp.MustCompile(`[ \t]+`)
	html = reMultiSpace.ReplaceAllString(html, " ")
	
	reMultiNewline := regexp.MustCompile(`\n{3,}`)
	html = reMultiNewline.ReplaceAllString(html, "\n\n")
	
	return strings.TrimSpace(html)
}