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

// This file is based on https://github.com/astrieanna/rose-warmups/tree/master/tasty/lib.

package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

type Scanner struct {
	r *bufio.Reader
}

func NewScanner(r io.Reader) *Scanner {
	return &Scanner{r: bufio.NewReader(r)}
}

func (s *Scanner) read() rune {
	ch, _, err := s.r.ReadRune()
	if err != nil {
		return rune(0)
	}
	return ch
}

func (s *Scanner) unread() {
	err := s.r.UnreadRune()
	if err != nil {
		fmt.Printf("!ERROR Unexpected error on UnreadRune: %v", err)
	}
}

func (s *Scanner) scanWhitespace() (tok Token, lit string) {
	var buf bytes.Buffer
	buf.WriteRune(s.read())

forLoop:
	for {
		ch := s.read()
		switch {
		case ch == eof:
			break forLoop
		case !isWhitespace(ch):
			s.unread()
			break forLoop
		default:
			_, _ = buf.WriteRune(ch)
		}
	}

	return WHITESPACE, buf.String()
}

func (s *Scanner) scanString() (tok Token, lit string) {
	var buf bytes.Buffer
	buf.WriteRune(s.read())

forLoop:
	for {
		ch := s.read()
		switch {
		case ch == eof:
			break forLoop
		case isWhitespace(ch), isNewline(ch):
			s.unread()
			break forLoop
		default:
			_, _ = buf.WriteRune(ch)
		}
	}

	switch buf.String() {
	case "Format:":
		return FORMAT, buf.String()
	case "Files:":
		return FILES, buf.String()
	case "Copyright:":
		return COPYRIGHT, buf.String()
	case "License:":
		return LICENSE, buf.String()
	case "Comment:":
		return COMMENT, buf.String()
	}

	return STRING, buf.String()
}

func (s *Scanner) Scan() (tok Token, lit string) {
	ch := s.read()

	switch {
	case isWhitespace(ch):
		s.unread()
		return s.scanWhitespace()
	case isCharacter(ch):
		s.unread()
		return s.scanString()
	case isNewline(ch):
		return NEWLINE, string(ch)
	case isHash(ch):
		return HASH, string(ch)
	case ch == eof:
		return EOF, ""
	default:
		return ILLEGAL, string(ch)
	}
}
