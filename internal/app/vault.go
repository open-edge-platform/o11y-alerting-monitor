// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	k8sauth "github.com/hashicorp/vault/api/auth/kubernetes"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
)

const vaultSecretName = "alert-monitor-client-secret"

type vaultConnection interface {
	getClientSecret(context.Context) (string, error)
	storeClientSecret(context.Context, string) error
}

type vault struct {
	client           *vaultapi.Client
	expirationPeriod time.Duration
}

func newVault(conf config.VaultConfig) (*vault, error) {
	if conf.KubernetesRole == "" {
		return nil, errors.New("no kubernetes role was configured")
	}

	expirationPeriod, err := time.ParseDuration(conf.ExpirationPeriod)
	if err != nil {
		return nil, fmt.Errorf("invalid expiration period configuration: %w", err)
	}

	vaultConfig := vaultapi.DefaultConfig()
	vaultConfig.Address = conf.Host

	client, err := vaultapi.NewClient(vaultConfig)
	if err != nil {
		return nil, err
	}

	return &vault{client, expirationPeriod}, nil
}

func (v *vault) renewToken(ctx context.Context, conf config.VaultConfig) error {
	if v.client == nil {
		return errors.New("vault client is not initialized")
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		authInfo, err := v.login(ctx, conf.KubernetesRole)
		if err != nil {
			return fmt.Errorf("failed to login to vault: %w", err)
		}

		err = v.manageTokenLifecycle(ctx, authInfo)
		if err != nil {
			return fmt.Errorf("failed to manage token lifecycle: %w", err)
		}
	}
}

func (v *vault) login(ctx context.Context, kubernetesRole string) (*vaultapi.Secret, error) {
	auth, err := k8sauth.NewKubernetesAuth(kubernetesRole)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize Kubernetes auth method: %w", err)
	}

	authInfo, err := v.client.Auth().Login(ctx, auth)
	if err != nil {
		return nil, fmt.Errorf("unable to log in to vault: %w", err)
	}
	if authInfo == nil {
		return nil, errors.New("no auth info was returned after login")
	}

	logger.Debug("Logged in to vault", slog.Any("LeaseDuration", authInfo.Auth.LeaseDuration), slog.Any("EntityID", authInfo.Auth.EntityID))

	return authInfo, nil
}

func (v *vault) manageTokenLifecycle(ctx context.Context, authInfo *vaultapi.Secret) error {
	watcher, err := v.client.NewLifetimeWatcher(&vaultapi.LifetimeWatcherInput{
		Secret: authInfo,
	})
	if err != nil {
		return fmt.Errorf("unable to create token renewal watcher: %w", err)
	}

	go watcher.Start()
	defer watcher.Stop()

	for {
		select {
		case renewal := <-watcher.RenewCh():
			logger.Debug("Successfully renewed Vault token",
				slog.Int("LeaseDuration", renewal.Secret.Auth.LeaseDuration), slog.Any("RenewalWarnings", renewal.Secret.Warnings))

		// `DoneCh` will return if renewal fails, or if the remaining lease duration is under a built-in threshold (which is set in vault config).
		case err := <-watcher.DoneCh():
			if err != nil {
				logger.Error("Vault token renewal failed, watcher stopped", slog.Any("error", err))
			}
			logger.Debug("Token can no longer be renewed. Re-attempting login")
			return nil

		case <-ctx.Done():
			return nil
		}
	}
}

func (v *vault) getClientSecret(ctx context.Context) (string, error) {
	secret, err := v.client.KVv2("secret").Get(ctx, vaultSecretName)
	if err != nil {
		return "", err
	}

	value, ok := secret.Data["clientSecret"].(string)
	if !ok {
		return "", errors.New("vault secret does not contain a client secret")
	}

	expirationDateString, ok := secret.Data["expirationDate"].(string)
	if !ok {
		return "", errors.New("vault secret does not contain an expiration date")
	}

	expirationDate, err := time.Parse(time.UnixDate, expirationDateString)
	if err != nil {
		return "", err
	}
	if time.Now().After(expirationDate) {
		return "", errors.New("client secret has expired")
	}

	return value, nil
}

func (v *vault) storeClientSecret(ctx context.Context, secret string) error {
	expirationDate := time.Now().Add(v.expirationPeriod).Format(time.UnixDate)
	secretData := map[string]interface{}{
		"clientSecret":   secret,
		"expirationDate": expirationDate,
	}

	_, err := v.client.KVv2("secret").Put(ctx, vaultSecretName, secretData)
	if err != nil {
		return err
	}

	return nil
}
