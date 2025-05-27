// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"gopkg.in/yaml.v2"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

const (
	DefaultTenantID = "edgenode"
	statusEndpoint  = "/api/v1/status"
)

// Regex used to check and parse the fields of an email address.
var EmailRegex = regexp.MustCompile(`^(.*?)\s*(\S+)\s+<(.*)>`)

// Fallback regex to facilitate upgrade procedure with old email address format.
var SimpleEmailRegex = regexp.MustCompile(`(?:<)?([^<>\s@]+@[^<>\s@]+\.[^<>\s@]+)(?:>)?`)

// Convert parameters form request to alert manager format.
func getAlertsParamsToURL(params api.GetProjectAlertsParams) url.Values {
	outparams := make(url.Values)
	if params.Alert != nil {
		log.Debugf("Params.Alert: %v", *params.Alert)
		outparams.Add("filter", "alertname="+*params.Alert)
	}

	if params.Host != nil {
		log.Debugf("Params.Host: %v", *params.Host)
		outparams.Add("filter", "host_uuid="+*params.Host)
	}

	if params.Cluster != nil {
		log.Debugf("Params.Cluster: %v", *params.Cluster)
		outparams.Add("filter", "cluster_name="+*params.Cluster)
	}

	if params.App != nil {
		log.Debugf("Params.App: %v", *params.App)
		outparams.Add("filter", "deployment_id="+*params.App)
	}

	if params.Active != nil {
		log.Debugf("Params.Active: %v", *params.Active)
		outparams.Add("active", strconv.FormatBool(*params.Active))
	}

	if params.Suppressed != nil {
		log.Debugf("Params.Suppressed: %v", *params.Suppressed)
		outparams.Add("inhibited", strconv.FormatBool(*params.Suppressed))
		outparams.Add("silenced", strconv.FormatBool(*params.Suppressed))
	}

	return outparams
}

// Helper to delete every unneeded annotations from Alert Manager response.
func filterAnnotations(alerts *[]api.Alert) error {
	// Iterate through alerts.
	for i := range *alerts {
		// Iterate through annotations in alert.
		for k, v := range *(*alerts)[i].Annotations {
			// Check if map key has am_ prefix.
			if strings.HasPrefix(k, "am_") {
				// Check if key is am_uuid and copy it to AlertDefinitionId field.
				if k == "am_uuid" {
					parsedUUID, err := uuid.Parse(v)
					if err != nil {
						return err
					}
					(*alerts)[i].AlertDefinitionId = &parsedUUID
				}
				// Delete unnecessary annotation.
				delete(*(*alerts)[i].Annotations, k)
			}
		}
	}
	return nil
}

// Helper to remove maintenance alerts.
func filterOutMaintenanceAlerts(alerts *[]api.Alert) {
	*alerts = slices.DeleteFunc(*alerts, func(alert api.Alert) bool {
		alertCategory, ok := (*alert.Labels)["alert_category"]
		if ok && alertCategory != "maintenance" {
			return false // don't remove alert with "alert_category" different from "maintenance"
		}
		return true // remove alert with "alert_category" equal to "maintenance" or without "alert_category"
	})
}

type alertManagerStatus struct {
	Status string `json:"status"`
}

type alertManagerInfo struct {
	Cluster alertManagerStatus `json:"cluster"`
}

func getAlertManagerStatus(serverURL string) (string, error) {
	u, err := url.Parse(fmt.Sprintf("%s%s", serverURL, "/api/v2/status"))
	if err != nil {
		return "", fmt.Errorf("failed to parse alert manager url: %w", err)
	}

	// Send request to alert manager: GET /api/v2/status
	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check if response code 200
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("alert manager returned status code: %v", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var info alertManagerInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return info.Cluster.Status, nil
}

func isMimirRulerReachable(serverURL string) (bool, error) {
	u, err := url.Parse(fmt.Sprintf("%s%s", serverURL, "/ready"))
	if err != nil {
		return false, fmt.Errorf("failed to parse mimir ruler url: %w", err)
	}

	resp, err := http.Get(u.String())
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check if response code 200
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("mimir returned status code: %v", resp.StatusCode)
	}

	return true, nil
}

func skipAuth(c echo.Context) bool {
	if c.Request().URL.Path == statusEndpoint && c.Request().Method == http.MethodGet {
		return true
	}
	return false
}

func skipLog(c echo.Context) bool {
	userAgent := c.Request().Header.Get("User-Agent")
	path := c.Request().URL.Path
	method := c.Request().Method

	if (strings.HasPrefix(userAgent, "curl") || strings.HasPrefix(userAgent, "kube-probe")) &&
		path == statusEndpoint &&
		method == http.MethodGet {
		return true
	}
	return false
}

func getAllowedEmailList(ctx echo.Context, m2m M2MConnection) (api.EmailRecipientList, error) {
	userList, err := m2m.GetUserList(ctx)
	if err != nil {
		return nil, err
	}
	allowedEmailList := convertEmailFormat(userList)
	if len(allowedEmailList) == 0 {
		return nil, errors.New("error converting email list/allowed email list empty")
	}
	return allowedEmailList, nil
}

func convertEmailFormat(userList []user) api.EmailRecipientList {
	var emailRecipientList api.EmailRecipientList
	for i := range userList {
		firstName, lastName, email := userList[i].FirstName, userList[i].LastName, userList[i].Email
		if firstName != "" && lastName != "" && email != "" {
			formattedEmail := fmt.Sprintf("%s %s <%s>", firstName, lastName, email)
			emailRecipientList = append(emailRecipientList, formattedEmail)
		}
	}
	return emailRecipientList
}

func validateRecipients(recipients, allowed api.EmailRecipientList) error {
	for _, recipient := range recipients {
		if !slices.Contains(allowed, recipient) {
			return fmt.Errorf("email recipient is not allowed: %q", recipient)
		}
	}
	return nil
}

func parseAlertDefinitionValues(req api.PatchProjectAlertDefinitionJSONBody) (*models.DBAlertDefinitionValues, error) {
	if req.Values == nil {
		return nil, errors.New("request values is nil")
	}

	if req.Values.Duration == nil && req.Values.Threshold == nil && req.Values.Enabled == nil {
		return nil, errors.New("request should contain at least one value to be set")
	}

	var values models.DBAlertDefinitionValues

	if req.Values.Duration != nil {
		durationStr := *req.Values.Duration
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse duration value: %w", err)
		}
		durationSecs := int64(duration.Seconds())
		if durationSecs == 0 {
			return nil, fmt.Errorf("duration should be a non zero value in the order of seconds: %q", durationStr)
		}
		values.Duration = &durationSecs
	}

	if req.Values.Threshold != nil {
		threshold, err := strconv.ParseInt(*req.Values.Threshold, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse threshold value: %w", err)
		}
		values.Threshold = &threshold
	}

	if req.Values.Enabled != nil {
		enabled, err := strconv.ParseBool(*req.Values.Enabled)
		if err != nil {
			return nil, fmt.Errorf("failed to parse enabled value: %w", err)
		}
		values.Enabled = &enabled
	}

	return &values, nil
}

func parseEmailRecipients(recipientList []string) ([]models.EmailAddress, error) {
	res := make([]models.EmailAddress, 0, len(recipientList))
	emailMap := make(map[string]struct{})

	for _, r := range recipientList {
		matches := EmailRegex.FindStringSubmatch(r)
		if len(matches) != 4 {
			return nil, fmt.Errorf("invalid format for email recipient: %q", r)
		}

		email := matches[3]
		if _, duplicate := emailMap[email]; duplicate {
			return nil, fmt.Errorf("duplicate email recipient: %q", email)
		}
		emailMap[email] = struct{}{}

		res = append(res, models.EmailAddress{
			FirstName: matches[1],
			LastName:  matches[2],
			Email:     email,
		})
	}

	return res, nil
}

func logError(ctx echo.Context, msg string, err error) {
	ctx.Logger().Errorf("(%s): %s: %v test", ctx.Path(), msg, err)
}
func logWarn(ctx echo.Context, msg string) {
	ctx.Logger().Warnf("(%s): %s test", ctx.Path(), msg)
}

// func logError(ctx echo.Context, msg string, err error) {
//     logger.LogAttrs(ctx.Request().Context(), slog.LevelError, "ERROR",
//         slog.String("uri", ctx.Path()),
//         slog.String("message", msg),
//         slog.String("error", err.Error()),
//     )
// }

// func logWarn(ctx echo.Context, msg string) {
//     logger.LogAttrs(ctx.Request().Context(), slog.LevelWarn, "WARN",
//         slog.String("uri", ctx.Path()),
//         slog.String("message", msg),
//     )
// }

func renderTemplate(values models.DBAlertDefinitionValues, template string) (api.AlertDefinitionTemplate, error) {
	if values.Threshold == nil || values.Duration == nil {
		return api.AlertDefinitionTemplate{}, fmt.Errorf("threshold or duration are nil: %v", values)
	}
	data := rules.TemplateData{
		Threshold: strconv.Itoa(int(*values.Threshold)),
		Duration:  FormatDuration(time.Duration(*values.Duration) * time.Second),
	}
	
	var tmpl api.AlertDefinitionTemplate
	err := yaml.Unmarshal([]byte(template), &tmpl)
	if err != nil {
		return api.AlertDefinitionTemplate{}, fmt.Errorf("failed to unmarshal template into struct: %w", err)
	}
	
	expr, err := rules.ParseExpression(data, *tmpl.Expr)
	if err != nil {
		return api.AlertDefinitionTemplate{}, fmt.Errorf("failed to parse the expression %q: %w", *tmpl.Expr, err)
	}
	tmpl.Expr = &expr
	
	return tmpl, nil
}

func FormatDuration(dur time.Duration) string {
	hours := dur / time.Hour
	minutes := (dur % time.Hour) / time.Minute
	seconds := (dur % time.Minute) / time.Second
	
	var builder strings.Builder
	
	// Add hours if non-zero
	if hours > 0 {
		builder.WriteString(fmt.Sprintf("%dh", hours))
	}
	// Add minutes if non-zero
	if minutes > 0 {
		builder.WriteString(fmt.Sprintf("%dm", minutes))
	}
	// Add seconds if non-zero or if the result is empty (meaning the duration is less than a minute)
	if seconds > 0 || builder.Len() == 0 {
		builder.WriteString(fmt.Sprintf("%ds", seconds))
	}

	return builder.String()
}

// TODO: Instead of relying on fallback and multiple regexes this should fully comply to RFC-5322,
// which requires more significant changes and the schema needs to allow empty names.
func GetEmailSender(from string) (firstName, lastName, email string, err error) {
	matches := EmailRegex.FindStringSubmatch(from)

	// New format
	if len(matches) == 4 {
		return matches[1], matches[2], matches[3], nil
	}

	// Handle fallback to simple email format without names
	if matches = SimpleEmailRegex.FindStringSubmatch(from); len(matches) == 2 && matches[0] == from {
		return "Open Edge Platform", "Alert", matches[1], nil
	}

	return "", "", "", fmt.Errorf("invalid format for email 'from' value: %q", from)
}
