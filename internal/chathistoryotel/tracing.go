package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

type System uint8

const (
	PostgreSQL System = iota + 1
	Redis
	MongoDB
	Cassandra
	AzureCosmosDB
	Neo4j
)

var (
	operationNameAttr     = attribute.Key("chat_history.operation.name")
	messageCountAttr      = attribute.Key("chat_history.message.count")
	conversationCountAttr = attribute.Key("chat_history.conversation.count")
)

func (system System) name() string {
	switch system {
	case PostgreSQL:
		return "postgres"
	case Redis:
		return "redis"
	case MongoDB:
		return "mongodb"
	case Cassandra:
		return "cassandra"
	case AzureCosmosDB:
		return "cosmosdb"
	case Neo4j:
		return "neo4j"
	default:
		return "unknown"
	}
}

func (system System) attribute() attribute.KeyValue {
	switch system {
	case PostgreSQL:
		return semconv.DBSystemNamePostgreSQL
	case Redis:
		return semconv.DBSystemNameRedis
	case MongoDB:
		return semconv.DBSystemNameMongoDB
	case Cassandra:
		return semconv.DBSystemNameCassandra
	case AzureCosmosDB:
		return semconv.DBSystemNameAzureCosmosDB
	case Neo4j:
		return semconv.DBSystemNameNeo4j
	default:
		return semconv.DBSystemNameKey.String("unknown")
	}
}

func tracerFor(system System) trace.Tracer {
	return otel.Tracer(
		"github.com/Tangerg/lynx/chathistory/"+system.name(),
		trace.WithSchemaURL(semconv.SchemaURL),
	)
}

func start(ctx context.Context, system System, operation, conversationID string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		system.attribute(),
		operationNameAttr.String(operation),
	}
	if conversationID != "" {
		attrs = append(attrs, semconv.GenAIConversationID(conversationID))
	}
	return tracerFor(system).Start(ctx, "chathistory."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// StartRead opens a span for a Store.Read operation.
func StartRead(ctx context.Context, system System, conversationID string) (context.Context, trace.Span) {
	return start(ctx, system, "read", conversationID)
}

// StartWrite opens a span for a Store.Write operation.
func StartWrite(ctx context.Context, system System, conversationID string, messageCount int) (context.Context, trace.Span) {
	ctx, span := start(ctx, system, "write", conversationID)
	if messageCount > 0 {
		span.SetAttributes(messageCountAttr.Int(messageCount))
	}
	return ctx, span
}

// StartClear opens a span for a Store.Clear operation.
func StartClear(ctx context.Context, system System, conversationID string) (context.Context, trace.Span) {
	return start(ctx, system, "clear", conversationID)
}

// StartList opens a span for a Lister.Conversations operation. A list is a
// cross-conversation scan, so it carries no conversation ID attribute.
func StartList(ctx context.Context, system System) (context.Context, trace.Span) {
	return start(ctx, system, "list", "")
}

// Finish records err on span and ends it.
func Finish(span trace.Span, err error, extra ...attribute.KeyValue) {
	if len(extra) > 0 {
		span.SetAttributes(extra...)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// RecordReadResult stamps the resulting message count onto a Read span
// before ending it.
func RecordReadResult(span trace.Span, err error, messageCount int) {
	Finish(span, err, messageCountAttr.Int(messageCount))
}

// RecordListResult stamps the number of conversations found onto a List
// span before ending it.
func RecordListResult(span trace.Span, err error, conversationCount int) {
	Finish(span, err, conversationCountAttr.Int(conversationCount))
}
