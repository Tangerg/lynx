// Package rag provides a comprehensive framework for building Retrieval-Augmented Generation (RAG) systems.
//
// # Overview
//
// RAG (Retrieval-Augmented Generation) is a technique that enhances large language model (LLM)
// responses by retrieving relevant information from external knowledge sources before generating
// answers. This approach combines the power of information retrieval with the generative
// capabilities of LLMs, enabling more accurate, grounded, and up-to-date responses.
//
// This package implements a flexible, production-ready RAG pipeline with support for query
// transformation, expansion, document retrieval, refinement, and context augmentation. The
// framework is designed with extensibility and performance in mind, supporting parallel
// execution, fault tolerance, and seamless integration with chat models.
//
// # Key Features
//
//   - Modular Architecture: Five distinct stages with pluggable components
//   - Parallel Execution: Concurrent document retrieval from multiple sources
//   - Fault Tolerance: Graceful degradation when some retrievers fail
//   - Context Management: Automatic propagation of metadata and conversation history
//   - Multilingual Support: Built-in translation and language handling
//   - Streaming Support: Real-time response generation with document context
//   - Thread-Safe: Designed for concurrent use in production systems
//   - Easy Integration: Middleware pattern for seamless chat model integration
//
// # Architecture
//
// The RAG framework processes queries through five sequential stages, each addressing
// specific challenges in information retrieval and response generation:
//
//	User Query
//	    ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│ Stage 1: Query Transformation (Sequential Pipeline)         │
//	│                                                              │
//	│ Purpose: Normalize and optimize the query for retrieval     │
//	│ Examples:                                                    │
//	│  • Translate to target language                             │
//	│  • Remove noise and redundant words                         │
//	│  • Compress conversation history                            │
//	│                                                              │
//	│ Flow: query → Transformer 1 → Transformer 2 → ... → Tn     │
//	└─────────────────────────────────────────────────────────────┘
//	    ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│ Stage 2: Query Expansion (One to Many)                      │
//	│                                                              │
//	│ Purpose: Generate query variants to improve recall          │
//	│ Examples:                                                    │
//	│  • Rephrase from different perspectives                     │
//	│  • Generate semantically similar questions                  │
//	│  • Break down complex queries into sub-questions            │
//	│                                                              │
//	│ Flow: single query → [variant 1, variant 2, ..., variant N]│
//	└─────────────────────────────────────────────────────────────┘
//	    ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│ Stage 3: Document Retrieval (Parallel Execution)            │
//	│                                                              │
//	│ Purpose: Fetch relevant documents from multiple sources     │
//	│ Features:                                                    │
//	│  • Parallel execution across retrievers                     │
//	│  • Support for hybrid search strategies                     │
//	│  • Automatic result merging                                 │
//	│                                                              │
//	│ Flow:                                                        │
//	│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
//	│   │ Vector Store │  │  BM25 Search │  │ Graph Store  │   │
//	│   └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
//	│          └──────────────────┴────────────────┘             │
//	│                    Merged Documents                         │
//	└─────────────────────────────────────────────────────────────┘
//	    ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│ Stage 4: Document Refinement (Sequential Pipeline)          │
//	│                                                              │
//	│ Purpose: Improve quality and reduce noise in results        │
//	│ Examples:                                                    │
//	│  • Remove duplicate documents                               │
//	│  • Rerank by relevance                                      │
//	│  • Filter by threshold                                      │
//	│  • Compress content to fit context window                   │
//	│                                                              │
//	│ Flow: docs → Refiner 1 → Refiner 2 → ... → Rn             │
//	└─────────────────────────────────────────────────────────────┘
//	    ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│ Stage 5: Query Augmentation (Context Injection)             │
//	│                                                              │
//	│ Purpose: Enrich query with retrieved document context       │
//	│ Process:                                                     │
//	│  • Format documents into context text                       │
//	│  • Inject context into prompt template                      │
//	│  • Add instructions to prevent hallucination                │
//	│                                                              │
//	│ Flow: original query + refined docs → augmented prompt      │
//	└─────────────────────────────────────────────────────────────┘
//	    ↓
//	Final Output: (Augmented Query, Retrieved Documents)
//	    ↓
//	LLM Generation
//
// Each stage is optional except for document retrieval, allowing you to configure
// the pipeline based on your specific requirements. For example, a simple RAG
// system might only use retrieval and augmentation, while a sophisticated system
// might employ all stages with multiple components in each.
//
// # Core Interfaces
//
// The framework defines five core interfaces that represent different stages of
// the RAG pipeline. Each interface is designed to be simple, composable, and
// easy to test.
//
// QueryTransformer transforms the input query to make it more effective for
// retrieval tasks. It addresses challenges such as:
//   - Poorly formed or ambiguous queries
//   - Language mismatches between query and documents
//   - Verbose queries with irrelevant information
//   - Multi-turn conversations requiring context compression
//
// Transformers are applied sequentially in the order they are configured,
// allowing you to build complex transformation pipelines. Each transformer
// receives the output of the previous transformer as its input.
//
// Built-in implementations:
//   - TranslationTransformer: Translates queries to match document language
//   - RewriteTransformer: Optimizes queries for specific search systems
//   - CompressionTransformer: Compresses conversation history into standalone queries
//
// QueryExpander expands a single query into multiple variations. This increases
// recall by exploring different semantic angles and perspectives. It addresses:
//   - Limited coverage of narrow queries
//   - Ambiguous intent that could be interpreted multiple ways
//   - Complex questions that benefit from decomposition
//
// The expander generates alternative formulations while maintaining the core
// intent of the original query. This is particularly useful when the exact
// phrasing of the query might not match the indexed documents.
//
// Built-in implementations:
//   - MultiExpander: Uses LLMs to generate semantically diverse query variants
//
// DocumentRetriever retrieves documents from underlying data sources based on
// the query. Multiple retrievers can be configured to run in parallel, enabling
// hybrid search strategies that combine different retrieval methods.
//
// Retrievers execute concurrently to minimize latency. The framework automatically
// merges results from all retrievers and handles partial failures gracefully.
// If some retrievers fail, results from successful retrievers are still returned.
//
// Built-in implementations:
//   - VectorStoreRetriever: Performs semantic search using vector embeddings
//
// Common retrieval strategies:
//   - Semantic search: Vector similarity for conceptual matches
//   - Keyword search: BM25 for exact term matches
//   - Hybrid search: Combine multiple methods for best recall
//   - Graph retrieval: Traverse knowledge graphs for related entities
//
// DocumentRefiner refines retrieved documents to improve quality and reduce
// noise. It addresses challenges such as:
//   - Duplicate documents from multiple retrievers
//   - Low-quality or irrelevant results
//   - Context window limitations requiring compression
//   - "Lost in the middle" effect where LLMs miss information
//
// Refiners are applied sequentially in the order they are configured. Each
// refiner receives the output of the previous refiner, allowing you to build
// complex refinement pipelines.
//
// Built-in implementations:
//   - DeduplicationRefiner: Removes duplicate documents based on IDs
//   - RankRefiner: Sorts by relevance and returns top-K results
//
// Common refinement strategies:
//   - Deduplication: Remove redundant information
//   - Reranking: Use advanced models to improve relevance ordering
//   - Filtering: Remove low-quality or off-topic results
//   - Compression: Reduce document length while preserving key information
//
// QueryAugmenter augments the original query with contextual data from retrieved
// documents. This is the final stage before LLM generation, where we inject the
// retrieved context into a structured prompt.
//
// The augmenter provides the LLM with:
//   - Retrieved document content as context
//   - Instructions to ground responses in provided context
//   - Guidelines to avoid hallucination
//   - Formatting to optimize LLM comprehension
//
// Built-in implementations:
//   - ContextualAugmenter: Injects document context into structured prompts
//
// # Pipeline Configuration
//
// The Pipeline orchestrates the complete RAG workflow by executing all stages
// in sequence. It handles error propagation, parallel execution, and context
// management automatically.
//
// Basic configuration:
//
//	pipeline, err := rag.NewPipeline(&rag.PipelineConfig{
//	    // Optional: Transform queries before retrieval
//	    QueryTransformers: []rag.QueryTransformer{
//	        translationTransformer,  // Translate to target language
//	        rewriteTransformer,      // Optimize for vector search
//	    },
//
//	    // Optional: Expand single query into variants
//	    QueryExpander: multiExpander,
//
//	    // Required: At least one retriever must be configured
//	    DocumentRetrievers: []rag.DocumentRetriever{
//	        vectorStoreRetriever,  // Semantic search
//	        bm25Retriever,         // Keyword search (optional)
//	    },
//
//	    // Optional: Refine retrieved documents
//	    DocumentRefiners: []rag.DocumentRefiner{
//	        deduplicationRefiner,  // Remove duplicates
//	        rankRefiner,           // Keep top results
//	    },
//
//	    // Optional: Augment query with context
//	    QueryAugmenter: contextualAugmenter,
//	})
//
// The pipeline provides:
//   - Automatic error handling and propagation with detailed error messages
//   - Parallel execution of retrievers with configurable concurrency
//   - Partial failure tolerance (returns results even if some retrievers fail)
//   - Context propagation for cancellation and timeouts
//   - Query metadata passing between stages
//
// Configuration validation:
//   - At least one retriever is required
//   - Optional components default to no-op implementations if not provided
//   - All configurations are validated before pipeline creation
//
// # Chat Model Integration
//
// The package provides seamless integration with chat models through middleware.
// The middleware pattern allows you to add RAG capabilities to any chat model
// without modifying your existing code structure.
//
// How it works:
//  1. Middleware intercepts chat requests before they reach the LLM
//  2. Executes the RAG pipeline to retrieve relevant documents
//  3. Augments the request with document context
//  4. Passes the augmented request to the LLM
//  5. Attaches retrieved documents to the response metadata
//
// This approach provides several benefits:
//   - Non-invasive: No changes to existing chat model code
//   - Transparent: Works with both synchronous and streaming APIs
//   - Traceable: Documents are attached to responses for citation
//   - Reusable: Same middleware works with different models
//
// Basic usage:
//
//	import (
//	    "github.com/Tangerg/lynx/ai/model/chat"
//	    "github.com/Tangerg/lynx/ai/rag"
//	    "github.com/Tangerg/lynx/ai/rag/document/retrievers"
//	)
//
//	// Create middleware from pipeline configuration
//	callMw, streamMw, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
//	    DocumentRetrievers: []rag.DocumentRetriever{vectorStoreRetriever},
//	})
//
//	// Create chat client with existing model
//	chatClient, err := chat.NewClientWithModel(openaiModel)
//
//	// Create request as usual
//	request, err := chat.NewRequest([]chat.Message{
//	    chat.NewUserMessage("How do I optimize database queries?"),
//	})
//
//	// Execute with middleware - RAG happens automatically
//	text, response, err := chatClient.
//	    ChatRequest(request).
//	    WithMiddlewares(callMw, streamMw).
//	    Call().
//	    Text(ctx)
//
//	// Retrieved documents are available in metadata
//	docs, _ := response.Metadata.Get(rag.DocumentContextKey)
//	documents := docs.([]*document.Document)
//
// The middleware supports both synchronous and streaming modes seamlessly.
// The behavior differs slightly:
//
// Synchronous mode (Call):
//   - RAG pipeline executes once before LLM call
//   - Documents attached to the single response
//
// Streaming mode (Stream):
//   - RAG pipeline executes once before streaming begins
//   - Same documents attached to every streamed chunk
//   - No additional retrieval during streaming
//
// Streaming example:
//
//	stream := chatClient.
//	    ChatRequest(request).
//	    WithMiddlewares(callMw, streamMw).
//	    Stream().
//	    Response(ctx)
//
//	for response, err := range stream {
//	    // Process each chunk
//	    if response.Result != nil {
//	        fmt.Print(response.Result.AssistantMessage.Text)
//	    }
//
//	    // Documents available in every chunk
//	    docs, _ := response.Metadata.Get(rag.DocumentContextKey)
//	}
//
// # Query Metadata
//
// The Query type supports arbitrary metadata through its Extra field. This enables
// flexible data passing between pipeline stages and allows components to access
// contextual information.
//
// Common use cases:
//   - Passing conversation history to compressors
//   - Providing user context to retrievers
//   - Storing filter expressions for vector stores
//   - Tracking request parameters across stages
//
// Basic usage:
//
//	query, _ := rag.NewQuery("How to improve performance?")
//
//	// Set arbitrary metadata
//	query.Set("user_id", "user123")
//	query.Set("session_id", "session456")
//	query.Set("timestamp", time.Now())
//
//	// Pass conversation history for context-aware processing
//	query.Set(rag.ChatHistoryKey, previousMessages)
//
//	// Retrieve metadata in components
//	if userID, ok := query.Get("user_id"); ok {
//	    // Use user context for personalization
//	}
//
// Reserved metadata keys (defined as constants):
//   - rag.ChatHistoryKey: Conversation history for context compression
//   - rag.DocumentContextKey: Retrieved documents in response metadata
//   - retrievers.FilterExprKey: Filter expressions for vector stores
//
// The middleware automatically propagates metadata from requests to queries,
// making it easy to pass request-level parameters through the pipeline.
//
// # Built-in Components
//
// The package includes production-ready implementations for all pipeline stages.
// These components are designed to work out-of-the-box while remaining extensible.
//
// Query Transformers (package rag/query/transformers):
//
// TranslationTransformer translates queries to match the language of indexed
// documents. Essential for multilingual systems where user queries might be
// in different languages than the knowledge base.
//
// Features:
//   - Automatic language detection
//   - Preserves queries already in target language
//   - Handles unknown languages gracefully
//   - Supports all languages supported by the underlying LLM
//
// Use cases:
//   - Multilingual knowledge bases
//   - International applications
//   - Cross-language information retrieval
//
// RewriteTransformer optimizes queries for specific search systems. It removes
// noise, extracts key concepts, and formats queries to maximize retrieval
// effectiveness.
//
// Features:
//   - Removes redundant words and filler
//   - Extracts core concepts and keywords
//   - Optimizes for target system (vector store, search engine, etc.)
//   - Maintains semantic intent
//
// Use cases:
//   - Verbose or poorly formed user queries
//   - Conversational queries needing formalization
//   - System-specific optimization
//
// CompressionTransformer compresses conversation history into standalone queries.
// Essential for multi-turn conversations where context is needed but the full
// history would be too long or expensive to process.
//
// Features:
//   - Extracts relevant context from conversation
//   - Creates self-contained queries
//   - Reduces token consumption
//   - Preserves semantic intent across turns
//
// Use cases:
//   - Multi-turn conversations
//   - Long conversation histories
//   - Context-dependent follow-up questions
//
// Query Expanders (package rag/query/expanders):
//
// MultiExpander generates multiple query variants using LLMs. Each variant
// explores a different perspective or aspect of the original query, increasing
// the likelihood of finding relevant information.
//
// Features:
//   - Generates semantically diverse variants
//   - Maintains core intent across variants
//   - Configurable number of variants
//   - Optional inclusion of original query
//
// Use cases:
//   - Improving recall for narrow queries
//   - Exploring multiple aspects of complex topics
//   - Handling ambiguous queries
//
// Document Retrievers (package rag/document/retrievers):
//
// VectorStoreRetriever performs semantic search using vector embeddings. It
// finds documents that are conceptually similar to the query, even if they
// don't share exact keywords.
//
// Features:
//   - Semantic similarity search
//   - Configurable top-K results
//   - Minimum similarity threshold
//   - Dynamic filtering support
//   - Integration with vector stores
//
// Use cases:
//   - Semantic search over embeddings
//   - Conceptual similarity matching
//   - When exact keywords are not known
//
// Document Refiners (package rag/document/refiners):
//
// DeduplicationRefiner removes duplicate documents based on IDs. Essential
// when using multiple retrievers or query expansion, which can result in
// the same document being retrieved multiple times.
//
// Features:
//   - ID-based deduplication
//   - Preserves first occurrence order
//   - O(n) time complexity
//   - Maintains deterministic results
//
// Use cases:
//   - Multiple retriever configurations
//   - Query expansion scenarios
//   - Hybrid search strategies
//
// RankRefiner sorts documents by relevance score and returns top-K results.
// This reduces the number of documents passed to the LLM, improving quality
// and reducing costs.
//
// Features:
//   - Sorts by similarity score
//   - Configurable top-K limit
//   - Non-destructive (doesn't modify input)
//   - Focuses on most relevant results
//
// Use cases:
//   - Limiting context window usage
//   - Improving response quality
//   - Reducing token costs
//
// Query Augmenters (package rag/query/augmenters):
//
// ContextualAugmenter injects retrieved documents into structured prompts.
// It formats documents as context and combines them with the original query,
// providing the LLM with grounded information.
//
// Features:
//   - Customizable prompt templates
//   - Automatic document formatting
//   - Empty context handling
//   - Hallucination prevention instructions
//
// Use cases:
//   - Grounding LLM responses in facts
//   - Preventing hallucination
//   - Providing source attribution
//
// # Complete Example
//
// Here's a complete example demonstrating a production-ready RAG system with
// all pipeline stages configured:
//
//	import (
//	    "context"
//	    "log"
//
//	    "github.com/Tangerg/lynx/ai/model/chat"
//	    "github.com/Tangerg/lynx/ai/rag"
//	    "github.com/Tangerg/lynx/ai/rag/document/refiners"
//	    "github.com/Tangerg/lynx/ai/rag/document/retrievers"
//	    "github.com/Tangerg/lynx/ai/rag/query/augmenters"
//	    "github.com/Tangerg/lynx/ai/rag/query/expanders"
//	    "github.com/Tangerg/lynx/ai/rag/query/transformers"
//	)
//
//	// Setup transformers
//	translationTx, _ := transformers.NewTranslationTransformer(
//	    &transformers.TranslationTransformerConfig{
//	        ChatModel:      chatModel,
//	        TargetLanguage: "English",
//	    })
//
//	rewriteTx, _ := transformers.NewRewriteTransformer(
//	    &transformers.RewriteTransformerConfig{
//	        ChatModel: chatModel,
//	    })
//
//	// Setup expander
//	expander, _ := expanders.NewMultiExpander(
//	    &expanders.MultiExpanderConfig{
//	        ChatModel:       chatModel,
//	        NumberOfQueries: 3,
//	    })
//
//	// Setup retriever
//	retriever, _ := retrievers.NewVectorStoreRetriever(
//	    &retrievers.VectorStoreRetrieverConfig{
//	        VectorStore: vectorStore,
//	        TopK:        10,
//	    })
//
//	// Setup augmenter
//	augmenter, _ := augmenters.NewContextualAugmenter(
//	    &augmenters.ContextualAugmenterConfig{})
//
//	// Create middleware
//	callMw, streamMw, _ := rag.NewPipelineMiddleware(&rag.PipelineConfig{
//	    QueryTransformers:  []rag.QueryTransformer{translationTx, rewriteTx},
//	    QueryExpander:      expander,
//	    DocumentRetrievers: []rag.DocumentRetriever{retriever},
//	    DocumentRefiners:   []rag.DocumentRefiner{
//	        refiners.NewDeduplicationRefiner(),
//	        refiners.NewRankRefiner(5),
//	    },
//	    QueryAugmenter: augmenter,
//	})
//
//	// Create chat client
//	chatClient, _ := chat.NewClientWithModel(chatModel)
//
//	// Create request
//	request, _ := chat.NewRequest([]chat.Message{
//	    chat.NewUserMessage("What is RAG?"),
//	})
//
//	// Execute with RAG
//	text, response, err := chatClient.
//	    ChatRequest(request).
//	    WithMiddlewares(callMw, streamMw).
//	    Call().
//	    Text(context.Background())
//
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Access results
//	log.Printf("Response: %s", text)
//
//	docs, _ := response.Metadata.Get(rag.DocumentContextKey)
//	documents := docs.([]*document.Document)
//	log.Printf("Retrieved %d documents", len(documents))
//
// # Error Handling
//
// The package provides comprehensive error handling with detailed error messages
// that include stage information for easy debugging. All errors are wrapped with
// context about where they occurred in the pipeline.
//
// Error message format:
//
//	pipeline stage 'transform' failed:
//	  query transformation failed at stage 2:
//	    translation error: unsupported language 'xyz'
//
// All components respect context cancellation and timeouts. This allows you to
// set execution limits and gracefully handle cancellations:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	result, err := pipeline.Execute(ctx, query)
//	if err != nil {
//	    if errors.Is(err, context.DeadlineExceeded) {
//	        // Handle timeout
//	    }
//	    if errors.Is(err, context.Canceled) {
//	        // Handle cancellation
//	    }
//	}
//
// Error categories:
//   - Configuration errors: Invalid or missing required parameters
//   - Execution errors: Failures during pipeline execution
//   - Context errors: Timeout or cancellation
//   - Component errors: Failures in specific transformers/retrievers/refiners
//
// Partial failure handling:
// The pipeline implements graceful degradation for retriever failures. If some
// retrievers fail but others succeed, the pipeline continues with available
// results instead of failing completely. This improves system resilience.
//
// # Concurrency and Performance
//
// The pipeline is designed for high performance and efficient resource usage.
// Key performance characteristics:
//
// Parallel Execution:
//   - Multiple retrievers execute concurrently using errgroup
//   - Query expansion variants are processed in parallel
//   - Configurable concurrency limits prevent resource exhaustion
//
// Time Complexity:
//   - Transform stage: O(n × t) where n = transformers, t = transform time
//   - Expand stage: O(1) single LLM call
//   - Retrieve stage: O(max(r1, r2, ..., rn)) with parallel execution
//   - Refine stage: O(n × r) where n = refiners, r = refine time
//   - Augment stage: O(1) template rendering or LLM call
//
// Resource Management:
//   - Automatic cleanup on context cancellation
//   - Connection pooling support in retrievers
//   - Memory-efficient document handling
//   - Configurable limits on result sizes
//
// Thread Safety:
// All components are designed to be thread-safe and can handle concurrent
// requests without coordination:
//
//	// Safe to use from multiple goroutines
//	for i := 0; i < 100; i++ {
//	    go func(query string) {
//	        request, _ := chat.NewRequest([]chat.Message{
//	            chat.NewUserMessage(query),
//	        })
//	        text, _, _ := chatClient.
//	            ChatRequest(request).
//	            WithMiddlewares(callMw, streamMw).
//	            Call().
//	            Text(ctx)
//	    }(queries[i])
//	}
//
// # Best Practices
//
// Pipeline Configuration:
//
//  1. Order transformers by cost: Place cheap transformations (rewrite) before
//     expensive ones (compression with LLM calls) to fail fast on invalid input.
//
//  2. Use query expansion judiciously: Each variant multiplies retrieval overhead.
//     Start with 3-5 variants and tune based on recall metrics.
//
//  3. Configure multiple retrievers for hybrid search: Combine semantic (vector)
//     and keyword (BM25) retrieval for optimal recall and precision.
//
//  4. Always include deduplication: Essential when using multiple retrievers or
//     query expansion to avoid processing duplicate documents.
//
//  5. Rank before augmentation: Limit documents to top-K most relevant to reduce
//     token usage and improve response quality.
//
// Operational Practices:
//
//  6. Set appropriate timeouts: RAG involves multiple LLM calls and can be slow.
//     Use context.WithTimeout to prevent hanging requests.
//
//  7. Monitor pipeline stages: Track execution time of each stage to identify
//     bottlenecks and optimize performance.
//
//  8. Handle empty results: Configure ContextualAugmenter with appropriate
//     empty context handling based on whether you want strict grounding or
//     allow the LLM to use its base knowledge.
//
//  9. Cache expensive operations: Consider caching translations, embeddings,
//     and retrieval results for frequently accessed queries.
//
//  10. Test with real queries: RAG quality depends heavily on your specific
//     documents and query patterns. Test with production-like data.
//
// # Multilingual Support
//
// The framework provides comprehensive multilingual support through the
// TranslationTransformer. This is essential for applications serving users
// in multiple languages or with multilingual knowledge bases.
//
// Key capabilities:
//   - Automatic language detection
//   - Translation to target language
//   - Preservation of queries already in target language
//   - Graceful handling of unknown languages
//   - Support for mixed-language conversations
//
// Example workflow:
//
//	// Setup translation to English (common for embeddings)
//	translator, _ := transformers.NewTranslationTransformer(
//	    &transformers.TranslationTransformerConfig{
//	        ChatModel:      chatModel,
//	        TargetLanguage: "English",
//	    })
//
//	// User queries in any language are automatically translated
//	// Chinese: "什么是向量数据库?" → "What is a vector database?"
//	// Japanese: "ベクトルデータベースとは?" → "What is a vector database?"
//	// Korean: "벡터 데이터베이스란?" → "What is a vector database?"
//
// This enables:
//   - Single-language document index with multilingual queries
//   - Consistent retrieval quality across languages
//   - Reduced infrastructure complexity (one embedding model)
//
// # Advanced Patterns
//
// The framework's flexibility enables advanced patterns for specific use cases:
//
// Conditional Execution:
// Skip expensive operations when not needed:
//
//	type ConditionalExpander struct {
//	    expander  rag.QueryExpander
//	    condition func(*rag.Query) bool
//	}
//
//	func (c *ConditionalExpander) Expand(ctx context.Context, query *rag.Query) ([]*rag.Query, error) {
//	    if !c.condition(query) {
//	        return []*rag.Query{query}, nil  // Skip expansion
//	    }
//	    return c.expander.Expand(ctx, query)
//	}
//
// Caching Layer:
// Reduce latency and costs for repeated queries:
//
//	type CachedRetriever struct {
//	    retriever rag.DocumentRetriever
//	    cache     Cache
//	}
//
//	func (c *CachedRetriever) Retrieve(ctx context.Context, query *rag.Query) ([]*document.Document, error) {
//	    key := computeKey(query)
//	    if docs, ok := c.cache.Get(key); ok {
//	        return docs, nil
//	    }
//
//	    docs, err := c.retriever.Retrieve(ctx, query)
//	    if err == nil {
//	        c.cache.Set(key, docs)
//	    }
//	    return docs, err
//	}
//
// Metrics Collection:
// Monitor pipeline performance:
//
//	type InstrumentedPipeline struct {
//	    pipeline *rag.Pipeline
//	    metrics  MetricsCollector
//	}
//
//	func (i *InstrumentedPipeline) Execute(ctx context.Context, query *rag.Query) (*rag.Query, []*document.Document, error) {
//	    start := time.Now()
//	    defer func() {
//	        i.metrics.RecordLatency("pipeline.total", time.Since(start))
//	    }()
//
//	    return i.pipeline.Execute(ctx, query)
//	}
//
// Custom Request Parameters:
// Pass application-specific context through the pipeline:
//
//	request, _ := chat.NewRequest(messages)
//	request.Params = map[string]any{
//	    "user_id":     "user123",
//	    "tenant_id":   "tenant456",
//	    "permissions": []string{"read", "write"},
//	}
//
//	// Access in custom components via query.Extra
//
// # Testing
//
// The package provides testing utilities to simplify component testing:
//
// Nop Implementation:
// Use no-op implementations for testing without actual LLM calls:
//
//	pipeline, _ := rag.NewPipeline(&rag.PipelineConfig{
//	    QueryTransformers:  []rag.QueryTransformer{rag.NewNop()},
//	    QueryExpander:      rag.NewNop(),
//	    DocumentRetrievers: []rag.DocumentRetriever{mockRetriever},
//	    DocumentRefiners:   []rag.DocumentRefiner{rag.NewNop()},
//	    QueryAugmenter:    rag.NewNop(),
//	})
//
// Mock Retrievers:
// Create test retrievers that return predefined documents:
//
//	type MockRetriever struct {
//	    documents []*document.Document
//	}
//
//	func (m *MockRetriever) Retrieve(ctx context.Context, query *rag.Query) ([]*document.Document, error) {
//	    return m.documents, nil
//	}
//
// # Performance Optimization
//
// For high-throughput production systems, consider these optimizations:
//
// Infrastructure:
//   - Connection pooling for vector stores and databases
//   - Load balancing across multiple retriever instances
//   - Caching layers for embeddings and frequent queries
//   - CDN or edge caching for common responses
//
// Pipeline:
//   - Batch processing of queries when possible
//   - Tuning concurrency limits based on available resources
//   - Using streaming APIs to reduce perceived latency
//   - Implementing circuit breakers for external services
//
// Monitoring:
//   - Track latency of each pipeline stage
//   - Monitor cache hit rates
//   - Measure retrieval quality metrics (precision, recall)
//   - Alert on error rates and timeouts
//
// # Extensibility
//
// The framework is designed for easy extension. To implement custom components:
//
// 1. Implement the relevant interface (QueryTransformer, DocumentRetriever, etc.)
// 2. Add configuration struct with validation
// 3. Handle context cancellation properly
// 4. Provide detailed error messages
// 5. Document the component's purpose and use cases
//
// Example custom retriever:
//
//	// CustomRetriever retrieves from a proprietary data source
//	type CustomRetriever struct {
//	    client *MyAPIClient
//	    config *CustomConfig
//	}
//
//	func (c *CustomRetriever) Retrieve(ctx context.Context, query *rag.Query) ([]*document.Document, error) {
//	    // Check context before expensive operations
//	    if err := ctx.Err(); err != nil {
//	        return nil, err
//	    }
//
//	    // Perform retrieval
//	    results, err := c.client.Search(ctx, query.Text)
//	    if err != nil {
//	        return nil, fmt.Errorf("custom retrieval failed: %w", err)
//	    }
//
//	    // Convert to standard format
//	    return convertToDocuments(results), nil
//	}
package rag
