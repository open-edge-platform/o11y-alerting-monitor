// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/oapi-codegen/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
)

var expectedPatchDefinitionCodes = []int{
	http.StatusNoContent,
	http.StatusBadRequest,
	http.StatusInternalServerError,
}

var expectedPatchReceiverCodes = []int{
	http.StatusNoContent,
	http.StatusBadRequest,
	http.StatusInternalServerError,
	http.StatusServiceUnavailable,
}

func FuzzPatchAlertDefinitionRandomInput(f *testing.F) {
	id := uuid.New()
	tenantID := "edgenode"

	e := echo.New()

	// Mocking the Alert Definition.
	mDefinition := &DefinitionMock{}
	mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, mock.Anything).Return(nil).Once()

	handler := &ServerInterfaceHandler{
		definitions: mDefinition,
	}

	api.RegisterHandlers(e, handler)

	f.Fuzz(func(t *testing.T, payload []byte) {
		t.Logf("Testing with payload: %s\n", string(payload))

		req := testutil.NewRequest().
			WithHeader("ActiveProjectID", tenantID).
			Patch("/api/v1/alerts/definitions/01e74407-0327-4e36-93cb-85801c098ba5").
			WithJsonBody(payload).
			GoWithHTTPHandler(t, e)

		require.Contains(t, expectedPatchDefinitionCodes, req.Recorder.Code)
	})
}

func FuzzPatchAlertDefinitionDuration(f *testing.F) {
	id := uuid.New()
	tenantID := "edgenode"

	e := echo.New()

	// Mocking the Alert Definition.
	mDefinition := &DefinitionMock{}
	mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, mock.Anything).Return(nil).Once()

	handler := &ServerInterfaceHandler{
		definitions: mDefinition,
	}

	api.RegisterHandlers(e, handler)

	f.Fuzz(func(t *testing.T, duration string) {
		payload := []byte(fmt.Sprintf(`{"values":{"duration":%q,"enabled":"true","threshold":"100"}}`,
			duration,
		))
		t.Logf("Testing with payload: %s\n", string(payload))

		req := testutil.NewRequest().
			WithHeader("ActiveProjectID", tenantID).
			Patch("/api/v1/alerts/definitions/f44d04eb-8213-4002-b1bc-c0d5c8fa56c6").
			WithJsonBody(payload).
			GoWithHTTPHandler(t, e)

		require.Contains(t, expectedPatchDefinitionCodes, req.Recorder.Code)
	})
}

func FuzzPatchAlertDefinitionEnabled(f *testing.F) {
	id := uuid.New()
	tenantID := "edgenode"

	e := echo.New()

	// Mocking the Alert Definition.
	mDefinition := &DefinitionMock{}
	mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, mock.Anything).Return(nil).Once()

	handler := &ServerInterfaceHandler{
		definitions: mDefinition,
	}

	api.RegisterHandlers(e, handler)

	f.Fuzz(func(t *testing.T, enabled string) {
		payload := []byte(fmt.Sprintf(`{"values":{"duration":"10m,"enabled":%q,"threshold":"100"}}`,
			enabled,
		))
		t.Logf("Testing with payload: %s\n", string(payload))

		req := testutil.NewRequest().
			WithHeader("ActiveProjectID", tenantID).
			Patch("/api/v1/alerts/definitions/f44d04eb-8213-4002-b1bc-c0d5c8fa56c6").
			WithJsonBody(payload).
			GoWithHTTPHandler(t, e)

		require.Contains(t, expectedPatchDefinitionCodes, req.Recorder.Code)
	})
}

func FuzzPatchAlertDefinitionThreshold(f *testing.F) {
	id := uuid.New()
	tenantID := "edgenode"

	e := echo.New()

	// Mocking the Alert Definition.
	mDefinition := &DefinitionMock{}
	mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, mock.Anything).Return(nil).Once()

	handler := &ServerInterfaceHandler{
		definitions: mDefinition,
	}

	api.RegisterHandlers(e, handler)

	f.Fuzz(func(t *testing.T, threshold int) {
		payload := []byte(fmt.Sprintf(`{"values":{"duration":"10m","enabled":"true","threshold":%q}}`,
			threshold,
		))
		t.Logf("Testing with payload: %s\n", string(payload))

		req := testutil.NewRequest().
			WithHeader("ActiveProjectID", tenantID).
			Patch("/api/v1/alerts/definitions/f44d04eb-8213-4002-b1bc-c0d5c8fa56c6").
			WithJsonBody(payload).
			GoWithHTTPHandler(t, e)

		require.Contains(t, expectedPatchDefinitionCodes, req.Recorder.Code)
	})
}
func FuzzPatchAlertDefinitionAllInputs(f *testing.F) {
	id := uuid.New()
	tenantID := "edgenode"
	e := echo.New()

	// Mocking the Alert Definition.
	mDefinition := &DefinitionMock{}
	mDefinition.On("SetAlertDefinitionValues", mock.Anything, tenantID, id, mock.Anything).Return(nil).Once()

	handler := &ServerInterfaceHandler{
		definitions: mDefinition,
	}

	api.RegisterHandlers(e, handler)

	durationUnits := []string{"ns", "us", "ms", "s", "m", "h"}

	f.Fuzz(func(t *testing.T, duration int, enabled bool, threshold int) {
		for _, unit := range durationUnits {
			payload := []byte(fmt.Sprintf(`{"values":{"duration":%q,"enabled":%q,"threshold":%q}}`,
				fmt.Sprintf("%d%s", duration, unit),
				strconv.FormatBool(enabled),
				strconv.Itoa(threshold),
			))

			req := testutil.NewRequest().
				WithHeader("ActiveProjectID", tenantID).
				Patch("/api/v1/alerts/definitions/f44d04eb-8213-4002-b1bc-c0d5c8fa56c6").
				WithJsonBody(payload).
				GoWithHTTPHandler(t, e)

			require.Contains(t, expectedPatchDefinitionCodes, req.Recorder.Code)
		}
	})
}

func FuzzPatchAlertReceiverRandomInput(f *testing.F) {
	id := uuid.New()
	tenantID := "edgenode"

	e := echo.New()

	// Mocking the M2MAuthenticator.
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
	}, nil)

	// Mocking the Receiver.
	mReceiver := &ReceiverMock{}
	mReceiver.On("SetReceiverEmailRecipients", mock.Anything, tenantID, id, mock.Anything).Return(nil)

	api.RegisterHandlers(e, &ServerInterfaceHandler{
		m2m:       mM2M,
		receivers: mReceiver,
	})

	// Seed the fuzzer with a valid payload
	f.Add([]byte(`{"emailConfig": {"to": {"enabled": ["foo bar <foo@bar.com>"]}}}`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		t.Logf("Testing with payload: %s\n", string(payload))
		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())

		req := testutil.NewRequest().
			WithHeader("ActiveProjectID", tenantID).
			Patch(uri).
			WithBody(payload).
			GoWithHTTPHandler(t, e)

		require.Contains(t, expectedPatchReceiverCodes, req.Recorder.Code)
	})
}

func FuzzPatchAlertReceiverAddress(f *testing.F) {
	id := uuid.New()
	tenantID := "edgenode"

	e := echo.New()

	// Mocking the M2MAuthenticator.
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
	}, nil)

	// Mocking the Receiver.
	mReceiver := &ReceiverMock{}
	mReceiver.On("SetReceiverEmailRecipients", mock.Anything, tenantID, id, mock.Anything).Return(nil)

	api.RegisterHandlers(e, &ServerInterfaceHandler{
		m2m:       mM2M,
		receivers: mReceiver,
	})

	// Seed the fuzzer with a valid payload.
	f.Add("foo bar <foo@bar.com>")

	f.Fuzz(func(t *testing.T, address string) {
		uri := fmt.Sprintf("/api/v1/alerts/receivers/%v", id.String())
		payload := []byte(fmt.Sprintf(`{"emailConfig":{"to":{"enabled":[%q]}}}`, address))
		t.Logf("Testing with payload: %s\n", string(payload))

		req := testutil.NewRequest().
			WithHeader("ActiveProjectID", tenantID).
			Patch(uri).
			WithBody(payload).
			GoWithHTTPHandler(t, e)

		require.Contains(t, expectedPatchReceiverCodes, req.Recorder.Code)
	})
}
