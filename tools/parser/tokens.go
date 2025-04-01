// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package parser

import "unicode"

type Token int

const (
	ILLEGAL Token = iota
	EOF
	WHITESPACE
	NEWLINE
	HASH

	// Literals.
	STRING

	// Keywords.
	FORMAT
	FILES
	COPYRIGHT
	LICENSE
	COMMENT
)

var eof = rune(0)

func isHash(ch rune) bool {
	return ch == '#'
}

func isWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t'
}

func isNewline(ch rune) bool {
	return ch == '\n'
}

func isCharacter(ch rune) bool {
	switch {
	case unicode.IsLetter(ch):
	case unicode.IsDigit(ch):
	case ch == '_':
	case ch == '.':
	case ch == '/':
	case ch == '*':
	case ch == '?':
	default:
		return false
	}
	return true
}
