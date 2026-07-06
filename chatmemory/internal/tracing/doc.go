// Package tracing centralizes the OTel span emission shared by every
// chat history provider in this module. Each provider's Read / Write /
// Clear entry point wraps its inner SDK work with the helpers here so
// the DB semconv attribute set stays consistent across backends.
//
// Per doc/OBSERVABILITY.md §3.5 the attributes follow the OTel DB
// semconv plus chat history specific extensions on the bare domain (no
// brand prefix):
//
//	db.system               — provider id ("postgres", "redis", ...)
//	db.operation.name       — "read" / "write" / "clear" / "list"
//	gen_ai.conversation.id  — conversation id (omitted on the list scan)
//	chat_history.msg_count   — message count (read result / write input)
//	chat_history.conv_count  — conversation count (list result)
package tracing
