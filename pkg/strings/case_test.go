package strings

import (
	"testing"
)

var camelCaseTestCases = []string{
	"UserAccountManager",
	"DataProcessingUnit",
	"NetworkConfigurationTool",
	"FileUploadService",
	"UserProfileSettings",
	"OrderTrackingSystem",
	"PaymentGatewayIntegration",
	"ProductInventoryManager",
	"ShoppingCartService",
	"AuthenticationMiddleware",
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
}

func TestCamelCaseSplit(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		for _, s := range CamelCaseSplit(testCase) {
			t.Log(s)
		}
	}
}

func TestCamelCaseSplitToLower(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		for _, s := range CamelCaseSplitToLower(testCase) {
			t.Log(s)
		}
	}
}

func TestCamelCaseSplitToUpper(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		for _, s := range CamelCaseSplitToUpper(testCase) {
			t.Log(s)
		}
	}
}

func TestCamelCaseToSnakeCase(t *testing.T) {
	for _, testCase := range camelCaseTestCases {
		t.Log(CamelCaseToSnakeCase(testCase))
	}
}

func BenchmarkCamelCaseSplit(b *testing.B) {
	var test = camelCaseTestCases[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CamelCaseSplit(test)
	}
	b.StopTimer()
}

func TestSnakeCaseSplit(t *testing.T) {
	for _, testCase := range snakeTestCases {
		for _, s := range SnakeCaseSplit(testCase) {
			t.Log(s)
		}
	}
}

func TestSnakeCaseSplitToLower(t *testing.T) {
	for _, testCase := range snakeTestCases {
		for _, s := range SnakeCaseSplitToLower(testCase) {
			t.Log(s)
		}
	}
}

func TestSnakeCaseSplitToUpper(t *testing.T) {
	for _, testCase := range snakeTestCases {
		for _, s := range SnakeCaseSplitToUpper(testCase) {
			t.Log(s)
		}
	}
}

func TestSnakeCaseToCamelCase(t *testing.T) {
	for _, testCase := range snakeTestCases {
		t.Log(SnakeCaseToCamelCase(testCase))
	}
}
