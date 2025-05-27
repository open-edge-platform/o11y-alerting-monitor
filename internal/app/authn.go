// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type AuthenticationHandler struct {
	oidcServer string
	oidcRealm  string
}

func NewAuthenticationHandler(oidcServer string, oidcRealm string) *AuthenticationHandler {
	return &AuthenticationHandler{
		oidcServer: oidcServer,
		oidcRealm:  oidcRealm,
	}
}

func (w *AuthenticationHandler) logError(ctx echo.Context, message string, err error, attrs ...slog.Attr) {
    slog.LogAttrs(ctx.Request().Context(), slog.LevelError, message,
        slog.String("path", ctx.Path()),
        slog.String("error", err.Error()),
		slog.String("additional_info", "test-error"),
    )
}

func (ah *AuthenticationHandler) authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := ah.ensureAuthenticated(c)
		if err != nil {
			ah.logError(c, "Failed to authenticate token", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "Failed to authenticate token")
		}
		return next(c)
	}
}

func (ah *AuthenticationHandler) getOIDCServerEndpoint() string {
	if ah.oidcServer == "" || ah.oidcRealm == "" {
		return ""
	}
	endpoint := fmt.Sprintf("realms/%s/protocol/openid-connect/certs", ah.oidcRealm)
	return fmt.Sprintf("%s/%s", ah.oidcServer, endpoint)
}

func (ah *AuthenticationHandler) ensureAuthenticated(c echo.Context) error {
	// skipping authentication for /status endpoint
	if skipAuth(c) {
		return nil
	}

	// Extracting JWT
	authorizationHeader := c.Request().Header.Get("Authorization")

	jwtB64, err := getB64JWT(authorizationHeader)
	if err != nil {
		return err
	}

	// Extracting JWKS
	jwksURL := ah.getOIDCServerEndpoint()
	if jwksURL == "" {
		return errors.New("OIDC server and/or realm not specified")
	}
	jwks, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return fmt.Errorf("failed to create JWKS from the resource at the given URL: %w", err)
	}

	// Parsing JWT.
	token, err := jwt.Parse(jwtB64, jwks.Keyfunc)
	if err != nil {
		return fmt.Errorf("error while parsing JWT: %w", err)
	}

	// Validating token
	if !token.Valid {
		return errors.New("invalid token")
	}
	return nil
}
