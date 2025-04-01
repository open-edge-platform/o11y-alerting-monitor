// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type AlertManagerConfig struct {
	URL                string `yaml:"url"`
	RequireTLS         bool   `yaml:"requireTLS"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
	Namespace          string `yaml:"namespace"`
}

type MimirConfig struct {
	Namespace string `yaml:"namespace"`
	RulerURL  string `yaml:"rulerURL"`
}

type VaultConfig struct {
	Host             string `yaml:"host"`
	ExpirationPeriod string `yaml:"expirationPeriod"`
	KubernetesRole   string `yaml:"kubernetesRole"`
}

type TaskExecutorConfig struct {
	UUIDLimit     int           `yaml:"uuidLimit"`
	RetryLimit    int           `yaml:"retryLimit"`
	TaskTimeout   time.Duration `yaml:"taskTimeout"`
	RetentionTime time.Duration `yaml:"retentionTime"`
	PoolingRate   time.Duration `yaml:"dbPoolingRate"`
}

type Config struct {
	AlertManager AlertManagerConfig `yaml:"alertmanager"`
	Mimir        MimirConfig        `yaml:"mimir"`
	Keycloak     struct {
		M2MClient string `yaml:"m2mClient"`
	} `yaml:"keycloak"`
	Vault          VaultConfig `yaml:"vault"`
	Authentication struct {
		OidcServer      string `yaml:"oidcServer"`
		OidcServerRealm string `yaml:"oidcServerRealm"`
	} `yaml:"authentication"`
	TaskExecutor TaskExecutorConfig `yaml:"taskExecutor"`
}

func LoadConfig(file string) (Config, error) {
	yfile, err := os.ReadFile(file)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read file %q: %w", file, err)
	}

	var config Config
	err = yaml.Unmarshal(yfile, &config)
	if err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal: %w", err)
	}
	return config, nil
}
