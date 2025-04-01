// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package alertmanager

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

const (
	// secret name of the secret that has the alertmanager configuration.
	secretName = "alert-monitor-config"
)

// AlertmanagerConfigurator updates the configuration manifest of an alertmanager instance given a receiver
// which comprises the list of email recipients.
type AlertmanagerConfigurator interface {
	UpdateReceiverConfig(ctx context.Context, receiver models.DBReceiver) error
}

// AlertManager refers to a standalone alertmanager instance. Implements UpdateReceiverConfig interface.
type AlertManager struct {
	client kubernetes.Interface

	config config.AlertManagerConfig
}

// New returns an AlertManager with the given configuration providing access to the Kubernetes API.
func New(conf config.AlertManagerConfig) (*AlertManager, error) {
	c, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes incluster config: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create new clientset: %w", err)
	}

	return &AlertManager{
		client: kubeClient,
		config: conf,
	}, nil
}

// UpdateReceiverConfig updates the configuration of the alertmanager manifest to match the list of email recipients
// of the given receiver.
func (am *AlertManager) UpdateReceiverConfig(ctx context.Context, receiver models.DBReceiver) error {
	manifest, err := getConfigManifest(ctx, am.config.Namespace, am.client)
	if err != nil {
		return fmt.Errorf("failed to get alertmanager config manifest: %w", err)
	}

	updatedManifest, err := manifest.ApplyReceiver(receiver, am.config)
	if err != nil {
		return fmt.Errorf("failed to apply receiver to alertmanager manifest: %w", err)
	}

	err = setConfigManifest(ctx, am.client, *updatedManifest, am.config.Namespace)
	if err != nil {
		return fmt.Errorf("failed to set alertmanager config manifest: %w", err)
	}
	return nil
}

// getConfigManifest takes a client with access to Kubernetes API and returns the config manifest of the
// alertmanager instance, which is stored as a secret.
func getConfigManifest(ctx context.Context, namespace string, client kubernetes.Interface) (*configManifest, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get alertmanager config secret: %w", err)
	}

	data := secret.Data["custom.yaml"]
	if data == nil {
		return nil, errors.New("config secret does not have \"custom.yaml\" field")
	}

	var manifest configManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal the content of the config secret: %w", err)
	}

	return &manifest, nil
}

// setConfigManifest takes a client with access to Kubernetes API and a config manifest. It sets the
// alertmanager config secret to match the given manifest.
func setConfigManifest(ctx context.Context, client kubernetes.Interface, manifest configManifest, namespace string) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal the content of the config secret: %w", err)
	}

	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get alertmanager config secret: %w", err)
	}

	secret.Data = map[string][]byte{
		"custom.yaml": data,
	}

	_, err = client.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update alertmanager config secret: %w", err)
	}

	return nil
}
