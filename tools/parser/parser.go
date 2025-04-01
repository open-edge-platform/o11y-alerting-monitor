// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

// The MIT License (MIT)

// Copyright (c) 2014 Ben Johnson

// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// Package parser implements a parser for machine readable debian/copyright format files: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
// This file is based on https://github.com/astrieanna/rose-warmups/tree/master/tasty/lib.
package parser

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type Parser struct {
	s   *Scanner
	buf struct {
		tok         Token
		lit         string
		isUnscanned bool
	}
}

func NewParser(r io.Reader) *Parser {
	return &Parser{s: NewScanner(r)}
}

func (p *Parser) scan() (tok Token, lit string) {
	if p.buf.isUnscanned {
		p.buf.isUnscanned = false
		return p.buf.tok, p.buf.lit
	}

	tok, lit = p.s.Scan()
	p.buf.tok, p.buf.lit = tok, lit

	return tok, lit
}

func (p *Parser) unscan() { p.buf.isUnscanned = true }

func (p *Parser) scanSkipWhitespace() (tok Token, lit string) {
	tok, lit = p.scan()
	if tok == WHITESPACE {
		tok, lit = p.scan()
	}
	return tok, lit
}

func (p *Parser) skipNonKeywords() {
	for {
		tok, _ := p.scan()
		switch tok {
		case NEWLINE:
		case WHITESPACE:
		default:
			p.unscan()
			return
		}
	}
}

func (p *Parser) skipLine() {
	for {
		if t, _ := p.scan(); t == NEWLINE || t == EOF {
			return
		}
	}
}

func (p *Parser) skipTextLines() {
	for {
		tok, _ := p.scan()

		switch tok {
		case WHITESPACE, HASH:
			p.skipLine()
		default:
			p.unscan()
			return
		}
	}
}

func (p *Parser) parseSingleValue() (string, error) {
	var line string
	tok, lit := p.scanSkipWhitespace()
	switch tok {
	case STRING:
		line = lit
	default:
		return "", fmt.Errorf("expected a string, found: %q", lit)
	}

	tok, lit = p.scanSkipWhitespace()
	if tok != NEWLINE {
		return "", fmt.Errorf("expected new line, found: %q", lit)
	}
	return line, nil
}

func (p *Parser) parseLineStrings() []string {
	lines := []string{}

forLoop:
	for {
		tok, lit := p.scanSkipWhitespace()
		switch tok {
		case STRING:
			lines = append(lines, lit)
		case HASH:
			p.skipLine()
			break forLoop
		case NEWLINE:
			break forLoop
		default:
			p.unscan()
			break forLoop
		}
	}
	return lines
}

func (p *Parser) parseLineList() []string {
	lines := []string{}

	firstLineStr := p.parseLineStrings()
	if len(firstLineStr) != 0 {
		lines = append(lines, firstLineStr...)
	}

forLoop:
	for {
		tok, _ := p.scan()
		switch tok {
		case WHITESPACE:
			lines = append(lines, p.parseLineStrings()...)
		case HASH:
			p.skipLine()
		case NEWLINE:
			break forLoop
		default:
			p.unscan()
			break forLoop
		}
	}

	return lines
}

func (p *Parser) parseLine() string {
	line := strings.Builder{}

	tok, lit := p.scanSkipWhitespace()
forLoop:
	for {
		switch tok {
		case STRING:
			line.WriteString(lit)
		case WHITESPACE:
			if tok, _ = p.scanSkipWhitespace(); tok != NEWLINE && tok != HASH {
				line.WriteString(lit)
			}
			p.unscan()
		case HASH:
			p.skipLine()
			break forLoop
		case NEWLINE:
			break forLoop
		default:
			p.unscan()
			break forLoop
		}
		tok, lit = p.scan()
	}

	return line.String()
}

func (p *Parser) parseLines() []string {
	lines := make([]string, 0)

	// The first line skips all whitespaces.
	line := p.parseLine()
	if len(line) == 0 {
		return lines
	}
	lines = append(lines, line)

forLoop:
	for {
		tok, _ := p.scan()

		switch tok {
		case WHITESPACE:
			if line := p.parseLine(); len(line) != 0 {
				lines = append(lines, line)
			}
		case HASH:
			p.skipLine()
		default:
			p.unscan()
			break forLoop
		}
	}

	return lines
}

type HeaderStanza struct {
	Format *string
}

func (h HeaderStanza) validate() error {
	if h.Format == nil {
		return errors.New("missing field Format")
	} else if len(*h.Format) == 0 {
		return errors.New("field Format cannot be empty")
	}
	return nil
}

type FilesStanza struct {
	Files     []string
	Copyright []string
	License   *string

	hasComment bool
}

func (f FilesStanza) validate() error {
	if f.Files == nil {
		return errors.New("missing field Files")
	} else if len(f.Files) == 0 {
		return errors.New("field Files cannot be empty")
	}

	if f.Copyright == nil {
		return errors.New("missing field Copyright")
	} else if len(f.Copyright) == 0 {
		return errors.New("field Copyright cannot be empty")
	}

	if f.License == nil {
		return errors.New("missing field License")
	} else if len(*f.License) == 0 {
		return errors.New("field License cannot be empty")
	}

	return nil
}

type dep5File struct {
	Header *HeaderStanza
	Files  []*FilesStanza
}

func (p *Parser) Parse() (*dep5File, error) {
	f := &dep5File{
		Files: make([]*FilesStanza, 0),
	}

	// Parse header stanza.
	header, err := p.parseHeaderStanza()
	if err != nil {
		return nil, fmt.Errorf("failed to parse header stanza: %w", err)
	}
	f.Header = header

	// Parse files stanza.
	files, err := p.parseFilesStanza()
	if err != nil {
		return nil, fmt.Errorf("failed to parse files stanza: %w", err)
	}
	f.Files = append(f.Files, files)

	return f, nil
}

func (p *Parser) parseHeaderStanza() (*HeaderStanza, error) {
	var hs HeaderStanza

	p.skipNonKeywords()
forLoop:
	for {
		tok, lit := p.scan()

		switch tok {
		case FORMAT:
			if hs.Format != nil {
				return nil, errors.New("duplicated Format field in header stanza")
			}

			format, err := p.parseSingleValue()
			if err != nil {
				return nil, fmt.Errorf("failed to parse Format field: %w", err)
			}
			hs.Format = &format
		case HASH:
			p.skipLine()
		case NEWLINE, EOF:
			break forLoop
		default:
			return nil, fmt.Errorf("found unexpected field: %q", lit)
		}
	}

	if err := hs.validate(); err != nil {
		return nil, fmt.Errorf("failed to validate header stanza: %w", err)
	}

	return &hs, nil
}

func (p *Parser) parseFilesStanza() (*FilesStanza, error) {
	var fs FilesStanza

	p.skipNonKeywords()
forLoop:
	for {
		tok, lit := p.scan()

		switch tok {
		case FILES:
			if fs.Files != nil {
				return nil, errors.New("duplicated Files field in files stanza")
			}
			fs.Files = p.parseLineList()
		case COPYRIGHT:
			if fs.Copyright != nil {
				return nil, errors.New("duplicated Copyright field in files stanza")
			}
			fs.Copyright = p.parseLines()
			p.unscan()
		case LICENSE:
			if fs.License != nil {
				return nil, errors.New("duplicated License field in files stanza")
			}
			license := p.parseLine()
			fs.License = &license
			p.skipTextLines()
			p.unscan()
		case COMMENT:
			if fs.hasComment {
				return nil, errors.New("duplicated Comment field in files stanza")
			}
			p.skipTextLines()
			p.unscan()
			fs.hasComment = true
		case HASH:
			p.skipLine()
		case NEWLINE, EOF:
			break forLoop
		default:
			return nil, fmt.Errorf("found unexpected field: %q", lit)
		}
	}

	if err := fs.validate(); err != nil {
		return nil, fmt.Errorf("failed to validate files stanza: %w", err)
	}
	return &fs, nil
}
