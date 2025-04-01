// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
)

type TaskState string

const (
	TaskNew     TaskState = "New"
	TaskTaken   TaskState = "Taken"
	TaskApplied TaskState = "Applied"
	TaskError   TaskState = "Error"
	TaskInvalid TaskState = "Invalid"
)

func (ts TaskState) Validate() error {
	switch ts {
	case TaskNew:
	case TaskTaken:
	case TaskApplied:
	case TaskError:
	case TaskInvalid:
	default:
		return fmt.Errorf("unknown task state: %q", ts)
	}
	return nil
}

type TaskType string

const (
	TypeReceiver        TaskType = "Receiver"
	TypeAlertDefinition TaskType = "AlertDefinition"
)

type TaskUUIDTenantID struct {
	UUID     uuid.UUID
	TenantID api.TenantID
}

type Task struct {
	ID                  int64      `gorm:"primaryKey;autoIncrement"`
	OwnerUUID           uuid.UUID  `gorm:"type:uuid"`
	State               TaskState  `gorm:"not null,type:enum('New','Taken','Applied','Error','Invalid'),default:New"`
	AlertDefinitionUUID *uuid.UUID `gorm:"uniqueIndex:idx_alert_uuid_tenant_version_key"`
	ReceiverUUID        *uuid.UUID `gorm:"uniqueIndex:idx_recv_uuid_tenant_version_key"`
	TenantID            string     `gorm:"not null;default:edgenode;uniqueIndex:idx_alert_uuid_tenant_version_key;uniqueIndex:idx_recv_uuid_tenant_version_key"`
	Version             int64      `gorm:"uniqueIndex:idx_alert_uuid_tenant_version_key;uniqueIndex:idx_recv_uuid_tenant_version_key"`
	CreationDate        time.Time  `gorm:"default:current_timestamp"`
	StartDate           time.Time
	CompletionDate      time.Time
	RetryCount          int64 `gorm:"default:0"`
}

func (t *Task) GetTaskUUID() uuid.UUID {
	if t.AlertDefinitionUUID != nil {
		return *t.AlertDefinitionUUID
	}
	return *t.ReceiverUUID
}

func (t *Task) GetTaskType() TaskType {
	if t.AlertDefinitionUUID != nil {
		return TypeAlertDefinition
	}
	return TypeReceiver
}

func (t *Task) validateUUIDs() error {
	if t.AlertDefinitionUUID == nil && t.ReceiverUUID == nil {
		return errors.New("alert definition and receiver UUID are nil")
	}
	if t.AlertDefinitionUUID != nil && t.ReceiverUUID != nil {
		return errors.New("both alert definition and receiver UUID are populated")
	}
	return nil
}

func (t *Task) BeforeCreate(*gorm.DB) error {
	if err := t.validateUUIDs(); err != nil {
		return err
	}
	return t.State.Validate()
}
