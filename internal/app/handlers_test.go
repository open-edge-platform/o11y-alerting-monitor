// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/oapi-codegen/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

const alertManagerResponse =
// First Alert
"[{\"annotations\":{\"am_test\":\"test\",\"am_uuid\":\"d3867dfb-e172-4fe6-bfdb-05603618a179\"}," +
	"\"endsAt\":\"2024-01-23T16:13:45.535+01:00\",\"fingerprint\":\"0c8d24dab761f647\"," +
	"\"receivers\":[{\"name\":\"web.hook\"}],\"startsAt\":\"2024-01-23T16:08:45.535+01:00\"," +
	"\"status\":{\"inhibitedBy\":[],\"silencedBy\":[],\"state\":\"active\"}," +
	"\"updatedAt\":\"2024-01-23T16:08:45.535+01:00\"," +
	"\"labels\":{\"alertname\":\"foo2\",\"cluster_name\":\"test\",\"alert_category\":\"test\"," +
	"\"host_uuid\":\"93bf6804-52a3-4ba1-a919-c7ef65a9cdef\",\"node\":\"bar\"," +
	"\"deployment_id\":\"1c87a656-594d-4300-b4ad-630914e11856\"}}" +
	// Second alert
	",{\"annotations\":{\"am_test\":\"test\",\"am_test2\":\"test2\",\"am_uuid\":\"c3d257e2-0140-4a8a-bcd3-c5d48ea4d47a\"}," +
	"\"endsAt\":\"2024-01-23T16:13:45.510+01:00\",\"fingerprint\":\"4bfbad375f9020af\"," +
	"\"receivers\":[{\"name\":\"web.hook\"}],\"startsAt\":\"2024-01-23T16:08:45.510+01:00\"," +
	"\"status\":{\"inhibitedBy\":[],\"silencedBy\":[],\"state\":\"active\"}," +
	"\"updatedAt\":\"2024-01-23T16:08:45.510+01:00\"," +
	"\"labels\":{\"alertname\":\"foo\",\"cluster_name\":\"test\",\"alert_category\":\"test\"," +
	"\"host_uuid\":\"93bf6804-52a3-4ba1-a919-c7ef65a9cdef\",\"node\":\"bar\"," +
	"\"deployment_id\":\"1c87a656-594d-4300-b4ad-630914e11856\"}}" +
	// Third alert
	",{\"annotations\":{\"am_test\":\"test\",\"am_test2\":\"test2\",\"am_test3\":\"test3\",\"am_uuid\":\"c6b2a291-a9a2-49d2-930f-f865457b1aa8\"}," +
	"\"endsAt\":\"2024-01-23T16:13:45.560+01:00\",\"fingerprint\":\"bf31b9c198429127\"," +
	"\"receivers\":[{\"name\":\"web.hook\"}],\"startsAt\":\"2024-01-23T16:08:45.560+01:00\"," +
	"\"status\":{\"inhibitedBy\":[],\"silencedBy\":[],\"state\":\"active\"}," +
	"\"updatedAt\":\"2024-01-23T16:08:45.560+01:00\"," +
	"\"labels\":{\"alertname\":\"foo3\",\"cluster_name\":\"test\",\"alert_category\":\"test\"," +
	"\"host_uuid\":\"93bf6804-52a3-4ba1-a919-c7ef65a9cdef\",\"node\":\"bar\"," +
	"\"deployment_id\":\"1c87a656-594d-4300-b4ad-630914e11856\"}}]"

const alertMonitorExpectedResponse =
// First Alert
"[{\"alertDefinitionId\":\"d3867dfb-e172-4fe6-bfdb-05603618a179\"," +
	"\"annotations\":{}," +
	"\"endsAt\":\"2024-01-23T16:13:45.535+01:00\",\"fingerprint\":\"0c8d24dab761f647\"," +
	"\"receivers\":[{\"name\":\"web.hook\"}],\"startsAt\":\"2024-01-23T16:08:45.535+01:00\"," +
	"\"status\":{\"inhibitedBy\":[],\"silencedBy\":[],\"state\":\"active\"}," +
	"\"updatedAt\":\"2024-01-23T16:08:45.535+01:00\"," +
	"\"labels\":{\"alertname\":\"foo2\",\"cluster_name\":\"test\",\"alert_category\":\"test\"," +
	"\"host_uuid\":\"93bf6804-52a3-4ba1-a919-c7ef65a9cdef\",\"node\":\"bar\"," +
	"\"deployment_id\":\"1c87a656-594d-4300-b4ad-630914e11856\"}}" +
	// Second alert
	",{\"alertDefinitionId\":\"c3d257e2-0140-4a8a-bcd3-c5d48ea4d47a\"," +
	"\"annotations\":{}," +
	"\"endsAt\":\"2024-01-23T16:13:45.510+01:00\",\"fingerprint\":\"4bfbad375f9020af\"," +
	"\"receivers\":[{\"name\":\"web.hook\"}],\"startsAt\":\"2024-01-23T16:08:45.510+01:00\"," +
	"\"status\":{\"inhibitedBy\":[],\"silencedBy\":[],\"state\":\"active\"}," +
	"\"updatedAt\":\"2024-01-23T16:08:45.510+01:00\"," +
	"\"labels\":{\"alertname\":\"foo\",\"cluster_name\":\"test\",\"alert_category\":\"test\"," +
	"\"host_uuid\":\"93bf6804-52a3-4ba1-a919-c7ef65a9cdef\",\"node\":\"bar\"," +
	"\"deployment_id\":\"1c87a656-594d-4300-b4ad-630914e11856\"}}" +
	// Third alert
	",{\"alertDefinitionId\":\"c6b2a291-a9a2-49d2-930f-f865457b1aa8\"," +
	"\"annotations\":{}," +
	"\"endsAt\":\"2024-01-23T16:13:45.560+01:00\",\"fingerprint\":\"bf31b9c198429127\"," +
	"\"receivers\":[{\"name\":\"web.hook\"}],\"startsAt\":\"2024-01-23T16:08:45.560+01:00\"," +
	"\"status\":{\"inhibitedBy\":[],\"silencedBy\":[],\"state\":\"active\"}," +
	"\"updatedAt\":\"2024-01-23T16:08:45.560+01:00\"," +
	"\"labels\":{\"alertname\":\"foo3\",\"cluster_name\":\"test\",\"alert_category\":\"test\"," +
	"\"host_uuid\":\"93bf6804-52a3-4ba1-a919-c7ef65a9cdef\",\"node\":\"bar\"," +
	"\"deployment_id\":\"1c87a656-594d-4300-b4ad-630914e11856\"}}]"

const alertMonitorResponseBadUUID = "[{\"annotations\":{\"am_test\":\"test\",\"am_uuid\":\"bad\"}," +
	"\"endsAt\":\"2024-01-23T16:13:45.535+01:00\",\"fingerprint\":\"0c8d24dab761f647\"," +
	"\"receivers\":[{\"name\":\"web.hook\"}],\"startsAt\":\"2024-01-23T16:08:45.535+01:00\"," +
	"\"status\":{\"inhibitedBy\":[],\"silencedBy\":[],\"state\":\"active\"}," +
	"\"updatedAt\":\"2024-01-23T16:08:45.535+01:00\"," +
	"\"labels\":{\"alertname\":\"foo2\",\"cluster_name\":\"test\",\"alert_category\":\"test\"," +
	"\"host_uuid\":\"93bf6804-52a3-4ba1-a919-c7ef65a9cdef\",\"node\":\"bar\"," +
	"\"deployment_id\":\"1c87a656-594d-4300-b4ad-630914e11856\"}}]"

const emptyAlertManagerResponse = "[]"

const badAlertManagerResponse = "bad response"

var conf = config.Config{
	AlertManager: config.AlertManagerConfig{
		URL: "http://localhost:49152",
	},
}

var alertDefTemplateNotRendered = `alert: HighCPUUsage
annotations:
  description: CPU usage has exceeded 80%
  summary: High CPU usage detected
expr: cpu_usage > {{ .Threshold }}
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m0s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "80"
`

var alertDefTemplateRendered = `alert: HighCPUUsage
annotations:
  description: CPU usage has exceeded 80%
  summary: High CPU usage detected
expr: cpu_usage > 80
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m0s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "80"
`

// There is one too many closing bracket ) on the expression.
var alertDefTemplateBadExpression = `alert: HighCPUUsage
annotations:
  description: CPU usage has exceeded 80%
  summary: High CPU usage detected
expr: (rate(net_bytes_sent{}[30s]) + rate(net_bytes_recv{}[30s]))) / 1000000 >= {{ .Threshold }}
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "80"
`

var alertDefTemplateRenderedDuration = `alert: HighCPUUsage
annotations:
  description: CPU usage has exceeded 80%
  summary: High CPU usage detected
expr: avg_over_time(cpu_usage[1m]) > 80
for: 1m
labels:
  alert_category: performance
  alert_context: host
  duration: 1m0s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "80"
`

type M2MAuthenticatorMock struct {
	mock.Mock
}

func (m *M2MAuthenticatorMock) GetUserList(eCtx echo.Context) ([]user, error) {
	args := m.Called(eCtx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]user), args.Error(1)
}

func TestGetAlerts(t *testing.T) {
	tests := map[string]struct {
		server              bool
		header              header
		managerResponse     string
		managerResponseCode int
		expectedCode        int
		expected            string
	}{
		"Test response when alert manager is not accessible - code should be 500": {
			server:              false,
			header:              header{"ActiveProjectID", "edgenode"},
			managerResponse:     "",
			managerResponseCode: 0,
			expectedCode:        http.StatusInternalServerError,
			expected:            "",
		},
		"Test response when alert manager response is invalid - code should be 500": {
			server:              true,
			header:              header{"ActiveProjectID", "edgenode"},
			managerResponse:     badAlertManagerResponse,
			managerResponseCode: http.StatusOK,
			expectedCode:        http.StatusInternalServerError,
			expected:            "",
		},
		"Test response when alert manager returns invalid uuid - code should be 500": {
			server:              true,
			header:              header{"ActiveProjectID", "edgenode"},
			managerResponse:     alertMonitorResponseBadUUID,
			managerResponseCode: http.StatusOK,
			expectedCode:        http.StatusInternalServerError,
			expected:            "",
		},
		"Test response when alert manager return non 200 code - code should be 500": {
			server:              true,
			header:              header{"ActiveProjectID", "edgenode"},
			managerResponse:     "forbidden",
			managerResponseCode: http.StatusForbidden,
			expectedCode:        http.StatusInternalServerError,
			expected:            "",
		},
		"Test response when alert manager is accessible - not empty alert list": {
			server:              true,
			header:              header{"ActiveProjectID", "edgenode"},
			managerResponse:     alertManagerResponse,
			managerResponseCode: http.StatusOK,
			expectedCode:        http.StatusOK,
			expected:            alertMonitorExpectedResponse,
		},
		"Test response when alert manager is accessible - empty alert list": {
			server:              true,
			header:              header{"ActiveProjectID", "edgenode"},
			managerResponse:     emptyAlertManagerResponse,
			managerResponseCode: http.StatusOK,
			expectedCode:        http.StatusOK,
			expected:            emptyAlertManagerResponse,
		},
		"Test response when invalid (empty) projectID is provided - code should be 400": {
			server:              true,
			header:              header{"ActiveProjectID", ""},
			managerResponse:     "",
			managerResponseCode: 0,
			expectedCode:        http.StatusBadRequest,
			expected:            "",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			configfile := conf
			var svr *httptest.Server

			// Creating new Echo server
			e := echo.New()

			if test.server {
				svr = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/v2/alerts" {
						w.WriteHeader(test.managerResponseCode)
						fmt.Fprint(w, test.managerResponse)
					}
				}))
				configfile.AlertManager.URL = svr.URL
				defer svr.Close()
			}
			serverInterface := NewServerInterfaceHandler(configfile, &gorm.DB{}, nil, logger)

			// Registering API call handlers
			api.RegisterHandlers(e, serverInterface)

			result := testutil.NewRequest().WithHeader(test.header.key, test.header.value).Get("/api/v1/alerts").GoWithHTTPHandler(t, e)
			require.Equal(t, test.expectedCode, result.Recorder.Code, "Response code does not equal %v", test.expectedCode)

			if test.expectedCode == http.StatusOK {
				assertResponse(t, test.expected, result.Recorder.Body)
			}
		})
	}
}

func assertResponse(t *testing.T, expected string, responseBody *bytes.Buffer) {
	unmarshalledResponse := new(api.AlertList)
	unmarshalledExpected := new(api.AlertList)

	body, err := io.ReadAll(responseBody)
	require.NoError(t, err, "Error reading response body")

	err = json.Unmarshal(body, &unmarshalledResponse)
	require.NoError(t, err, "Error unmarshalling api response")

	err = json.Unmarshal([]byte(expected), &unmarshalledExpected.Alerts)
	require.NoError(t, err, "Error unmarshalling expected json")

	expectedAlerts := unmarshalledExpected.Alerts
	responseAlerts := unmarshalledResponse.Alerts
	require.Len(t, *responseAlerts, len(*expectedAlerts), "Number of alerts in expected response and actual response does not match")

	require.Equal(t, unmarshalledExpected, unmarshalledResponse, "Response body different than expected")
}

// DefinitionMock represents a mock for alert definition database operations. Implements AlertDefinitionHandlerManager interface.
type DefinitionMock struct {
	mock.Mock
}

func (m *DefinitionMock) GetLatestAlertDefinitionList(ctx context.Context, tenantID api.TenantID) ([]*models.DBAlertDefinition, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DBAlertDefinition), args.Error(1)
}

func (m *DefinitionMock) GetLatestAlertDefinition(ctx context.Context, tenantID api.TenantID, id uuid.UUID) (*models.DBAlertDefinition, error) {
	args := m.Called(ctx, tenantID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.DBAlertDefinition), args.Error(1)
}

func (m *DefinitionMock) SetAlertDefinitionValues(ctx context.Context, tenantID api.TenantID, id uuid.UUID, values models.DBAlertDefinitionValues) error {
	args := m.Called(ctx, tenantID, id, values)
	return args.Error(0)
}

func TestGetAlertDefinitions(t *testing.T) {
	t.Run("Failed to get alert definitions from database", func(t *testing.T) {
		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definitions from database.
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, tenantID).Return(nil, errors.New("error mock")).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToGetAlertDefinitions)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Got empty alert definitions from database", func(t *testing.T) {
		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definitions from database.
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, tenantID).Return([]*models.DBAlertDefinition{}, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		definitionsExp := []api.AlertDefinition{}
		definitionsListExp := &api.AlertDefinitionList{
			AlertDefinitions: &definitionsExp,
		}

		definitions := []api.AlertDefinition{}
		definitionsList := &api.AlertDefinitionList{
			AlertDefinitions: &definitions,
		}
		require.NoError(t, json.Unmarshal(body, definitionsList))
		require.Equal(t, definitionsListExp, definitionsList)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Succeeded to get alert definitions from database", func(t *testing.T) {
		id := uuid.New()
		dur := int64(10)
		thres := int64(100)
		enabled := true
		tenantID := "edgenode"
		dbDef := &models.DBAlertDefinition{
			ID:    id,
			Name:  "alert1",
			State: "applied",
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur,
				Threshold: &thres,
				Enabled:   &enabled,
			},
			Category: models.CategoryHealth,
			TenantID: tenantID,
		}

		mDefinition := &DefinitionMock{}

		// mock getting alert definitions from database.
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, tenantID).Return([]*models.DBAlertDefinition{dbDef}, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		stateExp := api.StateDefinition(dbDef.State)
		versionExp := int(dbDef.Version)

		definitionsExp := []api.AlertDefinition{
			{
				Id:    &dbDef.ID,
				Name:  &dbDef.Name,
				State: &stateExp,
				Values: &map[string]string{
					"duration":  "10s",
					"threshold": "100",
					"enabled":   "true",
				},
				Version: &versionExp,
			},
		}
		definitionsListExp := &api.AlertDefinitionList{
			AlertDefinitions: &definitionsExp,
		}

		definitions := []api.AlertDefinition{}
		definitionsList := &api.AlertDefinitionList{
			AlertDefinitions: &definitions,
		}

		require.NoError(t, json.Unmarshal(body, definitionsList))
		require.Equal(t, definitionsListExp, definitionsList)
		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Get alert definitions among many tenants", func(t *testing.T) {
		id1 := uuid.New()
		dur1 := int64(10)
		thres1 := int64(100)
		enabled1 := true
		tenantID1 := "first_tenant"
		dbDef1 := &models.DBAlertDefinition{
			ID:    id1,
			Name:  "alert1",
			State: "applied",
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur1,
				Threshold: &thres1,
				Enabled:   &enabled1,
			},
			Category: models.CategoryHealth,
			TenantID: tenantID1,
		}

		id2 := uuid.New()
		dur2 := int64(10)
		thres2 := int64(100)
		enabled2 := true
		tenantID2 := "second_tenant"
		dbDef2 := &models.DBAlertDefinition{
			ID:    id2,
			Name:  "alert2",
			State: "applied",
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur2,
				Threshold: &thres2,
				Enabled:   &enabled2,
			},
			Category: models.CategoryHealth,
			TenantID: tenantID2,
		}

		mDefinition := &DefinitionMock{}

		// mock getting alert definitions from database.
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, tenantID1).Return([]*models.DBAlertDefinition{dbDef1}, nil).Once()
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, tenantID2).Return([]*models.DBAlertDefinition{dbDef2}, nil).Once()
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, "wrong_tenant").Return([]*models.DBAlertDefinition{}, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		// Getting alert definition from first tenant
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID1).Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)
		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		stateExp := api.StateDefinition(dbDef1.State)
		versionExp := int(dbDef1.Version)

		definitionsExp := []api.AlertDefinition{
			{
				Id:    &dbDef1.ID,
				Name:  &dbDef1.Name,
				State: &stateExp,
				Values: &map[string]string{
					"duration":  "10s",
					"threshold": "100",
					"enabled":   "true",
				},
				Version: &versionExp,
			},
		}
		definitionsListExp := &api.AlertDefinitionList{
			AlertDefinitions: &definitionsExp,
		}

		definitions := []api.AlertDefinition{}
		definitionsList := &api.AlertDefinitionList{
			AlertDefinitions: &definitions,
		}

		require.NoError(t, json.Unmarshal(body, definitionsList))
		require.Equal(t, definitionsListExp, definitionsList)

		// Getting alert definition from second tenant
		result = testutil.NewRequest().WithHeader("ActiveProjectID", tenantID2).Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err = io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		stateExp = api.StateDefinition(dbDef1.State)
		versionExp = int(dbDef2.Version)

		definitionsExp = []api.AlertDefinition{
			{
				Id:    &dbDef2.ID,
				Name:  &dbDef2.Name,
				State: &stateExp,
				Values: &map[string]string{
					"duration":  "10s",
					"threshold": "100",
					"enabled":   "true",
				},
				Version: &versionExp,
			},
		}
		definitionsListExp = &api.AlertDefinitionList{
			AlertDefinitions: &definitionsExp,
		}

		definitions = []api.AlertDefinition{}
		definitionsList = &api.AlertDefinitionList{
			AlertDefinitions: &definitions,
		}

		require.NoError(t, json.Unmarshal(body, definitionsList))
		require.Equal(t, definitionsListExp, definitionsList)

		// Getting no alert definition
		result = testutil.NewRequest().WithHeader("ActiveProjectID", "wrong_tenant").Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err = io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		definitionsExp = []api.AlertDefinition{}
		definitionsListExp = &api.AlertDefinitionList{
			AlertDefinitions: &definitionsExp,
		}

		definitions = []api.AlertDefinition{}
		definitionsList = &api.AlertDefinitionList{
			AlertDefinitions: &definitions,
		}
		require.NoError(t, json.Unmarshal(body, definitionsList))
		require.Equal(t, definitionsListExp, definitionsList)
		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Maintenance alert is filtered out and empty list is returned", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"
		dbDef := &models.DBAlertDefinition{
			ID:       id,
			Name:     "alert1",
			State:    "applied",
			Category: models.CategoryMaintenance,
			TenantID: tenantID,
		}

		mDefinition := &DefinitionMock{}

		// mock getting alert definitions from database.
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, tenantID).Return([]*models.DBAlertDefinition{dbDef}, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		definitionsExp := []api.AlertDefinition{}
		definitionsListExp := &api.AlertDefinitionList{
			AlertDefinitions: &definitionsExp,
		}

		definitions := []api.AlertDefinition{}
		definitionsList := &api.AlertDefinitionList{
			AlertDefinitions: &definitions,
		}
		require.NoError(t, json.Unmarshal(body, definitionsList))
		require.Equal(t, definitionsListExp, definitionsList)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Only maintenance alert is filtered out from the definitions list", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		dbMaintenanceDef := &models.DBAlertDefinition{
			ID:       id,
			Name:     "alert1",
			State:    "applied",
			Category: models.CategoryMaintenance,
			TenantID: tenantID,
		}
		id2 := uuid.New()
		dur := int64(10)
		thres := int64(100)
		enabled := true
		dbDef := &models.DBAlertDefinition{
			ID:    id2,
			Name:  "alert2",
			State: "applied",
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur,
				Threshold: &thres,
				Enabled:   &enabled,
			},
			Category: models.CategoryHealth,
			TenantID: tenantID,
		}

		mDefinition := &DefinitionMock{}

		// mock getting alert definitions from database.
		mDefinition.On("GetLatestAlertDefinitionList", mock.Anything, tenantID).Return([]*models.DBAlertDefinition{dbMaintenanceDef, dbDef}, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get("/api/v1/alerts/definitions").GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		stateExp := api.StateDefinition(dbDef.State)
		versionExp := int(dbDef.Version)
		definitionsExp := []api.AlertDefinition{
			{
				Id:    &dbDef.ID,
				Name:  &dbDef.Name,
				State: &stateExp,
				Values: &map[string]string{
					"duration":  "10s",
					"threshold": "100",
					"enabled":   "true",
				},
				Version: &versionExp,
			},
		}
		definitionsListExp := &api.AlertDefinitionList{
			AlertDefinitions: &definitionsExp,
		}

		definitions := []api.AlertDefinition{}
		definitionsList := &api.AlertDefinitionList{
			AlertDefinitions: &definitions,
		}
		require.NoError(t, json.Unmarshal(body, definitionsList))
		require.Equal(t, definitionsListExp, definitionsList)

		require.True(t, mDefinition.AssertExpectations(t))
	})
}

func TestGetAlertDefinition(t *testing.T) {
	t.Run("Alert definition not found", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition from database.
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(nil, fmt.Errorf("mock error: %w", gorm.ErrRecordNotFound)).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusNotFound, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPAlertDefinitionNotFound)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Failed to retrieve alert definition by UUID from database", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition from database.
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(nil, errors.New("error mock")).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToGetAlertDefinition)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Succeeded to retrieve alert definition by UUID from database", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition from database.
		dur := int64(10)
		thres := int64(100)
		enabled := true
		dbDef := &models.DBAlertDefinition{
			ID:    id,
			Name:  "alert1",
			State: "applied",
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur,
				Threshold: &thres,
				Enabled:   &enabled,
			},
			TenantID: tenantID,
		}
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(dbDef, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		stateExp := api.StateDefinition(dbDef.State)
		versionExp := int(dbDef.Version)
		definitionExp := &api.AlertDefinition{
			Id:    &dbDef.ID,
			Name:  &dbDef.Name,
			State: &stateExp,
			Values: &map[string]string{
				"duration":  "10s",
				"threshold": "100",
				"enabled":   "true",
			},
			Version: &versionExp,
		}

		definition := &api.AlertDefinition{}
		require.NoError(t, json.Unmarshal(body, definition))
		require.Equal(t, definitionExp, definition)

		require.True(t, mDefinition.AssertExpectations(t))
	})
}

func TestGetAlertDefinitionTemplate(t *testing.T) {
	t.Run("Alert definition template not found", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition from database.
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(nil, fmt.Errorf("mock error: %w", gorm.ErrRecordNotFound)).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v/template", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusNotFound, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPAlertDefinitionTemplateNotFound)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Failed to retrieve alert definition template by UUID from database", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition from database.
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(nil, errors.New("error mock")).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v/template", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToGetAlertDefinitionTemplate)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Succeeded to get alert def template with rendered false", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition template from database.
		dur := int64(60)
		thres := int64(80)
		dbDef := &models.DBAlertDefinition{
			Template: alertDefTemplateNotRendered,
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur,
				Threshold: &thres,
			},
			TenantID: tenantID,
		}
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(dbDef, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v/template?rendered=false", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		var outTemplate api.AlertDefinitionTemplate
		err = yaml.Unmarshal(body, &outTemplate) //nolint:musttag // api.AlertDefinitionTemplate contains autogenerated code
		require.NoError(t, err, "failed to unmarshal body response into template")

		var expectedTemplate api.AlertDefinitionTemplate
		err = yaml.Unmarshal([]byte(dbDef.Template), &expectedTemplate) //nolint:musttag // api.AlertDefinitionTemplate contains autogenerated code
		require.NoError(t, err, "failed to unmarshal expected body to yaml")

		require.Equal(t, expectedTemplate, outTemplate)
		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Succeeded to get alert def template with rendered true", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition template from database.
		dur := int64(60)
		thres := int64(80)
		enabled := true
		dbDef := &models.DBAlertDefinition{
			Template: alertDefTemplateRendered,
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur,
				Threshold: &thres,
				Enabled:   &enabled,
			},
			TenantID: tenantID,
		}
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(dbDef, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v/template?rendered=true", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		var outTemplate api.AlertDefinitionTemplate
		err = yaml.Unmarshal(body, &outTemplate) //nolint:musttag // api.AlertDefinitionTemplate contains autogenerated code
		require.NoError(t, err, "failed to unmarshal body response into template")

		var expectedTemplate api.AlertDefinitionTemplate
		err = yaml.Unmarshal([]byte(dbDef.Template), &expectedTemplate) //nolint:musttag // api.AlertDefinitionTemplate contains autogenerated code
		require.NoError(t, err, "failed to unmarshal expected body to yaml")

		require.Equal(t, expectedTemplate, outTemplate)
		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Failed to get alert def template with rendered false due to unmarshalling", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition template from database.
		dbDef := &models.DBAlertDefinition{
			Template: "invalid yaml -",
		}
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(dbDef, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v/template?rendered=false", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToGetAlertDefinitionTemplate)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Failed to get alert def template due to bad expression", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition template from database.
		dur := int64(60)
		thres := int64(80)
		enabled := true
		dbDef := &models.DBAlertDefinition{
			Template: alertDefTemplateBadExpression,
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur,
				Threshold: &thres,
				Enabled:   &enabled,
			},
			TenantID: tenantID,
		}
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(dbDef, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v/template?rendered=true", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToGetAlertDefinitionTemplate)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Succeeded to get alert def template with rendered true where duration is templated", func(t *testing.T) {
		id := uuid.New()

		mDefinition := &DefinitionMock{}
		tenantID := "edgenode"

		// mock getting alert definition template from database.
		dur := int64(60)
		thres := int64(80)
		enabled := true
		dbDef := &models.DBAlertDefinition{
			Template: alertDefTemplateRenderedDuration,
			Values: models.DBAlertDefinitionValues{
				Duration:  &dur,
				Threshold: &thres,
				Enabled:   &enabled,
			},
			TenantID: tenantID,
		}
		mDefinition.On("GetLatestAlertDefinition", mock.Anything, tenantID, id).Return(dbDef, nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v/template?rendered=true", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		var outTemplate api.AlertDefinitionTemplate
		err = yaml.Unmarshal(body, &outTemplate) //nolint:musttag // api.AlertDefinitionTemplate contains autogenerated code
		require.NoError(t, err, "failed to unmarshal body response into template")

		var expectedTemplate api.AlertDefinitionTemplate
		err = yaml.Unmarshal([]byte(dbDef.Template), &expectedTemplate) //nolint:musttag // api.AlertDefinitionTemplate contains autogenerated code
		require.NoError(t, err, "failed to unmarshal expected body to yaml")

		require.Equal(t, expectedTemplate, outTemplate)
		require.True(t, mDefinition.AssertExpectations(t))
	})
}

func stringPtr(s string) *string { return &s }

func int64Ptr(i int64) *int64 { return &i }

func boolPtr(b bool) *bool { return &b }

func TestPatchAlertDefinition(t *testing.T) {
	testCases := []struct {
		name     string
		payload  []byte
		httpCode int
		errMsg   string
	}{
		{
			name:    "Request body missing values field",
			payload: []byte(`{"threshold":"10","duration":"8m","enabled":true}`),
			errMsg:  errHTTPBadRequest,
		},
		{
			name:    "Request body has unknown fields",
			payload: []byte(`{"vals":{"threshold":"10","duration":"8m","enabled":true}}`),
			errMsg:  errHTTPBadRequest,
		},
		{
			name:    "Request body has unknown value fields",
			payload: []byte(`{"values":{"threshold":"10","time":"8m","enabled":true}}`),
			errMsg:  errHTTPBadRequest,
		},
		{
			name:    "Request body has no values to set",
			payload: []byte(`{"values":{}}`),
			errMsg:  errHTTPFailedToPatchAlertDefinition,
		},
		{
			name:    "Duration value format is invalid",
			payload: []byte(`{"values":{"duration":"2sec"}}`),
			errMsg:  errHTTPFailedToPatchAlertDefinition,
		},
		{
			name:    "Duration value cannot be fraction of a second",
			payload: []byte(`{"values":{"duration":"100ms"}}`),
			errMsg:  errHTTPFailedToPatchAlertDefinition,
		},
		{
			name:    "Duration value cannot be zero",
			payload: []byte(`{"values":{"duration":"0m"}}`),
			errMsg:  errHTTPFailedToPatchAlertDefinition,
		},
		{
			name:    "Enabled value is not a boolean",
			payload: []byte(`{"values":{"enabled":"yes"}}`),
			errMsg:  errHTTPFailedToPatchAlertDefinition,
		},
		{
			name:    "Threshold value is a non numeric string",
			payload: []byte(`{"values":{"threshold":"ten"}}`),
			errMsg:  errHTTPFailedToPatchAlertDefinition,
		},
		{
			name:    "Duration value string has invalid format",
			payload: []byte(`{"values":{"duration":"one second"}}`),
			errMsg:  errHTTPFailedToPatchAlertDefinition,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &ServerInterfaceHandler{}
			tenantID := "edgenode"

			// Creating new Echo server
			server := echo.New()

			// Registering API call handlers
			api.RegisterHandlers(server, handler)

			request := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).
				Patch("/api/v1/alerts/definitions/01e74407-0327-4e36-93cb-85801c098ba5").WithBody(tc.payload)
			result := request.GoWithHTTPHandler(t, server)

			body, err := io.ReadAll(result.Recorder.Body)
			require.NoError(t, err)

			httpErr := &api.HttpError{}
			require.NoError(t, json.Unmarshal(body, httpErr))

			require.Equal(t, http.StatusBadRequest, httpErr.Code)
			require.Contains(t, httpErr.Message, tc.errMsg)
		})
	}

	t.Run("Alert definition not found", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		threshold := int64(10)
		duration := int64(45)
		enabled := true

		values := models.DBAlertDefinitionValues{
			Threshold: &threshold,
			Duration:  &duration,
			Enabled:   &enabled,
		}

		mDefinition := &DefinitionMock{}

		// mock setting values to alert definition.
		mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, values).Return(fmt.Errorf("mock error: %w", gorm.ErrRecordNotFound)).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		bodyStr := fmt.Sprintf(`{"values":{"threshold":"%d","duration":"%ds","enabled":"%v"}}`, threshold, duration, enabled)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody([]byte(bodyStr)).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusNotFound, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPAlertDefinitionNotFound)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Alert definition value is out-of-bounds", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		threshold := int64(10)
		duration := int64(45)
		enabled := true

		values := models.DBAlertDefinitionValues{
			Threshold: &threshold,
			Duration:  &duration,
			Enabled:   &enabled,
		}

		mDefinition := &DefinitionMock{}

		// mock setting values to alert definition.
		mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, values).
			Return(fmt.Errorf("error mock: %w", database.ErrValueOutOfBounds)).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		bodyStr := fmt.Sprintf(`{"values":{"threshold":"%d","duration":"%ds","enabled":"%v"}}`, threshold, duration, enabled)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody([]byte(bodyStr)).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusBadRequest, httpErr.Code)
		require.Contains(t, httpErr.Message, "alert definition value/s out-of-bounds")

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Failed setting values to alert definition", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		threshold := int64(10)
		duration := int64(45)
		enabled := true

		values := models.DBAlertDefinitionValues{
			Threshold: &threshold,
			Duration:  &duration,
			Enabled:   &enabled,
		}

		mDefinition := &DefinitionMock{}

		// mock setting values to alert definition.
		mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, values).Return(errors.New("mock error")).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		bodyStr := fmt.Sprintf(`{"values":{"threshold":"%d","duration":"%ds","enabled":"%v"}}`, threshold, duration, enabled)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody([]byte(bodyStr)).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToPatchAlertDefinition)

		require.True(t, mDefinition.AssertExpectations(t))
	})

	t.Run("Succeeded setting values to alert definition", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		threshold := int64(10)
		duration := int64(45)
		enabled := true

		values := models.DBAlertDefinitionValues{
			Threshold: &threshold,
			Duration:  &duration,
			Enabled:   &enabled,
		}

		mDefinition := &DefinitionMock{}

		// mock setting values to alert definition.
		mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, values).Return(nil).Once()

		handler := &ServerInterfaceHandler{
			definitions: mDefinition,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		bodyStr := fmt.Sprintf(`{"values":{"threshold":"%d","duration":"%ds","enabled":"%v"}}`, threshold, duration, enabled)

		uri := fmt.Sprintf("/api/v1/alerts/definitions/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody([]byte(bodyStr)).GoWithHTTPHandler(t, server)
		require.Equal(t, http.StatusNoContent, result.Recorder.Code)

		require.True(t, mDefinition.AssertExpectations(t))
	})
}

// ReceiverMock represents a mock for receiver database operations. Implements ReceiverManager interface.
type ReceiverMock struct {
	mock.Mock
}

func (m *ReceiverMock) GetLatestReceiverWithEmailConfig(ctx context.Context, tenantID api.TenantID, id uuid.UUID) (*models.DBReceiver, error) {
	args := m.Called(ctx, tenantID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.DBReceiver), args.Error(1)
}

func (m *ReceiverMock) GetLatestReceiverListWithEmailConfig(ctx context.Context, tenantID api.TenantID) ([]*models.DBReceiver, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DBReceiver), args.Error(1)
}

func (m *ReceiverMock) SetReceiverEmailRecipients(ctx context.Context, tenantID api.TenantID, id uuid.UUID, recipients []models.EmailAddress) error {
	args := m.Called(ctx, tenantID, id, recipients)
	return args.Error(0)
}

func (m *ReceiverMock) GetReceiverWithEmailConfig(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64) (*models.DBReceiver, error) {
	args := m.Called(ctx, tenantID, id, version)
	return args.Get(0).(*models.DBReceiver), args.Error(1)
}

func TestGetAlertReceivers(t *testing.T) {
	t.Run("Failed to get receivers from database", func(t *testing.T) {
		mReceiver := &ReceiverMock{}
		tenantID := "edgenode"

		// mock getting receivers from database.
		mReceiver.On("GetLatestReceiverListWithEmailConfig", mock.Anything, tenantID).Return(nil, errors.New("error mock")).Once()

		handler := &ServerInterfaceHandler{
			receivers: mReceiver,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get("/api/v1/alerts/receivers").GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToGetAlertReceivers)

		require.True(t, mReceiver.AssertExpectations(t))
	})

	t.Run("Get receivers among many tenants", func(t *testing.T) {
		firstName := "test"
		lastName := "user"
		email := "test-1@user.com"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return([]user{
			{
				FirstName: firstName,
				LastName:  lastName,
				Email:     email,
			},
		}, nil)

		uuid1 := uuid.New()
		tenantID1 := "first_tenant"
		recv1 := &models.DBReceiver{
			UUID:    uuid1,
			Name:    "test-receiver-1",
			Version: 3,
			To: []string{
				"test user <test-1@user.com>",
			},
			From:       "sender user <sender@user.com>",
			MailServer: "smtp.com:443",
			TenantID:   tenantID1,
		}

		uuid2 := uuid.New()
		tenantID2 := "second_tenant"
		recv2 := &models.DBReceiver{
			UUID:    uuid2,
			Name:    "test-receiver-2",
			Version: 3,
			To: []string{
				"test user <test-1@user.com>",
			},
			From:       "sender user <sender@user.com>",
			MailServer: "smtp.com:443",
			TenantID:   tenantID2,
		}

		mReceiver := &ReceiverMock{}
		mReceiver.On("GetLatestReceiverListWithEmailConfig", mock.Anything, tenantID1).Return([]*models.DBReceiver{recv1}, nil).Once()
		mReceiver.On("GetLatestReceiverListWithEmailConfig", mock.Anything, tenantID2).Return([]*models.DBReceiver{recv2}, nil).Once()
		mReceiver.On("GetLatestReceiverListWithEmailConfig", mock.Anything, "wrong_tenant").Return([]*models.DBReceiver{}, nil).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m:       mM2M,
			receivers: mReceiver,
		})

		// Getting receiver from first tenant
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID1).Get("/api/v1/alerts/receivers").GoWithHTTPHandler(t, server)
		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		stateExp := api.StateDefinition(recv1.State)
		versionExp := recv1.Version
		mailServer := recv1.MailServer
		from := recv1.From
		to := recv1.To

		receiversExp := []api.Receiver{
			{
				Id:      &recv1.UUID,
				State:   &stateExp,
				Version: &versionExp,
				EmailConfig: &api.EmailConfig{
					From:       &from,
					MailServer: &mailServer,
					To: &struct {
						Allowed *api.EmailRecipientList `json:"allowed,omitempty"`
						Enabled *api.EmailRecipientList `json:"enabled,omitempty"`
					}{
						Allowed: &to,
						Enabled: &to,
					},
				},
			},
		}
		receiversListExp := &api.ReceiverList{
			Receivers: &receiversExp,
		}

		receivers := []api.Receiver{}
		receiversList := &api.ReceiverList{
			Receivers: &receivers,
		}

		require.NoError(t, json.Unmarshal(body, receiversList))
		require.Equal(t, receiversListExp, receiversList)

		// // Getting receiver from second tenant
		result = testutil.NewRequest().WithHeader("ActiveProjectID", tenantID2).Get("/api/v1/alerts/receivers").GoWithHTTPHandler(t, server)
		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err = io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		stateExp = api.StateDefinition(recv2.State)
		versionExp = recv2.Version
		mailServer = recv2.MailServer
		from = recv2.From
		to = recv2.To

		receiversExp = []api.Receiver{
			{
				Id:      &recv2.UUID,
				State:   &stateExp,
				Version: &versionExp,
				EmailConfig: &api.EmailConfig{
					From:       &from,
					MailServer: &mailServer,
					To: &struct {
						Allowed *api.EmailRecipientList `json:"allowed,omitempty"`
						Enabled *api.EmailRecipientList `json:"enabled,omitempty"`
					}{
						Allowed: &to,
						Enabled: &to,
					},
				},
			},
		}
		receiversListExp = &api.ReceiverList{
			Receivers: &receiversExp,
		}

		receivers = []api.Receiver{}
		receiversList = &api.ReceiverList{
			Receivers: &receivers,
		}

		require.NoError(t, json.Unmarshal(body, receiversList))
		require.Equal(t, receiversListExp, receiversList)

		// Getting no receivers
		result = testutil.NewRequest().WithHeader("ActiveProjectID", "wrong_tenant").Get("/api/v1/alerts/receivers").GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusOK, result.Recorder.Code)

		body, err = io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		receiversExp = []api.Receiver{}
		receiversListExp = &api.ReceiverList{
			Receivers: &receiversExp,
		}

		receivers = []api.Receiver{}
		receiversList = &api.ReceiverList{
			Receivers: &receivers,
		}

		require.NoError(t, json.Unmarshal(body, receiversList))
		require.Equal(t, receiversListExp, receiversList)
		require.True(t, mReceiver.AssertExpectations(t))
	})
}

func TestGetAlertReceiver(t *testing.T) {
	t.Run("Receiver not found", func(t *testing.T) {
		id := uuid.New()
		mReceiver := &ReceiverMock{}
		tenantID := "edgenode"

		// mock getting receiver by UUID from database.
		mReceiver.On("GetLatestReceiverWithEmailConfig", mock.Anything, tenantID, id).Return(nil, fmt.Errorf("mock error: %w", gorm.ErrRecordNotFound)).Once()

		handler := &ServerInterfaceHandler{
			receivers: mReceiver,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusNotFound, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPAlertReceiverNotFound)

		require.True(t, mReceiver.AssertExpectations(t))
	})

	t.Run("Failed to retrieve receiver by UUID from database", func(t *testing.T) {
		id := uuid.New()
		mReceiver := &ReceiverMock{}
		tenantID := "edgenode"

		// mock getting receiver by UUID from database.
		mReceiver.On("GetLatestReceiverWithEmailConfig", mock.Anything, tenantID, id).Return(nil, errors.New("mock error")).Once()

		handler := &ServerInterfaceHandler{
			receivers: mReceiver,
		}

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, handler)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Get(uri).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToGetAlertReceiver)

		require.True(t, mReceiver.AssertExpectations(t))
	})
}

func TestPatchAlertReceiver(t *testing.T) {
	t.Run("Invalid request body", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{})

		invalidBody := []byte(`{"emailConfig":{"to":["firstName lastName <emailtext@sampppple.com>"]}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(invalidBody).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusBadRequest, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPBadRequest)
		require.Equal(t, http.StatusBadRequest, result.Recorder.Code)
	})

	t.Run("Request body contains unknown extra fields", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{})

		invalidBody := []byte(`{"emailConfig":{"to":{"enabled":["first user <first.user@email.com>"], "allowed":["second user second.user@email.com"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(invalidBody).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusBadRequest, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPBadRequest)
	})

	t.Run("Fail to get allowed email recipients", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return(nil, errors.New("mock error")).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m: mM2M,
		})

		body := []byte(`{"emailConfig":{"to":{"enabled":["bar foo <foo@bar>"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(body).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToPatchAlertReceivers)

		require.True(t, mM2M.AssertExpectations(t))
	})

	t.Run("Allowed email recipients is empty", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return([]user{}, nil).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m: mM2M,
		})

		body := []byte(`{"emailConfig":{"to":{"enabled":["bar foo <foo@bar>"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(body).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToPatchAlertReceivers)

		require.True(t, mM2M.AssertExpectations(t))
	})

	t.Run("Email recipient not allowed", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return([]user{
			{
				FirstName: "foo",
				LastName:  "bar",
				Email:     "foo@bar.com",
			},
		}, nil).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m: mM2M,
		})

		body := []byte(`{"emailConfig":{"to":{"enabled":["bar foo <foo@bar>"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(body).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusBadRequest, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPBadRequest)

		require.True(t, mM2M.AssertExpectations(t))
	})

	t.Run("Duplicated email recipients", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return([]user{
			{
				FirstName: "foo",
				LastName:  "bar",
				Email:     "foo@bar.com",
			},
		}, nil).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m: mM2M,
		})

		body := []byte(`{"emailConfig":{"to":{"enabled":["foo bar <foo@bar.com>", "foo bar <foo@bar.com>"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(body).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusBadRequest, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPBadRequest)

		require.True(t, mM2M.AssertExpectations(t))
	})

	t.Run("Receiver not found", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		firstName := "foo"
		lastName := "bar"
		email := "foo@bar.com"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return([]user{
			{
				FirstName: firstName,
				LastName:  lastName,
				Email:     email,
			},
		}, nil).Once()

		mReceiver := &ReceiverMock{}
		mReceiver.On("SetReceiverEmailRecipients", mock.Anything, tenantID, id, []models.EmailAddress{
			{
				FirstName: firstName,
				LastName:  lastName,
				Email:     email,
			},
		}).Return(fmt.Errorf("mock error: %w", gorm.ErrRecordNotFound)).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m:       mM2M,
			receivers: mReceiver,
		})

		body := []byte(`{"emailConfig":{"to":{"enabled":["foo bar <foo@bar.com>"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(body).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusNotFound, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPAlertReceiverNotFound)

		require.True(t, mM2M.AssertExpectations(t))
		require.True(t, mReceiver.AssertExpectations(t))
	})

	t.Run("Fail to set email recipients", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		firstName := "foo"
		lastName := "bar"
		email := "foo@bar.com"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return([]user{
			{
				FirstName: firstName,
				LastName:  lastName,
				Email:     email,
			},
		}, nil).Once()

		mReceiver := &ReceiverMock{}
		mReceiver.On("SetReceiverEmailRecipients", mock.Anything, tenantID, id, []models.EmailAddress{
			{
				FirstName: firstName,
				LastName:  lastName,
				Email:     email,
			},
		}).Return(errors.New("mock error")).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m:       mM2M,
			receivers: mReceiver,
		})

		body := []byte(`{"emailConfig":{"to":{"enabled":["foo bar <foo@bar.com>"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(body).GoWithHTTPHandler(t, server)

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		httpErr := &api.HttpError{}
		require.NoError(t, json.Unmarshal(body, httpErr))

		require.Equal(t, http.StatusInternalServerError, httpErr.Code)
		require.Contains(t, httpErr.Message, errHTTPFailedToPatchAlertReceivers)

		require.True(t, mM2M.AssertExpectations(t))
		require.True(t, mReceiver.AssertExpectations(t))
	})

	t.Run("Succeeded to update email recipients", func(t *testing.T) {
		id := uuid.New()
		tenantID := "edgenode"

		firstName := "foo"
		lastName := "bar"
		email := "foo@bar.com"

		mM2M := &M2MAuthenticatorMock{}
		mM2M.On("GetUserList", mock.Anything).Return([]user{
			{
				FirstName: firstName,
				LastName:  lastName,
				Email:     email,
			},
		}, nil).Once()

		mReceiver := &ReceiverMock{}
		mReceiver.On("SetReceiverEmailRecipients", mock.Anything, tenantID, id, []models.EmailAddress{
			{
				FirstName: firstName,
				LastName:  lastName,
				Email:     email,
			},
		}).Return(nil).Once()

		// Creating new Echo server
		server := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(server, &ServerInterfaceHandler{
			m2m:       mM2M,
			receivers: mReceiver,
		})

		body := []byte(`{"emailConfig":{"to":{"enabled":["foo bar <foo@bar.com>"]}}}`)

		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		result := testutil.NewRequest().WithHeader("ActiveProjectID", tenantID).Patch(uri).WithBody(body).GoWithHTTPHandler(t, server)

		require.Equal(t, http.StatusNoContent, result.Recorder.Code)

		require.True(t, mM2M.AssertExpectations(t))
		require.True(t, mReceiver.AssertExpectations(t))
	})
}

func TestGetStatus(t *testing.T) {
	t.Run("Error - Could not reach alert manager", func(t *testing.T) {
		configfile := conf
		configfile.AlertManager.URL = "dummy-alert-manager:8080"
		serverInterface := NewServerInterfaceHandler(configfile, &gorm.DB{}, nil, logger)

		// Creating new Echo server
		e := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(e, serverInterface)

		result := testutil.NewRequest().Get("/api/v1/status").GoWithHTTPHandler(t, e)
		require.Equal(t, http.StatusOK, result.Recorder.Code, "Response code does not equal 200")

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		status := &api.ServiceStatus{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err, "Unexpected error unmarshalling response: %v", err)
		require.Equal(t, api.Failed, status.State)
	})

	t.Run("Error - Could not reach mimir ruler", func(t *testing.T) {
		configfile := conf

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/status" {
				w.WriteHeader(http.StatusOK)
				err := json.NewEncoder(w).Encode(alertManagerInfo{
					Cluster: alertManagerStatus{
						Status: "ready",
					},
				})
				require.NoError(t, err)
			}
		}))
		defer server.Close()

		configfile.AlertManager.URL = server.URL
		serverInterface := NewServerInterfaceHandler(configfile, &gorm.DB{}, nil, logger)

		// Creating new Echo server
		e := echo.New()

		// Registering API call handlers
		api.RegisterHandlers(e, serverInterface)

		result := testutil.NewRequest().Get("/api/v1/status").GoWithHTTPHandler(t, e)
		require.Equal(t, http.StatusOK, result.Recorder.Code, "Response code does not equal 200")

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		status := &api.ServiceStatus{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err, "Unexpected error unmarshalling response: %v", err)
		require.Equal(t, api.Failed, status.State)
	})

	t.Run("Status Failed - Alert manager is not ready", func(t *testing.T) {
		configfile := conf

		// Creating new Echo server
		e := echo.New()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/status" {
				w.WriteHeader(http.StatusOK)
				err := json.NewEncoder(w).Encode(alertManagerInfo{
					Cluster: alertManagerStatus{
						Status: "settling",
					},
				})
				require.NoError(t, err)
			}
		}))
		defer server.Close()

		configfile.AlertManager.URL = server.URL
		serverInterface := NewServerInterfaceHandler(configfile, &gorm.DB{}, nil, logger)

		// Registering API call handlers
		api.RegisterHandlers(e, serverInterface)

		result := testutil.NewRequest().Get("/api/v1/status").GoWithHTTPHandler(t, e)
		require.Equal(t, http.StatusOK, result.Recorder.Code, "Response code does not equal 200")

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		status := &api.ServiceStatus{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err, "Unexpected error unmarshalling response: %v", err)
		require.Equal(t, api.Failed, status.State)
	})

	t.Run("Status Failed - Mimir ruler not reachable", func(t *testing.T) {
		configfile := conf

		// Creating new Echo server
		e := echo.New()

		alertSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/status" {
				w.WriteHeader(http.StatusOK)
				err := json.NewEncoder(w).Encode(alertManagerInfo{
					Cluster: alertManagerStatus{
						Status: "ready",
					},
				})
				require.NoError(t, err)
			}
		}))
		defer alertSrv.Close()

		mimirSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ready" {
				w.WriteHeader(http.StatusUnauthorized)
			}
		}))
		defer mimirSrv.Close()

		configfile.AlertManager.URL = alertSrv.URL
		configfile.Mimir.RulerURL = mimirSrv.URL
		serverInterface := NewServerInterfaceHandler(configfile, &gorm.DB{}, nil, logger)

		// Registering API call handlers
		api.RegisterHandlers(e, serverInterface)

		result := testutil.NewRequest().Get("/api/v1/status").GoWithHTTPHandler(t, e)
		require.Equal(t, http.StatusOK, result.Recorder.Code, "Response code does not equal 200")

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		// TODO: Needs better distinction on which one of the server connections failed.
		status := &api.ServiceStatus{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err, "Unexpected error unmarshalling response: %v", err)
		require.Equal(t, api.Failed, status.State)
	})

	t.Run("Ready", func(t *testing.T) {
		configfile := conf

		// Creating new Echo server
		e := echo.New()

		alertSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/status" {
				w.WriteHeader(http.StatusOK)
				err := json.NewEncoder(w).Encode(alertManagerInfo{
					Cluster: alertManagerStatus{
						Status: "ready",
					},
				})
				require.NoError(t, err)
			}
		}))
		defer alertSrv.Close()

		namespace := "test-namespace"
		mimirSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/prometheus/config/v1/rules/"+namespace {
				orgID := r.Header.Get("X-Scope-OrgID")
				if len(orgID) == 0 {
					w.WriteHeader(http.StatusUnauthorized)
				} else {
					w.WriteHeader(http.StatusOK)
				}
			}
		}))
		defer mimirSrv.Close()

		configfile.AlertManager.URL = alertSrv.URL
		configfile.Mimir.RulerURL = mimirSrv.URL
		configfile.Mimir.Namespace = namespace
		serverInterface := NewServerInterfaceHandler(configfile, &gorm.DB{}, nil, logger)

		// Registering API call handlers
		api.RegisterHandlers(e, serverInterface)

		result := testutil.NewRequest().Get("/api/v1/status").GoWithHTTPHandler(t, e)
		require.Equal(t, http.StatusOK, result.Recorder.Code, "Response code does not equal 200")

		body, err := io.ReadAll(result.Recorder.Body)
		require.NoError(t, err)

		status := &api.ServiceStatus{}
		err = json.Unmarshal(body, &status)
		require.NoError(t, err, "Unexpected error unmarshalling response: %v", err)
		require.Equal(t, api.Ready, status.State)
	})
}
