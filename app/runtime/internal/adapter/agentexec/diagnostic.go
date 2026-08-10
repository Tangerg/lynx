package agentexec

import "strings"

const maxExecutorDiagnosticRunes = 1000

// executorDiagnostic keeps persisted internal diagnostics bounded without
// splitting UTF-8. It is intentionally presentation-neutral: classification
// remains in the structured failure values owned by the inward contract.
func executorDiagnostic(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	runes := []rune(value)
	if len(runes) <= maxExecutorDiagnosticRunes {
		return value
	}
	return string(runes[:maxExecutorDiagnosticRunes-1]) + "…"
}
