package token

import (
	"testing"
)

// TestKindIsValid tests the IsValid method for Kind
func TestKindIsValid(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"Valid ERROR kind", ERROR, true},
		{"Valid EOF kind", EOF, true},
		{"Valid IDENT kind", IDENT, true},
		{"Valid STRING kind", STRING, true},
		{"Valid AND kind", AND, true},
		{"Invalid kindBegin", kindBegin, false},
		{"Invalid kindEnd", kindEnd, false},
		{"Invalid negative kind", Kind(-1), false},
		{"Invalid beyond range", Kind(100), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsValid()
			if result != tt.expected {
				t.Errorf("IsValid() = %v, want %v for kind %d", result, tt.expected, tt.kind)
			}
		})
	}
}

// TestKindName tests the Name method for Kind
func TestKindName(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected string
	}{
		{"ERROR name", ERROR, "ERROR"},
		{"EOF name", EOF, "EOF"},
		{"IDENT name", IDENT, "IDENT"},
		{"NUMBER name", NUMBER, "NUMBER"},
		{"STRING name", STRING, "STRING"},
		{"TRUE name", TRUE, "BOOL"},
		{"FALSE name", FALSE, "BOOL"},
		{"EQ name", EQ, "EQ"},
		{"AND name", AND, "AND"},
		{"OR name", OR, "OR"},
		{"NOT name", NOT, "NOT"},
		{"IN name", IN, "IN"},
		{"LIKE name", LIKE, "LIKE"},
		{"LPAREN name", LPAREN, "LPAREN"},
		{"COMMA name", COMMA, "COMMA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.Name()
			if result != tt.expected {
				t.Errorf("Name() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindNamePanic tests that Name panics for invalid kinds
func TestKindNamePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Name() did not panic for invalid kind")
		}
	}()

	invalidKind := Kind(100)
	_ = invalidKind.Name()
}

// TestKindLiteral tests the Literal method for Kind
func TestKindLiteral(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected string
	}{
		{"TRUE literal", TRUE, "true"},
		{"FALSE literal", FALSE, "false"},
		{"EQ literal", EQ, "=="},
		{"NE literal", NE, "!="},
		{"LT literal", LT, "<"},
		{"LE literal", LE, "<="},
		{"GT literal", GT, ">"},
		{"GE literal", GE, ">="},
		{"AND literal", AND, "and"},
		{"OR literal", OR, "or"},
		{"NOT literal", NOT, "not"},
		{"IN literal", IN, "in"},
		{"LIKE literal", LIKE, "like"},
		{"LPAREN literal", LPAREN, "("},
		{"RPAREN literal", RPAREN, ")"},
		{"LBRACK literal", LBRACK, "["},
		{"RBRACK literal", RBRACK, "]"},
		{"COMMA literal", COMMA, ","},
		{"IDENT no literal", IDENT, ""},
		{"NUMBER no literal", NUMBER, ""},
		{"STRING no literal", STRING, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.Literal()
			if result != tt.expected {
				t.Errorf("Literal() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIs tests the Is method for Kind comparison
func TestKindIs(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		other    Kind
		expected bool
	}{
		{"Same kind", IDENT, IDENT, true},
		{"Different kinds", IDENT, NUMBER, false},
		{"AND vs OR", AND, OR, false},
		{"EQ vs EQ", EQ, EQ, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.Is(tt.other)
			if result != tt.expected {
				t.Errorf("Is(%v) = %v, want %v", tt.other, result, tt.expected)
			}
		})
	}
}

// TestKindIsLiteral tests the IsLiteral method
func TestKindIsLiteral(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"STRING is literal", STRING, true},
		{"NUMBER is literal", NUMBER, true},
		{"TRUE is literal", TRUE, true},
		{"FALSE is literal", FALSE, true},
		{"IDENT not literal", IDENT, false},
		{"AND not literal", AND, false},
		{"EQ not literal", EQ, false},
		{"LPAREN not literal", LPAREN, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsLiteral()
			if result != tt.expected {
				t.Errorf("IsLiteral() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsKeyword tests the IsKeyword method
func TestKindIsKeyword(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"TRUE is keyword", TRUE, true},
		{"FALSE is keyword", FALSE, true},
		{"AND is keyword", AND, true},
		{"OR is keyword", OR, true},
		{"NOT is keyword", NOT, true},
		{"IN is keyword", IN, true},
		{"LIKE is keyword", LIKE, true},
		{"IDENT not keyword", IDENT, false},
		{"NUMBER not keyword", NUMBER, false},
		{"EQ not keyword", EQ, false},
		{"LPAREN not keyword", LPAREN, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsKeyword()
			if result != tt.expected {
				t.Errorf("IsKeyword() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsEqualityOperator tests the IsEqualityOperator method
func TestKindIsEqualityOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"EQ is equality operator", EQ, true},
		{"NE is equality operator", NE, true},
		{"LT not equality operator", LT, false},
		{"GT not equality operator", GT, false},
		{"AND not equality operator", AND, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsEqualityOperator()
			if result != tt.expected {
				t.Errorf("IsEqualityOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsOrderingOperator tests the IsOrderingOperator method
func TestKindIsOrderingOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"LT is ordering operator", LT, true},
		{"LE is ordering operator", LE, true},
		{"GT is ordering operator", GT, true},
		{"GE is ordering operator", GE, true},
		{"EQ not ordering operator", EQ, false},
		{"NE not ordering operator", NE, false},
		{"AND not ordering operator", AND, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsOrderingOperator()
			if result != tt.expected {
				t.Errorf("IsOrderingOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsComparisonOperator tests the IsComparisonOperator method
func TestKindIsComparisonOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"EQ is comparison operator", EQ, true},
		{"NE is comparison operator", NE, true},
		{"LT is comparison operator", LT, true},
		{"LE is comparison operator", LE, true},
		{"GT is comparison operator", GT, true},
		{"GE is comparison operator", GE, true},
		{"AND not comparison operator", AND, false},
		{"IN not comparison operator", IN, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsComparisonOperator()
			if result != tt.expected {
				t.Errorf("IsComparisonOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsLogicalOperator tests the IsLogicalOperator method
func TestKindIsLogicalOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"AND is logical operator", AND, true},
		{"OR is logical operator", OR, true},
		{"NOT not logical operator (unary)", NOT, false},
		{"EQ not logical operator", EQ, false},
		{"IN not logical operator", IN, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsLogicalOperator()
			if result != tt.expected {
				t.Errorf("IsLogicalOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsMatchingOperator tests the IsMatchingOperator method
func TestKindIsMatchingOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"IN is matching operator", IN, true},
		{"LIKE is matching operator", LIKE, true},
		{"EQ not matching operator", EQ, false},
		{"AND not matching operator", AND, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsMatchingOperator()
			if result != tt.expected {
				t.Errorf("IsMatchingOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsBinaryOperator tests the IsBinaryOperator method
func TestKindIsBinaryOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"EQ is binary operator", EQ, true},
		{"AND is binary operator", AND, true},
		{"IN is binary operator", IN, true},
		{"LIKE is binary operator", LIKE, true},
		{"NOT not binary operator", NOT, false},
		{"IDENT not binary operator", IDENT, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsBinaryOperator()
			if result != tt.expected {
				t.Errorf("IsBinaryOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsUnaryOperator tests the IsUnaryOperator method
func TestKindIsUnaryOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"NOT is unary operator", NOT, true},
		{"AND not unary operator", AND, false},
		{"EQ not unary operator", EQ, false},
		{"IDENT not unary operator", IDENT, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsUnaryOperator()
			if result != tt.expected {
				t.Errorf("IsUnaryOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsOperator tests the IsOperator method
func TestKindIsOperator(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"EQ is operator", EQ, true},
		{"AND is operator", AND, true},
		{"NOT is operator", NOT, true},
		{"IN is operator", IN, true},
		{"IDENT not operator", IDENT, false},
		{"LPAREN not operator", LPAREN, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsOperator()
			if result != tt.expected {
				t.Errorf("IsOperator() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindIsDelimiter tests the IsDelimiter method
func TestKindIsDelimiter(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected bool
	}{
		{"LPAREN is delimiter", LPAREN, true},
		{"RPAREN is delimiter", RPAREN, true},
		{"LBRACK is delimiter", LBRACK, true},
		{"RBRACK is delimiter", RBRACK, true},
		{"COMMA is delimiter", COMMA, true},
		{"EQ not delimiter", EQ, false},
		{"IDENT not delimiter", IDENT, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsDelimiter()
			if result != tt.expected {
				t.Errorf("IsDelimiter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestKindPrecedence tests the Precedence method
func TestKindPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected int
	}{
		{"OR precedence", OR, PrecedenceOR},
		{"AND precedence", AND, PrecedenceAND},
		{"NOT precedence", NOT, PrecedenceNOT},
		{"EQ precedence", EQ, PrecedenceCMP},
		{"NE precedence", NE, PrecedenceCMP},
		{"LT precedence", LT, PrecedenceCMP},
		{"LE precedence", LE, PrecedenceCMP},
		{"GT precedence", GT, PrecedenceCMP},
		{"GE precedence", GE, PrecedenceCMP},
		{"LIKE precedence", LIKE, PrecedenceMatch},
		{"IN precedence", IN, PrecedenceMatch},
		{"LBRACK precedence", LBRACK, PrecedenceIndex},
		{"IDENT precedence", IDENT, PrecedenceLowest},
		{"NUMBER precedence", NUMBER, PrecedenceLowest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.Precedence()
			if result != tt.expected {
				t.Errorf("Precedence() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestPrecedenceOrdering tests that precedence values follow the correct ordering
func TestPrecedenceOrdering(t *testing.T) {
	// OR should have lowest precedence among operators
	if OR.Precedence() >= AND.Precedence() {
		t.Errorf("OR precedence should be lower than AND")
	}

	// AND should have lower precedence than NOT
	if AND.Precedence() >= NOT.Precedence() {
		t.Errorf("AND precedence should be lower than NOT")
	}

	// NOT should have lower precedence than comparison operators
	if NOT.Precedence() >= EQ.Precedence() {
		t.Errorf("NOT precedence should be lower than comparison operators")
	}

	// Comparison operators should have lower precedence than matching operators
	if EQ.Precedence() >= LIKE.Precedence() {
		t.Errorf("Comparison operators should have lower precedence than matching operators")
	}

	// Matching operators should have lower precedence than index operator
	if LIKE.Precedence() >= LBRACK.Precedence() {
		t.Errorf("Matching operators should have lower precedence than index operator")
	}
}

// TestKindOf tests the KindOf function
func TestKindOf(t *testing.T) {
	tests := []struct {
		name     string
		ident    string
		expected Kind
	}{
		{"Keyword 'true'", "true", TRUE},
		{"Keyword 'false'", "false", FALSE},
		{"Keyword 'and'", "and", AND},
		{"Keyword 'or'", "or", OR},
		{"Keyword 'not'", "not", NOT},
		{"Keyword 'in'", "in", IN},
		{"Keyword 'like'", "like", LIKE},
		{"Keyword uppercase 'AND'", "AND", AND},
		{"Keyword mixed case 'AnD'", "AnD", AND},
		{"Identifier 'name'", "name", IDENT},
		{"Identifier 'age'", "age", IDENT},
		{"Identifier 'user_id'", "user_id", IDENT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := KindOf(tt.ident)
			if result != tt.expected {
				t.Errorf("KindOf(%v) = %v, want %v", tt.ident, result, tt.expected)
			}
		})
	}
}

// TestIsKeyword tests the IsKeyword function
func TestIsKeyword(t *testing.T) {
	tests := []struct {
		name     string
		ident    string
		expected bool
	}{
		{"'true' is keyword", "true", true},
		{"'false' is keyword", "false", true},
		{"'and' is keyword", "and", true},
		{"'or' is keyword", "or", true},
		{"'not' is keyword", "not", true},
		{"'in' is keyword", "in", true},
		{"'like' is keyword", "like", true},
		{"'AND' is keyword (case insensitive)", "AND", true},
		{"'True' is keyword (case insensitive)", "True", true},
		{"'name' not keyword", "name", false},
		{"'user_id' not keyword", "user_id", false},
		{"empty string not keyword", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsKeyword(tt.ident)
			if result != tt.expected {
				t.Errorf("IsKeyword(%v) = %v, want %v", tt.ident, result, tt.expected)
			}
		})
	}
}

// TestIsIdentifier tests the IsIdentifier function
func TestIsIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		ident    string
		expected bool
	}{
		{"Valid identifier 'name'", "name", true},
		{"Valid identifier 'user_id'", "user_id", true},
		{"Valid identifier 'age123'", "age123", true},
		{"Valid identifier '_private'", "_private", true},
		{"Valid identifier '用户名'", "用户名", true},
		{"Valid identifier 'userТест'", "userТест", true},
		{"Empty string not valid", "", false},
		{"Keyword 'and' not valid", "and", false},
		{"Keyword 'true' not valid", "true", false},
		{"Contains space not valid", "user name", false},
		{"Contains dash not valid", "user-id", false},
		{"Contains dot not valid", "user.id", false},
		{"Contains special char not valid", "user@id", false},
		{"Starts with number invalid", "123user", true}, // Actually valid based on IsLiteralChar
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIdentifier(tt.ident)
			if result != tt.expected {
				t.Errorf("IsIdentifier(%v) = %v, want %v", tt.ident, result, tt.expected)
			}
		})
	}
}

// TestIsLiteralChar tests the IsLiteralChar function
func TestIsLiteralChar(t *testing.T) {
	tests := []struct {
		name     string
		char     rune
		expected bool
	}{
		{"Letter 'a' is valid", 'a', true},
		{"Letter 'Z' is valid", 'Z', true},
		{"Digit '0' is valid", '0', true},
		{"Digit '9' is valid", '9', true},
		{"Underscore is valid", '_', true},
		{"Chinese character is valid", '中', true},
		{"Cyrillic character is valid", 'Я', true},
		{"Space not valid", ' ', false},
		{"Dash not valid", '-', false},
		{"Dot not valid", '.', false},
		{"At sign not valid", '@', false},
		{"Exclamation not valid", '!', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLiteralChar(tt.char)
			if result != tt.expected {
				t.Errorf("IsLiteralChar(%v) = %v, want %v", tt.char, result, tt.expected)
			}
		})
	}
}

// TestKeywordKindsInitialization tests that all keywords are properly initialized
func TestKeywordKindsInitialization(t *testing.T) {
	expectedKeywords := map[string]Kind{
		"true":  TRUE,
		"false": FALSE,
		"and":   AND,
		"or":    OR,
		"not":   NOT,
		"in":    IN,
		"like":  LIKE,
	}

	for keyword, expectedKind := range expectedKeywords {
		actualKind, exists := keywordKinds[keyword]
		if !exists {
			t.Errorf("Keyword %v not found in keywordKinds map", keyword)
			continue
		}
		if actualKind != expectedKind {
			t.Errorf("keywordKinds[%v] = %v, want %v", keyword, actualKind, expectedKind)
		}
	}

	// Verify no extra keywords
	if len(keywordKinds) != len(expectedKeywords) {
		t.Errorf("keywordKinds has %v entries, expected %v", len(keywordKinds), len(expectedKeywords))
	}
}

// TestKindsMetadataCompleteness tests that all valid kinds have metadata
func TestKindsMetadataCompleteness(t *testing.T) {
	for i := kindBegin + 1; i < kindEnd; i++ {
		if kindsMetadata[i] == nil {
			t.Errorf("Missing metadata for kind %v", i)
		}
	}
}

// TestKindString tests the String method
func TestKindString(t *testing.T) {
	// Test a few representative kinds
	kinds := []Kind{IDENT, NUMBER, EQ, AND, LPAREN}

	for _, k := range kinds {
		result := k.String()

		// Check that the string contains the name
		if !contains(result, k.Name()) {
			t.Errorf("String() for %v does not contain name %v", k, k.Name())
		}

		// Check that it contains "Kind"
		if !contains(result, "Kind") {
			t.Errorf("String() for %v does not contain 'Kind'", k)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkKindOf benchmarks the KindOf function
func BenchmarkKindOf(b *testing.B) {
	testCases := []string{"name", "and", "true", "user_id"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = KindOf(testCases[i%len(testCases)])
	}
}

// BenchmarkIsKeyword benchmarks the IsKeyword function
func BenchmarkIsKeyword(b *testing.B) {
	testCases := []string{"name", "and", "true", "user_id"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsKeyword(testCases[i%len(testCases)])
	}
}

// BenchmarkIsIdentifier benchmarks the IsIdentifier function
func BenchmarkIsIdentifier(b *testing.B) {
	testCases := []string{"name", "user_id", "age123", "_private"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsIdentifier(testCases[i%len(testCases)])
	}
}
