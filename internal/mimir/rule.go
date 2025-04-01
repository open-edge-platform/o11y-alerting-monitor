// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package mimir

import (
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

// ConvertToRuleGroup takes DBAlertDefinition and converts it to a RuleGroup.
func ConvertToRuleGroup(d *models.DBAlertDefinition) (*rules.RuleGroup, error) {
	var defTemplate rules.Rule
	err := yaml.Unmarshal([]byte(d.Template), &defTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal into the template: %w", err)
	}
	defTemplate.Labels["threshold"] = strconv.Itoa(int(*d.Values.Threshold))
	defTemplate.Labels["duration"] = time.Duration(*d.Values.Duration * int64(time.Second)).String()

	err = defTemplate.ParseExpression(d.Values.Enabled)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expression: %w", err)
	}

	ruleGroup := rules.RuleGroup{
		Name:     d.ID.String(),
		Interval: time.Duration(d.Interval * int64(time.Second)).String(),
		Rules:    []rules.Rule{defTemplate},
	}

	return &ruleGroup, nil
}
