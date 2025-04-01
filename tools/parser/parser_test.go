// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestParseHeaderStanza(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		stanzaExp *HeaderStanza
		err       error
	}{
		{
			name: "MissingFormatField",
			content: "" +
				"\n",
			err: errors.New("failed to validate header stanza: missing field Format"),
		},

		{
			name: "EmptyFormatWithNewline",
			content: "" +
				"\n" +
				"Format:\n" +
				"\n",
			err: errors.New("expected a string, found: \"\\n\""),
		},

		{
			name: "EmptyFormatWithEOF",
			content: "" +
				"\n" +
				"Format:  ",
			err: errors.New("expected a string, found: \"\""),
		},

		{
			name: "DuplicatedFormat",
			content: "" +
				"Format: format1\n" +
				"Format: format2\n",
			err: errors.New("duplicated Format field in header stanza"),
		},

		{
			name: "UnexpectedField",
			content: "" +
				"Header: Header\n",
			err: errors.New("found unexpected field: \"Header:\""),
		},

		{
			name: "WithLineComment",
			content: "" +
				"# Line comment\n" +
				"Format: format\n",
			stanzaExp: &HeaderStanza{
				Format: strPtr("format"),
			},
		},

		{
			// Not allowed.
			name: "WithEmbeddedLineComment",
			content: "" +
				"Format: format # Line comment\n",
			err: errors.New("failed to parse Format field"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tc.content))
			stanzaOut, err := p.parseHeaderStanza()

			require.Equal(t, tc.stanzaExp, stanzaOut)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseFilesStanza(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		stanzaExp *FilesStanza
		err       error
	}{
		{
			name: "DuplicatedFiles",
			content: "" +
				"Files: file1 file2\n" +
				"Copyright: Test Corporation\n" +
				"License: LicenseTest-Ref\n" +
				"Files: file3\n",
			err: errors.New("duplicated Files field in files stanza"),
		},

		{
			name: "DuplicatedCopyright",
			content: "" +
				"Files: file1 file2\n" +
				"Copyright: Test1 Corporation\n" +
				"License: LicenseTest-Ref\n" +
				"Copyright: Test2 Corporation\n",
			err: errors.New("duplicated Copyright field in files stanza"),
		},

		{
			name: "DuplicatedLicense",
			content: "" +
				"Files: file1 file2\n" +
				"Copyright: Test Corporation\n" +
				"License: LicenseTest1-Ref\n" +
				"Comment: this is a comment\n" +
				"License: LicenseTest2-Ref\n",
			err: errors.New("duplicated License field in files stanza"),
		},

		{
			name: "DuplicatedComment",
			content: "" +
				"Files: file1 file2\n" +
				"Copyright: Test2 Corporation\n" +
				"License: LicenseTest-Ref\n" +
				"Comment: This is a comment\n" +
				"Comment: This is another comment\n",
			err: errors.New("duplicated Comment field in files stanza"),
		},

		{
			name: "UnexpectedField",
			content: "" +
				"Files: file1 file2\n" +
				"Format: format\n" +
				"Copyright: Test Corporation\n" +
				"License: LicenseTest-Ref\n",
			err: errors.New("found unexpected field: \"Format:\""),
		},

		{
			name: "MissingFilesField",
			content: "" +
				"Copyright: Test Corporation\n" +
				"License: LicenseTest-Ref\n",
			err: errors.New("failed to validate files stanza: missing field Files"),
		},

		{
			name: "MissingCopyrightField",
			content: "" +
				"Files: file1 file2\n" +
				"License: LicenseTest-Ref\n",
			err: errors.New("failed to validate files stanza: missing field Copyright"),
		},

		{
			name: "MissingLicenseField",
			content: "" +
				"Files: file1 file2\n" +
				"Copyright: Test Corporation\n",
			err: errors.New("failed to validate files stanza: missing field License"),
		},

		{
			name: "EmptyFiles",
			content: "" +
				"Files: # List of files\n" +
				"Copyright: Test Corporation\n" +
				"License: LicenseTest-Ref\n",
			err: errors.New("failed to validate files stanza: field Files cannot be empty"),
		},

		{
			name: "EmptyCopyright",
			content: "" +
				"Files: file1 file2\n" +
				"Copyright:\n" +
				"License: LicenseTest-Ref\n",
			err: errors.New("failed to validate files stanza: field Copyright cannot be empty"),
		},

		{
			name: "EmptyLicense",
			content: "" +
				"Files: file1 file2\n" +
				"Copyright: Test Corporation\n" +
				"License:\n",
			err: errors.New("failed to validate files stanza: field License cannot be empty"),
		},

		{
			name: "WithLineComment",
			content: "" +
				"# Line comment\n" +
				"Files: file1 file2\n" +
				"Copyright: Test Corporation\n" +
				"License: LicenseTest-Ref\n",
			stanzaExp: &FilesStanza{
				Files: []string{
					"file1",
					"file2",
				},
				Copyright: []string{"Test Corporation"},
				License:   strPtr("LicenseTest-Ref"),
			},
		},

		{
			name: "WithEmbeddedLineComment",
			content: "" +
				"Files: file1 file2 # Line comment\n" +
				"Copyright: Test Corporation\n" +
				"License: LicenseTest-Ref\n",
			stanzaExp: &FilesStanza{
				Files: []string{
					"file1",
					"file2",
				},
				Copyright: []string{"Test Corporation"},
				License:   strPtr("LicenseTest-Ref"),
			},
		},

		{
			name: "WithMultipleCopyrights",
			content: "" +
				"Files: file1 file2 # Line comment\n" +
				"Copyright: Test1 Corporation\n" +
				" Test2 Corporation\n" +
				"License: LicenseTest-Ref\n",
			stanzaExp: &FilesStanza{
				Files: []string{
					"file1",
					"file2",
				},
				Copyright: []string{
					"Test1 Corporation",
					"Test2 Corporation",
				},
				License: strPtr("LicenseTest-Ref"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tc.content))
			stanzaOut, err := p.parseFilesStanza()

			require.Equal(t, tc.stanzaExp, stanzaOut)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseLineList(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		linesExp []string
	}{
		{
			name:    "SingleLineList",
			content: "   file1 file2 # Valid files",
			linesExp: []string{
				"file1",
				"file2",
			},
		},

		{
			name: "MultipleLineList",
			content: "" +
				"   file1 file2 # First line\n" +
				" file3 file4\n" +
				" file5\n",
			linesExp: []string{
				"file1",
				"file2",
				"file3",
				"file4",
				"file5",
			},
		},

		{
			name: "WithoutLineIndentation",
			content: "" +
				" file1\n" +
				// No indentation is considered end of list.
				"file2\n",
			linesExp: []string{"file1"},
		},

		{
			name: "WithLineComments",
			content: "" +
				" file1\n" +
				" # file2\n" +
				"#file 3\n" +
				" file4",
			linesExp: []string{
				"file1",
				"file4",
			},
		},

		{
			name: "WithNewlineEnding",
			content: "" +
				" file1\n" +
				" file2 file3\n" +
				"\n" +
				// Beginning of other section.
				"Keyword:\n",
			linesExp: []string{
				"file1",
				"file2",
				"file3",
			},
		},

		{
			name: "WithKeywordEnding",
			content: "" +
				" file1\n" +
				// Beginning of other section.
				"LICENSE: Test Corpotation\n",
			linesExp: []string{"file1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tc.content))
			linesOut := p.parseLineList()
			require.Equal(t, tc.linesExp, linesOut)
		})
	}
}

func TestParseLines(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		linesExp []string
	}{
		{
			name: "MultipleIndentations",
			content: "   first line can have any whitespaces # and comments\n" +
				" consecutive lines need to have at least one whitespace\n" +
				"   but there may be more whitespaces\n" +
				"new line without indentation\n",
			linesExp: []string{
				"first line can have any whitespaces",
				"consecutive lines need to have at least one whitespace",
				"but there may be more whitespaces",
			},
		},

		{
			name: "WithLineComment",
			content: "   first line can have any whitespaces # and comments\n" +
				" consecutive lines need to have at least one whitespace\n" +
				"#   but there may be more whitespaces\n" +
				"  # new line without indentation\n" +
				"\n",
			linesExp: []string{
				"first line can have any whitespaces",
				"consecutive lines need to have at least one whitespace",
			},
		},

		{
			name: "WithNewline",
			content: "   first line can have any whitespaces # and comments\n" +
				" consecutive lines need to have at least one whitespace\n" +
				// Considered the end of the section.
				"\n" +
				"new line without indentation\n",
			linesExp: []string{
				"first line can have any whitespaces",
				"consecutive lines need to have at least one whitespace",
			},
		},

		{
			name: "WithKeyword",
			content: "   first line can have any whitespaces # and comments\n" +
				" consecutive lines need to have at least one whitespace\n" +
				// Considered the end of the section.
				"Comment:\n" +
				" This line is part of a comment\n",
			linesExp: []string{
				"first line can have any whitespaces",
				"consecutive lines need to have at least one whitespace",
			},
		},

		{
			name:    "WithEmbeddedKeyword",
			content: "   first line Comment: \n",
			linesExp: []string{
				"first line ",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tc.content))
			linesOut := p.parseLines()
			require.Equal(t, tc.linesExp, linesOut)
		})
	}
}

func TestParseSingleValue(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		valueExp string
		err      error
	}{
		{
			name:    "WithoutValue",
			content: "  \n",
			err:     errors.New("expected a string, found: \"\\n\""),
		},

		{
			name:    "WithCommentLine",
			content: " value # comment line\n",
			err:     errors.New("expected new line, found: \"#\""),
		},

		{
			name:    "MultipleValues",
			content: " value1 value2\n",
			err:     errors.New("expected new line, found: \"value2\""),
		},

		{
			name:     "SingleValue",
			content:  " value    \n",
			valueExp: "value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tc.content))
			valueOut, err := p.parseSingleValue()

			require.Equal(t, tc.valueExp, valueOut)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParse(t *testing.T) {
	testCases := []struct {
		name         string
		fileContent  string
		dep5Expected *dep5File
		err          error
	}{
		{
			name: "Valid",
			fileContent: "" +
				"# This is a line comment\n" +
				"Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/\n" +
				"\n" +
				"Files:\n" +
				"  file1 file2\n" +
				"        file3 # line comment\n" +
				"Copyright: 2024 Alpha Corporation # Custom license\n" +
				"			2020 Beta Corporation # Custom license\n" +
				"License: GPL-2+ with OpenSSL exception\n" +
				" Copying and distribution of this package, with or without modification,\n" +
				" are permitted in any medium without royalty provided the copyright notice\n" +
				" and this notice are preserved.\n" +
				"Comment:\n" +
				"  This is a multiline\n" +
				" comment which is not processed by reuse\n",
			dep5Expected: &dep5File{
				Header: &HeaderStanza{
					Format: strPtr("https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/"),
				},
				Files: []*FilesStanza{
					{
						Files: []string{"file1", "file2", "file3"},
						Copyright: []string{
							"2024 Alpha Corporation",
							"2020 Beta Corporation",
						},
						License: strPtr("GPL-2+ with OpenSSL exception"),

						hasComment: true,
					},
				},
			},
		},

		{
			name: "InvalidHeaderStanza",
			fileContent: "" +
				// Should contain a single value string.
				"Format: \n" +
				"Files:\n" +
				"  file1 file2\n" +
				"        file3 # line comment\n" +
				"Copyright: 2024 Alpha Corporation # Custom license\n" +
				"			2020 Beta Corporation # Custom license\n" +
				"License: GPL-2+ with OpenSSL exception\n",
			err: errors.New("failed to parse header stanza"),
		},

		{
			name: "InvalidFilesStanza",
			fileContent: "" +
				"# This is a line comment\n" +
				"Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/\n" +
				"\n" +
				"Files:\n" +
				"  file1 file2\n" +
				"        file3 # line comment\n" +
				"Copyright: 2024 Alpha Corporation # Custom license\n" +
				"			2020 Beta Corporation # Custom license\n" +
				// End of the files stanza section.
				"\n" +
				"License: GPL-2+ with OpenSSL exception\n" +
				" Copying and distribution of this package, with or without modification,\n" +
				" are permitted in any medium without royalty provided the copyright notice\n" +
				" and this notice are preserved.\n" +
				"Comment:\n" +
				"  This is a multiline\n" +
				" comment which is not processed by reuse\n",
			err: errors.New("failed to parse files stanza"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tc.fileContent))

			dep5Out, err := p.Parse()
			require.Equal(t, tc.dep5Expected, dep5Out)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
