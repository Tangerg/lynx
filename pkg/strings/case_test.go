package strings

import (
	"reflect"
	"strings"
	"testing"
)

func TestCamelCase_Split(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"x", []string{"x"}},
		{"getUserName", []string{"get", "User", "Name"}},
		{"GetUserName", []string{"Get", "User", "Name"}},
		{"HTTPRequestHandler", []string{"HTTP", "Request", "Handler"}},
		{"User123ProfileData", []string{"User", "123", "Profile", "Data"}},
		{"checkUser2FAStatus", []string{"check", "User", "2", "FA", "Status"}},
		{"OAuthTokenGenerator", []string{"O", "Auth", "Token", "Generator"}},
		{"Error404Handler", []string{"Error", "404", "Handler"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := AsCamelCase(tt.in).Split()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Split(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCamelCase_SplitWith(t *testing.T) {
	t.Run("nil fn no transform", func(t *testing.T) {
		got := AsCamelCase("getUserName").SplitWith(nil)
		want := []string{"get", "User", "Name"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("ToLower", func(t *testing.T) {
		got := AsCamelCase("getUserHTTPResponse").SplitWith(strings.ToLower)
		want := []string{"get", "user", "http", "response"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestCamelCase_SplitToLowerUpper(t *testing.T) {
	c := AsCamelCase("getUserName")
	if got, want := c.SplitToLower(), []string{"get", "user", "name"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SplitToLower = %v, want %v", got, want)
	}
	if got, want := c.SplitToUpper(), []string{"GET", "USER", "NAME"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SplitToUpper = %v, want %v", got, want)
	}
}

func TestCamelCase_ToSnakeCase(t *testing.T) {
	tests := []struct {
		in   string
		want SnakeCase
	}{
		{"", ""},
		{"getUserName", "get_user_name"},
		{"GetUserName", "get_user_name"},
		{"getUserHTTPResponse", "get_user_http_response"},
		{"HTTPServer", "http_server"},
		{"User123Name", "user_123_name"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := AsCamelCase(tt.in).ToSnakeCase(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCamelCase_String(t *testing.T) {
	if got := AsCamelCase("Foo").String(); got != "Foo" {
		t.Errorf("String() = %q", got)
	}
}

func TestSnakeCase_Split(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a_b_c", []string{"a", "b", "c"}},
		{"_leading", []string{"", "leading"}},
		{"trailing_", []string{"trailing", ""}},
		{"a__b", []string{"a", "", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := AsSnakeCase(tt.in).Split()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSnakeCase_SplitWith(t *testing.T) {
	got := AsSnakeCase("get_user_name").SplitWith(strings.ToUpper)
	want := []string{"GET", "USER", "NAME"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSnakeCase_SplitToLowerUpper(t *testing.T) {
	s := AsSnakeCase("Get_User_NAME")
	if got, want := s.SplitToLower(), []string{"get", "user", "name"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SplitToLower = %v, want %v", got, want)
	}
	if got, want := s.SplitToUpper(), []string{"GET", "USER", "NAME"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SplitToUpper = %v, want %v", got, want)
	}
}

func TestSnakeCase_ToCamelCase(t *testing.T) {
	tests := []struct {
		in   string
		want CamelCase
	}{
		{"", ""},
		{"a", "a"},
		{"get_user_name", "getUserName"},
		{"get_user_http_response", "getUserHttpResponse"},
		{"_leading_underscore", "LeadingUnderscore"}, // empty first word preserves second's case
		{"a__b", "aB"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := AsSnakeCase(tt.in).ToCamelCase(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Only multi-letter segments survive a round trip; single-letter
	// segments collapse during camel-case splitting.
	tests := []string{"get_user_name", "process_data_chunk"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			snake := AsSnakeCase(in)
			camel := snake.ToCamelCase()
			back := camel.ToSnakeCase()
			if string(back) != in {
				t.Errorf("round trip %q → %q → %q", in, camel, back)
			}
		})
	}
}

func BenchmarkCamelCase_Split(b *testing.B) {
	c := AsCamelCase("processDataJSONFileXMLParser")
	for b.Loop() {
		_ = c.Split()
	}
}

func BenchmarkSnakeCase_ToCamelCase(b *testing.B) {
	s := AsSnakeCase("get_user_http_response_data")
	for b.Loop() {
		_ = s.ToCamelCase()
	}
}
