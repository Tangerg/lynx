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
// `conversation_id` is the partition key and `seq` is a client-generated
// TIMEUUID clustering key. Reads always hit a single partition and stream in
// insertion order; each Write uses one unlogged batch for that partition.
//
// Example:
//
//	cluster := gocql.NewCluster("127.0.0.1")
//	cluster.Keyspace = "lynx"
//	sess, _ := cluster.CreateSession()
//	defer sess.Close()
//
//	store, _ := cassandra.New(cassandra.Config{
//	    Session:          sess,
//	    Keyspace:         "lynx",
//	    InitializeSchema: true,
//	})
package cassandra
