package storetest_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/azureaisearch"
	"github.com/Tangerg/lynx/vectorstores/azurecosmos"
	"github.com/Tangerg/lynx/vectorstores/cassandra"
	"github.com/Tangerg/lynx/vectorstores/chroma"
	"github.com/Tangerg/lynx/vectorstores/clickhouse"
	"github.com/Tangerg/lynx/vectorstores/couchbase"
	"github.com/Tangerg/lynx/vectorstores/elasticsearch"
	"github.com/Tangerg/lynx/vectorstores/mariadb"
	"github.com/Tangerg/lynx/vectorstores/milvus"
	"github.com/Tangerg/lynx/vectorstores/mongodb"
	"github.com/Tangerg/lynx/vectorstores/neo4j"
	"github.com/Tangerg/lynx/vectorstores/opensearch"
	"github.com/Tangerg/lynx/vectorstores/oracle"
	"github.com/Tangerg/lynx/vectorstores/pgvector"
	"github.com/Tangerg/lynx/vectorstores/pinecone"
	"github.com/Tangerg/lynx/vectorstores/qdrant"
	"github.com/Tangerg/lynx/vectorstores/redis"
	"github.com/Tangerg/lynx/vectorstores/s3vectors"
	"github.com/Tangerg/lynx/vectorstores/tidb"
	"github.com/Tangerg/lynx/vectorstores/typesense"
	"github.com/Tangerg/lynx/vectorstores/vectara"
	"github.com/Tangerg/lynx/vectorstores/vespa"
	"github.com/Tangerg/lynx/vectorstores/weaviate"
)

type compiler struct {
	visit    func(filter.Predicate) error
	snapshot func() any
}

type sqlResult struct {
	query string
	args  any
}

func TestVisitorLifecycle(t *testing.T) {
	factories := []struct {
		name string
		new  func() compiler
	}{
		{name: "azure_ai_search", new: func() compiler {
			visitor := azureaisearch.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "azure_cosmos", new: func() compiler {
			visitor := azurecosmos.NewVisitor("c", "metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "cassandra", new: func() compiler {
			visitor := cassandra.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "chroma", new: func() compiler {
			visitor := chroma.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "clickhouse", new: func() compiler {
			visitor := clickhouse.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "couchbase", new: func() compiler {
			visitor := couchbase.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "elasticsearch", new: func() compiler {
			visitor := elasticsearch.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "mariadb", new: func() compiler {
			visitor := mariadb.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "milvus", new: func() compiler {
			visitor := milvus.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "mongodb", new: func() compiler {
			visitor := mongodb.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "neo4j", new: func() compiler {
			visitor := neo4j.NewVisitor("n", "metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "opensearch", new: func() compiler {
			visitor := opensearch.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "oracle", new: func() compiler {
			visitor := oracle.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "pgvector", new: func() compiler {
			visitor := pgvector.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "pinecone", new: func() compiler {
			visitor := pinecone.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any {
				result, err := visitor.Filter()
				return struct {
					result any
					err    string
				}{result: result, err: errorString(err)}
			}}
		}},
		{name: "qdrant", new: func() compiler {
			visitor := qdrant.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Filter() }}
		}},
		{name: "redis", new: func() compiler {
			visitor := redis.NewVisitor(map[string]redis.MetadataFieldType{
				"a": redis.FieldNumeric,
				"b": redis.FieldNumeric,
			})
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "s3_vectors", new: func() compiler {
			visitor := s3vectors.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "tidb", new: func() compiler {
			visitor := tidb.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any {
				query, args := visitor.Result()
				return sqlResult{query: query, args: args}
			}}
		}},
		{name: "typesense", new: func() compiler {
			visitor := typesense.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "vectara", new: func() compiler {
			visitor := vectara.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "vespa", new: func() compiler {
			visitor := vespa.NewVisitor("metadata")
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
		{name: "weaviate", new: func() compiler {
			visitor := weaviate.NewVisitor()
			return compiler{visit: visitor.Visit, snapshot: func() any { return visitor.Result() }}
		}},
	}

	first := filter.EQ("a", 1)
	last := filter.EQ("b", 2)

	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			want := factory.new()
			if err := want.visit(last); err != nil {
				t.Fatalf("fresh Visit: %v", err)
			}

			got := factory.new()
			if err := got.visit(first); err != nil {
				t.Fatalf("first Visit: %v", err)
			}
			if err := got.visit(last); err != nil {
				t.Fatalf("reused Visit: %v", err)
			}
			assertSnapshot(t, got.snapshot(), want.snapshot())

			if err := got.visit(nil); err == nil {
				t.Fatal("nil predicate must fail")
			}
			if err := got.visit(last); err != nil {
				t.Fatalf("Visit after failure: %v", err)
			}
			assertSnapshot(t, got.snapshot(), want.snapshot())
		})
	}
}

func assertSnapshot(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result after reuse = %#v, want fresh result %#v", got, want)
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
