// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package mimir

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/app"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

// DefinitionConfigUpdater facilitates updating Mimir rules.
type DefinitionConfigUpdater interface {
	UpdateDefinitionConfig(ctx context.Context, alertDef *models.DBAlertDefinition) error
}

// Mimir instance is responsible for facilitating communication of alerting monitor with Mimir.
// Implements the DefinitionConfigUpdater interface.
type Mimir struct {
	Config *config.MimirConfig
}

// UpdateDefinitionConfig updates Mimir Ruler rule groups based on the passed alert definition
// and verifes if changes are indeed present.
func (mu *Mimir) UpdateDefinitionConfig(ctx context.Context, alertDef *models.DBAlertDefinition) error {
	ruleGroup, err := ConvertToRuleGroup(alertDef)
	if err != nil {
		return err
	}

	err = mu.postRuleGroup(ctx, *ruleGroup, alertDef.TenantID)
	if err != nil {
		return err
	}

	// verify if post was updated
	err = mu.compareRuleGroup(ctx, *ruleGroup, alertDef.TenantID)
	return err
}

// POST rule group to Mimir.
func (mu *Mimir) postRuleGroup(ctx context.Context, rg rules.RuleGroup, tenant string) error {
	alertYaml, err := yaml.Marshal(rg)
	if err != nil {
		return err
	}

	urlRaw := fmt.Sprintf("%v/prometheus/config/v1/rules/%v", mu.Config.RulerURL, mu.Config.Namespace)

	_, err = SendRequest(ctx, urlRaw, http.MethodPost, tenant, alertYaml)
	return err
}

// This function compares the rule group found in Mimir to the one passed as an argument.
func (mu *Mimir) compareRuleGroup(ctx context.Context, rg rules.RuleGroup, tenant string) error {
	// GET rule group from Mimir
	urlRaw := fmt.Sprintf("%v/prometheus/config/v1/rules/%v/%v", mu.Config.RulerURL, mu.Config.Namespace, rg.Name)
	out, err := SendRequest(ctx, urlRaw, http.MethodGet, tenant, nil)
	if err != nil {
		return fmt.Errorf("error while trying to receive rule group from mimir: %w", err)
	}

	var receivedRuleGroup rules.RuleGroup
	err = yaml.Unmarshal(out, &receivedRuleGroup)
	if err != nil {
		return fmt.Errorf("failed to unmarshal received data: %w", err)
	}

	if len(receivedRuleGroup.Rules) != 1 {
		return fmt.Errorf("one rule per rule group expected, %d found", len(receivedRuleGroup.Rules))
	}

	// 0s causes time duration to fail while parsing - host maintenance alert
	if len(rg.Rules) > 0 && rg.Rules[0].For != "" {
		dur, err := time.ParseDuration(rg.Rules[0].For)
		if err != nil {
			return fmt.Errorf("failed to parse duration %v: %w", rg.Rules[0].For, err)
		}
		rg.Rules[0].For = app.FormatDuration(dur)
	}

	if !reflect.DeepEqual(receivedRuleGroup, rg) {
		return fmt.Errorf("rule group present in Mimir does not match the expected one. Expected: %v, Received: %v", rg, receivedRuleGroup)
	}

	return nil
}

func createHTTPRequest(ctx context.Context, endpoint string, method string, tenant string, body []byte) (*http.Request, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse given URL %q: %w", endpoint, err)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create new http request: %w", err)
	}

	// For backward compatibility, a unique header must be set for edgenode tenant
	if tenant == app.DefaultTenantID {
		tenant = "edgenode-system"
	}

	req.Header.Add("X-Scope-OrgID", tenant)
	return req, nil
}

// SendRequest sends an http request to the specified URL, and injects the `X-Scope-OrgID` header.
func SendRequest(ctx context.Context, urlRaw string, method string, tenant string, requestBody []byte) ([]byte, error) {
	req, err := createHTTPRequest(ctx, urlRaw, method, tenant, requestBody)
	if err != nil {
		return nil, fmt.Errorf("error creating http request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error doing http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("got unexpected status code: %v", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// ParseDurationToSeconds returns the number of seconds from a time formatted string.
func ParseDurationToSeconds(durationStr string) (int64, error) {
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return 0, err
	}
	return int64(duration.Seconds()), nil
}
