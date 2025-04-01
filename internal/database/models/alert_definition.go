// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AlertDefinitionState string
type AlertDefinitionCategory string

const (
	DefinitionNew      AlertDefinitionState = "New"
	DefinitionModified AlertDefinitionState = "Modified"
	DefinitionPending  AlertDefinitionState = "Pending"
	DefinitionApplied  AlertDefinitionState = "Applied"
	DefinitionError    AlertDefinitionState = "Error"

	CategoryPerformance AlertDefinitionCategory = "performance"
	CategoryHealth      AlertDefinitionCategory = "health"
	CategoryMaintenance AlertDefinitionCategory = "maintenance"
)

func (ds AlertDefinitionState) Validate() error {
	switch ds {
	case DefinitionNew:
	case DefinitionModified:
	case DefinitionPending:
	case DefinitionApplied:
	case DefinitionError:
	default:
		return fmt.Errorf("unknown alert definition state: %q", ds)
	}
	return nil
}

func (c AlertDefinitionCategory) Validate() error {
	switch c {
	case CategoryHealth:
	case CategoryMaintenance:
	case CategoryPerformance:
	default:
		return fmt.Errorf("unknown alert definition category: %q", c)
	}
	return nil
}

type AlertDuration struct {
	ID                int64  `gorm:"primaryKey;autoIncrement"`
	Name              string `gorm:"not null;uniqueIndex:idx_duration_alert_id_name"`
	Duration          int64
	DurationMin       int64
	DurationMax       int64
	AlertDefinitionID int64 `gorm:"not null;uniqueIndex:idx_duration_alert_id_name"`
}

type AlertThreshold struct {
	ID                int64  `gorm:"primaryKey;autoIncrement"`
	Name              string `gorm:"not null;uniqueIndex:idx_threshold_alert_id_name"`
	Threshold         int64
	ThresholdMin      int64
	ThresholdMax      int64
	ThresholdType     string
	ThresholdUnit     string
	AlertDefinitionID int64 `gorm:"not null;uniqueIndex:idx_threshold_alert_id_name"`
}

type AlertDefinition struct {
	ID            int64                `gorm:"primaryKey;autoIncrement"`
	Enabled       bool                 `gorm:"not null"`
	UUID          uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:idx_def_uuid_version_tenant"`
	Version       int64                `gorm:"not null;uniqueIndex:idx_def_uuid_version_tenant;uniqueIndex:idx_name_severity_version_tenant"`
	Name          string               `gorm:"not null;uniqueIndex:idx_name_severity_version_tenant"`
	State         AlertDefinitionState `gorm:"not null,type:enum('New','Modified','Pending','Applied','Error'),default:New"`
	Template      string
	Category      AlertDefinitionCategory
	Context       string
	Severity      string `gorm:"not null;uniqueIndex:idx_name_severity_version_tenant"`
	AlertInterval int64
	TenantID      string `gorm:"not null;default:edgenode;uniqueIndex:idx_def_uuid_version_tenant;uniqueIndex:idx_name_severity_version_tenant"`
}

func (d *AlertDefinition) BeforeCreate(*gorm.DB) error {
	if err := d.Category.Validate(); err != nil {
		return err
	}
	return d.State.Validate()
}

func (d *AlertDefinition) AfterUpdate(*gorm.DB) error {
	if err := d.Category.Validate(); err != nil {
		return err
	}
	return d.State.Validate()
}

// DBAlertDefinitionValues represent the values of an alert definition that can be modified.
type DBAlertDefinitionValues struct {
	Duration  *int64 // in seconds.
	Threshold *int64
	Enabled   *bool
}

// DBAlertDefinition represents the info of an alert definition.
type DBAlertDefinition struct {
	ID       uuid.UUID
	Name     string
	State    AlertDefinitionState
	Template string
	Values   DBAlertDefinitionValues
	Interval int64
	Version  int64
	Category AlertDefinitionCategory
	TenantID string
}
