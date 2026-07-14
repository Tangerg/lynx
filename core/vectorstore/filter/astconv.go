package filter

import (
	"fmt"

	internalast "github.com/Tangerg/lynx/core/vectorstore/filter/internal/ast"
	internaltoken "github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"
)

func fromInternal(expr internalast.Expr) (Expr, error) {
	if expr == nil {
		return nil, nil
	}
	switch node := expr.(type) {
	case *internalast.Ident:
		return &Ident{Value: node.Value, start: fromInternalPosition(node.Start()), end: fromInternalPosition(node.End())}, nil
	case *internalast.Literal:
		kind, err := literalKindFromToken(node.Token.Kind)
		if err != nil {
			return nil, err
		}
		return &Literal{Kind: kind, Value: node.Value, start: fromInternalPosition(node.Start()), end: fromInternalPosition(node.End())}, nil
	case *internalast.ListLiteral:
		values := make([]*Literal, 0, len(node.Values))
		for _, value := range node.Values {
			converted, err := fromInternal(value)
			if err != nil {
				return nil, err
			}
			values = append(values, converted.(*Literal))
		}
		return &ListLiteral{Values: values, start: fromInternalPosition(node.Start()), end: fromInternalPosition(node.End())}, nil
	case *internalast.UnaryExpr:
		right, err := fromInternal(node.Right)
		if err != nil {
			return nil, err
		}
		op, err := operatorFromToken(node.Op.Kind)
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: op, Right: right.(ComputedExpr), start: fromInternalPosition(node.Start()), end: fromInternalPosition(node.End())}, nil
	case *internalast.BinaryExpr:
		left, err := fromInternal(node.Left)
		if err != nil {
			return nil, err
		}
		right, err := fromInternal(node.Right)
		if err != nil {
			return nil, err
		}
		op, err := operatorFromToken(node.Op.Kind)
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Left: left, Op: op, Right: right, start: fromInternalPosition(node.Start()), end: fromInternalPosition(node.End())}, nil
	case *internalast.IndexExpr:
		left, err := fromInternal(node.Left)
		if err != nil {
			return nil, err
		}
		index, err := fromInternal(node.Index)
		if err != nil {
			return nil, err
		}
		return &IndexExpr{Left: left, Index: index.(*Literal), start: fromInternalPosition(node.Start()), end: fromInternalPosition(node.End())}, nil
	default:
		return nil, fmt.Errorf("filter: unsupported parser expression %T", expr)
	}
}

func toInternal(expr Expr) (internalast.Expr, error) {
	if expr == nil {
		return nil, nil
	}
	switch node := expr.(type) {
	case *Ident:
		if node == nil {
			return nil, fmt.Errorf("filter: identifier is nil")
		}
		return &internalast.Ident{
			Token: internaltoken.OfIdent(node.Value, toInternalPosition(node.Start()), toInternalPosition(node.End())),
			Value: node.Value,
		}, nil
	case *Literal:
		if node == nil {
			return nil, fmt.Errorf("filter: literal is nil")
		}
		tok, err := literalToken(node)
		if err != nil {
			return nil, err
		}
		return &internalast.Literal{Token: tok, Value: node.Value}, nil
	case *ListLiteral:
		if node == nil {
			return nil, fmt.Errorf("filter: list literal is nil")
		}
		values := make([]*internalast.Literal, 0, len(node.Values))
		for index, value := range node.Values {
			converted, err := toInternal(value)
			if err != nil {
				return nil, fmt.Errorf("filter: list element %d: %w", index, err)
			}
			literal, ok := converted.(*internalast.Literal)
			if !ok {
				return nil, fmt.Errorf("filter: list element %d is %T, want literal", index, converted)
			}
			values = append(values, literal)
		}
		return &internalast.ListLiteral{
			Lparen: internaltoken.OfKind(internaltoken.LPAREN, toInternalPosition(node.Start()), toInternalPosition(node.Start())),
			Rparen: internaltoken.OfKind(internaltoken.RPAREN, toInternalPosition(node.End()), toInternalPosition(node.End())),
			Values: values,
		}, nil
	case *UnaryExpr:
		if node == nil {
			return nil, fmt.Errorf("filter: unary expression is nil")
		}
		right, err := toInternal(node.Right)
		if err != nil {
			return nil, err
		}
		kind, err := tokenFromOperator(node.Op)
		if err != nil {
			return nil, err
		}
		computed, ok := right.(internalast.ComputedExpr)
		if !ok {
			return nil, fmt.Errorf("filter: unary operand is %T, want computed expression", right)
		}
		return &internalast.UnaryExpr{
			Op:    internaltoken.OfKind(kind, toInternalPosition(node.Start()), toInternalPosition(node.Start())),
			Right: computed,
		}, nil
	case *BinaryExpr:
		if node == nil {
			return nil, fmt.Errorf("filter: binary expression is nil")
		}
		left, err := toInternal(node.Left)
		if err != nil {
			return nil, err
		}
		if left == nil {
			return nil, fmt.Errorf("filter: binary left operand is nil")
		}
		right, err := toInternal(node.Right)
		if err != nil {
			return nil, err
		}
		if right == nil {
			return nil, fmt.Errorf("filter: binary right operand is nil")
		}
		kind, err := tokenFromOperator(node.Op)
		if err != nil {
			return nil, err
		}
		return &internalast.BinaryExpr{
			Left:  left,
			Op:    internaltoken.OfKind(kind, toInternalPosition(node.Start()), toInternalPosition(node.Start())),
			Right: right,
		}, nil
	case *IndexExpr:
		if node == nil {
			return nil, fmt.Errorf("filter: index expression is nil")
		}
		left, err := toInternal(node.Left)
		if err != nil {
			return nil, err
		}
		if left == nil {
			return nil, fmt.Errorf("filter: indexed operand is nil")
		}
		index, err := toInternal(node.Index)
		if err != nil {
			return nil, err
		}
		literal, ok := index.(*internalast.Literal)
		if !ok {
			return nil, fmt.Errorf("filter: index is %T, want literal", index)
		}
		return &internalast.IndexExpr{
			LBrack: internaltoken.OfKind(internaltoken.LBRACK, toInternalPosition(node.Start()), toInternalPosition(node.Start())),
			RBrack: internaltoken.OfKind(internaltoken.RBRACK, toInternalPosition(node.End()), toInternalPosition(node.End())),
			Left:   left,
			Index:  literal,
		}, nil
	default:
		return nil, fmt.Errorf("filter: unsupported public expression %T", expr)
	}
}

func fromInternalPosition(p internaltoken.Position) Position {
	return Position{Line: p.Line, Column: p.Column}
}
func toInternalPosition(p Position) internaltoken.Position {
	return internaltoken.Position{Line: p.Line, Column: p.Column}
}

func operatorFromToken(kind internaltoken.Kind) (Operator, error) {
	switch kind {
	case internaltoken.EQ:
		return OpEqual, nil
	case internaltoken.NE:
		return OpNotEqual, nil
	case internaltoken.LT:
		return OpLess, nil
	case internaltoken.LE:
		return OpLessEqual, nil
	case internaltoken.GT:
		return OpGreater, nil
	case internaltoken.GE:
		return OpGreaterEqual, nil
	case internaltoken.AND:
		return OpAnd, nil
	case internaltoken.OR:
		return OpOr, nil
	case internaltoken.NOT:
		return OpNot, nil
	case internaltoken.IN:
		return OpIn, nil
	case internaltoken.LIKE:
		return OpLike, nil
	case internaltoken.IS:
		return OpIs, nil
	default:
		return "", fmt.Errorf("filter: token %s is not a public operator", kind)
	}
}

func tokenFromOperator(op Operator) (internaltoken.Kind, error) {
	switch op {
	case OpEqual:
		return internaltoken.EQ, nil
	case OpNotEqual:
		return internaltoken.NE, nil
	case OpLess:
		return internaltoken.LT, nil
	case OpLessEqual:
		return internaltoken.LE, nil
	case OpGreater:
		return internaltoken.GT, nil
	case OpGreaterEqual:
		return internaltoken.GE, nil
	case OpAnd:
		return internaltoken.AND, nil
	case OpOr:
		return internaltoken.OR, nil
	case OpNot:
		return internaltoken.NOT, nil
	case OpIn:
		return internaltoken.IN, nil
	case OpLike:
		return internaltoken.LIKE, nil
	case OpIs:
		return internaltoken.IS, nil
	default:
		return 0, fmt.Errorf("filter: invalid operator %q", op)
	}
}

func literalKindFromToken(kind internaltoken.Kind) (LiteralKind, error) {
	switch kind {
	case internaltoken.STRING:
		return LiteralString, nil
	case internaltoken.NUMBER:
		return LiteralNumber, nil
	case internaltoken.TRUE, internaltoken.FALSE:
		return LiteralBool, nil
	case internaltoken.NULL:
		return LiteralNull, nil
	default:
		return "", fmt.Errorf("filter: token %s is not a literal", kind)
	}
}

func literalToken(lit *Literal) (internaltoken.Token, error) {
	start, end := toInternalPosition(lit.Start()), toInternalPosition(lit.End())
	switch lit.Kind {
	case LiteralString:
		return internaltoken.OfLiteral(internaltoken.STRING, lit.Value, start, end), nil
	case LiteralNumber:
		return internaltoken.OfNumericLiteral(lit.Value, start, end), nil
	case LiteralBool:
		if lit.Value == "true" {
			return internaltoken.OfKind(internaltoken.TRUE, start, end), nil
		}
		if lit.Value == "false" {
			return internaltoken.OfKind(internaltoken.FALSE, start, end), nil
		}
		return internaltoken.Token{}, fmt.Errorf("filter: invalid boolean literal %q", lit.Value)
	case LiteralNull:
		return internaltoken.Of(internaltoken.NULL, "null", start, end), nil
	default:
		return internaltoken.Token{}, fmt.Errorf("filter: invalid literal kind %q", lit.Kind)
	}
}
