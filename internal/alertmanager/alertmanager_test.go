// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package alertmanager

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	testclient "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

const testNamespace = "orch-infra"

func TestGetConfigManifest(t *testing.T) {
	t.Run("Failed to get alertmanager config secret due to error", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset()

		fakeClient.Fake.PrependReactor("get", "secrets", func(_ ktesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("mock error")
		})

		manifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.Nil(t, manifest)
		require.ErrorContains(t, err, "failed to get alertmanager config secret")
	})

	t.Run("Alertmanager config secret has unexpected name", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-secret",
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": []byte("dummy config"),
			},
		})

		manifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.Nil(t, manifest)
		require.ErrorContains(t, err, "failed to get alertmanager config secret")
		require.ErrorContains(t, err, fmt.Sprintf("secrets %q not found", secretName))
	})

	t.Run("Alertmanager config secret not in the expected namespace", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: "test-namespace",
			},
			Data: map[string][]byte{
				"custom.yaml": []byte("dummy config"),
			},
		})

		manifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.Nil(t, manifest)
		require.ErrorContains(t, err, "failed to get alertmanager config secret")
		require.ErrorContains(t, err, fmt.Sprintf("secrets %q not found", secretName))
	})

	t.Run("Alertmanager config secret missing config.yaml field", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"data": []byte("dummy data"),
			},
		})

		manifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.Nil(t, manifest)
		require.ErrorContains(t, err, "config secret does not have \"custom.yaml\" field")
	})

	t.Run("Alertmanager config secret `custom.yaml` field content is not YAML format", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": []byte(`foo bar`),
			},
		})

		manifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.Nil(t, manifest)
		require.ErrorContains(t, err, "failed to unmarshal the content of the config secret")
	})

	t.Run("Successfully get the alertmanager config secret", func(t *testing.T) {
		data := []byte(`receivers:
  - name: 'alert-monitor-config-1'
    email_configs:
    - to: 'test receiver <test@receiver.com>'`)

		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": data,
			},
		})

		manifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Receivers: []receiver{
				{
					Name: "alert-monitor-config-1",
					EmailConfigs: []emailConfig{
						{
							To: "test receiver <test@receiver.com>",
						},
					},
				},
			},
		}, manifest)
	})
}

func TestSetConfigManifest(t *testing.T) {
	t.Run("Failed to get alertmanager config secret", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset()

		fakeClient.Fake.PrependReactor("get", "secrets", func(_ ktesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("mock error")
		})

		err := setConfigManifest(t.Context(), fakeClient, configManifest{}, testNamespace)

		require.ErrorContains(t, err, "failed to get alertmanager config secret")
	})

	t.Run("Alertmanager config secret has unexpected name", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-secret",
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": {},
			},
		})

		err := setConfigManifest(t.Context(), fakeClient, configManifest{
			Receivers: []receiver{
				{
					Name: "alert-monitor-config-1",
					EmailConfigs: []emailConfig{
						{
							To: "test receiver <test@receiver.com>",
						},
					},
				},
			},
		}, testNamespace)

		require.ErrorContains(t, err, "failed to get alertmanager config secret")
		require.ErrorContains(t, err, fmt.Sprintf("secrets %q not found", secretName))
	})

	t.Run("Alertmanager config secret not in the expected namespace", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: "test-namespace",
			},
			Data: map[string][]byte{
				"custom.yaml": []byte("dummy config"),
			},
		})

		err := setConfigManifest(t.Context(), fakeClient, configManifest{
			Receivers: []receiver{
				{
					Name: "alert-monitor-config-1",
					EmailConfigs: []emailConfig{
						{
							To: "test receiver <test@receiver.com>",
						},
					},
				},
			},
		}, testNamespace)

		require.ErrorContains(t, err, "failed to get alertmanager config secret")
		require.ErrorContains(t, err, fmt.Sprintf("secrets %q not found", secretName))
	})

	t.Run("Failed to set alertmanager config secret", func(t *testing.T) {
		data := []byte(`receivers:
  - name: 'alert-monitor-config-1'
    email_configs:
    - to: 'test receiver <test@receiver.com>'`)

		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": data,
			},
		})

		fakeClient.Fake.PrependReactor("update", "secrets", func(_ ktesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("mock error")
		})

		err := setConfigManifest(t.Context(), fakeClient, configManifest{}, testNamespace)

		require.ErrorContains(t, err, "failed to update alertmanager config secret")
	})

	t.Run("Successfully set alertmanager config secret", func(t *testing.T) {
		manifest := configManifest{
			Receivers: []receiver{
				{
					Name: "alert-monitor-config-1",
					EmailConfigs: []emailConfig{
						{
							To: "test receiver <test@receiver.com>",
						},
					},
				},
			},
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": {},
			},
		}

		fakeClient := testclient.NewSimpleClientset(secret)

		require.NoError(t, setConfigManifest(t.Context(), fakeClient, manifest, testNamespace))

		updatedManifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.NoError(t, err)
		require.Equal(t, manifest, *updatedManifest)
	})
}

func TestReceiverConfig_UpdateReceiverConfig(t *testing.T) {
	t.Run("FailToGetManifest", func(t *testing.T) {
		fakeClient := testclient.NewSimpleClientset()

		fakeClient.Fake.PrependReactor("get", "secrets", func(_ ktesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("mock error")
		})

		am := &AlertManager{
			client: fakeClient,
		}

		err := am.UpdateReceiverConfig(t.Context(), models.DBReceiver{})
		require.ErrorContains(t, err, "failed to get alertmanager config manifest")
	})

	t.Run("FailToApplyReceiver", func(t *testing.T) {
		data := []byte(`receivers:
  - name: test-receiver
    email_configs: []`)

		// mock getting the alertmanager config manifest.
		// returns an invalid manifest with no routes defined.
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": data,
			},
		})

		am := &AlertManager{
			client: fakeClient,
			config: config.AlertManagerConfig{
				Namespace: testNamespace,
			},
		}

		err := am.UpdateReceiverConfig(t.Context(), models.DBReceiver{
			Name:    "test-receiver",
			Version: 3,
			To: []string{
				"first user <first@user.com>",
				"second user <second@user.com>",
			},
			TenantID: "edgenode",
		})
		require.ErrorContains(t, err, "failed to apply receiver to alertmanager manifest")
	})

	t.Run("FailToSetManifest", func(t *testing.T) {
		emailRecipients := []string{
			"first user <first@user.com>",
		}

		receiver := models.DBReceiver{
			Name:     "receiver",
			TenantID: "tenant",
			Version:  3,
			To:       emailRecipients,
		}

		data := []byte(`receivers:
  - name: tenant-receiver-1
route:
  routes:
    - receiver: tenant-receiver-1`)

		// mock getting the alertmanager config manifest.
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": data,
			},
		})

		// mock setting the alertmanager config manifest.
		fakeClient.Fake.PrependReactor("update", "secrets", func(_ ktesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("mock error")
		})

		am := &AlertManager{
			client: fakeClient,
			config: config.AlertManagerConfig{
				RequireTLS:         true,
				InsecureSkipVerify: true,
				Namespace:          testNamespace,
			},
		}

		err := am.UpdateReceiverConfig(t.Context(), receiver)
		require.ErrorContains(t, err, "failed to set alertmanager config manifest")
	})

	t.Run("Updated", func(t *testing.T) {
		emailRecipients := []string{
			"first user <first@user.com>",
		}

		dbReceiver := models.DBReceiver{
			Name:     "receiver",
			TenantID: "tenant",
			Version:  3,
			To:       emailRecipients,
		}

		data := []byte(`receivers:
  - name: tenant-receiver-1
route:
  routes:
    - receiver: tenant-receiver-1`)

		// mock getting the alertmanager config manifest.
		fakeClient := testclient.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"custom.yaml": data,
			},
		})

		conf := config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: true,
			Namespace:          testNamespace,
		}

		am := &AlertManager{
			client: fakeClient,
			config: conf,
		}

		err := am.UpdateReceiverConfig(t.Context(), dbReceiver)
		require.NoError(t, err)

		updatedManifest, err := getConfigManifest(t.Context(), testNamespace, fakeClient)
		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Receivers: []receiver{
				{
					Name: "tenant-receiver-3",
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           emailRecipients[0],
							HTML:         emailHTMLTemplate,
							RequireTLS:   conf.RequireTLS,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: conf.InsecureSkipVerify,
							},
						},
					},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant-receiver-3",
						Matchers: []string{
							alertCategoryMatcher,
							`projectId=~"tenant"`,
						},
					},
				},
			},
		}, updatedManifest)
	})
}
