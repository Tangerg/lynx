package rag_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/rag"
	"github.com/Tangerg/lynx/ai/vectorstore"
)

// MultilingualMockVectorStore simulates a vector store with multilingual documents
type MultilingualMockVectorStore struct {
	documents map[string][]*document.Document
}

// NewMultilingualMockVectorStore creates a new mock vector store with multilingual sample documents
func NewMultilingualMockVectorStore() *MultilingualMockVectorStore {
	return &MultilingualMockVectorStore{
		documents: createMultilingualDocuments(),
	}
}

func (m *MultilingualMockVectorStore) Retrieve(ctx context.Context, request *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	// Combine all documents
	var allDocs []*document.Document
	for _, docs := range m.documents {
		allDocs = append(allDocs, docs...)
	}

	// Simulate vector similarity search
	var results []*document.Document
	for i, doc := range allDocs {
		// Simulate similarity score
		doc.Score = 1.0 - (float64(i) / float64(len(allDocs)))
		results = append(results, doc)
	}

	// Respect the top-k limit
	if request.TopK > 0 && request.TopK < len(results) {
		results = results[:request.TopK]
	}

	return results, nil
}

// createMultilingualDocuments creates sample documents in multiple languages
func createMultilingualDocuments() map[string][]*document.Document {
	samples := map[string][]struct {
		id      string
		content string
		lang    string
	}{
		"chinese": {
			{
				id: "zh_doc1",
				content: `RAG代表检索增强生成（Retrieval-Augmented Generation）。它是一种创新的技术方法，巧妙地结合了信息检索和文本生成两大核心能力。
通过这种结合，RAG能够提供更加准确、相关且符合上下文的响应。该技术特别适用于需要基于大量外部知识进行推理和生成的场景。
与传统的纯生成式模型相比，RAG通过引入外部知识源，显著减少了模型产生幻觉（hallucination）的可能性，提高了生成内容的可靠性和准确性。`,
				lang: "zh",
			},
			{
				id: "zh_doc2",
				content: `检索增强生成(RAG)是一种增强大型语言模型能力的先进架构。它的核心思想是在生成响应之前，先从结构化的知识库中检索相关文档。
这个过程确保了生成的内容有充分的事实依据支撑。RAG系统通常包含一个向量数据库，用于存储和快速检索大量的文档片段。
当用户提出查询时，系统会首先将查询转换为向量表示，然后在向量空间中搜索最相关的文档片段，最后将这些片段与原始查询一起输入到语言模型中生成最终答案。
这种方法不仅提高了答案的准确性，还使得模型能够引用具体的信息来源，增强了可解释性和可信度。`,
				lang: "zh",
			},
			{
				id: "zh_doc3",
				content: `RAG架构由三个核心组件紧密协作组成：查询处理模块、文档检索引擎和响应生成器。
查询处理模块负责理解用户意图，对原始查询进行优化、扩展或重写，以提高检索效果。它可能包含查询压缩、语义理解、实体识别等多个子功能。
文档检索引擎使用先进的向量相似度算法，从海量知识库中快速定位最相关的文档片段。这个过程通常涉及向量化、索引构建和高效搜索等技术。
响应生成器则接收检索到的上下文信息和原始查询，利用大型语言模型生成连贯、准确且有据可依的答案。三个组件的协同工作确保了整个系统的高效运行。`,
				lang: "zh",
			},
			{
				id: "zh_doc4",
				content: `RAG系统的核心技术之一是向量嵌入（Vector Embeddings）的使用。通过将文本转换为高维向量空间中的点，系统能够捕捉语义信息。
这些向量表示不仅保留了词汇层面的信息，更重要的是编码了深层的语义关系。在知识库中，每个文档片段都被转换为一个向量，存储在专门的向量数据库中。
当进行检索时，系统计算查询向量与文档向量之间的相似度（通常使用余弦相似度或欧氏距离），找出语义上最相近的文档。
这种基于语义的检索方式远比传统的关键词匹配更加智能和准确，能够理解同义词、近义词，甚至跨语言的语义关联，大大提高了检索信息的相关性和质量。`,
				lang: "zh",
			},
			{
				id: "zh_doc5",
				content: `RAG中的查询转换是提升检索质量的关键环节，包含多种先进技术。查询压缩技术能够去除冗余信息，提取核心语义，使检索更加精准高效。
查询重写则通过理解用户真实意图，将模糊或不完整的查询改写为更明确、更适合检索的形式。查询扩展技术会生成多个相关的查询变体，扩大检索范围，确保不遗漏重要信息。
此外，还包括实体链接、关系抽取、意图识别等技术，帮助系统更深入地理解查询背后的需求。
这些技术的综合运用，使得RAG系统能够应对各种复杂的查询场景，从简单的事实查询到需要深度推理的复杂问题，都能提供高质量的检索结果。`,
				lang: "zh",
			},
			{
				id: "zh_doc6",
				content: `RAG系统在实际应用中展现出强大的优势。它特别适用于需要频繁更新知识的场景，如新闻问答、法律咨询、医疗诊断辅助等领域。
通过简单更新知识库而无需重新训练整个模型，RAG实现了知识的快速迭代和更新。在企业应用中，RAG可以整合内部文档、产品手册、客户支持记录等私有数据源。
系统的模块化设计使得各个组件可以独立优化和升级，提供了良好的可扩展性。此外，RAG还支持多模态检索，可以处理文本、图像、表格等多种形式的信息。
其透明的检索过程使得生成的答案可追溯、可验证，大大提高了AI系统的可信度和实用价值。`,
				lang: "zh",
			},
		},
		"japanese": {
			{
				id: "ja_doc1",
				content: `RAGは検索拡張生成（Retrieval-Augmented Generation）を表します。これは情報検索とテキスト生成を組み合わせた革新的な技術アプローチです。
この組み合わせにより、RAGはより正確で関連性が高く、文脈に適した応答を提供することができます。特に大量の外部知識に基づいて推論と生成が必要なシナリオに適しています。
従来の純粋な生成モデルと比較して、RAGは外部知識ソースを導入することで、モデルが幻覚（hallucination）を生成する可能性を大幅に削減し、
生成されるコンテンツの信頼性と正確性を向上させます。この技術は現代のAIシステムにおいて重要な役割を果たしています。`,
				lang: "ja",
			},
			{
				id: "ja_doc2",
				content: `検索拡張生成（RAG）は、大規模言語モデルの能力を強化する先進的なアーキテクチャです。その核心的なアイデアは、応答を生成する前に、
構造化されたナレッジベースから関連文書を検索することです。このプロセスにより、生成されるコンテンツに十分な事実的根拠が提供されます。
RAGシステムは通常、大量の文書フラグメントを保存し、高速に検索するためのベクトルデータベースを含んでいます。
ユーザーがクエリを提出すると、システムはまずクエリをベクトル表現に変換し、次にベクトル空間で最も関連性の高い文書フラグメントを検索します。
最後に、これらのフラグメントと元のクエリを言語モデルに入力して最終的な回答を生成します。`,
				lang: "ja",
			},
			{
				id: "ja_doc3",
				content: `RAGアーキテクチャは、密接に連携する3つの核心コンポーネントで構成されています：クエリ処理モジュール、文書検索エンジン、応答生成器です。
クエリ処理モジュールは、ユーザーの意図を理解し、元のクエリを最適化、拡張、または書き換えて検索効果を向上させます。
これには、クエリ圧縮、意味理解、エンティティ認識などの複数のサブ機能が含まれる場合があります。
文書検索エンジンは、先進的なベクトル類似度アルゴリズムを使用して、膨大なナレッジベースから最も関連性の高い文書フラグメントを迅速に特定します。
このプロセスには通常、ベクトル化、インデックス構築、効率的な検索などの技術が含まれます。応答生成器は、検索されたコンテキスト情報と元のクエリを受け取り、
大規模言語モデルを利用して一貫性があり、正確で、根拠のある回答を生成します。`,
				lang: "ja",
			},
			{
				id: "ja_doc4",
				content: `RAGシステムの核心技術の1つは、ベクトル埋め込み（Vector Embeddings）の使用です。テキストを高次元ベクトル空間の点に変換することで、
システムは意味情報を捉えることができます。これらのベクトル表現は、語彙レベルの情報を保持するだけでなく、
より重要なことに、深い意味的関係をエンコードします。ナレッジベースでは、各文書フラグメントがベクトルに変換され、専用のベクトルデータベースに保存されます。
検索を実行する際、システムはクエリベクトルと文書ベクトル間の類似度（通常はコサイン類似度またはユークリッド距離を使用）を計算し、
意味的に最も近い文書を見つけます。この意味ベースの検索方法は、従来のキーワードマッチングよりもはるかに賢く正確で、
同義語、類義語、さらには言語を超えた意味的関連性を理解することができます。`,
				lang: "ja",
			},
			{
				id: "ja_doc5",
				content: `RAGにおけるクエリ変換は、検索品質を向上させる重要なステップであり、複数の先進技術が含まれます。
クエリ圧縮技術は、冗長な情報を削除し、核心的な意味を抽出して、検索をより正確かつ効率的にします。
クエリ書き換えは、ユーザーの真の意図を理解することで、曖昧または不完全なクエリをより明確で検索に適した形式に改変します。
クエリ拡張技術は、関連する複数のクエリバリエーションを生成し、検索範囲を拡大して重要な情報を見逃さないようにします。
さらに、エンティティリンク、関係抽出、意図認識などの技術も含まれ、システムがクエリの背後にあるニーズをより深く理解するのに役立ちます。
これらの技術の総合的な活用により、RAGシステムはさまざまな複雑なクエリシナリオに対応できます。`,
				lang: "ja",
			},
			{
				id: "ja_doc6",
				content: `RAGシステムは実際のアプリケーションにおいて強力な利点を示しています。特に知識の頻繁な更新が必要なシナリオ、
例えばニュース質問応答、法律相談、医療診断支援などの分野に適しています。知識ベースを更新するだけでモデル全体を再トレーニングする必要がなく、
RAGは知識の迅速な反復と更新を実現します。企業アプリケーションでは、RAGは内部文書、製品マニュアル、カスタマーサポート記録などの
プライベートデータソースを統合できます。システムのモジュール設計により、各コンポーネントを独立して最適化およびアップグレードでき、
優れた拡張性を提供します。さらに、RAGはマルチモーダル検索をサポートし、テキスト、画像、表など、さまざまな形式の情報を処理できます。
その透明な検索プロセスにより、生成された回答は追跡可能で検証可能であり、AIシステムの信頼性と実用的価値を大幅に向上させます。`,
				lang: "ja",
			},
		},
		"korean": {
			{
				id: "ko_doc1",
				content: `RAG는 검색 증강 생성(Retrieval-Augmented Generation)을 나타냅니다. 이는 정보 검색과 텍스트 생성을 결합한 혁신적인 기술 접근 방식입니다.
이러한 결합을 통해 RAG는 더욱 정확하고 관련성이 높으며 맥락에 맞는 응답을 제공할 수 있습니다. 특히 대량의 외부 지식을 기반으로 추론과 생성이 필요한 시나리오에 적합합니다.
기존의 순수 생성 모델과 비교하여 RAG는 외부 지식 소스를 도입함으로써 모델이 환각(hallucination)을 생성할 가능성을 크게 줄이고,
생성된 콘텐츠의 신뢰성과 정확성을 향상시킵니다. 이 기술은 현대 AI 시스템에서 중요한 역할을 수행하고 있습니다.`,
				lang: "ko",
			},
			{
				id: "ko_doc2",
				content: `검색 증강 생성(RAG)은 대형 언어 모델의 능력을 향상시키는 선진적인 아키텍처입니다. 핵심 아이디어는 응답을 생성하기 전에
구조화된 지식 베이스에서 관련 문서를 검색하는 것입니다. 이 프로세스는 생성된 콘텐츠에 충분한 사실적 근거를 제공합니다.
RAG 시스템은 일반적으로 대량의 문서 조각을 저장하고 빠르게 검색하기 위한 벡터 데이터베이스를 포함합니다.
사용자가 쿼리를 제출하면 시스템은 먼저 쿼리를 벡터 표현으로 변환한 다음 벡터 공간에서 가장 관련성이 높은 문서 조각을 검색합니다.
마지막으로 이러한 조각과 원래 쿼리를 언어 모델에 입력하여 최종 답변을 생성합니다. 이 방법은 답변의 정확성을 높일 뿐만 아니라
모델이 구체적인 정보 출처를 인용할 수 있게 하여 설명 가능성과 신뢰성을 강화합니다.`,
				lang: "ko",
			},
			{
				id: "ko_doc3",
				content: `RAG 아키텍처는 긴밀하게 협력하는 세 가지 핵심 구성 요소로 이루어져 있습니다: 쿼리 처리 모듈, 문서 검색 엔진, 응답 생성기입니다.
쿼리 처리 모듈은 사용자 의도를 이해하고 원래 쿼리를 최적화, 확장 또는 재작성하여 검색 효과를 향상시킵니다.
여기에는 쿼리 압축, 의미 이해, 엔티티 인식 등 여러 하위 기능이 포함될 수 있습니다.
문서 검색 엔진은 선진적인 벡터 유사도 알고리즘을 사용하여 방대한 지식 베이스에서 가장 관련성이 높은 문서 조각을 신속하게 찾아냅니다.
이 프로세스에는 일반적으로 벡터화, 인덱스 구축, 효율적인 검색 등의 기술이 포함됩니다.
응답 생성기는 검색된 컨텍스트 정보와 원래 쿼리를 받아 대형 언어 모델을 활용하여 일관성 있고 정확하며 근거 있는 답변을 생성합니다.
세 구성 요소의 협력적인 작업이 전체 시스템의 효율적인 운영을 보장합니다.`,
				lang: "ko",
			},
			{
				id: "ko_doc4",
				content: `RAG 시스템의 핵심 기술 중 하나는 벡터 임베딩(Vector Embeddings)의 사용입니다. 텍스트를 고차원 벡터 공간의 점으로 변환함으로써
시스템은 의미 정보를 포착할 수 있습니다. 이러한 벡터 표현은 어휘 수준의 정보를 유지할 뿐만 아니라,
더 중요하게는 깊은 의미적 관계를 인코딩합니다. 지식 베이스에서 각 문서 조각은 벡터로 변환되어 전용 벡터 데이터베이스에 저장됩니다.
검색을 수행할 때 시스템은 쿼리 벡터와 문서 벡터 간의 유사도(일반적으로 코사인 유사도 또는 유클리드 거리 사용)를 계산하여
의미적으로 가장 가까운 문서를 찾습니다. 이러한 의미 기반 검색 방법은 전통적인 키워드 매칭보다 훨씬 더 지능적이고 정확하며,
동의어, 유의어, 심지어 언어를 넘어선 의미적 연관성을 이해할 수 있어 검색된 정보의 관련성과 품질을 크게 향상시킵니다.`,
				lang: "ko",
			},
			{
				id: "ko_doc5",
				content: `RAG의 쿼리 변환은 검색 품질을 향상시키는 핵심 단계이며, 여러 선진 기술이 포함됩니다.
쿼리 압축 기술은 중복 정보를 제거하고 핵심 의미를 추출하여 검색을 더욱 정확하고 효율적으로 만듭니다.
쿼리 재작성은 사용자의 진정한 의도를 이해함으로써 모호하거나 불완전한 쿼리를 더 명확하고 검색에 적합한 형식으로 변환합니다.
쿼리 확장 기술은 관련된 여러 쿼리 변형을 생성하여 검색 범위를 확대하고 중요한 정보를 놓치지 않도록 합니다.
또한 엔티티 링크, 관계 추출, 의도 인식 등의 기술도 포함되어 시스템이 쿼리 뒤에 있는 요구 사항을 더 깊이 이해하는 데 도움을 줍니다.
이러한 기술의 종합적인 활용을 통해 RAG 시스템은 간단한 사실 쿼리부터 깊은 추론이 필요한 복잡한 문제까지
다양한 복잡한 쿼리 시나리오에 대응할 수 있으며, 고품질의 검색 결과를 제공할 수 있습니다.`,
				lang: "ko",
			},
			{
				id: "ko_doc6",
				content: `RAG 시스템은 실제 응용에서 강력한 장점을 보여줍니다. 특히 지식의 빈번한 업데이트가 필요한 시나리오,
예를 들어 뉴스 질의응답, 법률 상담, 의료 진단 보조 등의 분야에 적합합니다. 지식 베이스를 업데이트하는 것만으로
전체 모델을 재학습할 필요 없이 RAG는 지식의 신속한 반복과 업데이트를 실현합니다.
기업 애플리케이션에서 RAG는 내부 문서, 제품 매뉴얼, 고객 지원 기록 등의 비공개 데이터 소스를 통합할 수 있습니다.
시스템의 모듈식 설계로 각 구성 요소를 독립적으로 최적화하고 업그레이드할 수 있어 우수한 확장성을 제공합니다.
또한 RAG는 다중 모달 검색을 지원하여 텍스트, 이미지, 표 등 다양한 형식의 정보를 처리할 수 있습니다.
투명한 검색 프로세스로 인해 생성된 답변은 추적 가능하고 검증 가능하며, AI 시스템의 신뢰성과 실용적 가치를 크게 향상시킵니다.`,
				lang: "ko",
			},
		},
		"english": {
			{
				id: "en_doc1",
				content: `RAG stands for Retrieval-Augmented Generation, representing an innovative technological approach that ingeniously combines information retrieval with text generation capabilities.
Through this combination, RAG can provide more accurate, relevant, and contextually appropriate responses. This technology is particularly well-suited for scenarios requiring reasoning and generation based on extensive external knowledge.
Compared to traditional pure generative models, RAG significantly reduces the likelihood of model hallucinations by introducing external knowledge sources, thereby improving the reliability and accuracy of generated content.
This approach has become increasingly important in modern AI systems, where factual accuracy and verifiable information are crucial for real-world applications.`,
				lang: "en",
			},
			{
				id: "en_doc2",
				content: `Retrieval-Augmented Generation (RAG) is an advanced architecture that enhances the capabilities of Large Language Models. Its core idea is to retrieve relevant documents from a structured knowledge base before generating responses.
This process ensures that the generated content has sufficient factual support. RAG systems typically include a vector database for storing and quickly retrieving large numbers of document fragments.
When a user submits a query, the system first converts the query into a vector representation, then searches for the most relevant document fragments in the vector space.
Finally, these fragments along with the original query are input into the language model to generate the final answer. This method not only improves the accuracy of answers but also enables the model to cite specific information sources,
enhancing explainability and credibility. The architecture allows for seamless integration of up-to-date information without requiring model retraining.`,
				lang: "en",
			},
			{
				id: "en_doc3",
				content: `The RAG architecture consists of three core components working in close collaboration: the query processing module, document retrieval engine, and response generator.
The query processing module is responsible for understanding user intent and optimizing, expanding, or rewriting the original query to improve retrieval effectiveness. It may include multiple sub-functions such as query compression, semantic understanding, and entity recognition.
The document retrieval engine uses advanced vector similarity algorithms to quickly locate the most relevant document fragments from massive knowledge bases. This process typically involves techniques such as vectorization, index construction, and efficient search.
The response generator receives the retrieved contextual information and original query, utilizing large language models to generate coherent, accurate, and well-founded answers.
The collaborative work of these three components ensures the efficient operation of the entire system, creating a seamless flow from query to response.`,
				lang: "en",
			},
			{
				id: "en_doc4",
				content: `One of the core technologies of RAG systems is the use of vector embeddings. By converting text into points in high-dimensional vector space, the system can capture semantic information.
These vector representations not only preserve lexical-level information but, more importantly, encode deep semantic relationships. In the knowledge base, each document fragment is converted into a vector and stored in a specialized vector database.
When performing retrieval, the system calculates the similarity between query vectors and document vectors (usually using cosine similarity or Euclidean distance) to find semantically closest documents.
This semantic-based retrieval method is far more intelligent and accurate than traditional keyword matching, capable of understanding synonyms, near-synonyms, and even cross-lingual semantic associations.
This dramatically improves the relevance and quality of retrieved information, enabling the system to understand intent beyond literal word matching.`,
				lang: "en",
			},
			{
				id: "en_doc5",
				content: `Query transformation in RAG is a critical step for improving retrieval quality and includes multiple advanced techniques. Query compression technology can remove redundant information and extract core semantics, making retrieval more precise and efficient.
Query rewriting transforms ambiguous or incomplete queries into clearer, more retrieval-friendly forms by understanding the user's true intent. Query expansion technology generates multiple related query variants, broadening the retrieval scope to ensure important information isn't missed.
Additionally, it includes techniques such as entity linking, relation extraction, and intent recognition, helping the system more deeply understand the needs behind queries.
The comprehensive application of these technologies enables RAG systems to handle various complex query scenarios, from simple factual queries to complex problems requiring deep reasoning,
all while providing high-quality retrieval results. This multi-faceted approach to query processing is what sets RAG apart from simpler retrieval systems.`,
				lang: "en",
			},
			{
				id: "en_doc6",
				content: `RAG systems demonstrate powerful advantages in practical applications. They are particularly suitable for scenarios requiring frequent knowledge updates, such as news Q&A, legal consultation, medical diagnosis assistance, and other fields.
By simply updating the knowledge base without retraining the entire model, RAG achieves rapid iteration and updating of knowledge. In enterprise applications, RAG can integrate private data sources such as internal documents, product manuals, and customer support records.
The modular design of the system allows each component to be independently optimized and upgraded, providing excellent scalability. Furthermore, RAG supports multimodal retrieval, capable of processing various forms of information including text, images, and tables.
Its transparent retrieval process makes generated answers traceable and verifiable, greatly improving the trustworthiness and practical value of AI systems.
The ability to cite sources and provide evidence for claims makes RAG an invaluable tool for applications where accuracy and accountability are paramount.`,
				lang: "en",
			},
		},
	}

	result := make(map[string][]*document.Document)
	for lang, langSamples := range samples {
		var docs []*document.Document
		for _, sample := range langSamples {
			doc, err := document.NewDocument(sample.content, nil)
			if err != nil {
				continue
			}
			doc.ID = sample.id
			doc.Metadata = map[string]any{
				"source":   "knowledge_base",
				"type":     "rag_explanation",
				"language": sample.lang,
			}
			docs = append(docs, doc)
		}
		result[lang] = docs
	}

	return result
}

// TestPipeline_MultilingualQueries tests pipeline with queries in different languages
func TestPipeline_MultilingualQueries(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	testCases := []struct {
		name           string
		query          string
		language       string
		targetLanguage string
		description    string
	}{
		{
			name:           "Chinese to English",
			query:          "什么是RAG？它是如何工作的？",
			language:       "Chinese",
			targetLanguage: "English",
			description:    "Test translation from Chinese to English",
		},
		{
			name:           "Japanese to English",
			query:          "RAGとは何ですか？どのように機能しますか？",
			language:       "Japanese",
			targetLanguage: "English",
			description:    "Test translation from Japanese to English",
		},
		{
			name:           "Korean to English",
			query:          "RAG란 무엇입니까? 어떻게 작동합니까?",
			language:       "Korean",
			targetLanguage: "English",
			description:    "Test translation from Korean to English",
		},
		{
			name:           "English query",
			query:          "What is RAG and how does it work?",
			language:       "English",
			targetLanguage: "English",
			description:    "Test English query without translation",
		},
		{
			name:           "Mixed Chinese-English",
			query:          "RAG系统如何使用vector embeddings？",
			language:       "Mixed",
			targetLanguage: "English",
			description:    "Test mixed language query",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup pipeline with translation
			translationTransformer, err := rag.NewTranslationQueryTransformer(&rag.TranslationQueryTransformerConfig{
				ChatModel:      newTestChatModel(t),
				TargetLanguage: tc.targetLanguage,
			})
			require.NoError(t, err)

			vectorStoreRetriever, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
				VectorStore: NewMultilingualMockVectorStore(),
				TopK:        5,
			})
			require.NoError(t, err)

			pipeline, err := rag.NewPipeline(&rag.PipelineConfig{
				QueryTransformers: []rag.QueryTransformer{
					translationTransformer,
				},
				DocumentRetrievers: []rag.DocumentRetriever{
					vectorStoreRetriever,
				},
				DocumentRefiners: []rag.DocumentRefiner{
					rag.NewDeduplicationDocumentRefiner(),
					rag.NewRankDocumentRefiner(3),
				},
			})
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
			defer cancel()

			// Execute pipeline
			query, documents, err := pipeline.Run(ctx, tc.query)
			require.NoError(t, err)

			// Verify results
			assert.NotNil(t, query)
			assert.NotEmpty(t, query.Text)
			assert.NotEmpty(t, documents)
			assert.LessOrEqual(t, len(documents), 3)

			t.Logf("Description: %s", tc.description)
			t.Logf("Original query (%s): %s", tc.language, tc.query)
			t.Logf("Processed query: %s", query.Text)
			t.Logf("Retrieved %d documents:", len(documents))
			for i, doc := range documents {
				lang, _ := doc.Metadata["language"]
				t.Logf("  [%d] ID: %s, Language: %v, Score: %.4f", i+1, doc.ID, lang, doc.Score)
				t.Logf("      Text: %s", doc.Text)
			}
		})
	}
}

// TestPipeline_ChineseFullIntegration tests complete pipeline with Chinese queries
func TestPipeline_ChineseFullIntegration(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	// Setup transformers
	compressionTransformer, err := rag.NewCompressionQueryTransformer(&rag.CompressionQueryTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	rewriteTransformer, err := rag.NewRewriteQueryTransformer(&rag.RewriteQueryTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	translationTransformer, err := rag.NewTranslationQueryTransformer(&rag.TranslationQueryTransformerConfig{
		ChatModel:      newTestChatModel(t),
		TargetLanguage: "English",
	})
	require.NoError(t, err)

	multiExpander, err := rag.NewMultiQueryExpander(&rag.MultiQueryExpanderConfig{
		ChatModel:       newTestChatModel(t),
		IncludeOriginal: true,
		NumberOfQueries: 3,
	})
	require.NoError(t, err)

	vectorStoreRetriever, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        10,
	})
	require.NoError(t, err)

	augmenter, err := rag.NewContextualQueryAugmenter(&rag.ContextualQueryAugmenterConfig{})
	require.NoError(t, err)

	pipeline, err := rag.NewPipeline(&rag.PipelineConfig{
		QueryTransformers: []rag.QueryTransformer{
			compressionTransformer,
			rewriteTransformer,
			translationTransformer,
		},
		QueryExpander: multiExpander,
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		DocumentRefiners: []rag.DocumentRefiner{
			rag.NewDeduplicationDocumentRefiner(),
			rag.NewRankDocumentRefiner(5),
		},
		QueryAugmenter: augmenter,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	query, documents, err := pipeline.Run(ctx, "请详细解释一下RAG系统的工作原理和主要组成部分")
	require.NoError(t, err)

	assert.NotNil(t, query)
	assert.NotEmpty(t, query.Text)
	assert.NotEmpty(t, documents)
	assert.LessOrEqual(t, len(documents), 5)

	t.Logf("Original query: 请详细解释一下RAG系统的工作原理和主要组成部分")
	t.Logf("Augmented query: %s", query.Text)
	t.Logf("Retrieved %d documents:", len(documents))
	for i, doc := range documents {
		t.Logf("  [%d] ID: %s, Score: %.4f", i+1, doc.ID, doc.Score)
	}
}

// TestPipeline_JapaneseFullIntegration tests complete pipeline with Japanese queries
func TestPipeline_JapaneseFullIntegration(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	translationTransformer, err := rag.NewTranslationQueryTransformer(&rag.TranslationQueryTransformerConfig{
		ChatModel:      newTestChatModel(t),
		TargetLanguage: "English",
	})
	require.NoError(t, err)

	vectorStoreRetriever, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        5,
	})
	require.NoError(t, err)

	augmenter, err := rag.NewContextualQueryAugmenter(&rag.ContextualQueryAugmenterConfig{})
	require.NoError(t, err)

	pipeline, err := rag.NewPipeline(&rag.PipelineConfig{
		QueryTransformers: []rag.QueryTransformer{
			translationTransformer,
		},
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		QueryAugmenter: augmenter,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	query, documents, err := pipeline.Run(ctx, "RAGシステムの仕組みと主要なコンポーネントについて詳しく教えてください")
	require.NoError(t, err)

	assert.NotNil(t, query)
	assert.NotEmpty(t, query.Text)
	assert.NotEmpty(t, documents)

	t.Logf("Original query: RAGシステムの仕組みと主要なコンポーネントについて詳しく教えてください")
	t.Logf("Augmented query: %s", query.Text)
	t.Logf("Retrieved %d documents", len(documents))
}

// TestPipeline_KoreanFullIntegration tests complete pipeline with Korean queries
func TestPipeline_KoreanFullIntegration(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	translationTransformer, err := rag.NewTranslationQueryTransformer(&rag.TranslationQueryTransformerConfig{
		ChatModel:      newTestChatModel(t),
		TargetLanguage: "English",
	})
	require.NoError(t, err)

	vectorStoreRetriever, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        5,
	})
	require.NoError(t, err)

	augmenter, err := rag.NewContextualQueryAugmenter(&rag.ContextualQueryAugmenterConfig{})
	require.NoError(t, err)

	pipeline, err := rag.NewPipeline(&rag.PipelineConfig{
		QueryTransformers: []rag.QueryTransformer{
			translationTransformer,
		},
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		QueryAugmenter: augmenter,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	query, documents, err := pipeline.Run(ctx, "RAG 시스템의 작동 원리와 주요 구성 요소에 대해 자세히 설명해주세요")
	require.NoError(t, err)

	assert.NotNil(t, query)
	assert.NotEmpty(t, query.Text)
	assert.NotEmpty(t, documents)

	t.Logf("Original query: RAG 시스템의 작동 원리와 주요 구성 요소에 대해 자세히 설명해주세요")
	t.Logf("Augmented query: %s", query.Text)
	t.Logf("Retrieved %d documents", len(documents))
}

// TestPipeline_CrossLanguageComparison compares pipeline behavior across languages
func TestPipeline_CrossLanguageComparison(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	queries := map[string]string{
		"Chinese":  "RAG的主要优势是什么？",
		"Japanese": "RAGの主な利点は何ですか？",
		"Korean":   "RAG의 주요 이점은 무엇입니까？",
		"English":  "What are the main advantages of RAG?",
	}

	translationTransformer, err := rag.NewTranslationQueryTransformer(&rag.TranslationQueryTransformerConfig{
		ChatModel:      newTestChatModel(t),
		TargetLanguage: "English",
	})
	require.NoError(t, err)

	vectorStoreRetriever, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        3,
	})
	require.NoError(t, err)

	pipeline, err := rag.NewPipeline(&rag.PipelineConfig{
		QueryTransformers: []rag.QueryTransformer{
			translationTransformer,
		},
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		DocumentRefiners: []rag.DocumentRefiner{
			rag.NewRankDocumentRefiner(3),
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), config.timeout*4)
	defer cancel()

	results := make(map[string]*rag.Query)

	for lang, queryText := range queries {
		t.Run(lang, func(t *testing.T) {
			query, documents, err := pipeline.Run(ctx, queryText)
			require.NoError(t, err)

			assert.NotNil(t, query)
			assert.NotEmpty(t, documents)

			results[lang] = query

			t.Logf("Language: %s", lang)
			t.Logf("Original: %s", queryText)
			t.Logf("Processed: %s", query.Text)
			t.Logf("Documents: %d", len(documents))
		})
	}

	// Compare results
	t.Log("\n=== Cross-Language Comparison ===")
	for lang, query := range results {
		t.Logf("%s: %s", lang, query.Text)
	}
}

// TestPipeline_ComplexMultilingualQuery tests complex queries with multiple language aspects
func TestPipeline_ComplexMultilingualQuery(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	complexQueries := []struct {
		name  string
		query string
		desc  string
	}{
		{
			name:  "Technical Chinese",
			query: "在RAG系统中，如何使用向量数据库进行相似度搜索？请详细说明embedding的作用。",
			desc:  "Technical query about vector databases and embeddings",
		},
		{
			name:  "Technical Japanese",
			query: "RAGシステムにおいて、ベクトルデータベースを使用した類似性検索はどのように行われますか？埋め込みの役割を詳しく説明してください。",
			desc:  "Technical query about similarity search in Japanese",
		},
		{
			name:  "Technical Korean",
			query: "RAG 시스템에서 벡터 데이터베이스를 사용한 유사도 검색은 어떻게 수행됩니까? 임베딩의 역할을 자세히 설명해주세요.",
			desc:  "Technical query about vector search in Korean",
		},
		{
			name:  "Comparative English",
			query: "Compare and contrast different query transformation techniques used in RAG systems, including compression, rewriting, and translation.",
			desc:  "Comparative analysis query in English",
		},
	}

	for _, tc := range complexQueries {
		t.Run(tc.name, func(t *testing.T) {
			// Full pipeline setup
			compressionTransformer, err := rag.NewCompressionQueryTransformer(&rag.CompressionQueryTransformerConfig{
				ChatModel: newTestChatModel(t),
			})
			require.NoError(t, err)

			translationTransformer, err := rag.NewTranslationQueryTransformer(&rag.TranslationQueryTransformerConfig{
				ChatModel:      newTestChatModel(t),
				TargetLanguage: "English",
			})
			require.NoError(t, err)

			multiExpander, err := rag.NewMultiQueryExpander(&rag.MultiQueryExpanderConfig{
				ChatModel:       newTestChatModel(t),
				IncludeOriginal: true,
			})
			require.NoError(t, err)

			vectorStoreRetriever, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
				VectorStore: NewMultilingualMockVectorStore(),
				TopK:        8,
			})
			require.NoError(t, err)

			augmenter, err := rag.NewContextualQueryAugmenter(&rag.ContextualQueryAugmenterConfig{})
			require.NoError(t, err)

			pipeline, err := rag.NewPipeline(&rag.PipelineConfig{
				QueryTransformers: []rag.QueryTransformer{
					compressionTransformer,
					translationTransformer,
				},
				QueryExpander: multiExpander,
				DocumentRetrievers: []rag.DocumentRetriever{
					vectorStoreRetriever,
				},
				DocumentRefiners: []rag.DocumentRefiner{
					rag.NewDeduplicationDocumentRefiner(),
					rag.NewRankDocumentRefiner(5),
				},
				QueryAugmenter: augmenter,
			})
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
			defer cancel()

			query, documents, err := pipeline.Run(ctx, tc.query)
			require.NoError(t, err)

			assert.NotNil(t, query)
			assert.NotEmpty(t, query.Text)
			assert.NotEmpty(t, documents)
			assert.LessOrEqual(t, len(documents), 5)

			t.Logf("Description: %s", tc.desc)
			t.Logf("Original query: %s", tc.query)
			t.Logf("Augmented query: %s", query.Text)
			t.Logf("Retrieved %d documents with mixed languages", len(documents))
		})
	}
}
