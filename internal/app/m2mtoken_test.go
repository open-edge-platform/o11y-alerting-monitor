// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
)

type DummyVault struct{}

func (*DummyVault) getClientSecret(context.Context) (string, error) {
	return "FooBarSecret", nil
}

func (*DummyVault) storeClientSecret(context.Context, string) error {
	return nil
}

const oidcServerResponse = "{\"id\":\"FooBarID\"," +
	"\"value\":\"FooBarSecret\"," +
	"\"access_token\":\"FooBarToken\"}"

const oidcServerResponseID = "[{\"id\":\"FooBarID\"}," +
	"{\"id\":\"FooBarID2\"}," +
	"{\"id\":\"FooBarID3\"}]"

const oidcServerResponseUser = "[{\"id\":\"FooBarID\"," +
	"\"username\":\"FooBarUser\"," +
	"\"emailVerified\":true," +
	"\"firstName\":\"Foo\"," +
	"\"lastName\":\"Bar\"," +
	"\"email\":\"Foo Bar <testmail@test.com>\"}]"

func TestNewM2MClient(t *testing.T) {
	var conf = config.Config{
		Keycloak: struct {
			M2MClient string `yaml:"m2mClient"`
		}{M2MClient: ""},
		Authentication: struct {
			OidcServer      string `yaml:"oidcServer"`
			OidcServerRealm string `yaml:"oidcServerRealm"`
		}{
			OidcServer:      "",
			OidcServerRealm: "",
		},
	}
	_, err := NewM2MAuthenticator(conf, &DummyVault{})
	require.Error(t, err, "NewM2MClient function accepted an empty OIDC server")

	conf.Authentication.OidcServer = "test"
	_, err = NewM2MAuthenticator(conf, &DummyVault{})
	require.Error(t, err, "NewM2MClient function accepted an empty OIDC realm")

	conf.Authentication.OidcServerRealm = "test"
	_, err = NewM2MAuthenticator(conf, &DummyVault{})
	require.Error(t, err, "NewM2MClient function accepted an empty M2M client name")

	conf.Keycloak.M2MClient = "test"
	_, err = NewM2MAuthenticator(conf, &DummyVault{})
	require.NoError(t, err, "failed to create an M2MClient using a valid configuration")
}

func TestGetUserList(t *testing.T) {
	serverState := 0
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if serverState == 0 {
			fmt.Fprint(w, oidcServerResponse)
			serverState++
		} else {
			fmt.Fprint(w, oidcServerResponseUser)
		}
	}))
	defer svr.Close()

	req, err := http.NewRequest(http.MethodGet, "example.com", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)

	var conf = config.Config{
		Keycloak: struct {
			M2MClient string `yaml:"m2mClient"`
		}{M2MClient: "test"},
		Authentication: struct {
			OidcServer      string `yaml:"oidcServer"`
			OidcServerRealm string `yaml:"oidcServerRealm"`
		}{
			OidcServer:      svr.URL,
			OidcServerRealm: "master",
		},
	}
	m2m, err := NewM2MAuthenticator(conf, &DummyVault{})
	require.NoError(t, err)

	_, err = m2m.GetUserList(c)
	require.NoError(t, err, "getUserList function returned an error")
}

func TestGetClientID(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, oidcServerResponseID)
	}))
	defer svr.Close()

	var conf = config.Config{
		Keycloak: struct {
			M2MClient string `yaml:"m2mClient"`
		}{M2MClient: "test"},
		Authentication: struct {
			OidcServer      string `yaml:"oidcServer"`
			OidcServerRealm string `yaml:"oidcServerRealm"`
		}{
			OidcServer:      svr.URL,
			OidcServerRealm: "master",
		},
	}
	m2m, err := NewM2MAuthenticator(conf, &DummyVault{})
	require.NoError(t, err)

	id, err := m2m.getClientID("foo")
	require.NoError(t, err, "getClientID function returned an error")
	require.Equal(t, "FooBarID", id)
}

func TestGetClientSecret(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, oidcServerResponse)
	}))
	defer svr.Close()

	var conf = config.Config{
		Keycloak: struct {
			M2MClient string `yaml:"m2mClient"`
		}{M2MClient: "test"},
		Authentication: struct {
			OidcServer      string `yaml:"oidcServer"`
			OidcServerRealm string `yaml:"oidcServerRealm"`
		}{
			OidcServer:      svr.URL,
			OidcServerRealm: "master",
		},
	}
	m2m, err := NewM2MAuthenticator(conf, &DummyVault{})
	require.NoError(t, err)

	secret, err := m2m.getClientSecret("foo", "bar")
	require.NoError(t, err, "getClientID function returned an error")
	require.Equal(t, "FooBarSecret", secret)
}

func TestGetClientToken(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, oidcServerResponse)
	}))
	defer svr.Close()

	var conf = config.Config{
		Keycloak: struct {
			M2MClient string `yaml:"m2mClient"`
		}{M2MClient: "test"},
		Authentication: struct {
			OidcServer      string `yaml:"oidcServer"`
			OidcServerRealm string `yaml:"oidcServerRealm"`
		}{
			OidcServer:      svr.URL,
			OidcServerRealm: "master",
		},
	}
	m2m, err := NewM2MAuthenticator(conf, &DummyVault{})
	require.NoError(t, err)

	token, err := m2m.getClientToken("foo")
	require.NoError(t, err, "getClientID function returned an error")
	require.Equal(t, "FooBarToken", token)
}

func TestGetM2MToken(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, oidcServerResponse)
	}))
	defer svr.Close()

	req, err := http.NewRequest(http.MethodGet, "example.com", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)

	var conf = config.Config{
		Keycloak: struct {
			M2MClient string `yaml:"m2mClient"`
		}{M2MClient: "test"},
		Authentication: struct {
			OidcServer      string `yaml:"oidcServer"`
			OidcServerRealm string `yaml:"oidcServerRealm"`
		}{
			OidcServer:      svr.URL,
			OidcServerRealm: "master",
		},
	}
	m2m, err := NewM2MAuthenticator(conf, &DummyVault{})
	require.NoError(t, err)

	_, err = m2m.getM2MToken(c)
	require.NoError(t, err, "getM2MToken function returned an error")
}
