// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package alertmanager

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/app"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

func TestConfigManifest_ApplyReceiver(t *testing.T) {
	t.Run("ManifestHasNoReceivers", func(t *testing.T) {
		dbReceiver := models.DBReceiver{
			Name:    "test-receiver",
			Version: 3,
			To: []string{
				"first user <first@user.com>",
				"second user <second@user.com>",
			},
			TenantID: "edgenode",
		}

		manifestIn := configManifest{
			Receivers: []receiver{},
		}

		conf := config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: true,
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

		require.ErrorContains(t, err, "alertmanager config manifest does not have receivers")
		require.Nil(t, manifestOut)
	})

	t.Run("ManifestHasNoRoutes", func(t *testing.T) {
		dbReceiver := models.DBReceiver{
			Name:     "receiver",
			TenantID: "tenant",
			Version:  3,
			To: []string{
				"first user <first@user.com>",
				"second user <second@user.com>",
			},
		}

		manifestIn := configManifest{
			Receivers: []receiver{
				{
					Name:         "tenant-receiver-1",
					EmailConfigs: []emailConfig{},
				},
			},
			Route: route{
				Routes: []subRoute{},
			},
		}

		conf := config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: true,
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

		require.ErrorContains(t, err, "alertmanager config manifest does not have routes")
		require.Nil(t, manifestOut)
	})

	// This test case ensures that after an upgrade of alerting monitor from a single tenant to multitenant version the receivers
	// and routes of the alertmanager config secret are updated to the new format including the tenant ID as a prefix.
	t.Run("UpgradeScenario", func(t *testing.T) {
		t.Run("SetReceiverEmailConfigWithRequireTLSTrue", func(t *testing.T) {
			dbReceiver := models.DBReceiver{
				Name:     "receiver",
				TenantID: "tenant",
				Version:  3,
				To: []string{
					"first user <first@user.com>",
					"second user <second@user.com>",
				},
			}

			receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

			manifestIn := configManifest{
				Receivers: []receiver{
					{
						Name:         "receiver-1",
						EmailConfigs: []emailConfig{},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: "receiver-1",
						},
					},
				},
			}

			conf := config.AlertManagerConfig{
				RequireTLS:         true,
				InsecureSkipVerify: true,
			}

			manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

			require.NoError(t, err)
			require.Equal(t, &configManifest{
				Receivers: []receiver{
					{
						Name: receiverName,
						EmailConfigs: []emailConfig{
							{
								SendResolved: true,
								To:           dbReceiver.To[0],
								HTML:         emailHTMLTemplate,
								RequireTLS:   true,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
							{
								SendResolved: true,
								To:           dbReceiver.To[1],
								HTML:         emailHTMLTemplate,
								RequireTLS:   true,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
						},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: receiverName,
							Matchers: []string{
								alertCategoryMatcher,
								`projectId=~"tenant"`,
							},
						},
					},
				},
			}, manifestOut)

			emailConfigExp := `send_resolved: true
to: first user <first@user.com>
html: '{{ template "alert.monitor.mail" . }}'
require_tls: true
tls_config:
  insecure_skip_verify: true
`
			emailConfigOut, err := yaml.Marshal(manifestOut.Receivers[0].EmailConfigs[0])

			require.NoError(t, err)
			require.Equal(t, emailConfigExp, string(emailConfigOut))
		})

		t.Run("SetReceiverEmailConfigWithRequireTLSFalse", func(t *testing.T) {
			dbReceiver := models.DBReceiver{
				Name:     "receiver",
				TenantID: "tenant",
				Version:  3,
				To: []string{
					"first user <first@user.com>",
					"second user <second@user.com>",
				},
			}

			receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

			manifestIn := configManifest{
				Receivers: []receiver{
					{
						Name:         "receiver-1",
						EmailConfigs: []emailConfig{},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: "receiver-1",
						},
					},
				},
			}

			conf := config.AlertManagerConfig{
				RequireTLS:         false,
				InsecureSkipVerify: true,
			}

			manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

			require.NoError(t, err)
			require.Equal(t, &configManifest{
				Receivers: []receiver{
					{
						Name: receiverName,
						EmailConfigs: []emailConfig{
							{
								SendResolved: true,
								To:           dbReceiver.To[0],
								HTML:         emailHTMLTemplate,
								RequireTLS:   false,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
							{
								SendResolved: true,
								To:           dbReceiver.To[1],
								HTML:         emailHTMLTemplate,
								RequireTLS:   false,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
						},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: receiverName,
							Matchers: []string{
								alertCategoryMatcher,
								`projectId=~"tenant"`,
							},
						},
					},
				},
			}, manifestOut)

			// This check ensures that `require_tls` field is not omitted when its value is false.
			// Enforces the expected behavior after removing yaml omitempty tag from emailConfig.RequireTLS field.
			emailConfigExp := `send_resolved: true
to: first user <first@user.com>
html: '{{ template "alert.monitor.mail" . }}'
require_tls: false
tls_config:
  insecure_skip_verify: true
`
			emailConfigOut, err := yaml.Marshal(manifestOut.Receivers[0].EmailConfigs[0])

			require.NoError(t, err)
			require.Equal(t, emailConfigExp, string(emailConfigOut))
		})

		t.Run("SetLegacyReceiverEmailConfigWithRequireTLSFalse", func(t *testing.T) {
			dbReceiver := models.DBReceiver{
				Name:     "receiver",
				TenantID: app.DefaultTenantID,
				Version:  3,
				To: []string{
					"first user <first@user.com>",
					"second user <second@user.com>",
				},
			}

			receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

			manifestIn := configManifest{
				Receivers: []receiver{
					{
						Name:         "receiver-1",
						EmailConfigs: []emailConfig{},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: "receiver-1",
						},
					},
				},
			}

			conf := config.AlertManagerConfig{
				RequireTLS:         false,
				InsecureSkipVerify: true,
			}

			manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

			require.NoError(t, err)
			require.Equal(t, &configManifest{
				Receivers: []receiver{
					{
						Name: receiverName,
						EmailConfigs: []emailConfig{
							{
								SendResolved: true,
								To:           dbReceiver.To[0],
								HTML:         emailHTMLTemplate,
								RequireTLS:   false,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
							{
								SendResolved: true,
								To:           dbReceiver.To[1],
								HTML:         emailHTMLTemplate,
								RequireTLS:   false,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
						},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: receiverName,
							Matchers: []string{
								alertCategoryMatcher,
								`projectId=~""`,
							},
						},
					},
				},
			}, manifestOut)

			// This check ensures that `require_tls` field is not omitted when its value is false.
			// Enforces the expected behavior after removing yaml omitempty tag from emailConfig.RequireTLS field.
			emailConfigExp := `send_resolved: true
to: first user <first@user.com>
html: '{{ template "alert.monitor.mail" . }}'
require_tls: false
tls_config:
  insecure_skip_verify: true
`
			emailConfigOut, err := yaml.Marshal(manifestOut.Receivers[0].EmailConfigs[0])

			require.NoError(t, err)
			require.Equal(t, emailConfigExp, string(emailConfigOut))
		})

		t.Run("SetEmailConfigWithNonExistingRouteReceiver", func(t *testing.T) {
			dbReceiver := models.DBReceiver{
				Name:     "receiver2",
				TenantID: "tenant2",
				Version:  3,
				To: []string{
					"first user <first@user.com>",
					"second user <second@user.com>",
				},
			}

			manifestIn := configManifest{
				Receivers: []receiver{
					{
						Name:         "receiver1-1",
						EmailConfigs: []emailConfig{},
					},
					{
						Name:         "receiver2-1",
						EmailConfigs: []emailConfig{},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: "receiver1-1",
							Matchers: []string{
								"matcher",
							},
						},
					},
				},
			}

			manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, config.AlertManagerConfig{
				RequireTLS:         true,
				InsecureSkipVerify: true,
			})

			receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

			require.NoError(t, err)
			require.Equal(t, &configManifest{
				Receivers: []receiver{
					{
						Name:         "receiver1-1",
						EmailConfigs: []emailConfig{},
					},
					{
						Name: receiverName,
						EmailConfigs: []emailConfig{
							{
								SendResolved: true,
								To:           dbReceiver.To[0],
								HTML:         emailHTMLTemplate,
								RequireTLS:   true,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
							{
								SendResolved: true,
								To:           dbReceiver.To[1],
								HTML:         emailHTMLTemplate,
								RequireTLS:   true,
								TLSConfig: struct {
									InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
								}{
									InsecureSkipVerify: true,
								},
							},
						},
					},
				},
				Route: route{
					Routes: []subRoute{
						{
							Receiver: "receiver1-1",
							Matchers: []string{
								"matcher",
							},
						},
						{
							Receiver: receiverName,
							Matchers: []string{
								alertCategoryMatcher,
								`projectId=~"tenant2"`,
							},
						},
					},
				},
			}, manifestOut)
		})
	})

	t.Run("SetReceiverEmailConfigWithRequireTLSTrue", func(t *testing.T) {
		dbReceiver := models.DBReceiver{
			Name:     "receiver",
			TenantID: "tenant",
			Version:  3,
			To: []string{
				"first user <first@user.com>",
				"second user <second@user.com>",
			},
		}

		receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

		manifestIn := configManifest{
			Receivers: []receiver{
				{
					Name:         "tenant-receiver-1",
					EmailConfigs: []emailConfig{},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant-receiver-1",
					},
				},
			},
		}

		conf := config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: true,
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Receivers: []receiver{
				{
					Name: receiverName,
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           dbReceiver.To[0],
							HTML:         emailHTMLTemplate,
							RequireTLS:   true,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
						{
							SendResolved: true,
							To:           dbReceiver.To[1],
							HTML:         emailHTMLTemplate,
							RequireTLS:   true,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
					},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: receiverName,
						Matchers: []string{
							alertCategoryMatcher,
							`projectId=~"tenant"`,
						},
					},
				},
			},
		}, manifestOut)

		emailConfigExp := `send_resolved: true
to: first user <first@user.com>
html: '{{ template "alert.monitor.mail" . }}'
require_tls: true
tls_config:
  insecure_skip_verify: true
`
		emailConfigOut, err := yaml.Marshal(manifestOut.Receivers[0].EmailConfigs[0])

		require.NoError(t, err)
		require.Equal(t, emailConfigExp, string(emailConfigOut))
	})

	t.Run("SetReceiverEmailConfigWithRequireTLSFalse", func(t *testing.T) {
		dbReceiver := models.DBReceiver{
			Name:     "receiver",
			TenantID: "tenant",
			Version:  3,
			To: []string{
				"first user <first@user.com>",
				"second user <second@user.com>",
			},
		}

		receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

		manifestIn := configManifest{
			Receivers: []receiver{
				{
					Name:         "tenant-receiver-1",
					EmailConfigs: []emailConfig{},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant-receiver-1",
					},
				},
			},
		}

		conf := config.AlertManagerConfig{
			RequireTLS:         false,
			InsecureSkipVerify: true,
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Receivers: []receiver{
				{
					Name: receiverName,
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           dbReceiver.To[0],
							HTML:         emailHTMLTemplate,
							RequireTLS:   false,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
						{
							SendResolved: true,
							To:           dbReceiver.To[1],
							HTML:         emailHTMLTemplate,
							RequireTLS:   false,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
					},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: receiverName,
						Matchers: []string{
							alertCategoryMatcher,
							`projectId=~"tenant"`,
						},
					},
				},
			},
		}, manifestOut)

		// This check ensures that `require_tls` field is not omitted when its value is false.
		// Enforces the expected behavior after removing yaml omitempty tag from emailConfig.RequireTLS field.
		emailConfigExp := `send_resolved: true
to: first user <first@user.com>
html: '{{ template "alert.monitor.mail" . }}'
require_tls: false
tls_config:
  insecure_skip_verify: true
`
		emailConfigOut, err := yaml.Marshal(manifestOut.Receivers[0].EmailConfigs[0])

		require.NoError(t, err)
		require.Equal(t, emailConfigExp, string(emailConfigOut))
	})

	t.Run("SetReceiverEmailConfigWithNonExistingTenantReceiver", func(t *testing.T) {
		dbReceiver := models.DBReceiver{
			Name:     "receiver2",
			TenantID: "tenant2",
			Version:  1,
			To: []string{
				"first user <first@user.com>",
				"second user <second@user.com>",
			},
		}

		manifestIn := configManifest{
			Receivers: []receiver{
				{
					Name:         "tenant1-receiver1-1",
					EmailConfigs: []emailConfig{},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant1-receiver1-1",
					},
				},
			},
		}

		conf := config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: true,
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Receivers: []receiver{
				{
					Name:         "tenant1-receiver1-1",
					EmailConfigs: []emailConfig{},
				},
				{
					Name: "tenant2-receiver2-1",
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           dbReceiver.To[0],
							HTML:         emailHTMLTemplate,
							RequireTLS:   true,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
						{
							SendResolved: true,
							To:           dbReceiver.To[1],
							HTML:         emailHTMLTemplate,
							RequireTLS:   true,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
					},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant1-receiver1-1",
					},
					{
						Receiver: "tenant2-receiver2-1",
						Matchers: []string{
							alertCategoryMatcher,
							`projectId=~"tenant2"`,
						},
					},
				},
			},
		}, manifestOut)
	})

	t.Run("SetEmailConfigWithNonExistingRouteReceiver", func(t *testing.T) {
		dbReceiver := models.DBReceiver{
			Name:     "receiver2",
			TenantID: "tenant2",
			Version:  3,
			To: []string{
				"first user <first@user.com>",
				"second user <second@user.com>",
			},
		}

		manifestIn := configManifest{
			Receivers: []receiver{
				{
					Name:         "tenant1-receiver1-1",
					EmailConfigs: []emailConfig{},
				},
				{
					Name:         "tenant2-receiver2-1",
					EmailConfigs: []emailConfig{},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant1-receiver1-1",
						Matchers: []string{
							"matcher",
						},
					},
				},
			},
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: true,
		})

		receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Receivers: []receiver{
				{
					Name:         "tenant1-receiver1-1",
					EmailConfigs: []emailConfig{},
				},
				{
					Name: receiverName,
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           dbReceiver.To[0],
							HTML:         emailHTMLTemplate,
							RequireTLS:   true,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
						{
							SendResolved: true,
							To:           dbReceiver.To[1],
							HTML:         emailHTMLTemplate,
							RequireTLS:   true,
							TLSConfig: struct {
								InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
							}{
								InsecureSkipVerify: true,
							},
						},
					},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant1-receiver1-1",
						Matchers: []string{
							"matcher",
						},
					},
					{
						Receiver: receiverName,
						Matchers: []string{
							alertCategoryMatcher,
							`projectId=~"tenant2"`,
						},
					},
				},
			},
		}, manifestOut)
	})

	t.Run("SetSMTPGlobalConfigWithoutCredentials", func(t *testing.T) {
		t.Setenv("SMTP_USERNAME", "")
		t.Setenv("SMTP_PASSWORD", "")

		dbReceiver := models.DBReceiver{
			Name:     "receiver",
			TenantID: "tenant",
			Version:  3,
			To: []string{
				"test user <test@user.com>",
			},
			From:       "sender user <sender@user.com>",
			MailServer: "smtp.com:443",
		}

		receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

		manifestIn := configManifest{
			Receivers: []receiver{
				{
					Name: "tenant-receiver-1",
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           "foo bar <foo@bar.com>",
							RequireTLS:   false,
						},
					},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant-receiver-1",
					},
				},
			},
		}

		conf := config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: false,
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Global: global{
				SMTPFrom: dbReceiver.From,
				SMTPHost: dbReceiver.MailServer,
			},
			Receivers: []receiver{
				{
					Name: receiverName,
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           dbReceiver.To[0],
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
						Receiver: receiverName,
						Matchers: []string{
							alertCategoryMatcher,
							`projectId=~"tenant"`,
						},
					},
				},
			},
		}, manifestOut)
	})

	t.Run("SetSMTPGlobalConfigWithCredentials", func(t *testing.T) {
		smtpUser := "admin"
		smtpPass := "1234"
		t.Setenv("SMTP_USERNAME", smtpUser)
		t.Setenv("SMTP_PASSWORD", smtpPass)

		dbReceiver := models.DBReceiver{
			Name:     "receiver",
			TenantID: "tenant",
			Version:  3,
			To: []string{
				"test user <test@user.com>",
			},
			From:       "sender user <sender@user.com>",
			MailServer: "smtp.com:443",
		}

		receiverName := fmt.Sprintf("%s-%s-%d", dbReceiver.TenantID, dbReceiver.Name, dbReceiver.Version)

		manifestIn := configManifest{
			Receivers: []receiver{
				{
					Name: "tenant-receiver-1",
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           "foo bar <foo@bar.com>",
							RequireTLS:   false,
						},
					},
				},
			},
			Route: route{
				Routes: []subRoute{
					{
						Receiver: "tenant-receiver-1",
					},
				},
			},
		}

		conf := config.AlertManagerConfig{
			RequireTLS:         true,
			InsecureSkipVerify: false,
		}

		manifestOut, err := manifestIn.ApplyReceiver(dbReceiver, conf)

		require.NoError(t, err)
		require.Equal(t, &configManifest{
			Global: global{
				SMTPFrom:         dbReceiver.From,
				SMTPHost:         dbReceiver.MailServer,
				SMTPAuthUsername: smtpUser,
				SMTPAuthPassword: smtpPass,
			},
			Receivers: []receiver{
				{
					Name: receiverName,
					EmailConfigs: []emailConfig{
						{
							SendResolved: true,
							To:           dbReceiver.To[0],
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
						Receiver: receiverName,
						Matchers: []string{
							alertCategoryMatcher,
							`projectId=~"tenant"`,
						},
					},
				},
			},
		}, manifestOut)
	})
}
