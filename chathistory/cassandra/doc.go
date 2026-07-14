// Package cassandra is a chathistory Store backed by Apache Cassandra
// via gocql.
//
// Schema (created by InitializeSchema=true):
//
//	CREATE TABLE <keyspace>.<table> (
//	    conversation_id TEXT,
//	    seq             TIMEUUID,
//	    message         TEXT,
//	    PRIMARY KEY ((conversation_id), seq)
//	) WITH CLUSTERING ORDER BY (seq ASC);
//
// `conversation_id` is the partition key, `seq` (a TIMEUUID minted by
// the server's `now()` function) is the clustering key. Reads always
// hit a single partition and stream in insertion order; writes are
// monotone within a node and globally ordered by wall-clock time.
//
// Example:
//
//	cluster := gocql.NewCluster("127.0.0.1")
//	cluster.Keyspace = "lynx"
//	sess, _ := cluster.CreateSession()
//	defer sess.Close()
//
//	store, _ := cassandra.NewStore(cassandra.StoreConfig{
//	    Session:          sess,
//	    Keyspace:         "lynx",
//	    InitializeSchema: true,
//	})
package cassandra
