// Package cassandra is a history Store backed by Apache Cassandra
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
// `conversation_id` is the partition key and `seq` is a client-generated
// TIMEUUID clustering key. Each Write reserves a strictly increasing local
// sequence range and sends one unlogged batch to that partition. Concurrent
// calls and writes from distinct Store instances have no defined relative
// order.
//
// Example:
//
//	cluster := gocql.NewCluster("127.0.0.1")
//	cluster.Keyspace = "scope"
//	sess, _ := cluster.CreateSession()
//	defer sess.Close()
//
//	store, _ := cassandra.NewStore(ctx, cassandra.StoreConfig{
//	    Session:          sess,
//	    Keyspace:         "scope",
//	    InitializeSchema: true,
//	})
package cassandra
