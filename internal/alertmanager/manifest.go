// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0


package alertmanager

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/app"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

const (
	alertCategoryMatcher = `alert_category=~"health|performance"`
	emailHTMLTemplate    = `{{ template "alert.monitor.mail" . }}`
)

// global represents the global section of an alertmanager configuration file.
type global struct {
	SMTPFrom         string `yaml:"smtp_from"`
	SMTPHost         string `yaml:"smtp_smarthost"`
	SMTPAuthUsername string `yaml:"smtp_auth_username,omitempty"`
	SMTPAuthPassword string `yaml:"smtp_auth_password,omitempty"`
}

// subRoute represents a node in a routing tree and its children of an alertmanager configuration file.
type subRoute struct {
	Matchers []string `yaml:"matchers,omitempty"`
	Receiver string   `yaml:"receiver"`
}

// route represents the route section of an alertmanager configuration file. It describes how alerts are routed, aggregated, throttled and muted based on time.
type route struct {
	GroupBy        []string      `yaml:"group_by,omitempty"`
	GroupWait      time.Duration `yaml:"group_wait,omitempty"`
	GroupInterval  time.Duration `yaml:"group_interval,omitempty"`
	RepeatInterval time.Duration `yaml:"repeat_interval,omitempty"`
	Receiver       string        `yaml:"receiver"`
	Routes         []subRoute    `yaml:"routes,omitempty"`
}

// emailConfig represents the email_config subsection of an alertmanager configuration file. It describes the settings specific to a receiver.
type emailConfig struct {
	SendResolved bool   `yaml:"send_resolved,omitempty"`
	To           string `yaml:"to"`
	HTML         string `yaml:"html"`
	RequireTLS   bool   `yaml:"require_tls"`
	TLSConfig    struct {
		InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
	} `yaml:"tls_config,omitempty"`
}

// receiver represents the receiver section of an alertmanager configuration file. It describes the notification destinations (receivers).
type receiver struct {
	Name         string        `yaml:"name"`
	EmailConfigs []emailConfig `yaml:"email_configs,omitempty"`
}

// inhibitRule represents the inhibit_rule section of an alertmanager configuration file.
// It describes how alerts are muted based on the presence of other alerts.
type inhibitRule struct {
	SourceMatchers []string `yaml:"source_matchers,omitempty"`
	TargetMatchers []string `yaml:"target_matchers,omitempty"`
	Equal          []string `yaml:"equal,omitempty"`
}

// configManifest represents the configuration fields of an alertmanager configuration file.
type configManifest struct {
	Global       global        `yaml:"global,omitempty"`
	Route        route         `yaml:"route"`
	Receivers    []receiver    `yaml:"receivers"`
	InhibitRules []inhibitRule `yaml:"inhibit_rules,omitempty"`
	Templates    []string      `yaml:"templates,omitempty"`
}

// ApplyReceiver returns a modified version of an existing alertmanager config manifest. Sets SMTP config fields of the global section,
// email recipient list for each receiver, and routes based on the given input arguments.
func (m configManifest) ApplyReceiver(recv models.DBReceiver, conf config.AlertManagerConfig) (*configManifest, error) {
	manifest := m

	// Set global config fields.
	manifest.Global = global{
		SMTPFrom: recv.From,
		SMTPHost: recv.MailServer,
	}

	// username and password are optional based on helm values.
	if username := os.Getenv("SMTP_USERNAME"); len(username) != 0 {
		manifest.Global.SMTPAuthUsername = username
	}

	if password := os.Getenv("SMTP_PASSWORD"); len(password) != 0 {
		manifest.Global.SMTPAuthPassword = password
	}

	if len(m.Receivers) == 0 {
		return nil, errors.New("alertmanager config manifest does not have receivers")
	}

	// Create receiver email config.
	emailConfigs := make([]emailConfig, len(recv.To))
	for i := range recv.To {
		emailConfigs[i] = emailConfig{
			SendResolved: true,
			To:           recv.To[i],
			HTML:         emailHTMLTemplate,
			RequireTLS:   conf.RequireTLS,
			TLSConfig: struct {
				InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
			}{
				InsecureSkipVerify: conf.InsecureSkipVerify,
			},
		}
	}

	receiverName := fmt.Sprintf("%s-%s", recv.TenantID, recv.Name)
	receiverNameWithVersion := fmt.Sprintf("%s-%d", receiverName, recv.Version)
	newReceiver := receiver{
		Name:         receiverNameWithVersion,
		EmailConfigs: emailConfigs,
	}

	// When upgrading from single tenant to multitenant version of alerting monitor, alertmanager secret
	// receiver and routes names are not preceded by tenant ID. The 2nd check ensures the receivers
	// are still found and updated, having the tenant ID as prefix.
	index := slices.IndexFunc(m.Receivers, func(r receiver) bool {
		return strings.Contains(r.Name, receiverName) || strings.Contains(fmt.Sprintf("%s-%s", recv.TenantID, r.Name), receiverName)
	})
	if index < 0 {
		manifest.Receivers = append(manifest.Receivers, newReceiver)
	} else {
		manifest.Receivers[index] = newReceiver
	}

	if len(manifest.Route.Routes) == 0 {
		return nil, errors.New("alertmanager config manifest does not have routes")
	}

	// When upgrading from single tenant to multitenant version of alerting monitor, alertmanager secret
	// receiver and routes names are not preceded by tenant ID. The 2nd case ensures routes
	// are still found and updated, having the tenant ID as prefix.
	index = slices.IndexFunc(manifest.Route.Routes, func(r subRoute) bool {
		return strings.Contains(r.Receiver, receiverName) || strings.Contains(fmt.Sprintf("%s-%s", recv.TenantID, r.Receiver), receiverName)
	})

	var projectIDMatcher string
	// Special case where the legacy single tenant receiver should match exactly empty projectId,
	// otherwise any subsequent patch would overwrite the projectId label to match to it's tenant,
	// and no alerts would be triggered as a result (no alerts with such label).
	if recv.TenantID == app.DefaultTenantID {
		projectIDMatcher = `projectId=~""`
	} else {
		projectIDMatcher = fmt.Sprintf(`projectId=~"%v"`, recv.TenantID)
	}

	if index < 0 {
		// Add a new route
		manifest.Route.Routes = append(manifest.Route.Routes, subRoute{
			Receiver: receiverNameWithVersion,
			Matchers: []string{
				alertCategoryMatcher,
				projectIDMatcher,
			},
		})
	} else {
		// Overwrite the existing route
		manifest.Route.Routes[index] = subRoute{
			Receiver: receiverNameWithVersion,
			Matchers: []string{
				alertCategoryMatcher,
				projectIDMatcher,
			},
		}
	}

	return &manifest, nil
}
