// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

const filterAnnotationsTestData = `[{"annotations":{"am_test":"test","am_uuid":"c6b2a291-a9a2-49d2-930f-f865457b1aa8","foo":"bar"},` +
	`"endsAt":"2024-01-23T16:13:45.535+01:00","fingerprint":"0c8d24dab761f647",` +
	`"receivers":[{"name":"web.hook"}],"startsAt":"2024-01-23T16:08:45.535+01:00",` +
	`"status":{"inhibitedBy":[],"silencedBy":[],"state":"active"},` +
	`"updatedAt":"2024-01-23T16:08:45.535+01:00",` +
	`"labels":{"alertname":"foo2","cluster_name":"test",` +
	`"host_uuid":"93bf6804-52a3-4ba1-a919-c7ef65a9cdef","node":"bar",` +
	`"deployment_id":"1c87a656-594d-4300-b4ad-630914e11856"}}]`

const filterAnnotationsExpected = `[{"alertDefinitionId":"c6b2a291-a9a2-49d2-930f-f865457b1aa8",` +
	`"annotations":{"foo":"bar"},` +
	`"endsAt":"2024-01-23T16:13:45.535+01:00","fingerprint":"0c8d24dab761f647",` +
	`"receivers":[{"name":"web.hook"}],"startsAt":"2024-01-23T16:08:45.535+01:00",` +
	`"status":{"inhibitedBy":[],"silencedBy":[],"state":"active"},` +
	`"updatedAt":"2024-01-23T16:08:45.535+01:00",` +
	`"labels":{"alertname":"foo2","cluster_name":"test",` +
	`"host_uuid":"93bf6804-52a3-4ba1-a919-c7ef65a9cdef","node":"bar",` +
	`"deployment_id":"1c87a656-594d-4300-b4ad-630914e11856"}}]`

const filterWithMaintenanceAlertTestData = `[{"alertDefinitionId": "4c57b59e-8243-445d-beb1-9aef315c5100",` +
	`"annotations": {"description": "No connection to host ec2d7a14-ace3-5979-5961-6ce552eec60f",` +
	`"summary": "Lost connection to host ec2d7a14-ace3-5979-5961-6ce552eec60f"},` +
	`"endsAt": "2024-02-13T15:38:10.244Z",` +
	`"fingerprint": "3c564ba1b47ec6d5",` +
	`"labels": {"alert_category": "health","alert_context": "host","alertname": "HostStatusConnectionLost",` +
	`"deviceGuid": "ec2d7a14-ace3-5979-5961-6ce552eec60f","duration": "30s","hostID": "host-b6075c4e",` +
	`"host_uuid": "ec2d7a14-ace3-5979-5961-6ce552eec60f","http_scheme": "http","instance": "infra-exporter.orch-infra:9101",` +
	`"job": "infra-exporter","net_host_name": "infra-exporter.orch-infra","net_host_port": "9101",` +
	`"serial": "ec2d7a14-ace3-5979-5961-6ce552eec60f","service_instance_id": "infra-exporter.orch-infra:9101",` +
	`"service_name": "infra-exporter","status": "HOST_STATUS_CONNECTION_LOST","threshold": "1"},` +
	`"startsAt": "2024-02-13T07:40:40.244Z",` +
	`"status": {"state": "suppressed"},` +
	`"updatedAt": "2024-02-13T15:33:10.266Z"},` +
	`{"alertDefinitionId": "fcddd571-6028-48fd-88c5-c0e598b4cbe2",` +
	`"annotations": {"description": "Maintenance alert is used for inhibited alerts from host ec2d7a14-ace3-5979-5961-6ce552eec60f",` +
	`"summary": "Maintenance set on host ec2d7a14-ace3-5979-5961-6ce552eec60f"},` +
	`"endsAt": "2024-02-13T15:38:02.442Z","fingerprint": "bcd94c7377dc5547",` +
	`"labels": {"alert_category": "maintenance","alert_context": "host","alertname": "HostMaintenance",` +
	`"deviceGuid": "ec2d7a14-ace3-5979-5961-6ce552eec60f","duration": "30s","hostID": "host-b6075c4e",` +
	`"host_uuid": "ec2d7a14-ace3-5979-5961-6ce552eec60f","http_scheme": "http","instance": "infra-exporter.orch-infra:9101",` +
	`"job": "infra-exporter","net_host_name": "infra-exporter.orch-infra","net_host_port": "9101",` +
	`"serial": "ec2d7a14-ace3-5979-5961-6ce552eec60f","service_instance_id": "infra-exporter.orch-infra:9101",` +
	`"service_name": "infra-exporter","threshold": "0"},"startsAt": "2024-02-13T07:40:32.442Z",` +
	`"status": {"state": "active"},"updatedAt": "2024-02-13T15:33:02.514Z"}]`

const filterWithMaintenanceAlertExpected = `[{"alertDefinitionId": "4c57b59e-8243-445d-beb1-9aef315c5100",` +
	`"annotations": {"description": "No connection to host ec2d7a14-ace3-5979-5961-6ce552eec60f",` +
	`"summary": "Lost connection to host ec2d7a14-ace3-5979-5961-6ce552eec60f"},` +
	`"endsAt": "2024-02-13T15:38:10.244Z",` +
	`"fingerprint": "3c564ba1b47ec6d5",` +
	`"labels": {"alert_category": "health","alert_context": "host","alertname": "HostStatusConnectionLost",` +
	`"deviceGuid": "ec2d7a14-ace3-5979-5961-6ce552eec60f","duration": "30s","hostID": "host-b6075c4e",` +
	`"host_uuid": "ec2d7a14-ace3-5979-5961-6ce552eec60f","http_scheme": "http","instance": "infra-exporter.orch-infra:9101",` +
	`"job": "infra-exporter","net_host_name": "infra-exporter.orch-infra","net_host_port": "9101",` +
	`"serial": "ec2d7a14-ace3-5979-5961-6ce552eec60f","service_instance_id": "infra-exporter.orch-infra:9101",` +
	`"service_name": "infra-exporter","status": "HOST_STATUS_CONNECTION_LOST","threshold": "1"},` +
	`"startsAt": "2024-02-13T07:40:40.244Z",` +
	`"status": {"state": "suppressed"},` +
	`"updatedAt": "2024-02-13T15:33:10.266Z"}]`

func TestGetProjectAlertsParamsToURL(t *testing.T) {
	active := true
	alert := "test_alert"
	app := "test_app"
	cluster := "test_cluster"
	host := "test_host"
	suppressed := true

	inputData := api.GetProjectAlertsParams{
		Alert:      &alert,
		Host:       &host,
		Cluster:    &cluster,
		App:        &app,
		Active:     &active,
		Suppressed: &suppressed,
	}

	expectedOutputData := url.Values{
		"active":    {"true"},
		"filter":    {"alertname=test_alert", "host_uuid=test_host", "cluster_name=test_cluster", "deployment_id=test_app"},
		"inhibited": {"true"},
		"silenced":  {"true"},
	}

	output := getAlertsParamsToURL(inputData)

	require.Equal(t, expectedOutputData, output, "Output data is different from expected")
}

func TestFilterAnnotations(t *testing.T) {
	unmarshalledInput := new(api.AlertList)
	unmarshalledExpected := new(api.AlertList)

	err := json.Unmarshal([]byte(filterAnnotationsTestData), &unmarshalledInput.Alerts)
	require.NoError(t, err, "Error unmarshalling input data")

	err = json.Unmarshal([]byte(filterAnnotationsExpected), &unmarshalledExpected.Alerts)
	require.NoError(t, err, "Error unmarshalling expected json")

	err = filterAnnotations(unmarshalledInput.Alerts)
	require.NoError(t, err, "Error filtering annotations")
	require.Equal(t, unmarshalledExpected, unmarshalledInput, "Output data is different from expected")
}

func TestGetAlertManagerStatus(t *testing.T) {
	t.Run("Invalid alert manager URL", func(t *testing.T) {
		status, err := getAlertManagerStatus("http://alertmanager:-")
		require.Empty(t, status)
		require.ErrorContains(t, err, "failed to parse alert manager url")
	})

	t.Run("Error reaching alert manager", func(t *testing.T) {
		status, err := getAlertManagerStatus("http:dummy-alertmanager:8888")
		require.Empty(t, status)
		require.ErrorContains(t, err, "failed to send request")
	})

	t.Run("Response code not 200", func(t *testing.T) {
		// Start local HTTP server
		statusCode := http.StatusInternalServerError
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/status" {
				w.WriteHeader(statusCode)
			}
		}))
		defer server.Close()

		status, err := getAlertManagerStatus(server.URL)
		require.Empty(t, status)
		require.ErrorContains(t, err, fmt.Sprintf("alert manager returned status code: %v", statusCode))
	})

	t.Run("Malformed response body", func(t *testing.T) {
		// Start local HTTP server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err := io.WriteString(w, `{"cluster":{"status":"ready"}`)
				require.NoError(t, err)
			}
		}))
		defer server.Close()

		status, err := getAlertManagerStatus(server.URL)
		require.Empty(t, status)
		require.ErrorContains(t, err, "failed to unmarshal response")
	})

	t.Run("Status successfully retrieved", func(t *testing.T) {
		// Start local HTTP server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/status" {
				w.Header().Set("Content-Type", "application/json")
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

		status, err := getAlertManagerStatus(server.URL)
		require.NoError(t, err)
		require.Equal(t, "ready", status)
	})
}

func TestIsMimirRulerReachable(t *testing.T) {
	t.Run("Invalid mimir ruler URL", func(t *testing.T) {
		ok, err := isMimirRulerReachable("http://mimir-ruler:-")
		require.False(t, ok)
		require.ErrorContains(t, err, "failed to parse mimir ruler url")
	})

	t.Run("Server reachable, returns status code 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate "/ready" endpoint
			if r.URL.Path == "/ready" {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		ok, err := isMimirRulerReachable(server.URL)
		require.True(t, ok)
		require.NoError(t, err)
	})

	t.Run("Server reachable, returns non-200 status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ready" {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		// Call the function with the test server's URL
		ok, err := isMimirRulerReachable(server.URL)
		require.False(t, ok)
		require.Error(t, err)
		require.Contains(t, err.Error(), "mimir returned status code")
	})
}

func TestConvertEmailFormat(t *testing.T) {
	var user1 = user{
		FirstName:     "Foo",
		LastName:      "Bar",
		EmailVerified: true,
		ID:            "21231-23002",
		Username:      "test",
		Email:         "testmail@test.com"}
	var user2 = user{
		FirstName:     "Foo2",
		LastName:      "Bar",
		EmailVerified: true,
		ID:            "23431-23122",
		Username:      "test",
		Email:         "testmail2@test.com"}
	var user3 = user{
		FirstName:     "Foo3",
		LastName:      "Bar",
		EmailVerified: true,
		ID:            "23341-23352",
		Username:      "test",
		Email:         "testmail3@test.com"}
	var user4 = user{
		FirstName:     "",
		LastName:      "",
		EmailVerified: false,
		ID:            "23291-23292",
		Username:      "admin",
		Email:         ""}

	var expectedEmailOutput = api.EmailRecipientList{
		"Foo Bar <testmail@test.com>", "Foo2 Bar <testmail2@test.com>", "Foo3 Bar <testmail3@test.com>"}

	userList := []user{user1, user2, user3, user4}
	formattedEmails := convertEmailFormat(userList)
	require.ElementsMatch(t, expectedEmailOutput, formattedEmails)
}

func TestEmailRegex(t *testing.T) {
	f := func(in string, exp []string) {
		t.Helper()

		out := EmailRegex.FindStringSubmatch(in)
		if len(out) != 0 {
			out = out[1:]
		}

		require.Equal(t, exp, out)
	}

	f("", nil)
	f("<foo@bar.com>", nil)
	f("user user@mail.com", nil)

	f("user <user@mail.com>", []string{"", "user", "user@mail.com"})
	f("name1 name2 lastname <example@mail.com>", []string{"name1 name2", "lastname", "example@mail.com"})
	f("name (nickname) lastname <foo@bar.com>", []string{"name (nickname)", "lastname", "foo@bar.com"})
	f("one two [group] <ex@mail.com>", []string{"one two", "[group]", "ex@mail.com"})
	f("one [two] three (group) <email>", []string{"one [two] three", "(group)", "email"})
}

func TestGetEmailSender_Success(t *testing.T) {
	f := func(from string, expectedFirst, expectedLast, expectedEmail string) {
		t.Helper()

		firstName, lastName, email, err := GetEmailSender(from)
		require.NoError(t, err, "didn't expect an error but got one for input: %q, error: %v", from, err)

		require.Equal(t, expectedFirst, firstName, "unexpected firstName; got %q; want %q", firstName, expectedFirst)
		require.Equal(t, expectedLast, lastName, "unexpected email; got %q; want %q", email, expectedEmail)
		require.Equal(t, expectedEmail, email, "unexpected email; got %q; want %q", email, expectedEmail)
	}

	// Valid full format with name and email
	f("John Doe <john.doe@example.com>", "John", "Doe", "john.doe@example.com")

	// Valid full format with more names and email
	f("John James Doe <john.doe@example.com>", "John James", "Doe", "john.doe@example.com")

	// Valid simple email format
	f("jane.doe@example.com", "Open Edge Platform", "Alert", "jane.doe@example.com")

	f("<jane.doe@example.com>", "Open Edge Platform", "Alert", "jane.doe@example.com")
}

func TestGetEmailSender_Failure(t *testing.T) {
	f := func(from string) {
		t.Helper()

		_, _, _, err := GetEmailSender(from)
		require.Error(t, err, "expected error for input %q but got none", from)
	}

	// Invalid email format without angle brackets
	f("Invalid Format")

	// Not matching any format
	f("Jane <jane.doe@example.com")
}

func TestParseEmailRecipients(t *testing.T) {
	f := func(in []string, exp []models.EmailAddress, expErr error) {
		t.Helper()

		out, err := parseEmailRecipients(in)
		require.Equal(t, exp, out)
		if expErr != nil {
			require.ErrorContains(t, err, expErr.Error())
		} else {
			require.NoError(t, err)
		}
	}

	// Positive test cases.
	f([]string{}, []models.EmailAddress{}, nil)
	f([]string{"Site Reliability (SRE) <sre@example.com>"}, []models.EmailAddress{
		{
			FirstName: "Site Reliability",
			LastName:  "(SRE)",
			Email:     "sre@example.com",
		},
	}, nil)
	f([]string{"Admin <admin@mail.com>"}, []models.EmailAddress{
		{
			FirstName: "",
			LastName:  "Admin",
			Email:     "admin@mail.com",
		},
	}, nil)

	// Invalid format of email recipient.
	f([]string{""}, nil, errors.New("invalid format for email recipient"))
	f([]string{"user foo@bar>"}, nil, errors.New("invalid format for email recipient"))
	f([]string{
		"admin <admin@mail.com>",
		"foo bar@mail.com", // invalid format, missing angle brackets
	}, nil, errors.New("invalid format for email recipient"))

	// Duplicate email recipient.
	f([]string{
		"admin <admin@mail.com>",
		"Site Reliability (SRE) <sre@example.com>",
		"admin <admin@mail.com>", // duplicate email recipient
	}, nil, errors.New("duplicate email recipient"))
}

func TestSkipAuth(t *testing.T) {
	testCases := []struct {
		name     string
		endpoint string
		expSkip  bool
	}{
		{
			name:     "True",
			endpoint: "/api/v1/status",
			expSkip:  true,
		},
		{
			name:     "False",
			endpoint: "/api/v1/service",
			expSkip:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create new Echo server
			e := echo.New()

			// Create request
			r, err := http.NewRequest(http.MethodGet, tc.endpoint, nil)
			require.NoError(t, err)

			// Create request context
			c := e.NewContext(r, nil)
			require.Equal(t, tc.expSkip, skipAuth(c))
		})
	}
}

func TestFilterOutMaintenanceAlerts(t *testing.T) {
	unmarshalledInput := new(api.AlertList)
	unmarshalledExpected := new(api.AlertList)

	err := json.Unmarshal([]byte(filterWithMaintenanceAlertTestData), &unmarshalledInput.Alerts)
	require.NoError(t, err, "Error unmarshalling input data")

	err = json.Unmarshal([]byte(filterWithMaintenanceAlertExpected), &unmarshalledExpected.Alerts)
	require.NoError(t, err, "Error unmarshalling expected json")

	filterOutMaintenanceAlerts(unmarshalledInput.Alerts)
	require.Equal(t, unmarshalledExpected, unmarshalledInput, "Output data is different from expected")
}

func TestParseAlertDefinitionValues(t *testing.T) {
	testCases := []struct {
		name      string
		request   api.PatchProjectAlertDefinitionJSONBody
		valuesExp *models.DBAlertDefinitionValues
		err       error
	}{
		{
			name: "Request body value field is nil",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: nil,
			},
			err: errors.New("request values is nil"),
		},
		{
			name: "Request does not have any value to set",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: &struct {
					Duration  *string `json:"duration,omitempty"`
					Enabled   *string `json:"enabled,omitempty"`
					Threshold *string `json:"threshold,omitempty"`
				}{
					Duration:  nil,
					Enabled:   nil,
					Threshold: nil,
				},
			},
			err: errors.New("request should contain at least one value to be set"),
		},
		{
			name: "Duration value of the request does not have a valid format",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: &struct {
					Duration  *string `json:"duration,omitempty"`
					Enabled   *string `json:"enabled,omitempty"`
					Threshold *string `json:"threshold,omitempty"`
				}{
					Duration:  stringPtr("12"),
					Enabled:   nil,
					Threshold: nil,
				},
			},
			err: errors.New("failed to parse duration value"),
		},
		{
			name: "Duration value of the request not in the order of seconds",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: &struct {
					Duration  *string `json:"duration,omitempty"`
					Enabled   *string `json:"enabled,omitempty"`
					Threshold *string `json:"threshold,omitempty"`
				}{
					Duration:  stringPtr("2us"),
					Enabled:   nil,
					Threshold: nil,
				},
			},
			err: errors.New("duration should be a non zero value in the order of seconds"),
		},
		{
			name: "Duration value of the request is zero",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: &struct {
					Duration  *string `json:"duration,omitempty"`
					Enabled   *string `json:"enabled,omitempty"`
					Threshold *string `json:"threshold,omitempty"`
				}{
					Duration:  stringPtr("0s"),
					Enabled:   nil,
					Threshold: nil,
				},
			},
			err: errors.New("duration should be a non zero value in the order of seconds"),
		},
		{
			name: "Threshold value of the request is non numeric",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: &struct {
					Duration  *string `json:"duration,omitempty"`
					Enabled   *string `json:"enabled,omitempty"`
					Threshold *string `json:"threshold,omitempty"`
				}{
					Duration:  nil,
					Enabled:   nil,
					Threshold: stringPtr("ten"),
				},
			},
			err: errors.New("failed to parse threshold value"),
		},
		{
			name: "Enabled value of the request is not a boolean",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: &struct {
					Duration  *string `json:"duration,omitempty"`
					Enabled   *string `json:"enabled,omitempty"`
					Threshold *string `json:"threshold,omitempty"`
				}{
					Duration:  nil,
					Enabled:   stringPtr("yes"),
					Threshold: nil,
				},
			},
			err: errors.New("failed to parse enabled value"),
		},
		{
			name: "Succeeded to parse request values",
			request: api.PatchProjectAlertDefinitionJSONBody{
				Values: &struct {
					Duration  *string `json:"duration,omitempty"`
					Enabled   *string `json:"enabled,omitempty"`
					Threshold *string `json:"threshold,omitempty"`
				}{
					Duration:  stringPtr("3m20s"),
					Enabled:   stringPtr("false"),
					Threshold: stringPtr("300"),
				},
			},
			valuesExp: &models.DBAlertDefinitionValues{
				Duration:  int64Ptr(200),
				Threshold: int64Ptr(300),
				Enabled:   boolPtr(false),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valuesOut, err := parseAlertDefinitionValues(tc.request)
			require.Equal(t, tc.valuesExp, valuesOut)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	testCases := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{
			name:     "Less than a second",
			input:    5 * time.Nanosecond,
			expected: "0s",
		},
		{
			name:     "Exactly 1 second",
			input:    1 * time.Second,
			expected: "1s",
		},
		{
			name:     "Exactly 60 seconds should equal to 1 minute",
			input:    60 * time.Second,
			expected: "1m",
		},
		{
			name:     "Exactly 60 minutes should equal to 1 hour",
			input:    60 * time.Minute,
			expected: "1h",
		},
		{
			name:     "15 seconds with unit transform to time.Duration",
			input:    time.Duration(int64(15)) * time.Second,
			expected: "15s",
		},
		{
			// This will not work for UI but it should work for us.
			name:     "90 seconds should be 1 minute 30 seconds",
			input:    90 * time.Second,
			expected: "1m30s",
		},
		{
			name:     "3 minutes",
			input:    180 * time.Second,
			expected: "3m",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := FormatDuration(tc.input)
			require.Equal(t, tc.expected, out)
		})
	}
}

func TestValidateRecipients(t *testing.T) {
	t.Helper()

	f := func(recipients, allowed api.EmailRecipientList, expErr error) {
		err := validateRecipients(recipients, allowed)
		if expErr != nil {
			require.ErrorContains(t, err, expErr.Error())
		} else {
			require.NoError(t, err)
		}
	}

	f(
		api.EmailRecipientList{"user <user@test.com>"},
		api.EmailRecipientList{"user <user@test.com>", "foo bar <foo@bar.com>"},
		nil,
	)

	f(
		api.EmailRecipientList{"foo bar <foo@bar.com>"},
		api.EmailRecipientList{"bar foo <foo@bar.com>"},
		fmt.Errorf("email recipient is not allowed: %q", "foo bar <foo@bar.com>"),
	)

	f(
		api.EmailRecipientList{
			"foo bar <foo@bar.com>",
			"foo1 bar <foo@bar.com>",
			"foo2 bar <foo@bar.com>",
		},
		api.EmailRecipientList{
			"foo bar <foo@bar.com>",
			"bar foo <foo@bar.com>",
		},
		fmt.Errorf("email recipient is not allowed: %q", "foo1 bar <foo@bar.com>"),
	)
}
