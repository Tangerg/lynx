package azureopenai

import "testing"

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "canonical", input: "https://example.openai.azure.com/openai/v1/", want: "https://example.openai.azure.com/openai/v1/"},
		{name: "adds trailing slash", input: "https://example.openai.azure.com/openai/v1", want: "https://example.openai.azure.com/openai/v1/"},
		{name: "rejects resource root", input: "https://example.openai.azure.com", wantErr: true},
		{name: "rejects dated query", input: "https://example.openai.azure.com/openai/v1/?api-version=2024-12-01-preview", wantErr: true},
		{name: "rejects relative URL", input: "/openai/v1/", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeBaseURL(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeBaseURL() error = %v; wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("normalizeBaseURL() = %q; want %q", got, test.want)
			}
		})
	}
}
