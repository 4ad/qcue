// Copyright 2026 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

func (p *parser) quantifiedEnabled() bool {
	return p.experiments != nil && p.experiments.Quantified
}

// quantifierAhead leaves ordinary uses of the contextual keywords alone,
// including field labels, selectors, and references named forall.
func (p *parser) quantifierAhead(block bool) bool {
	if p.tok != token.IDENT || p.lit != "forall" {
		return false
	}
	p.inLookahead = true
	defer func() { p.inLookahead = false }()
	next := p.quantifiedLookahead()
	tok := next()
	if tok == token.IDENT {
		return true
	}
	if !p.quantifiedEnabled() || tok != token.LPAREN {
		return false
	}
	tok = next()
	if tok != token.IDENT && tok != token.RPAREN {
		return false
	}
	depth := 1
	constrained := false
	if tok == token.RPAREN {
		depth = 0
	}
	for depth > 0 {
		switch next() {
		case token.COLON:
			if depth == 1 {
				constrained = true
			}
		case token.LPAREN:
			depth++
		case token.RPAREN:
			depth--
		case token.EOF, token.INTERPOLATION:
			return false
		}
	}
	switch next() {
	case token.IDENT, token.FUNC, token.LBRACE, token.LBRACK, token.LPAREN,
		token.INT, token.FLOAT, token.STRING, token.TRUE, token.FALSE, token.NULL, token.BOTTOM:
		return true
	case token.COMMA:
		return block
	case token.ADD, token.SUB, token.NOT, token.MUL,
		token.LSS, token.LEQ, token.GEQ, token.GTR,
		token.NEQ, token.MAT, token.NMAT, token.EQL:
		// A constrained binder takes precedence over a contextual-keyword
		// call followed by an operator. An ordinary call with labeled
		// arguments can be parenthesized to disambiguate it. Unconstrained
		// calls such as forall(x) + 1 keep their ordinary interpretation.
		return constrained
	}
	return false
}

// quantifiedLookahead reads a copy of the scanner, including an already
// buffered token. Comments do not change recognition of contextual keywords.
func (p *parser) quantifiedLookahead() func() token.Token {
	s := p.scanner
	peeked := p.peekToken.scanned
	return func() token.Token {
		if peeked {
			peeked = false
			if p.peekToken.tok != token.COMMENT {
				return p.peekToken.tok
			}
		}
		for {
			_, tok, _ := s.Scan()
			if tok != token.COMMENT {
				return tok
			}
		}
	}
}

func (p *parser) parseTypeParam() (param *ast.TypeParam) {
	c := p.openComments()
	defer func() { c.closeNode(p, param) }()
	param = &ast.TypeParam{Name: p.parseIdentDecl()}
	if p.tok == token.COLON {
		param.Colon = p.expect(token.COLON)
		param.Bound = p.parseRHS()
	}
	return param
}

// Keep the delimiters inside the comment list so comments after the opener
// and before the closer attach to parameters, not the surrounding expression.
func (p *parser) parseTypeParams(start, end token.Token) (lparen token.Pos, params []*ast.TypeParam, rparen token.Pos) {
	p.openList()
	defer p.closeList()
	lparen = p.expect(start)
	for p.tok != end && p.tok != token.EOF {
		params = append(params, p.parseTypeParam())
		if p.tok != token.COMMA {
			break
		}
		p.next()
	}
	if len(params) == 0 {
		p.errf(p.pos, "quantifier requires at least one parameter")
	}
	rparen = p.expectClosing(end, "type parameter list")
	return lparen, params, rparen
}

func (p *parser) parseQuantifier() (expr ast.Expr) {
	c := p.openComments()
	defer func() { c.closeNode(p, expr) }()
	q := &ast.Quantifier{Quantifier: p.pos}
	if !p.quantifiedEnabled() {
		p.errf(p.pos, "quantifier syntax requires @experiment(quantified)")
	}
	p.next()
	if p.tok == token.LPAREN {
		q.Lparen, q.Params, q.Rparen = p.parseTypeParams(token.LPAREN, token.RPAREN)
	} else {
		q.Params = []*ast.TypeParam{{Name: p.parseIdentDecl()}}
	}
	q.Body = p.parseRHS()
	return q
}

// quantifiedFieldAhead distinguishes a declaration prefix from an embedded
// call expression. Scanning never changes the parser or resolves identifiers.
func (p *parser) quantifiedFieldAhead() bool {
	if !p.quantifiedEnabled() || p.tok != token.IDENT {
		return false
	}
	s := p.scanner
	p.inLookahead = true
	defer func() { p.inLookahead = false }()
	peeked := p.peekToken.scanned
	next := func() token.Token {
		if peeked {
			peeked = false
			return p.peekToken.tok
		}
		for {
			_, tok, _ := s.Scan()
			if tok != token.COMMENT {
				return tok
			}
		}
	}
	if next() != token.LPAREN {
		return false
	}
	depth := 1
	for depth > 0 {
		switch next() {
		case token.LPAREN:
			depth++
		case token.RPAREN:
			depth--
		case token.EOF, token.INTERPOLATION:
			return false
		}
	}
	tok := next()
	return tok == token.COLON || tok == token.BIND
}

func (p *parser) parseQuantifiedField() ast.Decl {
	name := p.parseIdentDecl()
	q := &ast.Quantifier{Quantifier: name.Pos(), Shorthand: true}
	q.Lparen, q.Params, q.Rparen = p.parseTypeParams(token.LPAREN, token.RPAREN)
	if p.tok == token.BIND {
		a := &ast.ParametricAlias{
			Name: name, Lparen: q.Lparen, Params: q.Params, Rparen: q.Rparen,
			Equal: p.expect(token.BIND), Body: p.parseRHS(),
		}
		p.consumeDeclComma()
		return a
	}
	colon := p.expect(token.COLON)
	q.Body = p.parseRHS()
	f := &ast.Field{Label: name, TokenPos: colon, Value: q}
	f.Attrs = p.parseAttributes()
	p.consumeDeclComma()
	return f
}

// parseBlockPrefix reads the ordered binder prefix of a record. A quantifier
// followed by an expression instead of a declaration separator is an ordinary
// embedding, and is returned separately. Bounds can mention earlier binders;
// resolution happens after the complete lexical tree has been assembled.
func (p *parser) parseBlockPrefix() (prefix []*ast.Quantifier, first ast.Decl) {
	for p.quantifiedEnabled() && p.quantifierAhead(true) {
		q := &ast.Quantifier{Quantifier: p.pos}
		p.next()
		if p.tok == token.LPAREN {
			q.Lparen, q.Params, q.Rparen = p.parseTypeParams(token.LPAREN, token.RPAREN)
		} else {
			q.Params = []*ast.TypeParam{p.parseTypeParam()}
		}
		if p.tok != token.COMMA {
			q.Body = p.parseRHS()
			p.consumeDeclComma()
			return prefix, &ast.EmbedDecl{Expr: q}
		}
		p.next()
		prefix = append(prefix, q)
	}
	return prefix, nil
}
