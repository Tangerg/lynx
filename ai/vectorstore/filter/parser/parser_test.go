package parser

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/ai/vectorstore/filter/visitors"
)

func TestNewParser(t *testing.T) {
	parser, err := NewParser(`(status == 'active' OR status == 'pending') 
AND age >= 18 
AND age <= 65 
AND (category IN ('premium', 'standard', 'basic') OR priority > 5)
AND NOT (blocked == true AND verified == false)
AND profile['country'] == 'US' 
AND metadata['settings']['notifications'] == true
AND tags['primary'] IN ('tech', 'business', 'finance')
AND score LIKE '%excellent%'
AND NOT user_level['tier'] LIKE '%trial%'
AND ((balance > 1000.50 AND currency == 'USD') OR (balance > 800.0 AND currency == 'EUR'))
AND permissions['admin'] == false
AND profile['preferences']['language'] IN ('en', 'es', 'fr')
AND NOT (suspended == true OR deleted == true)
AND activity_score >= 75.5
AND session['is_active'] == true`)
	if err != nil {
		t.Fatal(err)
	}

	expr, err := parser.Parse()
	if err != nil {
		var parseError *ParseError
		errors.As(err, &parseError)
		token := parseError.Token
		t.Log(token.String())
		t.Fatal(err)
	}

	visitor := visitors.NewSQLLikeVisitor()
	visitor.Visit(expr)
	t.Log(visitor.SQL())
}
