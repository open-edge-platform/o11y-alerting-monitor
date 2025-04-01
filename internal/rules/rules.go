// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v2"
)

// RuleGroup represents the rule group structure in a way it is present in Mimir.
type RuleGroup struct {
	Name          string   `yaml:"name"`
	Interval      string   `yaml:"interval,omitempty"`
	SourceTenants []string `yaml:"source_tenants,omitempty"`
	// We only ever expect one rule (and one alert) in the RuleGroup, which we will insert
	// However Prometheus does allow for more
	Rules []Rule `yaml:"rules"`
}

// Rule represents the rule structure in a way it is present in Mimir.
type Rule struct {
	Alert       string            `yaml:"alert,omitempty" json:"alert,omitempty"`
	Expr        string            `yaml:"expr" json:"expr"`
	For         string            `yaml:"for,omitempty" json:"for,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// ParseExpression parses the Rule expression template duration and threshold.
func (rule *Rule) ParseExpression(enabled *bool) error {
	data := TemplateData{
		Threshold: rule.Labels["threshold"],
		Duration:  rule.Labels["duration"],
	}

	expr := rule.Expr
	tpl, err := ParseExpression(data, expr)
	if err != nil {
		return fmt.Errorf("failed to parse expression %q: %w", expr, err)
	}

	if enabled != nil && !*enabled {
		tpl = tpl + " and false"
	}

	rule.Expr = tpl
	return nil
}

// ConstructTemplate returns the string representation of the Rule template.
func (rule *Rule) ConstructTemplate() (string, error) {
	tmpl := map[string]interface{}{
		"alert":       rule.Alert,
		"expr":        rule.Expr,
		"for":         rule.For,
		"labels":      rule.Labels,
		"annotations": rule.Annotations,
	}

	out, err := yaml.Marshal(&tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to marshal template: %w", err)
	}
	return string(out), nil
}

// RulesConfig represents deserialized config file.
type RulesConfig struct {
	Namespace string      `yaml:"namespace"`
	Groups    []RuleGroup `yaml:"groups"`
}

// LoadRulesConfig loads namespace and rule groups from the config file specified by its path.
func LoadRulesConfig(filePath string) (*RulesConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var conf RulesConfig
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return nil, err
	}

	return &conf, nil
}

// TemplateData holds threshold and duration required for parsing the rule expression.
type TemplateData struct {
	Threshold string
	Duration  string
}

// ParseExpression parses duration and threshold taken from `TemplateData` into the expression template.
func ParseExpression(data TemplateData, expr string) (string, error) {
	// Replace characters to have template
	expr = strings.ReplaceAll(expr, "[[", "{{")
	expr = strings.ReplaceAll(expr, "]]", "}}")

	tmpl, err := template.New("Expr").Parse(expr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, data); err != nil {
		return "", fmt.Errorf("failed to apply template: %w", err)
	}

	_, err = parser.ParseExpr(tpl.String())
	if err != nil {
		return "", fmt.Errorf("promql parser failed to parse: %w", err)
	}

	return tpl.String(), nil
}

// UpdateTemplateWithValues updates the Template part of Alert Definition,
// with new duration or threshold, if given.
func UpdateTemplateWithValues(rule string, duration, threshold *int64) (string, error) {
	var tmpl Rule
	err := yaml.Unmarshal([]byte(rule), &tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal template: %w", err)
	}

	if duration != nil {
		tmpl.Labels["duration"] = time.Duration(*duration * int64(time.Second)).String()
	}
	if threshold != nil {
		tmpl.Labels["threshold"] = strconv.Itoa(int(*threshold))
	}

	out, err := yaml.Marshal(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to marshal template: %w", err)
	}

	return string(out), nil
}
