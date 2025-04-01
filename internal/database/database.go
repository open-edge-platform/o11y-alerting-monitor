// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

// AlertDefinitionHandlerManager is used to get a single alert definition or a list or alert definitions.
// It also allows updating alert definition values such as duration, threshold, and enabled.
type AlertDefinitionHandlerManager interface {
	// GetLatestAlertDefinitionList gets a list with the info on the latest version of alert definitions, including duration and threshold values
	// as well as its enabled state.
	GetLatestAlertDefinitionList(ctx context.Context, tenantID api.TenantID) ([]*models.DBAlertDefinition, error)

	// GetLatestAlertDefinition gets the info on the latest version of alert definition, including its duration, threshold,
	// and a flag specifying if the alert is enabled.
	GetLatestAlertDefinition(ctx context.Context, tenantID api.TenantID, id uuid.UUID) (*models.DBAlertDefinition, error)

	// SetAlertDefinitionValues sets the duration and/or threshold values, and/or the enabled state of an alert definition
	// given its UUID.
	SetAlertDefinitionValues(ctx context.Context, tenantID api.TenantID, id uuid.UUID, values models.DBAlertDefinitionValues) error
}

// AlertDefinitionExecutorManager is used to get specific versions of alert definition.
// It also allows setting an alert definition state.
type AlertDefinitionExecutorManager interface {
	// GetAlertDefinition gets the info on a specific version of alert definition, including its duration, threshold,
	// and a flag specifying if the alert is enabled.
	GetAlertDefinition(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64) (*models.DBAlertDefinition, error)

	// SetAlertDefinitionState updates the `State` column of specific alert definition version.
	SetAlertDefinitionState(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64, state models.AlertDefinitionState) error
}

// ReceiverHandlerManager is used to get a versioned receiver or a list of versioned receivers. It also allows updating the list of email
// recipients of a receiver, creating a new version. These operations only apply to the latest version of a receiver.
type ReceiverHandlerManager interface {
	// GetLatestReceiverListWithEmailConfig gets a list with information of receivers including its email configuration
	// and its list of recipients.
	GetLatestReceiverListWithEmailConfig(ctx context.Context, tenantID api.TenantID) ([]*models.DBReceiver, error)

	// GetLatestReceiverWithEmailConfig gets the information of a specific receiver, given its UUID, including its email configuration
	// and its list of recipients.
	GetLatestReceiverWithEmailConfig(ctx context.Context, tenantID api.TenantID, id uuid.UUID) (*models.DBReceiver, error)

	// SetReceiverEmailRecipients sets the list of email recipients of a given receiver.
	SetReceiverEmailRecipients(ctx context.Context, tenantID api.TenantID, id uuid.UUID, recipients []models.EmailAddress) error
}

// ReceiverExecutorManager is used to get a specific version of a receiver as well as to set the state of a versioned receiver.
type ReceiverExecutorManager interface {
	// GetReceiverWithEmailConfig gets the information of a specific version of a receiver, given its UUID, including its email configuration
	// and its list of recipients.
	GetReceiverWithEmailConfig(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64) (*models.DBReceiver, error)

	// SetReceiverState sets the state of the specific version of a given receiver.
	SetReceiverState(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64, state models.ReceiverState) error
}

type TaskManager interface {
	// SetTakenTasksExceedingDurationAsFailed looks for tasks which have Taken state and the time lapsed between the current time and the start time
	// exceeds the given duration. If any are found, it sets them as failed which depends on the retry count.
	SetTakenTasksExceedingDurationAsFailed(ctx context.Context, dur time.Duration, retryLimit int) error

	// DeleteNotPendingTasksExceedingDuration takes a duration and deletes tasks with Applied and Invalid state
	// for which the time elapsed between the completion date and the current date exceeds the given duration.
	DeleteNotPendingTasksExceedingDuration(ctx context.Context, dur time.Duration) error

	// GetPendingTasks takes an owner UUID and a count. It returns a slice of tasks from database which have not been completed,
	// and are not currently in Taken state. The slice has tasks with unique UUID and latest version.
	GetPendingTasks(ctx context.Context, ownerUUID uuid.UUID, countLimit int) ([]models.Task, error)

	// SetOlderVersionsToInvalidState takes a slice of tasks, and sets tasks from database with same UUID and older versions as invalid.
	SetOlderVersionsToInvalidState(ctx context.Context, tasks []models.Task) error

	// SetTaskAsApplied takes a task and sets its state to Applied as well as the completion date.
	SetTaskAsApplied(ctx context.Context, task models.Task) error

	// SetTaskAsFailed takes a task and a retry limit. If the task retry count is less than the retry limit it sets the task
	// to Error state, otherwise it sets the task to Invalid state.
	SetTaskAsFailed(ctx context.Context, task models.Task, retryLimit int) error

	// SetTaskAsInvalid takes a task and sets its status to Invalid and the completion date. It also sets the status of its
	// secondary key (either alert definition or receiver) to Error.
	SetTaskAsInvalid(ctx context.Context, task models.Task) error

	// SetTaskStateToInvalid takes a task and sets its status to Invalid and the completion date.
	SetTaskStateToInvalid(ctx context.Context, task models.Task) error
}

func ConnectDB() (*gorm.DB, error) {
	host := os.Getenv("PGHOST")
	port := os.Getenv("PGPORT")
	user := os.Getenv("PGUSER")
	password := os.Getenv("PGPASSWORD")
	dbname := os.Getenv("PGDATABASE")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=prefer", host, user, password, dbname, port)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("failed to establish database connection: %w", err)
	}
	return db, nil
}
