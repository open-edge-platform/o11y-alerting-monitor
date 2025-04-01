// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/clock"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

// SetTakenTasksExceedingDurationAsFailed looks for tasks which have Taken state and the time lapsed between the current time and the start time
// exceeds the given duration. If any are found, it sets them as failed which depends on the retry count. If the retry count of the task does not
// exceed the given retry limit, the task is set to Error state, otherwise it is set to Invalid state.
func (d *DBService) SetTakenTasksExceedingDurationAsFailed(ctx context.Context, dur time.Duration, retryLimit int) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	timeDelta := clock.TimeNowFn().Add(-dur)

	var tasks []models.Task
	if err := tx.
		Where("state = ?", models.TaskTaken).
		Where("start_date < ?", timeDelta).
		Find(&tasks).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	for _, task := range tasks {
		if err := setTaskAsFailed(tx, task, retryLimit); err != nil {
			return fmt.Errorf("failed to set task as failed: %w", err)
		}
	}

	return tx.Commit().Error
}

// DeleteNotPendingTasksExceedingDuration takes a duration and deletes tasks with Applied and Invalid state
// for which the time elapsed between the completion date and the current date exceeds the given duration.
func (d *DBService) DeleteNotPendingTasksExceedingDuration(ctx context.Context, dur time.Duration) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	timeDelta := clock.TimeNowFn().Add(-dur)
	if err := tx.
		Where("state IN (?,?)", models.TaskApplied, models.TaskInvalid).
		Where("completion_date < ?", timeDelta).
		Delete(&models.Task{}).Error; err != nil {
		return err
	}

	return tx.Commit().Error
}

// GetTaskUUIDTenantIDPairs is a helper function that returns a slice of unique pairs of tasks UUIDs and tenants of tasks which are in pending state,
// either New or Error. If a task is in Taken state, its UUID is not included in the result. The slice has a maximum
// length of countLimit elements, and the UUIDs are ordered based on task ID in the tasks table of the database connection.
func GetTaskUUIDTenantIDPairs(tx *gorm.DB, countLimit int) ([]models.TaskUUIDTenantID, error) {
	var uuids []models.TaskUUIDTenantID

	txx := tx.Raw(`
		SELECT DISTINCT
			uuid, tenant_id
		FROM
			(
				SELECT
					id, alert_definition_uuid AS uuid, tenant_id
				FROM
					tasks
				WHERE
					alert_definition_uuid IS NOT NULL AND state IN ('New','Error')
				UNION ALL
				SELECT
					id, receiver_uuid AS uuid, tenant_id
				FROM
					tasks
				WHERE
					receiver_uuid IS NOT NULL AND state IN ('New','Error')
				ORDER BY id
			)
		AS uuids
		WHERE NOT EXISTS
			(
				SELECT 1
				FROM
					tasks t
				WHERE
					(t.alert_definition_uuid = uuids.uuid OR t.receiver_uuid = uuids.uuid) AND t.state = 'Taken'
			)
		LIMIT ?;
	`, countLimit).Scan(&uuids)

	if err := txx.Error; err != nil {
		return nil, err
	}

	return uuids, nil
}

// GetPendingTasks takes an owner UUID and a count. It returns a slice of tasks from database which have not been completed,
// and are not currently in Taken state. The slice has tasks with unique UUID and latest version. The state, start_date, and
// owner_uuid columns of the returned tasks are also updated within the database.
func (d *DBService) GetPendingTasks(ctx context.Context, ownerUUID uuid.UUID, count int) ([]models.Task, error) {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	taskUUIDTenantIDPairs, err := GetTaskUUIDTenantIDPairs(tx, count)
	if err != nil {
		return nil, err
	}

	tasks := make([]models.Task, 0, count)
	for _, pair := range taskUUIDTenantIDPairs {
		// Get latest version of a task by UUID.
		var task models.Task
		err := tx.
			Where("(alert_definition_uuid = ? OR receiver_uuid = ?)", pair.UUID, pair.UUID).
			Where("tenant_id = ?", pair.TenantID).
			Where("state IN (?,?)", models.TaskNew, models.TaskError).
			Order("version desc").
			First(&task).Error
		if err != nil {
			return nil, err
		}

		// Set values of task to taken.
		err = tx.Model(&task).Updates(map[string]interface{}{
			"start_date": clock.TimeNowFn(),
			"state":      models.TaskTaken,
			"owner_uuid": ownerUUID,
		}).Error
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, task)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return tasks, nil
}

// SetOlderVersionsToInvalidState takes a slice of tasks, and sets tasks from database with same UUID and older versions as invalid.
func (d *DBService) SetOlderVersionsToInvalidState(ctx context.Context, tasks []models.Task) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	for _, task := range tasks {
		err := tx.Model(models.Task{}).
			Where("(alert_definition_uuid = ? OR receiver_uuid = ?)", task.AlertDefinitionUUID, task.ReceiverUUID).
			Where("tenant_id = ?", task.TenantID).
			Where("state IN (?,?)", models.TaskNew, models.TaskError).
			Where("version < ?", task.Version).
			Updates(models.Task{
				State:          models.TaskInvalid,
				CompletionDate: clock.TimeNowFn(),
			}).Error

		if err != nil {
			return fmt.Errorf("failed to set invalid state to task with UUID %q and version %d for tenant %q: %w",
				task.GetTaskUUID(), task.Version, task.TenantID, err)
		}
	}

	return tx.Commit().Error
}

// SetTaskAsApplied takes a task and sets its state to Applied as well as the completion date.
func (d *DBService) SetTaskAsApplied(ctx context.Context, task models.Task) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	if err := tx.Model(&task).Updates(models.Task{
		State:          models.TaskApplied,
		CompletionDate: clock.TimeNowFn(),
	}).Error; err != nil {
		return fmt.Errorf("failed to set task %q with version %d for tenant %q as Applied: %w",
			task.GetTaskUUID(), task.Version, task.TenantID, err)
	}

	switch task.GetTaskType() {
	case models.TypeAlertDefinition:
		if err := setAlertDefinitionState(tx, task.TenantID, *task.AlertDefinitionUUID, task.Version, models.DefinitionApplied); err != nil {
			return fmt.Errorf("failed to set alert definition %q with version %v for tenant %q to state 'Applied': %w",
				task.AlertDefinitionUUID.String(), task.Version, task.TenantID, err)
		}
	case models.TypeReceiver:
		if err := setReceiverState(tx, task.TenantID, *task.ReceiverUUID, task.Version, models.ReceiverApplied); err != nil {
			return fmt.Errorf("failed to set receiver %q with version %v for tenant %q to state 'Applied': %w",
				task.ReceiverUUID.String(), task.Version, task.TenantID, err)
		}
	}

	return tx.Commit().Error
}

// SetTaskAsFailed takes a task and a retry limit. If the task retry count is less than the retry limit it sets the task
// to Error state, otherwise it sets the task to Invalid state.
func (d *DBService) SetTaskAsFailed(ctx context.Context, task models.Task, retryLimit int) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	if err := setTaskAsFailed(tx, task, retryLimit); err != nil {
		return err
	}

	return tx.Commit().Error
}

func setTaskAsFailed(tx *gorm.DB, task models.Task, retryLimit int) error {
	if task.RetryCount < int64(retryLimit) {
		if err := tx.Model(&task).Updates(models.Task{
			State:      models.TaskError,
			RetryCount: task.RetryCount + 1,
		}).Error; err != nil {
			return fmt.Errorf("failed to set task %q with version %d for tenant %q as Error",
				task.GetTaskUUID(), task.Version, task.TenantID)
		}
	} else if err := tx.Model(&task).Updates(models.Task{
		State:          models.TaskInvalid,
		CompletionDate: clock.TimeNowFn(),
	}).Error; err != nil {
		return fmt.Errorf("failed to set task %q with version %d for tenant %q as Invalid: %w",
			task.GetTaskUUID(), task.Version, task.TenantID, err)
	}

	switch task.GetTaskType() {
	case models.TypeAlertDefinition:
		if err := setAlertDefinitionState(tx, task.TenantID, *task.AlertDefinitionUUID, task.Version, models.DefinitionError); err != nil {
			return fmt.Errorf("failed to set alert definition %q with version %v for tenant %q to state 'Error': %w",
				task.AlertDefinitionUUID.String(), task.Version, task.TenantID, err)
		}
	case models.TypeReceiver:
		if err := setReceiverState(tx, task.TenantID, *task.ReceiverUUID, task.Version, models.ReceiverError); err != nil {
			return fmt.Errorf("failed to set receiver %q with version %v for tenant %q to state 'Error': %w",
				task.ReceiverUUID.String(), task.Version, task.TenantID, err)
		}
	}

	return nil
}

// SetTaskAsInvalid takes a task and sets its status to Invalid and the completion date. It also sets the status of its
// secondary key (either alert definition or receiver) to Error.
func (d *DBService) SetTaskAsInvalid(ctx context.Context, task models.Task) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	if err := tx.Model(&task).Updates(models.Task{
		State:          models.TaskInvalid,
		CompletionDate: clock.TimeNowFn(),
	}).Error; err != nil {
		return fmt.Errorf("failed to set task %q with version %d for tenant %q as Invalid: %w",
			task.GetTaskUUID(), task.Version, task.TenantID, err)
	}

	switch task.GetTaskType() {
	case models.TypeAlertDefinition:
		if err := setAlertDefinitionState(tx, task.TenantID, *task.AlertDefinitionUUID, task.Version, models.DefinitionError); err != nil {
			return fmt.Errorf("failed to set alert definition %q with version %v for tenant %q to state 'Error': %w",
				task.AlertDefinitionUUID.String(), task.Version, task.TenantID, err)
		}
	case models.TypeReceiver:
		if err := setReceiverState(tx, task.TenantID, *task.ReceiverUUID, task.Version, models.ReceiverError); err != nil {
			return fmt.Errorf("failed to set receiver %q with version %v for tenant %q to state 'Error': %w",
				task.ReceiverUUID.String(), task.Version, task.TenantID, err)
		}
	}

	return tx.Commit().Error
}

// SetTaskStateToInvalid takes a task and sets its status to Invalid and the completion date.
func (d *DBService) SetTaskStateToInvalid(ctx context.Context, task models.Task) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	if err := tx.Model(&task).Updates(models.Task{
		State:          models.TaskInvalid,
		CompletionDate: clock.TimeNowFn(),
	}).Error; err != nil {
		return fmt.Errorf("failed to set task %q with version %d for tenant %q as Invalid: %w", task.GetTaskUUID(), task.Version, task.TenantID, err)
	}

	return tx.Commit().Error
}
