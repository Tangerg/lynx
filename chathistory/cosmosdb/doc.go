// Package cosmosdb is a chathistory Store backed by Azure Cosmos DB
// (NoSQL API) via the official Azure SDK.
//
// Each message is stored as a document keyed by a synthesized
// composite id (`<conversation_id>_<seq>`):
//
//	{
//	    "id":              "u-42_1716210000000123456",
//	    "conversation_id": "u-42",
//	    "seq":             1716210000000123456,
//	    "message":         "<json>",
//	    "created_at":      "2026-05-20T08:00:00Z"
//	}
//
// `conversation_id` is the partition key, set when provisioning the
// container. Reads issue a single-partition query ordered by `seq`.
// `seq` is a Go-side nanosecond timestamp + batch offset, so all
// messages from one Write call are strictly ordered.
//
// Example:
//
//	cosmos, _ := azcosmos.NewClient(endpoint, cred, nil)
//	container, _ := cosmos.NewContainer("lynx", "chat_history")
//	store, _ := cosmosdb.New(cosmosdb.Config{Container: container})
package cosmosdb
