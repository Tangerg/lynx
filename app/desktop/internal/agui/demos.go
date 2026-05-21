package agui

// Demo registry. The threadId sent from the frontend (= active session id)
// picks a demo here; unknown ids fall through to the default coding demo
// so first-run / fresh-thread behaviour is unchanged.

var demos = map[string][]Step{
	"s1": refactorDemo(),
	"s2": pastaDemo(),
	"s3": travelDemo(),
	"s4": writingDemo(),
	"s5": photosynthesisDemo(),
	"s6": loginFlowDemo(),
	"s7": cloudCompareDemo(),
}

func resolveDemo(threadID string) []Step {
	if d, ok := demos[threadID]; ok && len(d) > 0 {
		return d
	}
	return demos["s1"]
}

// ---------------------------------------------------------------------------
// s1 — Refactor auth.ts → Result<T, E> (coding scenario, the original demo)
// ---------------------------------------------------------------------------

func refactorDemo() []Step {
	return []Step{
		Pause(250, 600),
		CustomVal(customPlan, map[string]any{"items": planItems}),
		Pause(300, 700),
		User(userPrompt),
		Pause(500, 1000),

		Say(introText),
		Pause(350, 800),
		CustomFn(customPlanBlock, func(e *env) any {
			return map[string]any{"messageId": e.assistantID}
		}),
		Pause(400, 900),
		Say(" " + postPlanText),
		Pause(300, 700),

		RunTool("t1"),
		Pause(250, 650),
		RunTool("t2"),
		Pause(300, 800),

		Say(" " + postGrepText),
		Pause(400, 900),
		Think(reasoning1Text),
		Pause(400, 900),

		RunTool("tw"),
		Pause(400, 900),
		CustomFn(customSearchResults, func(e *env) any {
			return map[string]any{"parentMessageId": e.assistantID, "results": searchResults}
		}),
		Pause(500, 1000),

		Say(" " + postSearchText),
		Pause(400, 800),
		CustomFn(customCodeProposal, func(e *env) any {
			return map[string]any{
				"parentMessageId": e.assistantID,
				"lang":            "typescript",
				"file":            "src/lib/result.ts",
				"text":            proposedCode,
			}
		}),
		Pause(500, 1000),
		RunTool("t3"),
		Pause(300, 700),

		Say(" " + postWriteText),
		Pause(300, 700),
		RunTool("t4"),
		Pause(350, 800),

		Say(" " + postEditText),
		Pause(350, 800),
		RunTool("t5"),
		Pause(400, 900),

		Say(" " + postTypecheckText),
		Pause(300, 700),
		RunTool("t6"),
		Pause(400, 900),

		Say(" " + postBillingFixText),
		Pause(500, 1000),
		Think(reasoning2Text),
		Pause(400, 900),

		CustomFn(customApproval, func(e *env) any {
			return map[string]any{
				"parentMessageId": e.assistantID,
				"text":            "Run integration tests for the auth + billing slice",
				"command":         "pnpm test --filter=auth --filter=billing",
				"reason":          "Tests touch the Stripe sandbox API. Output is logged but no charges are made.",
			}
		}),
		Pause(500, 1000),
		Say(" " + postApprovalText),
		Pause(300, 700),
		CloseAssistant(),

		// Final long-running bash tool — never closes, surfaces "still
		// working" telemetry until the user disconnects.
		Tool("bash", "pnpm test --filter=auth --filter=billing", ToolSummary{}),

		TelemetryLoop(activityLines),
	}
}
