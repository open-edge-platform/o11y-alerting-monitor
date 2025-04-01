// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("Valid config file", func(t *testing.T) {
		configFile, err := LoadConfig("_testdata/test_config.yaml")
		require.NoError(t, err)
		require.Equal(t, "http://localhost:9093", configFile.AlertManager.URL, "Read value different from expected")
		require.Equal(t, "test-namespace", configFile.AlertManager.Namespace, "Read value different from expected")
		require.Equal(t, "http://localhost:8081", configFile.Mimir.RulerURL, "Read value different from expected")
		require.Equal(t, "test-namespace", configFile.Mimir.Namespace, "Read value different from expected")
		require.Equal(t, "host-manager-m2m-client", configFile.Keycloak.M2MClient, "Read value different from expected")
		require.Equal(t, "https://keycloak.kind.internal", configFile.Authentication.OidcServer, "Read value different from expected")
		require.Equal(t, "master", configFile.Authentication.OidcServerRealm, "Read value different from expected")
		require.Equal(t, 240*time.Hour, configFile.TaskExecutor.RetentionTime, "Read value different from expected")
		require.Equal(t, 10, configFile.TaskExecutor.RetryLimit, "Read value different from expected")
		require.Equal(t, 10*time.Minute, configFile.TaskExecutor.TaskTimeout, "Read value different from expected")
		require.Equal(t, 3, configFile.TaskExecutor.UUIDLimit, "Read value different from expected")
		require.Equal(t, 10*time.Second, configFile.TaskExecutor.PoolingRate, "Read value different from expected")
	})

	t.Run("Invalid config file name", func(t *testing.T) {
		_, err := LoadConfig("_testdata/invalid_file_name.yaml")
		require.Error(t, err)
	})

	t.Run("Invalid config file", func(t *testing.T) {
		_, err := LoadConfig("_testdata/test_config_malformed.yaml")
		require.Error(t, err)
	})
}
