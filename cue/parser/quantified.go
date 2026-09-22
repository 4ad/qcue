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
func (p *parser) quantifierAhead(block bool) bool {
	if p.tok != token.IDENT || (p.lit != "forall" && p.lit != "exists") {
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
	ranged := false
	if tok == token.RPAREN {
		depth = 0
	}
	for depth > 0 {
		switch next() {
		case token.IN:
			if depth == 1 {
				ranged = true
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
		// A range binder cannot be a call argument. Without that marker,
		// preserve ordinary expressions such as forall(x) + 1.
		return ranged
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

func (p *parser) packageBoundaryAhead() bool {
	p.inLookahead = true
	defer func() { p.inLookahead = false }()
	next := p.quantifiedLookahead()
	tok := next()
	if tok == token.IDENT || tok == token.LBRACE {
		return true
	}
	if tok != token.LPAREN {
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
	// An ordinary call ends here. A package boundary continues with the
	// contextual separator "as" or "with", checked by the real parser.
	return next() == token.IDENT
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
	tok := next()
	return tok == token.COLON || tok == token.BIND
}

func (p *parser) parseQuantifiedField() ast.Decl {
	name := p.parseIdentDecl()
	q := &ast.Quantifier{Quantifier: name.Pos()}
	q.Lparen = p.expect(token.LPAREN)
	q.Params = p.parseTypeParams(token.RPAREN)
	q.Rparen = p.expectClosing(token.RPAREN, "quantified field")
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

func (p *parser) expectContextual(word string) token.Pos {
	pos := p.pos
	if p.tok != token.IDENT || p.lit != word {
		p.errorExpected(pos, word)
	} else {
		p.next()
	}
	return pos
}

func (p *parser) parseSeal() ast.Expr {
	s := &ast.SealExpr{Seal: p.expectContextual("seal")}
	s.Interface = p.parseRHS()
	s.With = p.expectContextual("with")
	s.Lparen = p.expect(token.LPAREN)
	for p.tok != token.RPAREN && p.tok != token.EOF {
		w := &ast.Alias{Ident: p.parseIdentDecl(), Equal: p.expect(token.BIND)}
		w.Expr = p.parseRHS()
		s.Witnesses = append(s.Witnesses, w)
		if p.tok != token.COMMA {
			break
		}
		p.next()
	}
	s.Rparen = p.expectClosing(token.RPAREN, "seal witnesses")
	s.Body = p.parseStruct()
	return s
}

func (p *parser) parseOpen() ast.Expr {
	o := &ast.OpenExpr{Open: p.expectContextual("open")}
	o.Value = p.parseRHS()
	o.As = p.expectContextual("as")
	o.Lparen = p.expect(token.LPAREN)
	o.Type = p.parseIdentDecl()
	p.expect(token.COMMA)
	o.View = p.parseIdentDecl()
	o.Rparen = p.expectClosing(token.RPAREN, "opened type and view")
	o.Body = p.parseStruct()
	return o
}

// parseBlockPrefix reads the ordered binder prefix of a record. A quantifier
// followed by an expression instead of a declaration separator is an ordinary
// embedding, and is returned separately. Bounds can mention earlier binders;
// resolution happens after the complete lexical tree has been assembled.
func (p *parser) parseBlockPrefix() (prefix []*ast.Quantifier, first ast.Decl) {
	for p.quantifiedEnabled() && p.quantifierAhead(true) {
		q := &ast.Quantifier{Quantifier: p.pos, Exists: p.lit == "exists"}
		p.next()
		if p.tok == token.LPAREN {
			q.Lparen = p.expect(token.LPAREN)
			q.Params = p.parseTypeParams(token.RPAREN)
			q.Rparen = p.expectClosing(token.RPAREN, "block quantifier")
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
