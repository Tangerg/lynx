// Package tools ships a small, dependency-free library of built-in
// [chat.Tool]s agents can hand to an LLM — the lynx counterpart to
// embabel's tools/file + tools/math.
//
// Deliberately narrow: it covers general-purpose arithmetic and sandboxed
// file reads. It intentionally omits blackboard- and process-introspection
// tools (lynx is planner-centric — the planner reads the blackboard, the
// LLM should not) and OS-automation tools (out of scope for a portable
// library).
//
// All constructors return ready-to-register [chat.Tool] values:
//
//	mathTools := tools.MathTools()
//	fileTools, _ := tools.FileTools("/srv/docs")
//	callMW, streamMW := chat.NewToolMiddleware()
//	resp, _ := pc.Chat().
//	    WithMiddlewares(callMW, streamMW).
//	    WithTools(append(mathTools, fileTools...)...).
//	    Call().Text(ctx)
package tools
