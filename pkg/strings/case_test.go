package strings

import (
	"reflect"
	"testing"
)

// Test data
var camelCaseTestCases = []struct {
	name     string
	input    string
	expected []string
}{
	{
		name:     "uppercase abbreviation mixed",
		input:    "HTTPRequestHandler",
		expected: []string{"HTTP", "Request", "Handler"},
	},
	{
		name:     "mixed with numbers",
		input:    "User123ProfileData",
		expected: []string{"User", "123", "Profile", "Data"},
	},
	{
		name:     "uppercase abbreviation prefix",
		input:    "XMLParserUtility",
		expected: []string{"XML", "Parser", "Utility"},
	},
	{
		name:     "uppercase abbreviation with regular words",
		input:    "GetHTTPResponseCode",
		expected: []string{"Get", "HTTP", "Response", "Code"},
	},
	{
		name:     "lowerCamelCase with numbers",
		input:    "checkUser2FAStatus",
		expected: []string{"check", "User", "2", "FA", "Status"},
	},
	{
		name:     "uppercase abbreviation",
		input:    "APIServerHealthCheck",
		expected: []string{"API", "Server", "Health", "Check"},
	},
	{
		name:     "lowerCamelCase with uppercase abbreviation",
		input:    "processDataJSONFile",
		expected: []string{"process", "Data", "JSON", "File"},
	},
	{
		name:     "OAuth mixed case",
		input:    "OAuthTokenGenerator",
		expected: []string{"O", "Auth", "Token", "Generator"},
	},
	{
		name:     "number with uppercase letter",
		input:    "Error404Handler",
		expected: []string{"Error", "404", "Handler"},
	},
	{
		name:     "version number mixed",
		input:    "UserProfilePageV2",
		expected: []string{"User", "Profile", "Page", "V", "2"},
	},
	{
		name:     "lowerCamelCase with uppercase abbreviation",
		input:    "emailServiceSMTPConfig",
		expected: []string{"email", "Service", "SMTP", "Config"},
	},
	{
		name:     "regular camelCase",
		input:    "DatabaseConnectionPool",
		expected: []string{"Database", "Connection", "Pool"},
	},
	{
		name:     "uppercase abbreviation, number and word mixed",
		input:    "CheckIOStatusPort45",
		expected: []string{"Check", "IO", "Status", "Port", "45"},
	},
	{
		name:     "year suffix",
		input:    "ImageProcessingTool2024",
		expected: []string{"Image", "Processing", "Tool", "2024"},
	},
	{
		name:     "contains non-letter symbol (underscore)",
		input:    "verifyUserInput_UTF8Encoding",
		expected: []string{"verify", "User", "Input", "_", "UTF", "8", "Encoding"},
	},
}

var snakeCaseTestCases = []struct {
	name     string
	input    string
	expected []string
}{
	{
		name:     "simple snake case",
		input:    "user_account_manager",
		expected: []string{"user", "account", "manager"},
	},
	{
		name:     "data processing unit",
		input:    "data_processing_unit",
		expected: []string{"data", "processing", "unit"},
	},
	{
		name:     "network configuration",
		input:    "network_configuration_tool",
		expected: []string{"network", "configuration", "tool"},
	},
	{
		name:     "file upload service",
		input:    "file_upload_service",
		expected: []string{"file", "upload", "service"},
	},
	{
		name:     "user profile settings",
		input:    "user_profile_settings",
		expected: []string{"user", "profile", "settings"},
	},
	{
		name:     "order tracking system",
		input:    "order_tracking_system",
		expected: []string{"order", "tracking", "system"},
	},
	{
		name:     "payment gateway integration",
		input:    "payment_gateway_integration",
		expected: []string{"payment", "gateway", "integration"},
	},
	{
		name:     "product inventory manager",
		input:    "product_inventory_manager",
		expected: []string{"product", "inventory", "manager"},
	},
	{
		name:     "shopping cart service",
		input:    "shopping_cart_service",
		expected: []string{"shopping", "cart", "service"},
	},
	{
		name:     "authentication middleware",
		input:    "authentication_middleware",
		expected: []string{"authentication", "middleware"},
	},
	{
		name:     "with numbers",
		input:    "user_123_name",
		expected: []string{"user", "123", "name"},
	},
	{
		name:     "id card",
		input:    "id_card",
		expected: []string{"id", "card"},
	},
}

// TestAsCamelCase tests the AsCamelCase constructor
func TestAsCamelCase(t *testing.T) {
	t.Run("creates CamelCase type", func(t *testing.T) {
		input := "TestString"
		result := AsCamelCase(input)

		if string(result) != input {
			t.Errorf("AsCamelCase(%q) = %q, want %q", input, result, input)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		result := AsCamelCase("")
		if result != "" {
			t.Errorf("AsCamelCase(\"\") = %q, want empty", result)
		}
	})
}

// TestCamelCase_String tests the String method
func TestCamelCase_String(t *testing.T) {
	testCases := []string{
		"TestString",
		"anotherTest",
		"HTTPServer",
		"",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			camel := AsCamelCase(tc)
			result := camel.String()

			if result != tc {
				t.Errorf("String() = %q, want %q", result, tc)
			}
		})
	}
}

// TestCamelCase_Split tests the Split method
func TestCamelCase_Split(t *testing.T) {
	for _, tc := range camelCaseTestCases {
		t.Run(tc.name, func(t *testing.T) {
			camel := AsCamelCase(tc.input)
			result := camel.Split()

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Split() = %v, want %v", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		camel := AsCamelCase("")
		result := camel.Split()

		if result != nil {
			t.Errorf("Split() = %v, want nil", result)
		}
	})

	t.Run("single character", func(t *testing.T) {
		camel := AsCamelCase("a")
		result := camel.Split()
		expected := []string{"a"}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Split() = %v, want %v", result, expected)
		}
	})

	t.Run("all lowercase", func(t *testing.T) {
		camel := AsCamelCase("alllowercase")
		result := camel.Split()
		expected := []string{"alllowercase"}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Split() = %v, want %v", result, expected)
		}
	})

	t.Run("all uppercase", func(t *testing.T) {
		camel := AsCamelCase("ALLUPPERCASE")
		result := camel.Split()
		expected := []string{"ALLUPPERCASE"}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Split() = %v, want %v", result, expected)
		}
	})
}

// TestCamelCase_SplitToLower tests the SplitToLower method
func TestCamelCase_SplitToLower(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple case",
			input:    "HTTPRequestHandler",
			expected: []string{"http", "request", "handler"},
		},
		{
			name:     "with numbers",
			input:    "User123Data",
			expected: []string{"user", "123", "data"},
		},
		{
			name:     "mixed case",
			input:    "XMLParser",
			expected: []string{"xml", "parser"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			camel := AsCamelCase(tc.input)
			result := camel.SplitToLower()

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("SplitToLower() = %v, want %v", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		camel := AsCamelCase("")
		result := camel.SplitToLower()

		if result != nil {
			t.Errorf("SplitToLower() = %v, want nil", result)
		}
	})
}

// TestCamelCase_SplitToUpper tests the SplitToUpper method
func TestCamelCase_SplitToUpper(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple case",
			input:    "httpRequestHandler",
			expected: []string{"HTTP", "REQUEST", "HANDLER"},
		},
		{
			name:     "with numbers",
			input:    "user123Data",
			expected: []string{"USER", "123", "DATA"},
		},
		{
			name:     "mixed case",
			input:    "xmlParser",
			expected: []string{"XML", "PARSER"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			camel := AsCamelCase(tc.input)
			result := camel.SplitToUpper()

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("SplitToUpper() = %v, want %v", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		camel := AsCamelCase("")
		result := camel.SplitToUpper()

		if result != nil {
			t.Errorf("SplitToUpper() = %v, want nil", result)
		}
	})
}

// TestCamelCase_ToSnakeCase tests the ToSnakeCase method
func TestCamelCase_ToSnakeCase(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple camelCase",
			input:    "userAccountManager",
			expected: "user_account_manager",
		},
		{
			name:     "PascalCase",
			input:    "UserAccountManager",
			expected: "user_account_manager",
		},
		{
			name:     "with abbreviation",
			input:    "HTTPRequestHandler",
			expected: "http_request_handler",
		},
		{
			name:     "with numbers",
			input:    "User123ProfileData",
			expected: "user_123_profile_data",
		},
		{
			name:     "XMLParser",
			input:    "XMLParserUtility",
			expected: "xml_parser_utility",
		},
		{
			name:     "single word",
			input:    "user",
			expected: "user",
		},
		{
			name:     "single uppercase word",
			input:    "USER",
			expected: "user",
		},
		{
			name:     "with underscore",
			input:    "user_Name",
			expected: "user_name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			camel := AsCamelCase(tc.input)
			result := camel.ToSnakeCase()

			if string(result) != tc.expected {
				t.Errorf("ToSnakeCase() = %q, want %q", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		camel := AsCamelCase("")
		result := camel.ToSnakeCase()

		if result != "" {
			t.Errorf("ToSnakeCase() = %q, want empty", result)
		}
	})
}

// TestAsSnakeCase tests the AsSnakeCase constructor
func TestAsSnakeCase(t *testing.T) {
	t.Run("creates SnakeCase type", func(t *testing.T) {
		input := "test_string"
		result := AsSnakeCase(input)

		if string(result) != input {
			t.Errorf("AsSnakeCase(%q) = %q, want %q", input, result, input)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		result := AsSnakeCase("")
		if result != "" {
			t.Errorf("AsSnakeCase(\"\") = %q, want empty", result)
		}
	})
}

// TestSnakeCase_String tests the String method
func TestSnakeCase_String(t *testing.T) {
	testCases := []string{
		"test_string",
		"another_test",
		"http_server",
		"",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			snake := AsSnakeCase(tc)
			result := snake.String()

			if result != tc {
				t.Errorf("String() = %q, want %q", result, tc)
			}
		})
	}
}

// TestSnakeCase_Split tests the Split method
func TestSnakeCase_Split(t *testing.T) {
	for _, tc := range snakeCaseTestCases {
		t.Run(tc.name, func(t *testing.T) {
			snake := AsSnakeCase(tc.input)
			result := snake.Split()

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Split() = %v, want %v", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		snake := AsSnakeCase("")
		result := snake.Split()

		if result != nil {
			t.Errorf("Split() = %v, want nil", result)
		}
	})

	t.Run("no underscore", func(t *testing.T) {
		snake := AsSnakeCase("single")
		result := snake.Split()
		expected := []string{"single"}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Split() = %v, want %v", result, expected)
		}
	})

	t.Run("multiple consecutive underscores", func(t *testing.T) {
		snake := AsSnakeCase("test__multiple___underscores")
		result := snake.Split()
		// strings.Split behavior: creates empty strings for consecutive separators
		expected := []string{"test", "", "multiple", "", "", "underscores"}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Split() = %v, want %v", result, expected)
		}
	})

	t.Run("leading underscore", func(t *testing.T) {
		snake := AsSnakeCase("_leading")
		result := snake.Split()
		expected := []string{"", "leading"}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Split() = %v, want %v", result, expected)
		}
	})

	t.Run("trailing underscore", func(t *testing.T) {
		snake := AsSnakeCase("trailing_")
		result := snake.Split()
		expected := []string{"trailing", ""}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Split() = %v, want %v", result, expected)
		}
	})
}

// TestSnakeCase_SplitToLower tests the SplitToLower method
func TestSnakeCase_SplitToLower(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "already lowercase",
			input:    "user_account_manager",
			expected: []string{"user", "account", "manager"},
		},
		{
			name:     "mixed case",
			input:    "User_Account_Manager",
			expected: []string{"user", "account", "manager"},
		},
		{
			name:     "all uppercase",
			input:    "USER_ACCOUNT_MANAGER",
			expected: []string{"user", "account", "manager"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snake := AsSnakeCase(tc.input)
			result := snake.SplitToLower()

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("SplitToLower() = %v, want %v", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		snake := AsSnakeCase("")
		result := snake.SplitToLower()

		if result != nil {
			t.Errorf("SplitToLower() = %v, want nil", result)
		}
	})
}

// TestSnakeCase_SplitToUpper tests the SplitToUpper method
func TestSnakeCase_SplitToUpper(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "lowercase",
			input:    "user_account_manager",
			expected: []string{"USER", "ACCOUNT", "MANAGER"},
		},
		{
			name:     "mixed case",
			input:    "User_Account_Manager",
			expected: []string{"USER", "ACCOUNT", "MANAGER"},
		},
		{
			name:     "already uppercase",
			input:    "USER_ACCOUNT_MANAGER",
			expected: []string{"USER", "ACCOUNT", "MANAGER"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snake := AsSnakeCase(tc.input)
			result := snake.SplitToUpper()

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("SplitToUpper() = %v, want %v", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		snake := AsSnakeCase("")
		result := snake.SplitToUpper()

		if result != nil {
			t.Errorf("SplitToUpper() = %v, want nil", result)
		}
	})
}

// TestSnakeCase_ToCamelCase tests the ToCamelCase method
func TestSnakeCase_ToCamelCase(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple snake case",
			input:    "user_account_manager",
			expected: "userAccountManager",
		},
		{
			name:     "with numbers",
			input:    "user_123_name",
			expected: "user123Name",
		},
		{
			name:     "single word",
			input:    "user",
			expected: "user",
		},
		{
			name:     "two words",
			input:    "user_name",
			expected: "userName",
		},
		{
			name:     "three words",
			input:    "user_account_manager",
			expected: "userAccountManager",
		},
		{
			name:     "with uppercase",
			input:    "HTTP_REQUEST_HANDLER",
			expected: "httpRequestHandler",
		},
		{
			name:     "mixed case input",
			input:    "User_Account_Manager",
			expected: "userAccountManager",
		},
		{
			name:     "single character words",
			input:    "a_b_c",
			expected: "aBC",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snake := AsSnakeCase(tc.input)
			result := snake.ToCamelCase()

			if string(result) != tc.expected {
				t.Errorf("ToCamelCase() = %q, want %q", result, tc.expected)
			}
		})
	}

	t.Run("empty string", func(t *testing.T) {
		snake := AsSnakeCase("")
		result := snake.ToCamelCase()

		if result != "" {
			t.Errorf("ToCamelCase() = %q, want empty", result)
		}
	})

	t.Run("with empty parts", func(t *testing.T) {
		snake := AsSnakeCase("user__name")
		result := snake.ToCamelCase()
		expected := "userName"

		if string(result) != expected {
			t.Errorf("ToCamelCase() = %q, want %q", result, expected)
		}
	})
}

// TestRoundTrip tests conversion round trips
func TestRoundTrip(t *testing.T) {
	t.Run("camelCase to snake_case and back", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected string
		}{
			{
				name:     "simple case",
				input:    "userAccountManager",
				expected: "userAccountManager",
			},
			{
				name:     "with numbers",
				input:    "user123Name",
				expected: "user123Name",
			},
			{
				name:     "single word",
				input:    "user",
				expected: "user",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				camel := AsCamelCase(tc.input)
				snake := camel.ToSnakeCase()
				result := snake.ToCamelCase()

				if string(result) != tc.expected {
					t.Errorf("round trip failed: %q -> %q -> %q, want %q",
						tc.input, snake, result, tc.expected)
				}
			})
		}
	})

	t.Run("snake_case to camelCase and back", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected string
		}{
			{
				name:     "simple case",
				input:    "user_account_manager",
				expected: "user_account_manager",
			},
			{
				name:     "with numbers",
				input:    "user_123_name",
				expected: "user_123_name",
			},
			{
				name:     "single word",
				input:    "user",
				expected: "user",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				snake := AsSnakeCase(tc.input)
				camel := snake.ToCamelCase()
				result := camel.ToSnakeCase()

				if string(result) != tc.expected {
					t.Errorf("round trip failed: %q -> %q -> %q, want %q",
						tc.input, camel, result, tc.expected)
				}
			})
		}
	})
}

// TestEdgeCases tests edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("unicode characters in camelCase", func(t *testing.T) {
		camel := AsCamelCase("用户账户管理器")
		result := camel.Split()
		// Unicode characters don't split like ASCII
		if len(result) == 0 {
			t.Error("Split() returned empty for unicode string")
		}
	})

	t.Run("special characters in snake_case", func(t *testing.T) {
		snake := AsSnakeCase("user@account#manager")
		result := snake.Split()
		// Should split only on underscores
		if len(result) != 1 {
			t.Errorf("Split() = %v, should not split on special chars", result)
		}
	})

	t.Run("very long string", func(t *testing.T) {
		longCamel := AsCamelCase("VeryLongCamelCaseStringWithManyManyManyWords")
		result := longCamel.Split()

		if len(result) == 0 {
			t.Error("Split() failed on long string")
		}
	})
}

// BenchmarkCamelCase_Split benchmarks the Split method
func BenchmarkCamelCase_Split(b *testing.B) {
	camel := AsCamelCase("HTTPRequestHandler")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = camel.Split()
	}
}

// BenchmarkCamelCase_ToSnakeCase benchmarks the ToSnakeCase method
func BenchmarkCamelCase_ToSnakeCase(b *testing.B) {
	camel := AsCamelCase("HTTPRequestHandler")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = camel.ToSnakeCase()
	}
}

// BenchmarkSnakeCase_Split benchmarks the Split method
func BenchmarkSnakeCase_Split(b *testing.B) {
	snake := AsSnakeCase("http_request_handler")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = snake.Split()
	}
}

// BenchmarkSnakeCase_ToCamelCase benchmarks the ToCamelCase method
func BenchmarkSnakeCase_ToCamelCase(b *testing.B) {
	snake := AsSnakeCase("http_request_handler")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = snake.ToCamelCase()
	}
}

// BenchmarkRoundTrip benchmarks conversion round trips
func BenchmarkRoundTrip(b *testing.B) {
	b.Run("camel to snake to camel", func(b *testing.B) {
		camel := AsCamelCase("HTTPRequestHandler")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			snake := camel.ToSnakeCase()
			_ = snake.ToCamelCase()
		}
	})

	b.Run("snake to camel to snake", func(b *testing.B) {
		snake := AsSnakeCase("http_request_handler")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			camel := snake.ToCamelCase()
			_ = camel.ToSnakeCase()
		}
	})
}
