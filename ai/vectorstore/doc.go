// Package vectorstore provides a unified abstraction layer for vector database operations,
// enabling seamless integration with multiple vector store providers through a consistent API.
//
// # Overview
//
// The vectorstore package offers a high-level interface for managing document storage and
// retrieval in vector databases. It abstracts away provider-specific implementation details
// while providing type-safe operations for creating, retrieving, and deleting documents based
// on semantic similarity and metadata filtering.
//
// Key features:
//   - Unified interface for multiple vector database providers (Pinecone, Weaviate, Milvus, etc.)
//   - Type-safe metadata filtering using the filter package
//   - Similarity-based document retrieval with configurable thresholds
//   - Batch operations support for efficient document processing
//   - Provider capability discovery for feature detection
//
// # Architecture
//
// The package follows the Interface Segregation Principle, providing three core interfaces
// that can be composed to create a complete VectorStore:
//
//	┌─────────────────────────────────────────────────────────┐
//	│                    VectorStore                          │
//	│  (Combines Creator, Retriever, Deleter)                │
//	└─────────────────────────────────────────────────────────┘
//	                         ↓
//	┌──────────────┬─────────────────┬──────────────────────┐
//	│   Creator    │    Retriever    │      Deleter         │
//	│  (Write)     │    (Search)     │     (Delete)         │
//	└──────────────┴─────────────────┴──────────────────────┘
//	                         ↓
//	┌─────────────────────────────────────────────────────────┐
//	│              Provider Implementations                    │
//	│  Pinecone, Weaviate, Milvus, Qdrant, Chroma...         │
//	└─────────────────────────────────────────────────────────┘
//
// # Core Interfaces
//
// Creator - Document Creation Interface:
//
// The Creator interface handles document insertion into the vector store. Documents are
// automatically embedded (converted to vector representations) and indexed for similarity search.
//
//	type Creator interface {
//	    Create(ctx context.Context, request *CreateRequest) error
//	}
//
// Retriever - Document Retrieval Interface:
//
// The Retriever interface performs similarity-based document search. Results are ranked by
// semantic similarity and can be filtered by metadata and similarity thresholds.
//
//	type Retriever interface {
//	    Retrieve(ctx context.Context, request *RetrievalRequest) ([]*document.Document, error)
//	}
//
// Deleter - Document Deletion Interface:
//
// The Deleter interface removes documents from the vector store based on metadata filtering.
// This enables selective deletion of documents matching specific criteria.
//
//	type Deleter interface {
//	    Delete(ctx context.Context, request *DeleteRequest) error
//	}
//
// VectorStore - Complete Vector Store Interface:
//
// The VectorStore interface combines all three core interfaces plus metadata access,
// providing a complete solution for vector store operations.
//
//	type VectorStore interface {
//	    Creator
//	    Retriever
//	    Deleter
//	    Info() StoreInfo
//	}
//
// # Request Objects
//
// The package uses strongly-typed request objects for all operations, providing:
//   - Compile-time type safety
//   - Clear API contracts
//   - Comprehensive validation
//   - Fluent builder patterns
//
// RetrievalRequest - Similarity Search Parameters:
//
// Configures document retrieval based on query similarity and metadata filtering:
//
//	type RetrievalRequest struct {
//	    Query    string      // Search query text
//	    TopK     int         // Maximum results to return (default: 5)
//	    MinScore float64     // Minimum similarity threshold (range: 0.0-1.0)
//	    Filter   ast.Expr    // Optional metadata filter expression
//	}
//
// CreateRequest - Document Creation Parameters:
//
// Specifies documents to be embedded and indexed in the vector store:
//
//	type CreateRequest struct {
//	    Documents []*document.Document  // Documents to create
//	}
//
// DeleteRequest - Document Deletion Parameters:
//
// Defines which documents to delete based on metadata filtering:
//
//	type DeleteRequest struct {
//	    Filter ast.Expr  // Filter expression for document selection
//	}
//
// # Constants and Defaults
//
// The package defines sensible defaults and limits for common operations:
//
//	DefaultTopK            = 5    // Default maximum results
//	MinSimilarityScore    = 0.0  // Minimum valid similarity score
//	MaxSimilarityScore    = 1.0  // Maximum valid similarity score
//	AcceptAllScores       = 0.0  // Accept all results regardless of score
//
// Similarity scores represent the semantic closeness between the query and documents,
// where 1.0 indicates perfect similarity and 0.0 indicates no similarity. The actual
// score calculation depends on the vector store's distance metric (cosine, euclidean, etc.).
//
// # Basic Usage Examples
//
// ## Creating Documents
//
// Store documents in the vector store with automatic embedding generation:
//
//	// Create vector store instance (provider-specific)
//	store := newPineconeStore(config)
//
//	// Prepare documents
//	docs := []*document.Document{
//	    document.New("Introduction to machine learning"),
//	    document.New("Deep learning fundamentals"),
//	}
//
//	// Create request
//	req, err := vectorstore.NewCreateRequest(docs)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Store documents
//	if err := store.Create(ctx, req); err != nil {
//	    log.Fatal(err)
//	}
//
// ## Retrieving Documents
//
// Search for semantically similar documents with configurable parameters:
//
//	// Create retrieval request
//	req, err := vectorstore.NewRetrievalRequest("machine learning algorithms")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Configure search parameters
//	req.WithTopK(10).           // Return top 10 results
//	    WithMinScore(0.7)       // Filter by similarity threshold
//
//	// Execute search
//	results, err := store.Retrieve(ctx, req)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Process results
//	for _, doc := range results {
//	    fmt.Printf("Content: %s\nScore: %.2f\n\n", doc.Content, doc.Score)
//	}
//
// ## Retrieving with Metadata Filtering
//
// Combine similarity search with metadata-based filtering using the filter package:
//
//	// Build filter expression
//	filterExpr := filter.And(
//	    filter.EQ("category", "AI"),
//	    filter.GT("year", 2020),
//	)
//
//	// Create request with filter
//	req, _ := vectorstore.NewRetrievalRequest("neural networks")
//	req.WithTopK(5).
//	    WithMinScore(0.8).
//	    WithFilter(filterExpr)
//
//	// Retrieve filtered results
//	results, err := store.Retrieve(ctx, req)
//
// ## Deleting Documents
//
// Remove documents based on metadata criteria:
//
//	// Define deletion criteria
//	deleteFilter := filter.And(
//	    filter.EQ("status", "archived"),
//	    filter.LT("updated_at", "2023-01-01"),
//	)
//
//	// Create delete request
//	req, err := vectorstore.NewDeleteRequest(deleteFilter)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Execute deletion
//	if err := store.Delete(ctx, req); err != nil {
//	    log.Fatal(err)
//	}
//
// # Advanced Usage
//
// ## Fluent Request Building
//
// All request objects support method chaining for convenient configuration:
//
//	// Fluent retrieval request
//	req, _ := vectorstore.NewRetrievalRequest("search query")
//	req.WithTopK(20).
//	    WithMinScore(0.75).
//	    WithFilter(filter.In("tags", []string{"important", "urgent"}))
//
//	results, _ := store.Retrieve(ctx, req)
//
// ## Complex Metadata Filtering
//
// Leverage the full power of the filter package for sophisticated queries:
//
//	// Complex filter with multiple conditions
//	complexFilter := filter.Or(
//	    filter.And(
//	        filter.EQ("category", "research"),
//	        filter.GT("citations", 100),
//	    ),
//	    filter.And(
//	        filter.EQ("category", "tutorial"),
//	        filter.Like("title", "%beginner%"),
//	    ),
//	)
//
//	req, _ := vectorstore.NewRetrievalRequest("machine learning")
//	req.WithFilter(complexFilter)
//
//	results, _ := store.Retrieve(ctx, req)
//
// ## Checking Provider Capabilities
//
// Use StoreInfo to detect supported features and adjust behavior accordingly:
//
//	info := store.Info()
//	fmt.Printf("Provider: %s\n", info.Provider)
//
//	// Access native client for advanced operations
//	if info.Provider == "pinecone" {
//	    nativeClient := info.NativeClient.(*pinecone.Client)
//	    // Use provider-specific features
//	}
//
// ## Document Writer Adapter
//
// Convert a Creator to a document.Writer for seamless integration:
//
//	// Create adapter
//	writer := vectorstore.NewDocumentWriter(store)
//
//	// Use in document processing pipelines
//	processor := document.NewProcessor(writer)
//	if err := processor.Process(documents); err != nil {
//	    log.Fatal(err)
//	}
//
// # Validation and Error Handling
//
// All request objects provide comprehensive validation before execution:
//
//	// Validation is automatic during request creation
//	req, err := vectorstore.NewRetrievalRequest("")
//	// Returns error: "query text cannot be empty"
//
//	req, _ := vectorstore.NewRetrievalRequest("valid query")
//	req.WithTopK(-5)  // Invalid value is silently ignored, TopK remains at default
//
//	// Manual validation
//	if err := req.Validate(); err != nil {
//	    log.Printf("Request validation failed: %v", err)
//	    return
//	}
//
// Validation checks performed:
//
// RetrievalRequest validation:
//   - Request must not be nil
//   - Query text must not be empty
//   - TopK must be greater than 0
//   - MinScore must be within [0.0, 1.0] range
//   - Filter expression must be syntactically valid (if provided)
//
// CreateRequest validation:
//   - Request must not be nil
//   - Documents list must not be empty
//
// DeleteRequest validation:
//   - Request must not be nil
//   - Filter must not be nil (prevents accidental deletion of all documents)
//   - Filter expression must be syntactically valid
//
// Filter Expression Validation:
//
// When a filter expression is provided, it is validated using filter.Analyze() to ensure:
//   - Correct syntax according to the filter grammar
//   - Type compatibility between operators and operands
//   - Valid operator usage (e.g., LT/GT only with numeric types)
//   - Proper nesting and parentheses matching
//
// Example of filter validation:
//
//	// Valid filter
//	validFilter := filter.And(
//	    filter.GT("age", 18),           // ✓ Numeric comparison
//	    filter.EQ("status", "active"),  // ✓ String equality
//	)
//	req.WithFilter(validFilter)
//	err := req.Validate()  // err == nil
//
//	// Invalid filter (type mismatch)
//	invalidFilter := filter.GT("name", "John")  // ✗ Compile error: string not allowed with GT
//
//	// Invalid filter (caught at runtime if constructed from string)
//	expr, _ := filter.Parse("age > 'invalid'")
//	req.WithFilter(expr)
//	err := req.Validate()  // err != nil: type mismatch error
//
// The filter validation ensures that only well-formed, type-safe expressions are passed
// to the vector store, preventing runtime errors and improving query reliability.
//
// # Integration Patterns
//
// ## Partial Interface Implementation
//
// Implement only the interfaces needed for your use case:
//
//	// Read-only vector store
//	type ReadOnlyStore struct {
//	    client *SomeClient
//	}
//
//	func (s *ReadOnlyStore) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
//	    // Implementation
//	}
//
//	func (s *ReadOnlyStore) Info() vectorstore.StoreInfo {
//	    return vectorstore.StoreInfo{
//	        Provider:     "readonly-provider",
//	        NativeClient: s.client,
//	    }
//	}
//
//	// Use as Retriever only
//	var retriever vectorstore.Retriever = &ReadOnlyStore{client}
//
// ## Context-Aware Operations
//
// All operations accept context for timeout and cancellation control:
//
//	// Set timeout
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	// Operations respect context
//	results, err := store.Retrieve(ctx, req)
//	if err != nil {
//	    if errors.Is(err, context.DeadlineExceeded) {
//	        log.Println("Search timed out")
//	    }
//	    return err
//	}
//
// ## Batch Processing
//
// For large document sets, consider batch processing to avoid timeouts and memory issues:
//
//	// Simple batch processing
//	const batchSize = 100
//	for i := 0; i < len(allDocs); i += batchSize {
//	    end := i + batchSize
//	    if end > len(allDocs) {
//	        end = len(allDocs)
//	    }
//
//	    batch := allDocs[i:end]
//	    req, _ := vectorstore.NewCreateRequest(batch)
//	    if err := store.Create(ctx, req); err != nil {
//	        log.Printf("Batch %d failed: %v", i/batchSize, err)
//	        return err
//	    }
//	}
//
// # Similarity Scores
//
// Understanding similarity score ranges and thresholds:
//
// Score Interpretation:
//   - 1.0: Perfect match (identical content)
//   - 0.9-1.0: Very high similarity (near-duplicates)
//   - 0.7-0.9: High similarity (semantically related)
//   - 0.5-0.7: Medium similarity (topically related)
//   - 0.3-0.5: Low similarity (loosely related)
//   - 0.0-0.3: Very low similarity (unrelated)
//
// Recommended thresholds by use case:
//
//	// Duplicate detection
//	req.WithMinScore(0.95)
//
//	// Semantic search
//	req.WithMinScore(0.7)
//
//	// Broad topic matching
//	req.WithMinScore(0.5)
//
//	// Accept all results
//	req.WithMinScore(vectorstore.AcceptAllScores)
//
// Note: Actual score ranges depend on the vector store's distance metric:
//   - Cosine similarity: typically 0.0 to 1.0
//   - Euclidean distance: converted to similarity score
//   - Dot product: normalized to similarity score
//
// # Metadata Filtering
//
// The package integrates with the filter package for powerful metadata-based filtering.
// See the filter package documentation for complete syntax and examples.
//
// Common filtering patterns:
//
//	// Equality
//	filter.EQ("status", "active")
//
//	// Numeric comparison
//	filter.GT("price", 100)
//	filter.LT("age", 18)
//
//	// Membership test
//	filter.In("category", []string{"tech", "science"})
//
//	// Pattern matching
//	filter.Like("title", "%tutorial%")
//
//	// Logical combinations
//	filter.And(
//	    filter.EQ("verified", true),
//	    filter.GT("rating", 4.0),
//	)
//
//	filter.Or(
//	    filter.EQ("priority", "high"),
//	    filter.EQ("urgent", true),
//	)
//
//	// Negation
//	filter.Not(filter.In("status", []string{"deleted", "archived"}))
//
//	// Array/nested field access
//	filter.EQ(filter.Index("metadata", "author"), "John Doe")
//	filter.GT(filter.Index("scores", 0), 90)
//
// # Provider Implementation Guide
//
// To implement a new vector store provider:
//
//	type MyVectorStore struct {
//	    client *MyProviderClient
//	    config Config
//	}
//
//	// Implement Creator
//	func (s *MyVectorStore) Create(ctx context.Context, req *vectorstore.CreateRequest) error {
//	    if err := req.Validate(); err != nil {
//	        return err
//	    }
//
//	    // 1. Synthesize embeddings for documents
//	    embeddings := s.generateEmbeddings(req.Documents)
//
//	    // 2. Store in provider's format
//	    return s.client.Insert(ctx, embeddings)
//	}
//
//	// Implement Retriever
//	func (s *MyVectorStore) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
//	    if err := req.Validate(); err != nil {
//	        return nil, err
//	    }
//
//	    // 1. Synthesize query embedding
//	    queryEmbed := s.generateEmbedding(req.Query)
//
//	    // 2. Convert filter expression to provider's query format
//	    providerFilter := s.convertFilter(req.Filter)
//
//	    // 3. Execute similarity search with filters
//	    results := s.client.Search(ctx, queryEmbed, req.TopK, providerFilter)
//
//	    // 4. Filter by similarity threshold
//	    filtered := filterByScore(results, req.MinScore)
//
//	    // 5. Convert to documents
//	    return s.convertToDocuments(filtered)
//	}
//
//	// Implement Deleter
//	func (s *MyVectorStore) Delete(ctx context.Context, req *vectorstore.DeleteRequest) error {
//	    if err := req.Validate(); err != nil {
//	        return err
//	    }
//
//	    // Convert filter expression to provider's query format
//	    providerQuery := s.convertFilter(req.Filter)
//
//	    return s.client.Delete(ctx, providerQuery)
//	}
//
//	// Implement Info
//	func (s *MyVectorStore) Info() vectorstore.StoreInfo {
//	    return vectorstore.StoreInfo{
//	        NativeClient: s.client,
//	        Provider:     "myprovider",
//	    }
//	}
//
//	// Type assertion to ensure full interface implementation
//	var _ vectorstore.VectorStore = (*MyVectorStore)(nil)
//
// Key implementation considerations:
//   - Filter expression conversion: Transform AST expressions to provider-specific queries
//   - Error handling: Wrap provider errors with context
//   - Validation: Always validate requests before processing
//   - Embeddings: Handle embedding generation or use external service
//   - Batch operations: Implement batching if provider supports it
//
// # Error Handling Best Practices
//
// Handle errors appropriately based on operation type:
//
//	// Creation errors
//	if err := store.Create(ctx, req); err != nil {
//	    // Check for specific error types
//	    if errors.Is(err, context.DeadlineExceeded) {
//	        log.Println("Creation timed out, consider batching")
//	    } else if strings.Contains(err.Error(), "validation") {
//	        log.Println("Invalid request format")
//	    } else {
//	        log.Printf("Creation failed: %v", err)
//	    }
//	    return err
//	}
//
//	// Retrieval errors
//	results, err := store.Retrieve(ctx, req)
//	if err != nil {
//	    // Distinguish between no results and errors
//	    log.Printf("Retrieval failed: %v", err)
//	    return err
//	}
//	if len(results) == 0 {
//	    log.Println("No matching documents found")
//	}
//
//	// Deletion errors
//	if err := store.Delete(ctx, req); err != nil {
//	    log.Printf("Deletion failed: %v", err)
//	    return err
//	}
//
// # Performance Considerations
//
// Optimize vector store operations for production use:
//
// Batching:
//   - Group document operations to reduce network overhead
//   - Typical batch sizes: 50-500 documents depending on provider
//   - Consider provider-specific limits
//
// Concurrency:
//   - Vector stores are generally thread-safe
//   - Use goroutines for parallel batch processing
//   - Respect provider rate limits
//
// Caching:
//   - Cache frequently accessed documents
//   - Cache query embeddings for repeated searches
//   - Consider TTL for cache invalidation
//
// Query Optimization:
//   - Use appropriate MinScore thresholds to reduce result sets
//   - Apply metadata filters to narrow search space before vector search
//   - Use TopK wisely (smaller = faster)
//   - Consider pagination for large result sets
//
// Example optimized batch processing:
//
//	func CreateInBatches(ctx context.Context, store vectorstore.Creator, docs []*document.Document, batchSize int) error {
//	    numBatches := (len(docs) + batchSize - 1) / batchSize
//	    errChan := make(chan error, numBatches)
//	    sem := make(chan struct{}, 5) // Limit concurrency to 5
//
//	    for i := 0; i < len(docs); i += batchSize {
//	        end := i + batchSize
//	        if end > len(docs) {
//	            end = len(docs)
//	        }
//
//	        batch := docs[i:end]
//	        sem <- struct{}{}
//
//	        go func(b []*document.Document) {
//	            defer func() { <-sem }()
//
//	            req, err := vectorstore.NewCreateRequest(b)
//	            if err != nil {
//	                errChan <- err
//	                return
//	            }
//
//	            errChan <- store.Create(ctx, req)
//	        }(batch)
//	    }
//
//	    // Collect errors
//	    for i := 0; i < numBatches; i++ {
//	        if err := <-errChan; err != nil {
//	            return err
//	        }
//	    }
//
//	    return nil
//	}
//
// # Testing
//
// Mock implementations for testing:
//
//	type MockVectorStore struct {
//	    CreateFunc   func(context.Context, *vectorstore.CreateRequest) error
//	    RetrieveFunc func(context.Context, *vectorstore.RetrievalRequest) ([]*document.Document, error)
//	    DeleteFunc   func(context.Context, *vectorstore.DeleteRequest) error
//	}
//
//	func (m *MockVectorStore) Create(ctx context.Context, req *vectorstore.CreateRequest) error {
//	    if m.CreateFunc != nil {
//	        return m.CreateFunc(ctx, req)
//	    }
//	    return nil
//	}
//
//	func (m *MockVectorStore) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
//	    if m.RetrieveFunc != nil {
//	        return m.RetrieveFunc(ctx, req)
//	    }
//	    return nil, nil
//	}
//
//	func (m *MockVectorStore) Delete(ctx context.Context, req *vectorstore.DeleteRequest) error {
//	    if m.DeleteFunc != nil {
//	        return m.DeleteFunc(ctx, req)
//	    }
//	    return nil
//	}
//
//	func (m *MockVectorStore) Info() vectorstore.StoreInfo {
//	    return vectorstore.StoreInfo{Provider: "mock"}
//	}
//
//	// Usage in tests
//	func TestMyFunction(t *testing.T) {
//	    mock := &MockVectorStore{
//	        RetrieveFunc: func(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
//	            return []*document.Document{
//	                document.New("test content"),
//	            }, nil
//	        },
//	    }
//
//	    result := MyFunction(mock)
//	    // Assert on result
//	}
//
// # Related Packages
//
// This package integrates with:
//
//   - filter: Expression-based metadata filtering (github.com/Tangerg/lynx/ai/vectorstore/filter)
//   - document: Document abstraction and processing (github.com/Tangerg/lynx/ai/media/document)
//   - embedding: Vector embedding generation (provider-specific)
//
// # Provider Compatibility
//
// The package is designed to work with various vector database providers:
//
//   - Pinecone: Fully-managed vector database
//   - Weaviate: Open-source vector search engine
//   - Milvus: Open-source vector database
//   - Qdrant: Vector similarity search engine
//   - Chroma: AI-native embedding database
//   - PostgreSQL with pgvector: SQL database with vector support
//   - Redis with RediSearch: In-memory database with vector capabilities
//
// Each provider implementation should handle:
//   - Embedding generation (if not handled externally)
//   - Provider-specific query optimization
//   - Filter expression translation to provider's query language
//   - Error handling and retry logic
//   - Connection management and pooling
//
// # See Also
//
// Related documentation:
//   - filter package: Expression-based filtering syntax and API
//   - document package: Document structure and metadata handling
//   - Provider-specific implementation guides
//
// External resources:
//   - Vector database comparison guide
//   - Embedding model selection guide
//   - Similarity metrics explained
package vectorstore
