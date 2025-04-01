// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	am "github.com/open-edge-platform/o11y-alerting-monitor/internal/alertmanager"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/mimir"
)

// asyncExecutor represents a mechanism that allows to process tasks asynchronously. It supports two types of tasks:
// receiver and definition tasks. Receiver tasks are related to configuration of alertmanager receivers and routing actions,
// whereas definition tasks are related to configuration of alert definitions of mimir.
type asyncExecutor struct {
	ownerUUID      uuid.UUID
	executorConfig config.TaskExecutorConfig
	logger         *slog.Logger
	quit           chan struct{}

	tasks       database.TaskManager
	definitions database.AlertDefinitionExecutorManager
	receivers   database.ReceiverExecutorManager

	receiversCfg   am.AlertmanagerConfigurator
	definitionsCfg mimir.DefinitionConfigUpdater
}

// NewAsyncExecutor creates a new asyncExecutor, initializing the UUID of the corresponding instance, configuration parameters,
// connection to the database where tasks are stored, and the struct that allows to reconfigure alertmanager config.
func NewAsyncExecutor(
	ownerUUID uuid.UUID, cfg config.Config, dbConn *gorm.DB, loglevel string, alertManager *am.AlertManager) *asyncExecutor {
	opts := setLogLvl(loglevel)
	return &asyncExecutor{
		ownerUUID:      ownerUUID,
		executorConfig: cfg.TaskExecutor,
		logger:         slog.New(slog.NewTextHandler(os.Stdout, &opts)),
		quit:           make(chan struct{}),

		definitionsCfg: &mimir.Mimir{Config: &cfg.Mimir},
		receiversCfg:   alertManager,

		definitions: &database.DBService{DB: dbConn},
		receivers:   &database.DBService{DB: dbConn},
		tasks:       &database.DBService{DB: dbConn},
	}
}

// Start allows the receiver to start processing tasks stored into the database. Tasks are processed periodically by means of a ticker.
// NOTE: Once this method is invoked, to stop processing tasks, we need to explicitly call Stop method from the receiver.
func (ae *asyncExecutor) Start(ctx context.Context) {
	go func() {
		i := 0

		processTicker := time.NewTicker(ae.executorConfig.PoolingRate)
		defer processTicker.Stop()

		for {
			select {
			case <-ae.quit:
				ae.logger.Info("Received signal: stopping executor")
				return
			case <-processTicker.C:
				// TODO: What if ticker is exceeded? Skips it.
				ae.processTasks(ctx)

				if i%30 == 0 {
					if err := ae.tasks.SetTakenTasksExceedingDurationAsFailed(ctx, ae.executorConfig.TaskTimeout, ae.executorConfig.RetryLimit); err != nil {
						ae.logger.Error("failed to set tasks which exceed timeout to failed", slog.Any("error", err))
					}
				}

				// TODO: This can be run in a separate goroutine with separate ticker.
				// needs to pass quit channel to stop.
				// Delete (check) old tasks every 1000th loop run
				if i == 5 {
					err := ae.tasks.DeleteNotPendingTasksExceedingDuration(ctx, ae.executorConfig.RetentionTime)
					if err != nil {
						ae.logger.Error("failed to clean up not pending tasks", slog.Any("error", err))
					}
				}

				i = (i + 1) % 1000
			}
		}
	}()
}

// Stop allows the receiver to stop processing tasks.
func (ae *asyncExecutor) Stop() {
	close(ae.quit)
}

// processTasks fetches tasks from database which are pending and attempt to execute them. A task is considered to be pending
// if its state is either 'New' or 'Error'. It also checks if there are older versions of the taken tasks in the database. If so,
// they are set to 'Invalid' state.
func (ae *asyncExecutor) processTasks(ctx context.Context) {
	takenTasks, err := ae.tasks.GetPendingTasks(ctx, ae.ownerUUID, ae.executorConfig.UUIDLimit)
	if err != nil {
		ae.logger.Error("failed to get pending tasks", slog.Any("error", err))
		return
	}

	if len(takenTasks) == 0 {
		return
	}

	if err := ae.tasks.SetOlderVersionsToInvalidState(ctx, takenTasks); err != nil {
		ae.logger.Error("failed to set older versions of taken tasks to 'Invalid' state", slog.Any("error", err))
	}

	for _, task := range takenTasks {
		t := task

		if err := ae.executeTask(ctx, &t); err != nil {
			ae.logger.Error(
				fmt.Sprintf("failed to execute task %q with version %d", t.GetTaskUUID(), t.Version),
				slog.Any("error", err),
			)
		}
	}
}

// executeTask attempts to execute a given task with a specific timeout.
func (ae *asyncExecutor) executeTask(ctx context.Context, task *models.Task) error {
	errChan := make(chan error)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, ae.executorConfig.TaskTimeout)
	defer cancel()

	go func() {
		var err error
		switch task.GetTaskType() {
		case models.TypeReceiver:
			err = ae.handleReceiverTask(ctxWithTimeout, task)
		case models.TypeAlertDefinition:
			err = ae.handleDefinitionTask(ctxWithTimeout, task)
		}

		errChan <- err
		close(errChan)
	}()

	for {
		select {
		case <-ctxWithTimeout.Done():
			if err := ae.tasks.SetTaskAsFailed(ctx, *task, ae.executorConfig.RetryLimit); err != nil {
				ae.logger.Error("failed to handle task exceeding timeout", slog.Any("error", err))
			}

			return ctxWithTimeout.Err()
		case err := <-errChan:
			return err
		}
	}
}

func (ae *asyncExecutor) handleReceiverTask(ctx context.Context, task *models.Task) error {
	r, err := ae.receivers.GetReceiverWithEmailConfig(ctx, task.TenantID, *task.ReceiverUUID, task.Version)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ae.logger.Error(
			fmt.Sprintf("associated receiver for task %q with version %d not found", task.ReceiverUUID.String(), task.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskStateToInvalid(ctx, *task)
	} else if err != nil {
		ae.logger.Error(
			fmt.Sprintf("failed to retrieve receiver %q with version %d", task.ReceiverUUID.String(), task.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskAsFailed(ctx, *task, ae.executorConfig.RetryLimit)
	}

	if err := ae.receivers.SetReceiverState(ctx, r.TenantID, r.UUID, int64(r.Version), models.ReceiverPending); err != nil {
		ae.logger.Error(
			fmt.Sprintf("failed to set receiver %q with version %d state to 'Pending'", r.UUID.String(), r.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskAsFailed(ctx, *task, ae.executorConfig.RetryLimit)
	}

	err = ae.receiversCfg.UpdateReceiverConfig(ctx, *r)
	if err != nil {
		ae.logger.Error(
			fmt.Sprintf("failed to apply receiver %q and version %d due to internal error", r.UUID.String(), r.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskAsFailed(ctx, *task, ae.executorConfig.RetryLimit)
	}

	return ae.tasks.SetTaskAsApplied(ctx, *task)
}

func (ae *asyncExecutor) handleDefinitionTask(ctx context.Context, task *models.Task) error {
	alertDef, err := ae.definitions.GetAlertDefinition(ctx, task.TenantID, *task.AlertDefinitionUUID, task.Version)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ae.logger.Error(
			fmt.Sprintf("associated alert definition for task %q with version %d not found", task.AlertDefinitionUUID.String(), task.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskStateToInvalid(ctx, *task)
	} else if err != nil {
		ae.logger.Error(
			fmt.Sprintf("failed to retrieve alert definition %q with version %d", task.AlertDefinitionUUID.String(), task.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskAsFailed(ctx, *task, ae.executorConfig.RetryLimit)
	}
	err = ae.definitions.SetAlertDefinitionState(ctx, alertDef.TenantID, alertDef.ID, alertDef.Version, models.DefinitionPending)
	if err != nil {
		ae.logger.Error(
			fmt.Sprintf("failed to set alert definition %q with version %d state to 'Pending'", alertDef.ID.String(), alertDef.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskAsFailed(ctx, *task, ae.executorConfig.RetryLimit)
	}

	err = ae.definitionsCfg.UpdateDefinitionConfig(ctx, alertDef)
	if err != nil {
		ae.logger.Error(
			fmt.Sprintf("failed to update Mimir alert definition %q with version %d", alertDef.ID.String(), alertDef.Version),
			slog.Any("error", err),
		)
		return ae.tasks.SetTaskAsFailed(ctx, *task, ae.executorConfig.RetryLimit)
	}

	return ae.tasks.SetTaskAsApplied(ctx, *task)
}

func setLogLvl(logLvl string) slog.HandlerOptions {
	switch logLvl {
	case "debug":
		return slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
	case "info":
		return slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
	case "warn":
		return slog.HandlerOptions{
			Level: slog.LevelWarn,
		}
	case "error":
		return slog.HandlerOptions{
			Level: slog.LevelError,
		}
	default:
		return slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
	}
}
