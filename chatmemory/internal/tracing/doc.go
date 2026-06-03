// Package tracing centralizes the OTel span emission shared by every
// chat-memory provider in this module. Each provider's Read / Write /
// Clear entry point wraps its inner SDK work with the helpers here so
// the DB semconv attribute set stays consistent across backends.
//
// Per doc/OBSERVABILITY.md §3.5 the attributes follow the OTel DB
// semconv plus the lynx chat-memory specific extensions:
//
//	db.system                   — provider id ("postgres", "redis", ...)
//	db.operation.name           — "read" / "write" / "clear"
//	lynx.chat_memory.conv_id    — conversation id
//	lynx.chat_memory.msg_count  — number of messages (read result / write input)
package tracing
