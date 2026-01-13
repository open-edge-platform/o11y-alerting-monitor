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

	"github.com/buger/jsonparser"
	"github.com/labstack/echo/v4"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
)

type user struct {
	ID            string `json:"id,omitempty"`
	Username      string `json:"username,omitempty"`
	EmailVerified bool   `json:"emailVerified,omitempty"`
	FirstName     string `json:"firstName,omitempty"`
	LastName      string `json:"lastName,omitempty"`
	Email         string `json:"email,omitempty"`
}

type header struct {
	key   string
	value string
}

type query struct {
	key   string
	value string
}

type requestData struct {
	httpMethod     string
	rawURL         string
	requestHeaders []header
	queries        []query
	postData       []byte
	user           *string
	secret         *string
}

type M2MConnection interface {
	GetUserList(echo.Context) ([]user, error)
}

type M2MAuthenticator struct {
	client          string
	oidcServer      string
	oidcRealm       string
	tokenEndpoint   string
	usersEndpoint   string
	clientsEndpoint string
	vault           vaultConnection
}

func NewM2MAuthenticator(conf config.Config, vault vaultConnection) (*M2MAuthenticator, error) {
	if conf.Authentication.OidcServer == "" {
		return nil, errors.New("OIDC server not specified")
	}

	if conf.Authentication.OidcServerRealm == "" {
		return nil, errors.New("OIDC realm not specified")
	}

	if conf.Keycloak.M2MClient == "" {
		return nil, errors.New("M2M client not specified")
	}

	oidcServer := conf.Authentication.OidcServer
	oidcRealm := conf.Authentication.OidcServerRealm
	client := conf.Keycloak.M2MClient

	tokenEndpoint := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", oidcServer, oidcRealm)
	usersEndpoint := fmt.Sprintf("%s/admin/realms/%s/users", oidcServer, oidcRealm)
	clientsEndpoint := fmt.Sprintf("%s/admin/realms/%s/clients", oidcServer, oidcRealm)

	return &M2MAuthenticator{
		client:          client,
		oidcServer:      oidcServer,
		oidcRealm:       oidcRealm,
		tokenEndpoint:   tokenEndpoint,
		usersEndpoint:   usersEndpoint,
		clientsEndpoint: clientsEndpoint,
		vault:           vault,
	}, nil
}

func (a *M2MAuthenticator) GetUserList(ctx echo.Context) ([]user, error) {
	requestHeaders := make([]header, 0, 2)

	m2mToken, err := a.getM2MToken(ctx)
	if err != nil {
		return nil, err
	}
	bearer := "Bearer " + m2mToken
	requestHeaders = append(
		requestHeaders,
		// Token
		header{"Authorization", bearer},
		// Content type
		header{"Content-Type", "application/json"},
	)

	requestData := requestData{
		httpMethod:     http.MethodGet,
		rawURL:         a.usersEndpoint,
		requestHeaders: requestHeaders}

	body, err := sendRequestToOIDC(requestData)
	if err != nil {
		return nil, err
	}

	var userList []user
	err = json.Unmarshal(body, &userList)
	if err != nil {
		return nil, err
	}

	return userList, nil
}

func (a *M2MAuthenticator) getM2MToken(ctx echo.Context) (string, error) {
	authorizationHeader := ctx.Request().Header.Get("Authorization")

	clientsecret, err := a.vault.getClientSecret(ctx.Request().Context())
	if err != nil {
		logWarn(ctx, fmt.Sprintf("Failed to retrieve client secret from vault, attempting to retrieve from keycloak: %v", err))

		jwtB64, err := getB64JWT(authorizationHeader)
		if err != nil {
			return "", err
		}

		clientID, err := a.getClientID(jwtB64)
		if err != nil {
			return "", err
		}

		clientsecret, err = a.getClientSecret(clientID, jwtB64)
		if err != nil {
			return "", err
		}

		err = a.vault.storeClientSecret(ctx.Request().Context(), clientsecret)
		if err != nil {
			return "", err
		}
	}

	clientToken, err := a.getClientToken(clientsecret)
	if err != nil {
		return "", err
	}

	return clientToken, nil
}

// Function for getting client ID from keycloak.
func (a *M2MAuthenticator) getClientID(token string) (string, error) {
	var bearer = "Bearer " + token
	requestHeaders := make([]header, 0, 2)
	queries := make([]query, 0, 1)
	requestHeaders = append(
		requestHeaders,
		// Token
		header{"Authorization", bearer},
		// Content type
		header{"Content-Type", "application/json"},
	)
	queries = append(queries, query{"clientId", a.client})

	requestData := requestData{
		httpMethod:     http.MethodGet,
		rawURL:         a.clientsEndpoint,
		requestHeaders: requestHeaders,
		queries:        queries,
	}

	body, err := sendRequestToOIDC(requestData)
	if err != nil {
		return "", err
	}

	var target []map[string]any
	err = json.Unmarshal(body, &target)
	if err != nil {
		return "", err
	}

	if len(target) == 0 {
		return "", errors.New("unmarshalled body has no data")
	}

	firstJSON, err := json.Marshal(target[0])
	if err != nil {
		return "", err
	}

	clientID, err := jsonparser.GetString(firstJSON, "id")
	if err != nil {
		return "", err
	}

	return clientID, nil
}

// Function for getting client secret from keycloak.
func (a *M2MAuthenticator) getClientSecret(clientID string, token string) (string, error) {
	secretsEndpoint := fmt.Sprintf("%s/admin/realms/%s/clients/%s/client-secret", a.oidcServer, a.oidcRealm, clientID)

	var bearer = "Bearer " + token
	requestHeaders := make([]header, 0, 2)
	requestHeaders = append(
		requestHeaders,
		// Token
		header{"Authorization", bearer},
		// Content type
		header{"Content-Type", "application/json"},
	)

	requestData := requestData{
		httpMethod:     http.MethodGet,
		rawURL:         secretsEndpoint,
		requestHeaders: requestHeaders}

	body, err := sendRequestToOIDC(requestData)
	if err != nil {
		return "", err
	}

	clientsecret, err := jsonparser.GetString(body, "value")
	if err != nil {
		return "", err
	}

	return clientsecret, nil
}

// Function for getting client token from keycloak.
func (a *M2MAuthenticator) getClientToken(secret string) (string, error) {
	requestHeaders := make([]header, 0, 1)
	requestHeaders = append(requestHeaders, header{"Content-Type", "application/x-www-form-urlencoded"})

	requestData := requestData{
		httpMethod:     http.MethodPost,
		rawURL:         a.tokenEndpoint,
		requestHeaders: requestHeaders,
		postData:       []byte("grant_type=client_credentials"),
		user:           &a.client,
		secret:         &secret}

	body, err := sendRequestToOIDC(requestData)
	if err != nil {
		return "", err
	}
	clientToken, err := jsonparser.GetString(body, "access_token")
	if err != nil {
		return "", err
	}

	return clientToken, nil
}

func sendRequestToOIDC(requestData requestData) ([]byte, error) {
	req, err := http.NewRequest(requestData.httpMethod, requestData.rawURL, bytes.NewBuffer(requestData.postData))
	if err != nil {
		return nil, err
	}
	// If user and secret are not nil, set basic auth.
	if requestData.user != nil && requestData.secret != nil {
		req.SetBasicAuth(*requestData.user, *requestData.secret)
	}

	if queries := requestData.queries; queries != nil {
		q := req.URL.Query()
		for i := range queries {
			query := queries[i]
			q.Add(query.key, query.value)
		}
		req.URL.RawQuery = q.Encode()
	}

	if headers := requestData.requestHeaders; headers != nil {
		for i := range headers {
			header := headers[i]
			req.Header.Add(header.key, header.value)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received not expected status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
