// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	opaURL = "http://localhost:8181/v1/data/httpapi/authz"
)

func authorize(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := ensureAuthorized(c)
		if err != nil {
			logError(c, logger, "Failed to authorize request", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "Failed to authorize request")
		}
		return next(c)
	}
}

type opaResponse struct {
	Result map[string]bool `json:"result"`
}

func checkAuthz(values map[string]map[string]interface{}) (opaResponse, error) {
	var response opaResponse
	jsonValues, err := json.Marshal(values)
	if err != nil {
		return opaResponse{}, err
	}

	resp, err := http.Post(opaURL, "application/json", bytes.NewBuffer(jsonValues))
	if err != nil {
		return opaResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return opaResponse{}, err
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return opaResponse{}, err
	}

	return response, nil
}

func ensureAuthorized(c echo.Context) error {
	// skipping authorization for /status endpoint
	if skipAuth(c) {
		return nil
	}

	authorizationHeader := c.Request().Header.Get("Authorization")
	token, err := getB64JWT(authorizationHeader)
	if err != nil {
		return err
	}

	roles, err := extractRolesFromJWT(token)
	if err != nil {
		return err
	}

	project := c.Request().Header.Get("ActiveProjectID")

	authPayload := map[string]map[string]interface{}{
		"input": {
			"roles":   roles,
			"project": project,
			"method":  c.Request().Method,
			"path":    trimPath(c.Request().URL.Path),
		},
	}

	resp, err := checkAuthz(authPayload)
	if err != nil {
		return fmt.Errorf("unable to check authorization: %w", err)
	}

	// request authorized if any policy agrees
	// returns nil to not trigger the error check
	for _, val := range resp.Result {
		if val {
			return nil
		}
	}
	return errors.New("access denied, no policy allowed this request")
}

func trimPath(url string) []string {
	tmppath := strings.TrimSpace(url)
	path := strings.Split(tmppath, "/")
	if path[0] == "/" || path[0] == "" {
		path = path[1:]
	}
	return path
}
