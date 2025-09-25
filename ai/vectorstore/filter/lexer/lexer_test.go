package lexer

import (
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/token"
)

func TestNewLexer(t *testing.T) {
	lexer, err := NewLexer("name == 'Tom' AND age >= 1.8.1 or age < -15")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestNewLexer2(t *testing.T) {
	lexer, err := NewLexer("name == 'John' AND \n" +
		" age >= 18.5 OR \n" +
		" status IN ['active', 'pending'] and ( email == 'Join@gmail.com' )")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestComplexNestedConditions(t *testing.T) {
	lexer, err := NewLexer("((name == 'Alice' OR name == 'Bob') AND age >= 21) OR " +
		"(status == 'premium' AND (score > 85.5 OR level IN ['gold', 'platinum']))")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestMixedDataTypes(t *testing.T) {
	lexer, err := NewLexer("id == 12345 AND price == 99.99 AND active == true AND " +
		"description == 'High-quality product' AND category IS NOT null")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestLikeOperatorWithPatterns(t *testing.T) {
	lexer, err := NewLexer("name LIKE 'John%' OR email LIKE '%@gmail.com' OR " +
		"phone LIKE '555-____' AND city NOT LIKE 'New%'")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestArrayOperations(t *testing.T) {
	lexer, err := NewLexer("status IN ['active', 'pending', 'suspended'] AND " +
		"tags IN ['urgent', 'important'] AND category NOT IN ['spam', 'deleted']")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestNullChecksAndComparisons(t *testing.T) {
	lexer, err := NewLexer("name IS NOT NULL AND age IS NULL OR " +
		"description IS NOT NULL AND (status == NULL OR status != 'inactive')")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestBooleanLogic(t *testing.T) {
	lexer, err := NewLexer("(active == true AND verified == false) OR " +
		"(premium == TRUE AND trial == FALSE) AND NOT (suspended == true)")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestComplexMathematicalConditions(t *testing.T) {
	lexer, err := NewLexer("((price >= 100.0 AND price <= 500.99) OR discount > 0.25) AND " +
		"(quantity != 0 AND stock_level > 10) AND rating >= 4.5")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestMultilineComplexQuery(t *testing.T) {
	query := `
		(
			user.name == 'Administrator' AND 
			user.role IN ['admin', 'superuser'] AND
			user.last_login >= '2024-01-01'
		) OR (
			user.department == 'Engineering' AND
			user.level >= 5 AND
			user.active == true AND
			user.projects IN ['project_alpha', 'project_beta'] AND
			user.salary > 75000.50
		) AND NOT (
			user.status == 'terminated' OR
			user.access_level IS NULL
		)
	`
	lexer, err := NewLexer(query)
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestSpecialCharactersInStrings(t *testing.T) {
	lexer, err := NewLexer("name == 'O''Brien' AND description == 'Contains \"quotes\" and spaces' AND " +
		"path == '/home/user/documents' AND email == 'user+tag@domain.co.uk'")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestEdgeCaseNumbers(t *testing.T) {
	lexer, err := NewLexer("value == 0 AND negative == -123 AND decimal == 0.001 AND " +
		"large == 999999999 AND scientific == 1.23e-10 AND percentage == 99.99")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestMixedCaseKeywords(t *testing.T) {
	lexer, err := NewLexer("Name == 'John' and AGE >= 18 Or STATUS in ['Active', 'Pending'] " +
		"AND verified == True AND deleted == FALSE and created_at IS not NULL")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestDeepNestedConditions(t *testing.T) {
	lexer, err := NewLexer("((((a == 1 AND b == 2) OR (c == 3 AND d == 4)) AND " +
		"((e == 5 OR f == 6) AND (g == 7 OR h == 8))) OR " +
		"(((i == 9 AND j == 10) OR (k == 11 AND l == 12)) AND m == 13))")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestArrayWithMixedTypes(t *testing.T) {
	lexer, err := NewLexer("mixed_array IN [1, 'string', true, null, 99.99] AND " +
		"status_codes IN [200, 201, 400, 404, 500] AND " +
		"permissions IN ['read', 'write', 'execute', 'admin']")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestComplexLikePatterns(t *testing.T) {
	lexer, err := NewLexer("filename LIKE '%.pdf' AND path LIKE '/uploads/%/%' AND " +
		"email LIKE '%@company.com' AND phone NOT LIKE '+1%' AND " +
		"code LIKE 'ABC___' AND description LIKE '%important%'")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestErrorHandling(t *testing.T) {
	testCases := []struct {
		name  string
		query string
	}{
		{"Unmatched quote", "name == 'John"},
		{"Invalid character", "name == 'John' && age == 20"},
		{"Incomplete operator", "age > == 18"},
		{"Empty brackets", "status IN []"},
		{"Nested unmatched brackets", "((name == 'John')"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lexer, err := NewLexer(tc.query)
			if err != nil {
				t.Log("Expected error:", err)
				return
			}
			tokens := lexer.AllTokens()

			// Check for ERROR or ILLEGAL tokens
			for _, tk := range tokens {
				if tk.Kind.Is(token.EOF) {
					t.Log("Found error Token:", tk.String())
				}
			}
		})
	}
}

func TestWhitespaceHandling(t *testing.T) {
	lexer, err := NewLexer("   name    ==     'John'   AND   \n\t  age   >=   18   \r\n   ")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}

func TestSemicolonTermination(t *testing.T) {
	lexer, err := NewLexer("name == 'John' AND age >= 18; status == 'active';")
	if err != nil {
		t.Fatal(err)
	}
	tokens := lexer.AllTokens()

	for _, tk := range tokens {
		t.Log(tk.String())
	}
}
