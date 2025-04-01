// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseExpression(t *testing.T) {
	tests := map[string]struct {
		expression    string
		templateData  TemplateData
		expected      string
		expectedError error
	}{
		"Correct expression": {
			expression: "edge_host_status{status=\"HOST_STATUS_ERROR\"} == \u007B\u007B.Threshold\u007D\u007D",
			templateData: TemplateData{
				Threshold: "85",
			},
			expected:      `edge_host_status{status="HOST_STATUS_ERROR"} == 85`,
			expectedError: nil,
		},
		"Invalid promql expression": {
			// extra >
			expression:    "edge_host_status{status=\"HOST_STATUS_ERROR\"} =>= \u007B\u007B.Threshold\u007D\u007D",
			expected:      ``,
			expectedError: errors.New("promql parser failed to parse"),
		},
		"Unable to parse template - missing closing brackets": {
			expression:    "edge_host_status{status=\"HOST_STATUS_ERROR\"} == \u007B\u007B.Threshold",
			expected:      ``,
			expectedError: errors.New("failed to parse template"),
		},
		"Unable to apply template - field doesn't exist in template data": {
			expression:    "{{ .Invalid }}",
			expected:      ``,
			expectedError: errors.New("failed to apply template"),
		},
		"Real given expression - { instead of unicode": {
			expression: "(rate(net_bytes_sent{}[30s]) + rate(net_bytes_recv{}[30s])) / 1000000 >= {{.Threshold}}",
			templateData: TemplateData{
				Threshold: "100",
			},
			expected:      `(rate(net_bytes_sent{}[30s]) + rate(net_bytes_recv{}[30s])) / 1000000 >= 100`,
			expectedError: nil,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := ParseExpression(test.templateData, test.expression)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, result, "Expression parsed incorrectly")
			}
		})
	}
}
func TestConstructTemplate(t *testing.T) {
	tests := map[string]struct {
		rule           Rule
		expectedOutput string
		wantErr        bool
	}{
		"Basic rule": {
			rule: Rule{
				Alert: "HighCPUUsage",
				Expr:  "cpu_usage > {{ .Threshold }}",
				For:   "1m",
				Annotations: map[string]string{
					"summary":     "High CPU usage detected",
					"description": "CPU usage has exceeded 80%",
				},
				Labels: map[string]string{
					"alert_category": "performance",
					"alert_context":  "host",
					"threshold":      "80",
					"duration":       "1m",
					"host_uuid":      "{{$labels.hostGuid}}",
				},
			},
			expectedOutput: `alert: HighCPUUsage
annotations:
  description: CPU usage has exceeded 80%
  summary: High CPU usage detected
expr: cpu_usage > {{ .Threshold }}
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "80"
`,
			wantErr: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := test.rule.ConstructTemplate()
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedOutput, output)
			}
		})
	}
}

func int64Ptr(i int64) *int64 {
	return &i
}

func TestUpdateTemplateWithValues(t *testing.T) {
	tests := map[string]struct {
		ruleString    string
		threshold     *int64
		duration      *int64
		expectedOut   string
		expectedError error
	}{
		"Given Rule string is a bad yaml": {
			ruleString:    "- - - bad yaml",
			threshold:     int64Ptr(5),
			duration:      int64Ptr(10),
			expectedError: errors.New("failed to unmarshal template"),
		},
		"Successfully substituted threshold": {
			ruleString: `expr: ""
labels:
  duration: 10s
  threshold: "20"`,
			threshold: int64Ptr(5),
			duration:  int64Ptr(10),
			expectedOut: `expr: ""
labels:
  duration: 10s
  threshold: "5"
`,
		},
		"Successfully substituted duration": {
			ruleString: `expr: ""
labels:
  duration: 10s
  threshold: "20"`,
			threshold: int64Ptr(20),
			duration:  int64Ptr(20),
			expectedOut: `expr: ""
labels:
  duration: 20s
  threshold: "20"
`,
		},
		"Successfully substituted duration with a unit change": {
			ruleString: `expr: ""
labels:
  duration: 10s
  threshold: "20"`,
			threshold: int64Ptr(20),
			duration:  int64Ptr(120),
			expectedOut: `expr: ""
labels:
  duration: 2m0s
  threshold: "20"
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := UpdateTemplateWithValues(test.ruleString, test.duration, test.threshold)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedOut, out)
			}
		})
	}
}
