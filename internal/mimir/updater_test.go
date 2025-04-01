// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package mimir

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

const validMimirOutput = `
name: 01e74407-0327-4e36-93cb-85801c098ba5
interval: 15s
rules:
    - alert: ClusterRAMUsageExceedsThreshold
      expr: doesn't matter
      for: 30s
`

func TestCompareRuleGroup(t *testing.T) {
	tests := map[string]struct {
		input         rules.RuleGroup
		statusCode    int
		mimirOutput   string
		errorExpected error
	}{
		"Mimir responds with status code 400": {
			input:         rules.RuleGroup{},
			statusCode:    400,
			mimirOutput:   "",
			errorExpected: errors.New("error while trying to receive rule group from mimir"),
		},
		"Empty rule groups": {
			input:         rules.RuleGroup{},
			statusCode:    200,
			mimirOutput:   "",
			errorExpected: errors.New("one rule per rule group expected"),
		},
		"Malformed yaml": {
			input:         rules.RuleGroup{},
			statusCode:    200,
			mimirOutput:   "invalid_yaml: : : :",
			errorExpected: errors.New("failed to unmarshal received data"),
		},
		"Valid output": {
			input: rules.RuleGroup{
				Name:     "01e74407-0327-4e36-93cb-85801c098ba5",
				Interval: "15s",
				Rules: []rules.Rule{
					{
						Alert: "ClusterRAMUsageExceedsThreshold",
						Expr:  "doesn't matter",
						For:   "30s",
					},
				},
			},
			statusCode:    200,
			mimirOutput:   validMimirOutput,
			errorExpected: nil,
		},
		"Valid mimir response but response different than expected": {
			input: rules.RuleGroup{
				Name:     "01e74407-0327-4e36-93cb-85801c098ba5",
				Interval: "15s",
				Rules: []rules.Rule{
					{
						Alert: "This name does not match",
						Expr:  "doesn't matter",
						For:   "30s",
					},
				},
			},
			statusCode:    200,
			mimirOutput:   validMimirOutput,
			errorExpected: errors.New("rule group present in Mimir does not match the expected one"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				fmt.Fprint(w, test.mimirOutput)
			}))
			defer server.Close()

			mimirConfig := config.MimirConfig{
				Namespace: "test",
				RulerURL:  server.URL,
			}
			mimir := Mimir{&mimirConfig}
			tenantID := "test"

			err := mimir.compareRuleGroup(t.Context(), test.input, tenantID)

			if test.errorExpected != nil {
				require.ErrorContains(t, err, test.errorExpected.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateHTTPRequest(t *testing.T) {
	ctx := t.Context()
	tests := map[string]struct {
		endpoint       string
		method         string
		tenant         string
		body           []byte
		expectedError  error
		expectedURL    string
		expectedTenant string
	}{
		"Valid Request": {
			endpoint:       "http://example.com",
			method:         "GET",
			tenant:         "testTenant",
			body:           []byte("test body"),
			expectedError:  nil,
			expectedURL:    "http://example.com",
			expectedTenant: "testTenant",
		},
		"Invalid URL": {
			endpoint:      "://badurl",
			method:        "GET",
			tenant:        "testTenant",
			body:          []byte("test body"),
			expectedError: errors.New("failed to parse given URL"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			req, err := createHTTPRequest(ctx, test.endpoint, test.method, test.tenant, test.body)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedURL, req.URL.String())
				require.Equal(t, test.expectedTenant, req.Header.Get("X-Scope-OrgID"))
			}
		})
	}
}

func TestSendRequest(t *testing.T) {
	ctx := t.Context()
	tests := map[string]struct {
		address       string
		response      string
		statusCode    int
		expectedError error
		expectedBody  string
	}{
		"successful request": {
			response:      `{"result":"success"}`,
			statusCode:    http.StatusOK,
			expectedError: nil,
			expectedBody:  `{"result":"success"}`,
		},
		"unexpected status code": {
			statusCode:    http.StatusBadRequest,
			expectedError: errors.New("unexpected status code"),
		},
		"bad URL": {
			address:       "://badurl",
			statusCode:    http.StatusBadRequest,
			expectedError: errors.New("error creating http request: failed to parse given URL"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				fmt.Fprint(w, test.response)
			}))
			defer server.Close()

			var body []byte
			var err error
			if test.address != "" {
				body, err = SendRequest(ctx, test.address, http.MethodGet, "testTenant", nil)
			} else {
				body, err = SendRequest(ctx, server.URL, http.MethodGet, "testTenant", nil)
			}

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())
			} else {
				require.NoError(t, err, "Did not expect an error but got one")
				require.Equal(t, test.expectedBody, string(body), "The response body does not match the expected body")
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input          string
		expectedOutput int64
		expectedError  bool
	}{
		{"1s", 1, false},
		{"30s", 30, false},
		{"1m", 60, false},
		{"1h", 3600, false},
		{"1m30s", 90, false},
		{"2h45m", 9900, false},
		{"", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			output, err := ParseDurationToSeconds(tt.input)
			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedOutput, output)
			}
		})
	}
}
