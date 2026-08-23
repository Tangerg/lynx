package codeintel

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func (service *Service) DiagnoseMutation(ctx context.Context, root, path string, apply func() (string, error)) (string, error) {
	if apply == nil {
		return "", fmt.Errorf("codeintel: mutation callback is required")
	}
	if !service.Supported(path) {
		return apply()
	}
	baseline, _ := service.fileDiagnostics(ctx, root, path)
	output, err := apply()
	if err != nil {
		return output, err
	}
	after, diagnosticErr := service.fileDiagnostics(ctx, root, path)
	if diagnosticErr != nil {
		return output, nil
	}
	section := formatNewDiagnostics(relativePath(root, path), newDiagnostics(baseline, after))
	if section == "" {
		return output, nil
	}
	return output + "\n\n" + section, nil
}

func (service *Service) Supported(path string) bool {
	if service == nil {
		return false
	}
	_, found := service.byExtension[strings.ToLower(filepath.Ext(path))]
	return found
}

func (service *Service) fileDiagnostics(ctx context.Context, root, path string) ([]diagnostic, error) {
	path, err := confinedPath(root, path)
	if err != nil {
		return nil, err
	}
	client, err := service.clientFor(ctx, root, path)
	if err != nil {
		return nil, err
	}
	uri, signal, changed, err := client.sync(ctx, path)
	if err != nil {
		return nil, err
	}
	return client.awaitDiagnostics(ctx, uri, signal, changed), nil
}

func newDiagnostics(before, after []diagnostic) []diagnostic {
	seen := make(map[string]int, len(before))
	for _, value := range before {
		seen[diagnosticKey(value)]++
	}
	result := make([]diagnostic, 0)
	for _, value := range after {
		key := diagnosticKey(value)
		if seen[key] > 0 {
			seen[key]--
			continue
		}
		result = append(result, value)
	}
	return result
}

func diagnosticKey(value diagnostic) string {
	return fmt.Sprintf("%d\x00%s\x00%s", value.Severity, value.Source, value.Message)
}

func formatNewDiagnostics(path string, values []diagnostic) string {
	problems := make([]diagnostic, 0, len(values))
	for _, value := range values {
		if value.Severity == 0 || value.Severity == 1 || value.Severity == 2 {
			problems = append(problems, value)
		}
	}
	if len(problems) == 0 {
		return ""
	}
	return fmt.Sprintf("Language server flagged %d new problem(s) in %s after this edit:\n%s", len(problems), path, formatDiagnostics(path, problems))
}
