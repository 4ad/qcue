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
// including field labels, selectors, and references named forall or exists.
func (p *parser) quantifierAhead() bool {
	if p.tok != token.IDENT || (p.lit != "forall" && p.lit != "exists") {
		return false
	}
	s := p.scanner
	tok := p.peekToken.tok
	if !p.peekToken.scanned {
		_, tok, _ = s.Scan()
	}
	return tok == token.IDENT || (p.quantifiedEnabled() && tok == token.LPAREN)
}

func (p *parser) parseTypeParam() (param *ast.TypeParam) {
	c := p.openComments()
	defer func() { c.closeNode(p, param) }()
	param = &ast.TypeParam{Name: p.parseIdentDecl()}
	if p.tok == token.IN {
		param.In = p.expect(token.IN)
		param.Sort = p.parseRHS()
	}
	if p.tok == token.COLON {
		param.Colon = p.expect(token.COLON)
		param.Bound = p.parseRHS()
	}
	return param
}

func (p *parser) parseTypeParams(end token.Token) (params []*ast.TypeParam) {
	p.openList()
	defer p.closeList()
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
	return params
}

func (p *parser) parseQuantifier() (expr ast.Expr) {
	c := p.openComments()
	defer func() { c.closeNode(p, expr) }()
	q := &ast.Quantifier{Quantifier: p.pos, Exists: p.lit == "exists"}
	if !p.quantifiedEnabled() {
		p.errf(p.pos, "quantifier syntax requires @experiment(quantified)")
	}
	p.next()
	if p.tok == token.LPAREN {
		q.Lparen = p.expect(token.LPAREN)
		q.Params = p.parseTypeParams(token.RPAREN)
		q.Rparen = p.expectClosing(token.RPAREN, "quantifier parameter list")
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
	return next() == token.COLON
}

func (p *parser) parseQuantifiedField() ast.Decl {
	name := p.parseIdentDecl()
	q := &ast.Quantifier{Quantifier: name.Pos()}
	q.Lparen = p.expect(token.LPAREN)
	q.Params = p.parseTypeParams(token.RPAREN)
	q.Rparen = p.expectClosing(token.RPAREN, "quantified field")
	colon := p.expect(token.COLON)
	q.Body = p.parseRHS()
	f := &ast.Field{Label: name, TokenPos: colon, Value: q}
	f.Attrs = p.parseAttributes()
	p.consumeDeclComma()
	return f
}
