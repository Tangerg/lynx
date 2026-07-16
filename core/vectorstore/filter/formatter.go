package filter

import (
	"errors"
	"fmt"
	"strings"
)

// Formatter renders a predicate as the canonical textual filter DSL. Its zero
// value is ready to use and may be reused; each successful Visit replaces the
// previous result. Formatter is useful for logs, persistence, and adapters
// that consume the textual form.
type Formatter struct {
	formatted string
}

var _ Visitor = (*Formatter)(nil)

// Visit renders predicate. Callers that invoke Visit directly must first
// ensure predicate is accepted by [Validate]; [Visit] does that automatically
// when dispatching this Formatter alongside other visitors.
func (f *Formatter) Visit(predicate Predicate) error {
	if f == nil {
		return errors.New("filter.Formatter.Visit: formatter is nil")
	}
	f.formatted = ""

	var output strings.Builder
	if err := f.format(&output, predicate, formatRoot, false); err != nil {
		return err
	}
	f.formatted = output.String()
	return nil
}

// String returns the result of the latest successful Visit. It returns an
// empty string before the first visit, after a failed visit, or on a nil
// receiver.
func (f *Formatter) String() string {
	if f == nil {
		return ""
	}
	return f.formatted
}

type formatPrecedence uint8

const (
	formatRoot formatPrecedence = iota
	formatOr
	formatAnd
	formatTest
)

func (f *Formatter) format(output *strings.Builder, expr Expr, parent formatPrecedence, right bool) error {
	if isNilExpr(expr) {
		return errors.New("filter.Formatter: expression is nil")
	}

	switch node := expr.(type) {
	case *Ident:
		output.WriteString(node.Value)
	case *Literal:
		f.formatLiteral(output, node)
	case *ListLiteral:
		output.WriteByte('(')
		for i, value := range node.Values {
			if i > 0 {
				output.WriteString(", ")
			}
			if err := f.format(output, value, formatRoot, false); err != nil {
				return err
			}
		}
		output.WriteByte(')')
	case *IndexExpr:
		if err := f.format(output, node.Left, formatTest, false); err != nil {
			return err
		}
		output.WriteByte('[')
		if err := f.format(output, node.Index, formatRoot, false); err != nil {
			return err
		}
		output.WriteByte(']')
	case *UnaryExpr:
		output.WriteString(node.Op.String())
		output.WriteString(" (")
		if err := f.format(output, node.Right, formatRoot, false); err != nil {
			return err
		}
		output.WriteByte(')')
	case *BinaryExpr:
		return f.formatBinary(output, node, parent, right)
	default:
		return fmt.Errorf("filter.Formatter: unsupported expression %T", expr)
	}
	return nil
}

func (f *Formatter) formatBinary(output *strings.Builder, binary *BinaryExpr, parent formatPrecedence, right bool) error {
	precedence := binaryPrecedence(binary)
	wrapped := precedence < parent || right && precedence == parent && precedence != formatTest
	if wrapped {
		output.WriteByte('(')
	}
	if err := f.format(output, binary.Left, precedence, false); err != nil {
		return err
	}
	output.WriteByte(' ')
	output.WriteString(binary.Op.String())
	output.WriteByte(' ')
	if err := f.format(output, binary.Right, precedence, true); err != nil {
		return err
	}
	if wrapped {
		output.WriteByte(')')
	}
	return nil
}

func binaryPrecedence(binary *BinaryExpr) formatPrecedence {
	switch binary.Op {
	case OpOr:
		return formatOr
	case OpAnd:
		return formatAnd
	default:
		return formatTest
	}
}

var filterStringEscaper = strings.NewReplacer(
	`\`, `\\`,
	`'`, `\'`,
	"\n", `\n`,
	"\t", `\t`,
	"\r", `\r`,
)

func (f *Formatter) formatLiteral(output *strings.Builder, literal *Literal) {
	if literal.IsString() {
		output.WriteByte('\'')
		output.WriteString(filterStringEscaper.Replace(literal.Value))
		output.WriteByte('\'')
		return
	}
	output.WriteString(literal.Value)
}
