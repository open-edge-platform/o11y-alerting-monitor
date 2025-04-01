// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package mimir

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

type Values struct {
	duration  int64
	threshold int64
	enabled   bool
}

var validAlertDefTemplate = `alert: HighCPUUsage
annotations:
  description: CPU usage has exceeded 80%
  summary: High CPU usage detected
expr: cpu_usage > {{ .Threshold }}
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m
  threshold: "80"
`
var invalidAlertDefTemplateUnmarshal = `alert: HighCPUUsage
annotations: "string instead of map"
expr: cpu_usage > {{ .Threshold }}
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m
  threshold: "80"
`
var invalidAlertDefTemplateParse = `alert: HighCPUUsage
annotations:
  description: CPU usage has exceeded 80%
  summary: High CPU usage detected
expr: cpu_usage ==>= {{ .Threshold }}
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m
  threshold: "80"
`

func TestConvertToRuleGroup(t *testing.T) {
	testUUID, err := uuid.NewUUID()
	require.NoError(t, err, "Failed to generate UUID")

	testScenarios := []struct {
		name           string
		alertDef       models.DBAlertDefinition
		values         Values
		expectedOutput *rules.RuleGroup
		expectedError  error
	}{
		{
			name: "Valid alert definition",
			alertDef: models.DBAlertDefinition{
				ID:       testUUID,
				Name:     "HighCPUUsage",
				State:    "SomeState",
				Interval: 15,
				Template: validAlertDefTemplate,
				TenantID: "edgenode",
			},
			values: Values{
				duration:  int64(60),
				threshold: int64(80),
				enabled:   true,
			},
			expectedOutput: &rules.RuleGroup{
				Name:     testUUID.String(),
				Interval: "15s",
				Rules: []rules.Rule{{
					Alert: "HighCPUUsage",
					Expr:  "cpu_usage > 80",
					For:   "1m",
					Annotations: map[string]string{
						"description": "CPU usage has exceeded 80%",
						"summary":     "High CPU usage detected",
					},
					Labels: map[string]string{
						"threshold":      "80",
						"duration":       "1m0s",
						"alert_category": "performance",
						"alert_context":  "host",
					},
				},
				},
			},
			expectedError: nil,
		},
		{
			name: "Invalid alert definition template - failed to unmarshal",
			alertDef: models.DBAlertDefinition{
				ID:       testUUID,
				Name:     "HighCPUUsage",
				State:    "SomeState",
				Interval: 15,
				Template: invalidAlertDefTemplateUnmarshal,
				TenantID: "edgenode",
			},
			values: Values{
				duration:  int64(60),
				threshold: int64(80),
				enabled:   true,
			},
			expectedOutput: nil,
			expectedError:  errors.New("failed to unmarshal into the template"),
		},
		{
			name: "Invalid alert definition template - failed to parse",
			alertDef: models.DBAlertDefinition{
				ID:       testUUID,
				Name:     "HighCPUUsage",
				State:    "SomeState",
				Interval: 15,
				Template: invalidAlertDefTemplateParse,
				TenantID: "edgenode",
			},
			values: Values{
				duration:  int64(60),
				threshold: int64(80),
				enabled:   true,
			},
			expectedOutput: nil,
			expectedError:  errors.New("failed to parse expression"),
		},
	}

	for _, tc := range testScenarios {
		t.Run(tc.name, func(t *testing.T) {
			tcValues := tc
			alertDef := tc.alertDef
			alertDef.Values = models.DBAlertDefinitionValues{
				Duration:  &tcValues.values.duration,
				Threshold: &tcValues.values.threshold,
				Enabled:   &tcValues.values.enabled,
			}
			ruleGroup, err := ConvertToRuleGroup(&alertDef)

			if tc.expectedError != nil {
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedOutput, ruleGroup)
		})
	}
}
