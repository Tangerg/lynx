package strings

import (
	"testing"
)

var camelCaseTestCases = []string{
	"HTTPRequestHandler",           // 大写缩写混合
	"User123ProfileData",           // 数字混合
	"XMLParserUtility",             // 大写缩写前缀
	"GetHTTPResponseCode",          // 大写缩写与常规单词混合
	"checkUser2FAStatus",           // 小驼峰命名，包含数字
	"APIServerHealthCheck",         // 大写缩写
	"processDataJSONFile",          // 小驼峰命名，包含大写缩写
	"OAuthTokenGenerator",          // 大写缩写与常规单词混合
	"Error404Handler",              // 数字与大写字母混合
	"UserProfilePageV2",            // 版本号与单词混合
	"emailServiceSMTPConfig",       // 小驼峰命名，带大写缩写
	"DatabaseConnectionPool",       // 常规驼峰命名
	"CheckIOStatusPort45",          // 大写缩写、数字和单词混合
	"ImageProcessingTool2024",      // 年份后缀
	"verifyUserInput_UTF8Encoding", // 包含非字母符号（下划线）
}
var snakeTestCases = []string{
	"user_account_manager",
	"data_processing_unit",
	"network_configuration_tool",
	"file_upload_service",
	"user_profile_settings",
	"order_tracking_system",
	"payment_gateway_integration",
	"product_inventory_manager",
	"shopping_cart_service",
	"authentication_middleware",
	"user_123_name",
	"id_card",
}

func TestCamelCaseSplit(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		for _, s := range AsCamelCase(testCase).Split() {
			t.Log(s)
		}
	}
}

func TestCamelCaseSplitToLower(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		for _, s := range AsCamelCase(testCase).SplitToLower() {
			t.Log(s)
		}
	}
}

func TestCamelCaseSplitToUpper(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		for _, s := range AsCamelCase(testCase).SplitToUpper() {
			t.Log(s)
		}
	}
}

func TestCamelCaseToSnakeCase(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		t.Log(AsCamelCase(testCase).ToSnakeCase())
	}
}

func BenchmarkCamelCaseSplit(b *testing.B) {
	var test = camelCaseTestCases[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AsCamelCase(test).Split()
	}
	b.StopTimer()
}

func TestSnakeCaseSplit(t *testing.T) {
	for _, testCase := range snakeTestCases {
		for _, s := range AsSnakeCase(testCase).Split() {
			t.Log(s)
		}
	}
}

func TestSnakeCaseSplitToLower(t *testing.T) {
	for _, testCase := range snakeTestCases {
		for _, s := range AsSnakeCase(testCase).SplitToLower() {
			t.Log(s)
		}
	}
}

func TestSnakeCaseSplitToUpper(t *testing.T) {
	for _, testCase := range snakeTestCases {
		for _, s := range AsSnakeCase(testCase).SplitToUpper() {
			t.Log(s)
		}
	}
}

func TestSnakeCaseToCamelCase(t *testing.T) {
	for _, testCase := range snakeTestCases {
		t.Log(AsSnakeCase(testCase).ToCamelCase())
	}
}
