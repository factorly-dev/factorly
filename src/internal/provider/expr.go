// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
)

// EvalCondition evaluates a condition expression against workflow variables.
// Returns true if the expression is truthy. Errors are treated as false (fail-closed).
func EvalCondition(expr string, vars map[string]string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	tokens := tokenize(expr)
	node := parse(tokens)
	if node == nil {
		return false
	}
	val, err := evaluate(node, vars)
	if err != nil {
		return false
	}
	return isTruthy(val)
}

// --- Tokenizer ---

type tokenType int

const (
	tokString tokenType = iota
	tokNumber
	tokIdent
	tokTrue
	tokFalse
	tokEq  // ==
	tokNeq // !=
	tokLt
	tokGt
	tokLte // <=
	tokGte // >=
	tokAnd
	tokOr
	tokNot
	tokDot
	tokLParen
	tokRParen
	tokComma
	tokEOF
)

type token struct {
	typ tokenType
	val string
}

func tokenize(s string) []token {
	var tokens []token
	i := 0
	for i < len(s) {
		ch := s[i]

		// Whitespace
		if unicode.IsSpace(rune(ch)) {
			i++
			continue
		}

		// String literals
		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			start := i
			for i < len(s) && s[i] != quote {
				i++
			}
			tokens = append(tokens, token{tokString, s[start:i]})
			if i < len(s) {
				i++ // closing quote
			}
			continue
		}

		// Numbers
		if ch >= '0' && ch <= '9' {
			start := i
			for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
				i++
			}
			tokens = append(tokens, token{tokNumber, s[start:i]})
			continue
		}

		// Operators
		if ch == '=' && i+1 < len(s) && s[i+1] == '=' {
			tokens = append(tokens, token{tokEq, "=="})
			i += 2
			continue
		}
		if ch == '!' && i+1 < len(s) && s[i+1] == '=' {
			tokens = append(tokens, token{tokNeq, "!="})
			i += 2
			continue
		}
		if ch == '<' && i+1 < len(s) && s[i+1] == '=' {
			tokens = append(tokens, token{tokLte, "<="})
			i += 2
			continue
		}
		if ch == '>' && i+1 < len(s) && s[i+1] == '=' {
			tokens = append(tokens, token{tokGte, ">="})
			i += 2
			continue
		}
		if ch == '<' {
			tokens = append(tokens, token{tokLt, "<"})
			i++
			continue
		}
		if ch == '>' {
			tokens = append(tokens, token{tokGt, ">"})
			i++
			continue
		}
		if ch == '.' {
			tokens = append(tokens, token{tokDot, "."})
			i++
			continue
		}
		if ch == '(' {
			tokens = append(tokens, token{tokLParen, "("})
			i++
			continue
		}
		if ch == ')' {
			tokens = append(tokens, token{tokRParen, ")"})
			i++
			continue
		}
		if ch == ',' {
			tokens = append(tokens, token{tokComma, ","})
			i++
			continue
		}

		// Identifiers and keywords
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			start := i
			for i < len(s) && (s[i] == '_' || (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= '0' && s[i] <= '9')) {
				i++
			}
			word := s[start:i]
			switch word {
			case "true":
				tokens = append(tokens, token{tokTrue, word})
			case "false":
				tokens = append(tokens, token{tokFalse, word})
			case "and":
				tokens = append(tokens, token{tokAnd, word})
			case "or":
				tokens = append(tokens, token{tokOr, word})
			case "not":
				tokens = append(tokens, token{tokNot, word})
			default:
				tokens = append(tokens, token{tokIdent, word})
			}
			continue
		}

		// Skip unknown
		i++
	}
	tokens = append(tokens, token{tokEOF, ""})
	return tokens
}

// --- Parser (recursive descent) ---

type node interface{}

type literalNode struct{ val any }
type identNode struct{ name string }
type binaryNode struct {
	op          tokenType
	left, right node
}
type unaryNode struct {
	op      tokenType
	operand node
}
type memberNode struct {
	object node
	field  string
}
type callNode struct {
	name string
	args []node
}

type parser struct {
	tokens []token
	pos    int
}

func parse(tokens []token) node {
	p := &parser{tokens: tokens}
	return p.parseOr()
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{tokEOF, ""}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) parseOr() node {
	left := p.parseAnd()
	for p.peek().typ == tokOr {
		p.advance()
		right := p.parseAnd()
		left = &binaryNode{tokOr, left, right}
	}
	return left
}

func (p *parser) parseAnd() node {
	left := p.parseComparison()
	for p.peek().typ == tokAnd {
		p.advance()
		right := p.parseComparison()
		left = &binaryNode{tokAnd, left, right}
	}
	return left
}

func (p *parser) parseComparison() node {
	left := p.parseUnary()
	for {
		t := p.peek()
		if t.typ == tokEq || t.typ == tokNeq || t.typ == tokLt || t.typ == tokGt || t.typ == tokLte || t.typ == tokGte {
			p.advance()
			right := p.parseUnary()
			left = &binaryNode{t.typ, left, right}
		} else {
			break
		}
	}
	return left
}

func (p *parser) parseUnary() node {
	if p.peek().typ == tokNot {
		p.advance()
		operand := p.parseUnary()
		return &unaryNode{tokNot, operand}
	}
	return p.parseAccess()
}

func (p *parser) parseAccess() node {
	n := p.parsePrimary()
	for {
		if p.peek().typ == tokDot {
			p.advance()
			field := p.advance()
			n = &memberNode{n, field.val}
		} else {
			break
		}
	}
	return n
}

func (p *parser) parsePrimary() node {
	t := p.advance()
	switch t.typ {
	case tokString:
		return &literalNode{t.val}
	case tokNumber:
		if f, err := strconv.ParseFloat(t.val, 64); err == nil {
			return &literalNode{f}
		}
		return &literalNode{t.val}
	case tokTrue:
		return &literalNode{true}
	case tokFalse:
		return &literalNode{false}
	case tokIdent:
		// Check for function call
		if p.peek().typ == tokLParen {
			p.advance() // consume (
			var args []node
			if p.peek().typ != tokRParen {
				args = append(args, p.parseOr())
				for p.peek().typ == tokComma {
					p.advance()
					args = append(args, p.parseOr())
				}
			}
			if p.peek().typ == tokRParen {
				p.advance() // consume )
			}
			return &callNode{t.val, args}
		}
		return &identNode{t.val}
	case tokLParen:
		n := p.parseOr()
		if p.peek().typ == tokRParen {
			p.advance()
		}
		return n
	}
	return &literalNode{nil}
}

// --- Evaluator ---

func evaluate(n node, vars map[string]string) (any, error) {
	switch v := n.(type) {
	case *literalNode:
		return v.val, nil
	case *identNode:
		return vars[v.name], nil
	case *binaryNode:
		left, _ := evaluate(v.left, vars)
		right, _ := evaluate(v.right, vars)
		return evalBinaryOp(v.op, left, right), nil
	case *unaryNode:
		operand, _ := evaluate(v.operand, vars)
		if v.op == tokNot {
			return !isTruthy(operand), nil
		}
		return nil, nil
	case *memberNode:
		obj, _ := evaluate(v.object, vars)
		return accessMember(obj, v.field), nil
	case *callNode:
		return evalCall(v.name, v.args, vars), nil
	}
	return nil, nil
}

func evalBinaryOp(op tokenType, left, right any) any {
	switch op {
	case tokEq:
		return toComparable(left) == toComparable(right)
	case tokNeq:
		return toComparable(left) != toComparable(right)
	case tokLt:
		return toFloat(left) < toFloat(right)
	case tokGt:
		return toFloat(left) > toFloat(right)
	case tokLte:
		return toFloat(left) <= toFloat(right)
	case tokGte:
		return toFloat(left) >= toFloat(right)
	case tokAnd:
		return isTruthy(left) && isTruthy(right)
	case tokOr:
		return isTruthy(left) || isTruthy(right)
	}
	return false
}

func accessMember(obj any, field string) any {
	// If obj is a string that looks like JSON, parse it
	if s, ok := obj.(string); ok {
		var parsed map[string]any
		if json.Unmarshal([]byte(s), &parsed) == nil {
			if val, ok := parsed[field]; ok {
				return val
			}
		}
		return nil
	}
	// If obj is already a map
	if m, ok := obj.(map[string]any); ok {
		return m[field]
	}
	return nil
}

func evalCall(name string, args []node, vars map[string]string) any {
	switch name {
	case "contains":
		if len(args) >= 2 {
			haystack, _ := evaluate(args[0], vars)
			needle, _ := evaluate(args[1], vars)
			return strings.Contains(toString(haystack), toString(needle))
		}
	case "jsonpath":
		if len(args) >= 2 {
			data, _ := evaluate(args[0], vars)
			expr, _ := evaluate(args[1], vars)
			return evalJSONPath(toString(data), toString(expr))
		}
	}
	return false
}

func evalJSONPath(data, path string) any {
	parsed, err := jp.ParseString(path)
	if err != nil {
		return nil
	}
	obj, err := oj.ParseString(data)
	if err != nil {
		return nil
	}
	results := parsed.Get(obj)
	if len(results) == 0 {
		return nil
	}
	if len(results) == 1 {
		return results[0]
	}
	// Multiple results → return as JSON string
	b, _ := json.Marshal(results)
	return string(b)
}

// --- Helpers ---

func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != "" && val != "0" && val != "false"
	case float64:
		return val != 0
	case int:
		return val != 0
	case int64:
		return val != 0
	}
	return true
}

func toComparable(v any) string {
	return toString(v)
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
