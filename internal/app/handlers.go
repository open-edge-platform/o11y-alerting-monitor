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
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	db "github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

type ServerInterfaceHandler struct {
	receivers   db.ReceiverHandlerManager
	definitions db.AlertDefinitionHandlerManager
	m2m         M2MConnection

	configuration config.Config
}

const (
	errHTTPFailedToGetAlerts                  = "failed to get alerts"
	errHTTPFailedToGetAlertDefinitions        = "failed to get alert definitions"
	errHTTPAlertDefinitionNotFound            = "alert definition not found"
	errHTTPFailedToGetAlertDefinition         = "failed to get alert definition"
	errHTTPBadRequest                         = "bad request"
	errHTTPFailedToPatchAlertDefinition       = "failed to patch alert definition"
	errHTTPAlertDefinitionTemplateNotFound    = "alert definition template not found"
	errHTTPFailedToGetAlertDefinitionTemplate = "failed to get alert definition template"
	errHTTPFailedToGetAlertReceivers          = "failed to get alert receivers"
	errHTTPFailedToGetAlertReceiver           = "failed to get alert receiver"
	errHTTPAlertReceiverNotFound              = "alert receiver not found"
	errHTTPFailedToPatchAlertReceivers        = "failed to patch alert receivers"
	errHTTPFailedToExtractProjectID           = "failed to extract projectID"
)

func NewServerInterfaceHandler(configuration config.Config, dbConn *gorm.DB, m2m M2MConnection) *ServerInterfaceHandler {
	return &ServerInterfaceHandler{
		configuration: configuration,
		receivers: &db.DBService{
			DB: dbConn,
		},
		definitions: &db.DBService{
			DB: dbConn,
		},
		m2m: m2m,
	}
}

func (w *ServerInterfaceHandler) GetAlerts(ctx echo.Context, tenantID api.TenantID, params api.GetProjectAlertsParams) error {
	unmarshalledResponse := new(api.AlertList)
	conf := w.configuration
	urlRaw := conf.AlertManager.URL
	outparams := getAlertsParamsToURL(params)

	// Filtering by tenant
	outparams.Add("filter", "projectId="+tenantID)

	// Sending GET request to alertmanager
	encodedParams := outparams.Encode()
	if encodedParams == "" {
		urlRaw = fmt.Sprintf("%v/api/v2/alerts", urlRaw)
	} else {
		urlRaw = fmt.Sprintf("%v/api/v2/alerts?%v", urlRaw, encodedParams)
	}

	u, err := url.Parse(urlRaw)
	if err != nil {
		logError(ctx, "Error parsing alertmanager URL", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlerts,
		})
	}

	resp, err := http.Get(u.String())
	if err != nil {
		logError(ctx, "Failed to reach alertmanager", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlerts,
		})
	}

	defer resp.Body.Close()

	// Check if GET request have http code 200
	if resp.StatusCode != http.StatusOK {
		logWarn(ctx, fmt.Sprintf("Alertmanager returned HTTP status code: %v", resp.StatusCode))
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlerts,
		})
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logError(ctx, "Failed to read response body", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlerts,
		})
	}

	err = json.Unmarshal(body, &unmarshalledResponse.Alerts)
	if err != nil {
		logError(ctx, "Error unmarshalling response body", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlerts,
		})
	}

	err = filterAnnotations(unmarshalledResponse.Alerts)
	if err != nil {
		logError(ctx, "Error filtering annotations", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlerts,
		})
	}

	filterOutMaintenanceAlerts(unmarshalledResponse.Alerts)

	// Response formatted as AlertList structure
	return ctx.JSONPretty(http.StatusOK, unmarshalledResponse, "\t")
}

func (w *ServerInterfaceHandler) GetAlertDefinitions(ctx echo.Context, tenantID api.TenantID) error {

	logger.LogAttrs(ctx.Request().Context(), slog.LevelDebug, "GetAlertDefinitions handler started")
	dbDefinitions, err := w.definitions.GetLatestAlertDefinitionList(ctx.Request().Context(), tenantID)
	if err != nil {
		logError(ctx, errHTTPFailedToGetAlertDefinitions, err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertDefinitions,
		})
	}

	definitions := make([]api.AlertDefinition, 0, len(dbDefinitions))
	for _, d := range dbDefinitions {
		if d.Category == models.CategoryMaintenance {
			continue
		}
		uuid := d.ID
		name := d.Name
		state := api.StateDefinition(d.State)
		values := map[string]string{
			"duration":  FormatDuration(time.Duration(*d.Values.Duration) * time.Second),
			"threshold": strconv.FormatInt(*d.Values.Threshold, 10),
			"enabled":   strconv.FormatBool(*d.Values.Enabled),
		}
		version := int(d.Version)
		definitions = append(definitions, api.AlertDefinition{
			Id:      &uuid,
			Name:    &name,
			State:   &state,
			Values:  &values,
			Version: &version,
		})
	}

	return ctx.JSON(http.StatusOK, api.AlertDefinitionList{
		AlertDefinitions: &definitions,
	})
}

func (w *ServerInterfaceHandler) GetAlertDefinition(ctx echo.Context, tenantID api.TenantID, id api.AlertDefinitionId) error {
	ad, err := w.definitions.GetLatestAlertDefinition(ctx.Request().Context(), tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logError(ctx, fmt.Sprintf("Alert definition not found: %q", id), err)
		return ctx.JSON(http.StatusNotFound, api.HttpError{
			Code:    http.StatusNotFound,
			Message: errHTTPAlertDefinitionNotFound,
		})
	} else if err != nil {
		logError(ctx, fmt.Sprintf("Failed to retrieve alert definition: %q", id), err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertDefinition,
		})
	}

	state := api.StateDefinition(ad.State)
	values := map[string]string{
		"threshold": strconv.FormatInt(*ad.Values.Threshold, 10),
		"duration":  FormatDuration(time.Duration(*ad.Values.Duration) * time.Second),
		"enabled":   strconv.FormatBool(*ad.Values.Enabled),
	}
	version := int(ad.Version)
	return ctx.JSON(http.StatusOK, api.AlertDefinition{
		Id:      &ad.ID,
		Name:    &ad.Name,
		State:   &state,
		Values:  &values,
		Version: &version,
	})
}

func (w *ServerInterfaceHandler) PatchAlertDefinition(ctx echo.Context, tenantID api.TenantID, id api.AlertDefinitionId) error {
	var reqBody api.PatchProjectAlertDefinitionJSONBody

	dec := json.NewDecoder(ctx.Request().Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&reqBody); err != nil {
		logError(ctx, "Failed to parse body of alert definition", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPBadRequest,
		})
	}

	values, err := parseAlertDefinitionValues(reqBody)
	if err != nil {
		logError(ctx, "Failed to parse alert definition values", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToPatchAlertDefinition,
		})
	}

	if err := w.definitions.SetAlertDefinitionValues(ctx.Request().Context(), tenantID, id, *values); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			logError(ctx, fmt.Sprintf("Alert definition not found: %q", id), err)
			return ctx.JSON(http.StatusNotFound, api.HttpError{
				Code:    http.StatusNotFound,
				Message: errHTTPAlertDefinitionNotFound,
			})
		case errors.Is(err, db.ErrValueOutOfBounds):
			logError(ctx, fmt.Sprintf("Alert definition value/s are out-of-bounds: %q", id), err)
			return ctx.JSON(http.StatusBadRequest, api.HttpError{
				Code:    http.StatusBadRequest,
				Message: "alert definition value/s out-of-bounds",
			})
		default:
			logError(ctx, fmt.Sprintf("Failed to set alert definition values: %q", id), err)
			return ctx.JSON(http.StatusInternalServerError, api.HttpError{
				Code:    http.StatusInternalServerError,
				Message: errHTTPFailedToPatchAlertDefinition,
			})
		}
	}

	return ctx.NoContent(http.StatusNoContent)
}

func (w *ServerInterfaceHandler) GetAlertDefinitionRule(ctx echo.Context, tenantID api.TenantID, id api.AlertDefinitionId,
	params api.GetProjectAlertDefinitionRuleParams) error {
	ad, err := w.definitions.GetLatestAlertDefinition(ctx.Request().Context(), tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logError(ctx, fmt.Sprintf("Alert definition not found: %q", id), err)
		return ctx.JSON(http.StatusNotFound, api.HttpError{
			Code:    http.StatusNotFound,
			Message: errHTTPAlertDefinitionTemplateNotFound,
		})
	} else if err != nil {
		logError(ctx, fmt.Sprintf("Failed to retrieve alert definition template: %q", id), err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertDefinitionTemplate,
		})
	}

	// TODO: Instead of relying on having values in Labels and Annotations return an API object that lists
	// these fields and tells us what we actually expect to have.
	// This will require changes on webUI side to map to these changes.
	var apiResponse api.AlertDefinitionTemplate

	// Don't render the expression.
	if params.Rendered != nil && !*params.Rendered {
		//nolint:musttag // api.AlertDefinitionTemplate contains autogenerated code
		if err := yaml.Unmarshal([]byte(ad.Template), &apiResponse); err != nil {
			logError(ctx, fmt.Sprintf("Failed to unmarshal template into template api response struct: %q", id), err)
			return ctx.JSON(http.StatusInternalServerError, api.HttpError{
				Code:    http.StatusInternalServerError,
				Message: errHTTPFailedToGetAlertDefinitionTemplate,
			})
		}
		return ctx.JSON(http.StatusOK, apiResponse)
	}

	apiResponse, err = renderTemplate(ad.Values, ad.Template)
	if err != nil {
		logError(ctx, fmt.Sprintf("Failed to render alert definition template: %q", id), err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertDefinitionTemplate,
		})
	}

	return ctx.JSON(http.StatusOK, apiResponse)
}

func (w *ServerInterfaceHandler) GetAlertReceivers(ctx echo.Context, tenantID api.TenantID) error {
	dbRecvs, err := w.receivers.GetLatestReceiverListWithEmailConfig(ctx.Request().Context(), tenantID)
	if err != nil {
		logError(ctx, "Failed to get alert receivers", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertReceivers,
		})
	}

	allowedEmailRecipients, err := getAllowedEmailList(ctx, w.m2m)
	if err != nil {
		logError(ctx, "Failed to get allowed email recipient list", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertReceivers,
		})
	}

	receivers := make([]api.Receiver, len(dbRecvs))
	for i, recv := range dbRecvs {
		uuid := recv.UUID
		state := api.StateDefinition(recv.State)
		version := recv.Version
		mailServer := recv.MailServer
		from := recv.From
		to := recv.To
		receivers[i] = api.Receiver{
			Id:      &uuid,
			State:   &state,
			Version: &version,
			EmailConfig: &api.EmailConfig{
				From:       &from,
				MailServer: &mailServer,
				To: &struct {
					Allowed *api.EmailRecipientList `json:"allowed,omitempty"`
					Enabled *api.EmailRecipientList `json:"enabled,omitempty"`
				}{
					Allowed: &allowedEmailRecipients,
					Enabled: &to,
				},
			},
		}
	}

	return ctx.JSON(http.StatusOK, api.ReceiverList{Receivers: &receivers})
}

func (w *ServerInterfaceHandler) GetAlertReceiver(ctx echo.Context, tenantID api.TenantID, id api.ReceiverId) error {
	recv, err := w.receivers.GetLatestReceiverWithEmailConfig(ctx.Request().Context(), tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logError(ctx, fmt.Sprintf("Alert receiver not found: %q", id), err)
		return ctx.JSON(http.StatusNotFound, api.HttpError{
			Code:    http.StatusNotFound,
			Message: errHTTPAlertReceiverNotFound,
		})
	} else if err != nil {
		logError(ctx, fmt.Sprintf("Failed to get alert receiver with UUID: %q", id), err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertReceiver,
		})
	}

	allowedEmailRecipients, err := getAllowedEmailList(ctx, w.m2m)
	if err != nil {
		logError(ctx, "Failed to get allowed email recipient list", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToGetAlertReceiver,
		})
	}

	state := api.StateDefinition(recv.State)
	return ctx.JSON(http.StatusOK, api.Receiver{
		Id:      &recv.UUID,
		Version: &recv.Version,
		State:   &state,
		EmailConfig: &api.EmailConfig{
			MailServer: &recv.MailServer,
			From:       &recv.From,
			To: &struct {
				Allowed *api.EmailRecipientList `json:"allowed,omitempty"`
				Enabled *api.EmailRecipientList `json:"enabled,omitempty"`
			}{
				Allowed: &allowedEmailRecipients,
				Enabled: &recv.To,
			},
		},
	})
}

func (w *ServerInterfaceHandler) PatchAlertReceiver(ctx echo.Context, tenantID api.TenantID, id api.ReceiverId) error {
	var reqBody api.PatchProjectAlertReceiverJSONBody
	dec := json.NewDecoder(ctx.Request().Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&reqBody); err != nil {
		logError(ctx, "Failed to parse body of alert receiver", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPBadRequest,
		})
	}

	allowed, err := getAllowedEmailList(ctx, w.m2m)
	if err != nil {
		logError(ctx, "Failed to get allowed email recipients", err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToPatchAlertReceivers,
		})
	}

	// Ensures email recipients are allowed.
	if err := validateRecipients(reqBody.EmailConfig.To.Enabled, allowed); err != nil {
		logError(ctx, "Email recipient list contains not allowed email recipient/s", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPBadRequest,
		})
	}

	emailRecipients, err := parseEmailRecipients(reqBody.EmailConfig.To.Enabled)
	if err != nil {
		logError(ctx, "Failed to parse email recipients", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPBadRequest,
		})
	}

	err = w.receivers.SetReceiverEmailRecipients(ctx.Request().Context(), tenantID, id, emailRecipients)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logError(ctx, fmt.Sprintf("Alert receiver not found: %q", id), err)
		return ctx.JSON(http.StatusNotFound, api.HttpError{
			Code:    http.StatusNotFound,
			Message: errHTTPAlertReceiverNotFound,
		})
	} else if err != nil {
		logError(ctx, fmt.Sprintf("Failed to update email recipients for receiver with UUID: %q", id), err)
		return ctx.JSON(http.StatusInternalServerError, api.HttpError{
			Code:    http.StatusInternalServerError,
			Message: errHTTPFailedToPatchAlertReceivers,
		})
	}

	return ctx.NoContent(http.StatusNoContent)
}

// GetStatus does not depend on tenantID thus here is a blank identifier.
func (w *ServerInterfaceHandler) GetStatus(ctx echo.Context, _ api.TenantID) error {
	conf := w.configuration

	alertManagerStatus, err := getAlertManagerStatus(conf.AlertManager.URL)
	if err != nil {
		logError(ctx, "Failed to get alert manager status", err)
		return ctx.JSON(http.StatusOK, &api.ServiceStatus{
			State: api.Failed,
		})
	}

	if alertManagerStatus != "ready" {
		logWarn(ctx, "Alert manager not ready")
		return ctx.JSON(http.StatusOK, &api.ServiceStatus{
			State: api.Failed,
		})
	}

	mimirRulerStatusOK, err := isMimirRulerReachable(conf.Mimir.RulerURL)
	if err != nil {
		logError(ctx, "Failed to reach Mimir ruler", err)
		return ctx.JSON(http.StatusOK, &api.ServiceStatus{
			State: api.Failed,
		})
	}

	if !mimirRulerStatusOK {
		logWarn(ctx, "Mimir response invalid status code")
		return ctx.JSON(http.StatusOK, &api.ServiceStatus{
			State: api.Failed,
		})
	}

	return ctx.JSON(http.StatusOK, &api.ServiceStatus{
		State: api.Ready,
	})
}

func (w *ServerInterfaceHandler) GetProjectAlerts(ctx echo.Context, params api.GetProjectAlertsParams) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.GetAlerts(ctx, projectID, params)
}

func (w *ServerInterfaceHandler) GetProjectAlertDefinitions(ctx echo.Context) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.GetAlertDefinitions(ctx, projectID)
}

func (w *ServerInterfaceHandler) GetProjectAlertDefinition(ctx echo.Context, alertDefinitionID api.AlertDefinitionId) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.GetAlertDefinition(ctx, projectID, alertDefinitionID)
}

func (w *ServerInterfaceHandler) PatchProjectAlertDefinition(ctx echo.Context, alertDefinitionID api.AlertDefinitionId) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.PatchAlertDefinition(ctx, projectID, alertDefinitionID)
}

func (w *ServerInterfaceHandler) GetProjectAlertDefinitionRule(
	ctx echo.Context, alertDefinitionID api.AlertDefinitionId, params api.GetProjectAlertDefinitionRuleParams,
) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.GetAlertDefinitionRule(ctx, projectID, alertDefinitionID, params)
}

func (w *ServerInterfaceHandler) GetProjectAlertReceivers(ctx echo.Context) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.GetAlertReceivers(ctx, projectID)
}

func (w *ServerInterfaceHandler) GetProjectAlertReceiver(ctx echo.Context, receiverID api.ReceiverId) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.GetAlertReceiver(ctx, projectID, receiverID)
}

func (w *ServerInterfaceHandler) PatchProjectAlertReceiver(ctx echo.Context, receiverID api.ReceiverId) error {
	projectID, err := extractProjectID(ctx)
	if err != nil {
		logError(ctx, "Failed to extract projectID", err)
		return ctx.JSON(http.StatusBadRequest, api.HttpError{
			Code:    http.StatusBadRequest,
			Message: errHTTPFailedToExtractProjectID,
		})
	}

	return w.PatchAlertReceiver(ctx, projectID, receiverID)
}

func (w *ServerInterfaceHandler) GetServiceStatus(ctx echo.Context) error {
	// projectID will be ignored (status doesn't depend on projectID/tenantID)
	return w.GetStatus(ctx, DefaultTenantID)
}

func extractProjectID(ctx echo.Context) (string, error) {
	projectID := ctx.Request().Header.Get("ActiveProjectID")

	if len(strings.TrimSpace(projectID)) == 0 {
		return "", errors.New("projectID cannot be empty")
	}

	return projectID, nil
}
