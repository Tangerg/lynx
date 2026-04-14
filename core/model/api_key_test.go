package model

import (
	"strings"
	"sync"
	"testing"
)

func TestNewApiKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantKey string
	}{
		{
			name:    "standard OpenAI key",
			input:   "sk-1234567890abcdef",
			wantKey: "sk-1234567890abcdef",
		},
		{
			name:    "empty key for no-auth",
			input:   "",
			wantKey: "",
		},
		{
			name:    "short development key",
			input:   "test-key",
			wantKey: "test-key",
		},
		{
			name:    "very long key",
			input:   "sk-proj-" + strings.Repeat("x", 100),
			wantKey: "sk-proj-" + strings.Repeat("x", 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ak := NewApiKey(tt.input)
			if ak == nil {
				t.Fatal("NewApiKey returned nil")
			}
			if got := ak.Get(); got != tt.wantKey {
				t.Errorf("Get() = %v, want %v", got, tt.wantKey)
			}
		})
	}
}

func TestApiKey_Get(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "returns exact key value",
			key:  "sk-test123",
			want: "sk-test123",
		},
		{
			name: "returns empty string",
			key:  "",
			want: "",
		},
		{
			name: "returns key with special characters",
			key:  "key-with-dash_underscore.dot",
			want: "key-with-dash_underscore.dot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ak := NewApiKey(tt.key)
			if got := ak.Get(); got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApiKey_Get_MultipleCallsReturnSameValue(t *testing.T) {
	key := "sk-consistent-key"
	ak := NewApiKey(key)

	// Call Get() multiple times
	for i := 0; i < 10; i++ {
		if got := ak.Get(); got != key {
			t.Errorf("Get() call %d = %v, want %v", i+1, got, key)
		}
	}
}

func TestApiKey_String(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "empty key",
			key:  "",
			want: "api_key=<empty>",
		},
		{
			name: "single character",
			key:  "a",
			want: "api_key=*",
		},
		{
			name: "short key (3 chars)",
			key:  "abc",
			want: "api_key=***",
		},
		{
			name: "exactly 10 chars",
			key:  "1234567890",
			want: "api_key=**********",
		},
		{
			name: "11 chars (first masking pattern)",
			key:  "12345678901",
			want: "api_key=12*******01",
		},
		{
			name: "standard OpenAI key",
			key:  "sk-1234567890abcdef",
			want: "api_key=sk***************ef",
		},
		{
			name: "very long key",
			key:  "sk-proj-" + strings.Repeat("x", 100),
			want: "api_key=sk" + strings.Repeat("*", 104) + strings.Repeat("x", 100)[98:100],
		},
		{
			name: "key with special characters",
			key:  "key-with_special.chars!@#",
			want: "api_key=ke*********************@#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ak := NewApiKey(tt.key).(*apiKey)
			if got := ak.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApiKey_String_CachedValue(t *testing.T) {
	key := "sk-test-key-for-caching"
	ak := NewApiKey(key).(*apiKey)

	// Get the string representation multiple times
	first := ak.String()
	for i := 0; i < 100; i++ {
		if got := ak.String(); got != first {
			t.Errorf("String() call %d returned different value: %v, want %v", i+1, got, first)
		}
	}
}

func TestApiKey_String_NoCredentialExposure(t *testing.T) {
	sensitiveKey := "sk-super-secret-key-12345"
	ak := NewApiKey(sensitiveKey).(*apiKey)

	masked := ak.String()

	// Ensure the masked string doesn't contain the full key
	if strings.Contains(masked, sensitiveKey) {
		t.Errorf("String() exposed full credential: %v", masked)
	}

	// Ensure it still contains the prefix for identification
	if !strings.HasPrefix(masked, "api_key=") {
		t.Errorf("String() should start with 'api_key=', got: %v", masked)
	}
}

func TestApiKey_ConcurrentAccess(t *testing.T) {
	key := "sk-concurrent-test-key"
	ak := NewApiKey(key).(*apiKey)

	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines*iterations)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Test Get()
				if got := ak.Get(); got != key {
					errors <- &testError{msg: "Get() returned wrong value"}
					return
				}

				// Test String()
				if got := ak.String(); !strings.HasPrefix(got, "api_key=") {
					errors <- &testError{msg: "String() returned invalid format"}
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Error(err)
	}
}

func TestApiKey_InterfaceCompliance(t *testing.T) {
	var _ ApiKey = (*apiKey)(nil)
	var _ ApiKey = NewApiKey("test")
}

func TestApiKey_StringerInterface(t *testing.T) {
	ak := NewApiKey("sk-test")

	// Verify it implements fmt.Stringer
	if _, ok := interface{}(ak).(interface{ String() string }); !ok {
		t.Error("apiKey doesn't implement fmt.Stringer interface")
	}
}

// Benchmark tests
func BenchmarkApiKey_Get(b *testing.B) {
	ak := NewApiKey("sk-benchmark-key")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ak.Get()
	}
}

func BenchmarkApiKey_String(b *testing.B) {
	ak := NewApiKey("sk-benchmark-key-for-string-method").(*apiKey)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ak.String()
	}
}

func BenchmarkNewApiKey(b *testing.B) {
	key := "sk-benchmark-key"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewApiKey(key)
	}
}

func BenchmarkApiKey_ConcurrentGet(b *testing.B) {
	ak := NewApiKey("sk-concurrent-benchmark")
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = ak.Get()
		}
	})
}

// Helper types
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
