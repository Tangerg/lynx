package tracing

import (
	"testing"

	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func TestSystemsUseCanonicalDatabaseAttributes(t *testing.T) {
	tests := []struct {
		system System
		name   string
		value  string
	}{
		{system: PostgreSQL, name: "postgres", value: "postgresql"},
		{system: Redis, name: "redis", value: "redis"},
		{system: MongoDB, name: "mongodb", value: "mongodb"},
		{system: Cassandra, name: "cassandra", value: "cassandra"},
		{system: AzureCosmosDB, name: "cosmosdb", value: "azure.cosmosdb"},
		{system: Neo4j, name: "neo4j", value: "neo4j"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.system.name(); got != test.name {
				t.Fatalf("name = %q, want %q", got, test.name)
			}
			attribute := test.system.attribute()
			if attribute.Key != semconv.DBSystemNameKey || attribute.Value.AsString() != test.value {
				t.Fatalf("attribute = %v, want %s=%q", attribute, semconv.DBSystemNameKey, test.value)
			}
		})
	}
}
