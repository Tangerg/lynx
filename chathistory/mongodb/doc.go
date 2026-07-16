// Package mongodb is a chathistory Store backed by MongoDB via the
// official mongo-driver v2.
//
// Each message is a document in the configured collection:
//
//	{
//	    "_id":             ObjectId(...),     // assigned by the driver
//	    "conversation_id": "u-42",
//	    "message":         "<json>",          // canonical chat.Message wire shape
//	    "created_at":      ISODate(...),
//	}
//
// Documents are read back in `_id` ascending order — ObjectIDs are
// monotonically increasing within a single InsertMany batch, so
// chronological order is recovered without a separate `seq` field.
//
// Example:
//
//	col := client.Database("lynx").Collection("chat_history")
//	store, _ := mongodb.New(mongodb.Config{
//	    Collection:       col,
//	    InitializeSchema: true, // create the conversation_id index
//	})
package mongodb
