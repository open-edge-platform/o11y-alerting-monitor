// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/clock"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

var defTemplate = `alert: TestAlertDef
annotations:
description: CPU usage has exceeded 80%
summary: High CPU usage detected
expr: cpu_usage > {{ .Threshold }}
for: 1m
labels:
alert_category: performance
alert_context: host
duration: 1m
threshold: "80"
`

type DefConfigMock struct {
	mock.Mock
}

func (m *DefConfigMock) UpdateDefinitionConfig(ctx context.Context, aDef *models.DBAlertDefinition) error {
	args := m.Called(ctx, aDef)
	return args.Error(0)
}

type RecvConfigMock struct {
	mock.Mock
}

func (m *RecvConfigMock) UpdateReceiverConfig(ctx context.Context, receiver models.DBReceiver) error {
	args := m.Called(ctx, receiver)
	return args.Error(0)
}

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

type ExecuteReceiverTaskSuite struct {
	suite.Suite

	db    *gorm.DB
	dbSrv database.DBService

	recv *models.DBReceiver
	task *models.Task
}

func (s *ExecuteReceiverTaskSuite) SetupSubTest() {
	clock.SetFakeClock()
	clock.FakeClock.Set(time.Now())

	var err error
	s.db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	s.Require().NoError(err)

	s.Require().NoError(s.db.AutoMigrate(
		&models.EmailAddress{},
		&models.EmailConfig{},
		&models.Receiver{},
		&models.EmailRecipient{},
		&models.Task{},
	))

	s.dbSrv = database.DBService{DB: s.db}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	recvInfo := &models.DBReceiver{}

	// Add default tenant ID
	recvInfo.TenantID = "edgenode"

	// Create email address of the sender.
	fromEmailID := int64(10)
	sender := &models.EmailAddress{
		ID:        fromEmailID,
		FirstName: "testOrg",
		LastName:  "testSubOrg",
		Email:     "test_org@email.com",
	}
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(sender).Error)
	recvInfo.From = sender.String()

	// Create email config.
	emailConfigID := int64(100)
	mailServer := "smtp.server.com"
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&models.EmailConfig{
		ID:         emailConfigID,
		MailServer: mailServer,
		From:       fromEmailID,
	}).Error)
	recvInfo.MailServer = mailServer

	// Create new receiver with associated email config.
	recvInfo.UUID = uuid.New()
	recvInfo.State = models.ReceiverModified
	recvInfo.Name = "receiver"
	recvInfo.TenantID = "edgenode"
	recvInfo.Version = 5
	receiverID := int64(10)
	recv := models.Receiver{
		ID:            receiverID,
		UUID:          recvInfo.UUID,
		Name:          recvInfo.Name,
		State:         recvInfo.State,
		Version:       int64(recvInfo.Version),
		EmailConfigID: emailConfigID,
		TenantID:      recvInfo.TenantID,
	}
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&recv).Error)

	// Create a recipient email address.
	toEmailID := int64(100)
	recipient := &models.EmailAddress{
		ID:        toEmailID,
		FirstName: "first",
		LastName:  "user",
		Email:     "first.user@email.com",
	}
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(recipient).Error)
	recvInfo.To = append(recvInfo.To, recipient.String())

	// Set email recipient to the receiver.
	emailRecipientID := int64(99)
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&models.EmailRecipient{
		ID:             emailRecipientID,
		ReceiverID:     receiverID,
		EmailAddressID: toEmailID,
	}).Error)

	recvInfoOut, err := s.dbSrv.GetReceiverWithEmailConfig(ctx, recv.TenantID, recv.UUID, recv.Version)
	s.Require().NoError(err)
	s.Require().Equal(recvInfo, recvInfoOut)

	// Create receiver task.
	taskID := int64(10)
	creationDate := clock.FakeClock.Now().UTC()
	recvTask := &models.Task{
		ID:           taskID,
		State:        models.TaskNew,
		ReceiverUUID: &recv.UUID,
		Version:      recv.Version,
		CreationDate: creationDate,
		TenantID:     recv.TenantID,
	}
	// TODO: If not in the db the SetInvalidState method fails.
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(recvTask).Error)

	s.recv = recvInfo
	s.task = recvTask
}

func (s *ExecuteReceiverTaskSuite) TearDownSubTest() {
	clock.UnsetFakeClock()

	s.db.Exec("DELETE FROM tasks")
	s.db.Exec("DELETE FROM email_recipients")
	s.db.Exec("DELETE FROM receivers")
	s.db.Exec("DELETE FROM email_configs")
	s.db.Exec("DELETE FROM email_addresses")

	dbConn, err := s.db.DB()
	s.Require().NoError(err)
	dbConn.Close()
}

func TestAsyncExecutorReceiver(t *testing.T) {
	t.Setenv("TZ", "UTC")
	suite.Run(t, new(ExecuteReceiverTaskSuite))
}

func (s *ExecuteReceiverTaskSuite) TestExecuteTask() {
	s.Run("Fails to execute task", func() {
		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			receivers: &database.DBService{DB: s.db},
			tasks:     &database.DBService{DB: s.db},
			logger:    slog.New(slog.NewTextHandler(os.Stdout, nil)),
		}

		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(errors.New("mock error")).Once()
		aExec.receiversCfg = mReceivers

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Task gets updated into db with TaskError state.
		s.Require().NoError(aExec.executeTask(ctx, s.task))

		// Check task has error state.
		var taskOut models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).First(&taskOut, s.task.ID).Error)
		s.Require().Equal(models.Task{
			ID:           s.task.ID,
			ReceiverUUID: s.task.ReceiverUUID,
			Version:      s.task.Version,
			State:        models.TaskError,
			CreationDate: s.task.CreationDate,
			RetryCount:   1,
			TenantID:     s.task.TenantID,
		}, taskOut)

		// Check receiver status was set to error as well.
		recvInfoOut, err := aExec.receivers.GetReceiverWithEmailConfig(ctx, s.recv.TenantID, s.recv.UUID, int64(s.recv.Version))
		s.Require().NoError(err)
		s.Require().Equal(&models.DBReceiver{
			UUID:       s.recv.UUID,
			Name:       s.recv.Name,
			Version:    s.recv.Version,
			MailServer: s.recv.MailServer,
			From:       s.recv.From,
			To:         s.recv.To,
			State:      models.ReceiverError,
			TenantID:   s.recv.TenantID,
		}, recvInfoOut)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})

	s.Run("Fails to execute a task due to timeout exceeded", func() {
		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:     2,
				RetryLimit:    5,
				TaskTimeout:   1 * time.Nanosecond,
				RetentionTime: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			quit:   make(chan struct{}),

			tasks:     &database.DBService{DB: s.db},
			receivers: &database.DBService{DB: s.db},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Task gets updated into db with TaskError state.
		s.Require().Error(aExec.executeTask(ctx, s.task))

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:           s.task.ID,
				ReceiverUUID: s.task.ReceiverUUID,
				State:        models.TaskError,
				Version:      s.task.Version,
				CreationDate: s.task.CreationDate,
				RetryCount:   s.task.RetryCount + 1,
				TenantID:     s.task.TenantID,
				// StartDate:    clock.FakeClock.Now().UTC(),
			},
		}, res)
	})

	s.Run("Succeeds to execute task", func() {
		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(nil).Once()

		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			receivers: &database.DBService{DB: s.db},
			tasks:     &database.DBService{DB: s.db},
			logger:    slog.New(slog.NewTextHandler(os.Stdout, nil)),

			receiversCfg: mReceivers,
		}

		clock.FakeClock.Set(clock.FakeClock.Now().Add(10 * time.Second))
		completionDate := clock.FakeClock.Now().UTC()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Task gets stored into db with TaskApplied
		s.Require().NoError(aExec.executeTask(ctx, s.task))

		var updatedTask models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).First(&updatedTask, s.task.ID).Error)
		s.Require().Equal(models.Task{
			ID:             s.task.ID,
			State:          models.TaskApplied,
			ReceiverUUID:   s.task.ReceiverUUID,
			Version:        s.task.Version,
			CreationDate:   s.task.CreationDate,
			CompletionDate: completionDate,
			TenantID:       s.task.TenantID,
		}, updatedTask)

		// Check receiver status was set to applied.
		recvInfoOut, err := aExec.receivers.GetReceiverWithEmailConfig(ctx, s.recv.TenantID, s.recv.UUID, int64(s.recv.Version))
		s.Require().NoError(err)
		s.Require().Equal(&models.DBReceiver{
			UUID:       s.recv.UUID,
			Name:       s.recv.Name,
			Version:    s.recv.Version,
			MailServer: s.recv.MailServer,
			From:       s.recv.From,
			To:         s.recv.To,
			State:      models.ReceiverApplied,
			TenantID:   s.recv.TenantID,
		}, recvInfoOut)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})
}

func (s *ExecuteReceiverTaskSuite) TestExecutor() {
	// 1. Test that checks if the task was taken and applied.
	s.Run("A new task is taken and successfully applied", func() {
		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(nil).Once()

		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:     2,
				RetryLimit:    5,
				PoolingRate:   10 * time.Millisecond,
				TaskTimeout:   30 * time.Second,
				RetentionTime: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			quit:   make(chan struct{}),

			tasks:        &database.DBService{DB: s.db},
			receivers:    &database.DBService{DB: s.db},
			receiversCfg: mReceivers,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Advance time.
		clock.FakeClock.Set(clock.FakeClock.Now().Add(2 * time.Second))

		aExec.Start(ctx)
		<-time.After(200 * time.Millisecond)
		aExec.Stop()

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:             s.task.ID,
				ReceiverUUID:   s.task.ReceiverUUID,
				State:          models.TaskApplied,
				Version:        s.task.Version,
				CreationDate:   s.task.CreationDate,
				StartDate:      clock.FakeClock.Now().UTC(),
				CompletionDate: clock.FakeClock.Now().UTC(),
				TenantID:       s.task.TenantID,
			},
		}, res)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})

	// 2. Test that checks when we insert two of the same type of tasks with different version.
	// Check if the older one is invalidated.
	s.Run("Only the latest version of a task is applied", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		olderTask := models.Task{
			ID:           s.task.ID + 1,
			ReceiverUUID: s.task.ReceiverUUID,
			State:        s.task.State,
			Version:      1,
			CreationDate: s.task.CreationDate,
			TenantID:     s.task.TenantID,
		}
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&olderTask).Error)

		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(nil).Once()

		ownerUUID := uuid.New()
		aExec := &asyncExecutor{
			ownerUUID: ownerUUID,
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:     2,
				RetryLimit:    5,
				PoolingRate:   10 * time.Millisecond,
				TaskTimeout:   30 * time.Second,
				RetentionTime: 5 * time.Minute,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			quit:   make(chan struct{}),

			tasks:     &database.DBService{DB: s.db},
			receivers: &database.DBService{DB: s.db},

			receiversCfg: mReceivers,
		}

		// Advance time.
		clock.FakeClock.Set(clock.FakeClock.Now().Add(2 * time.Second))

		aExec.Start(ctx)
		<-time.After(500 * time.Millisecond)
		aExec.Stop()

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:             s.task.ID,
				OwnerUUID:      ownerUUID,
				ReceiverUUID:   s.task.ReceiverUUID,
				State:          models.TaskApplied,
				Version:        s.task.Version,
				CreationDate:   s.task.CreationDate,
				StartDate:      clock.FakeClock.Now().UTC(),
				CompletionDate: clock.FakeClock.Now().UTC(),
				TenantID:       s.task.TenantID,
			},
			{
				ID:             olderTask.ID,
				ReceiverUUID:   olderTask.ReceiverUUID,
				State:          models.TaskInvalid,
				Version:        olderTask.Version,
				CreationDate:   olderTask.CreationDate,
				CompletionDate: clock.FakeClock.Now().UTC(),
				TenantID:       s.task.TenantID,
			},
		}, res)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})

	s.Run("Taken tasks are set to fail if timeout is exceeded", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		ownerUUID := uuid.New()
		retryLimit := 5

		recv := &models.Receiver{
			ID:       30,
			UUID:     uuid.New(),
			State:    models.ReceiverModified,
			Version:  1,
			TenantID: "edgenode",
		}
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(recv).Error)

		takenTask := models.Task{
			ID:           30,
			OwnerUUID:    ownerUUID,
			ReceiverUUID: &recv.UUID,
			Version:      recv.Version,
			State:        models.TaskTaken,
			CreationDate: clock.FakeClock.Now().UTC(),
			StartDate:    clock.FakeClock.Now().UTC(),
			RetryCount:   int64(retryLimit),
			TenantID:     recv.TenantID,
		}
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&takenTask).Error)

		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(nil).Once()

		aExec := &asyncExecutor{
			ownerUUID: ownerUUID,
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:     2,
				RetryLimit:    5,
				PoolingRate:   10 * time.Millisecond,
				TaskTimeout:   30 * time.Second,
				RetentionTime: 50 * time.Minute,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			quit:   make(chan struct{}),

			receivers: &database.DBService{DB: s.db},
			tasks:     &database.DBService{DB: s.db},

			receiversCfg: mReceivers,
		}

		// Advance time.
		clock.FakeClock.Set(clock.FakeClock.Now().Add(1 * time.Minute))

		aExec.Start(ctx)
		<-time.After(1 * time.Second)
		aExec.Stop()

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:             s.task.ID,
				OwnerUUID:      ownerUUID,
				ReceiverUUID:   s.task.ReceiverUUID,
				State:          models.TaskApplied,
				Version:        s.task.Version,
				CreationDate:   s.task.CreationDate,
				StartDate:      clock.FakeClock.Now().UTC(),
				CompletionDate: clock.FakeClock.Now().UTC(),
				TenantID:       s.task.TenantID,
			},
			{
				ID:             takenTask.ID,
				OwnerUUID:      ownerUUID,
				ReceiverUUID:   takenTask.ReceiverUUID,
				State:          models.TaskInvalid,
				CreationDate:   takenTask.CreationDate,
				StartDate:      takenTask.StartDate,
				CompletionDate: clock.FakeClock.Now().UTC(),
				RetryCount:     takenTask.RetryCount,
				Version:        takenTask.Version,
				TenantID:       takenTask.TenantID,
			},
		}, res)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})

	// 3. Check if applied and invalid tasks will be deleted.
	s.Run("Applied and invalid tasks get deleted after exceeding retention period", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		appliedTask := models.Task{
			ID:             4,
			ReceiverUUID:   uuidPtr(uuid.New()),
			State:          models.TaskApplied,
			Version:        1,
			CreationDate:   clock.FakeClock.Now(),
			StartDate:      clock.FakeClock.Now().UTC(),
			CompletionDate: clock.FakeClock.Now().UTC(),
			TenantID:       "edgenode",
		}
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&appliedTask).Error)

		invalidTask := models.Task{
			ID:             5,
			ReceiverUUID:   uuidPtr(uuid.New()),
			State:          models.TaskInvalid,
			Version:        1,
			CreationDate:   clock.FakeClock.Now(),
			CompletionDate: clock.FakeClock.Now().UTC(),
			TenantID:       "edgenode",
		}
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&invalidTask).Error)

		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(nil).Once()

		ownerUUID := uuid.New()
		aExec := &asyncExecutor{
			ownerUUID: ownerUUID,
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:     2,
				RetryLimit:    5,
				PoolingRate:   10 * time.Millisecond,
				TaskTimeout:   30 * time.Second,
				RetentionTime: 1 * time.Minute,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			quit:   make(chan struct{}),

			tasks:     &database.DBService{DB: s.db},
			receivers: &database.DBService{DB: s.db},

			receiversCfg: mReceivers,
		}

		// Advance time.
		clock.FakeClock.Set(clock.FakeClock.Now().Add(5 * time.Minute))

		aExec.Start(ctx)
		<-time.After(600 * time.Millisecond)
		aExec.Stop()

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:             s.task.ID,
				OwnerUUID:      ownerUUID,
				ReceiverUUID:   s.task.ReceiverUUID,
				State:          models.TaskApplied,
				Version:        s.task.Version,
				CreationDate:   s.task.CreationDate,
				StartDate:      clock.FakeClock.Now().UTC(),
				CompletionDate: clock.FakeClock.Now().UTC(),
				TenantID:       s.task.TenantID,
			},
		}, res)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})

	// 8. Check if errored out task will be taken and retried and it's retry count has been incremented.
	s.Run("A task is applied after failing but not exceeding retry limit", func() {
		retries := 3

		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(errors.New("mock error")).Once()

		errorRecv := *s.recv
		errorRecv.State = models.ReceiverError
		mReceivers.On("UpdateReceiverConfig", mock.Anything, errorRecv).Return(errors.New("mock error")).Times(retries - 1)
		mReceivers.On("UpdateReceiverConfig", mock.Anything, errorRecv).Return(nil).Once()

		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:     2,
				RetryLimit:    5,
				PoolingRate:   10 * time.Millisecond,
				TaskTimeout:   30 * time.Second,
				RetentionTime: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			quit:   make(chan struct{}),

			tasks:     &database.DBService{DB: s.db},
			receivers: &database.DBService{DB: s.db},

			receiversCfg: mReceivers,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		aExec.Start(ctx)
		<-time.After(100 * time.Millisecond)
		aExec.Stop()

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:             s.task.ID,
				ReceiverUUID:   s.task.ReceiverUUID,
				State:          models.TaskApplied,
				Version:        s.task.Version,
				RetryCount:     int64(retries),
				CreationDate:   s.task.CreationDate,
				StartDate:      clock.FakeClock.Now().UTC(),
				CompletionDate: clock.FakeClock.Now().UTC(),
				TenantID:       s.task.TenantID,
			},
		}, res)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})

	s.Run("A task is set to Invalid state after exceeding retry limit", func() {
		retryLimit := 5

		mReceivers := &RecvConfigMock{}
		mReceivers.On("UpdateReceiverConfig", mock.Anything, *s.recv).Return(errors.New("mock error")).Once()

		errorRecv := *s.recv
		errorRecv.State = models.ReceiverError
		mReceivers.On("UpdateReceiverConfig", mock.Anything, errorRecv).Return(errors.New("mock error")).Times(retryLimit)

		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:     2,
				RetryLimit:    retryLimit,
				PoolingRate:   10 * time.Millisecond,
				TaskTimeout:   30 * time.Second,
				RetentionTime: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			quit:   make(chan struct{}),

			tasks:     &database.DBService{DB: s.db},
			receivers: &database.DBService{DB: s.db},

			receiversCfg: mReceivers,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		aExec.Start(ctx)
		<-time.After(100 * time.Millisecond)
		aExec.Stop()

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:             s.task.ID,
				ReceiverUUID:   s.task.ReceiverUUID,
				State:          models.TaskInvalid,
				Version:        s.task.Version,
				RetryCount:     int64(retryLimit),
				CreationDate:   s.task.CreationDate,
				StartDate:      clock.FakeClock.Now().UTC(),
				CompletionDate: clock.FakeClock.Now().UTC(),
				TenantID:       s.task.TenantID,
			},
		}, res)

		s.Require().True(mReceivers.AssertExpectations(s.T()))
	})
}

type ExecuteDefinitionTaskTestSuite struct {
	suite.Suite

	db    *gorm.DB
	dbSrv database.DBService

	def  *models.DBAlertDefinition
	task *models.Task
}

func TestAsyncExecutorDefinitionUpdate(t *testing.T) {
	t.Setenv("TZ", "UTC")
	suite.Run(t, new(ExecuteDefinitionTaskTestSuite))
}

func (s *ExecuteDefinitionTaskTestSuite) SetupSubTest() {
	clock.SetFakeClock()
	clock.FakeClock.Set(time.Now())

	var err error
	s.db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	s.Require().NoError(err)

	s.Require().NoError(s.db.AutoMigrate(
		&models.Task{},
		&models.AlertDefinition{},
		&models.AlertThreshold{},
		&models.AlertDuration{},
	))

	// TODO: To be removed.
	s.dbSrv = database.DBService{DB: s.db}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	defID := int64(99)
	// Create alert threshold
	threshold := &models.AlertThreshold{
		Name:              "test-threshold",
		Threshold:         90,
		ThresholdMin:      50,
		ThresholdMax:      100,
		AlertDefinitionID: defID,
	}
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(threshold).Error)

	duration := &models.AlertDuration{
		Name:              "test-duration",
		Duration:          60,
		DurationMin:       10,
		DurationMax:       90,
		AlertDefinitionID: defID,
	}
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&duration).Error)

	def := &models.AlertDefinition{
		ID:            defID,
		UUID:          uuid.New(),
		Version:       3,
		Name:          "test-alert-definition",
		Template:      defTemplate,
		Category:      models.CategoryHealth,
		State:         models.DefinitionNew,
		Severity:      "High",
		AlertInterval: int64(15),
		Enabled:       true,
		TenantID:      "edgenode",
	}
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(def).Error)

	s.def = &models.DBAlertDefinition{
		ID:       def.UUID,
		Name:     def.Name,
		State:    def.State,
		Template: def.Template,
		Category: def.Category,
		Values: models.DBAlertDefinitionValues{
			Duration:  &duration.Duration,
			Threshold: &threshold.Threshold,
			Enabled:   &def.Enabled,
		},
		Interval: def.AlertInterval,
		Version:  def.Version,
		TenantID: def.TenantID,
	}

	defTask := &models.Task{
		ID:                  int64(9),
		State:               models.TaskNew,
		AlertDefinitionUUID: &def.UUID,
		Version:             def.Version,
		CreationDate:        clock.FakeClock.Now().UTC(),
		TenantID:            "edgenode",
	}
	s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(defTask).Error)

	s.task = defTask
}

func (s *ExecuteDefinitionTaskTestSuite) TearDownSubTest() {
	clock.UnsetFakeClock()

	s.db.Exec("DELETE FROM tasks")
	s.db.Exec("DELETE FROM alert_thresholds")
	s.db.Exec("DELETE FROM alert_durations")
	s.db.Exec("DELETE FROM alert_definitions")

	dbConn, err := s.db.DB()
	s.Require().NoError(err)
	dbConn.Close()
}

func (s *ExecuteDefinitionTaskTestSuite) TestProcessTasks() {
	// TODO: Add test where there are no pending tasks.
	s.Run("Succeeded to apply a new task", func() {
		mDefinitions := &DefConfigMock{}
		mDefinitions.On("UpdateDefinitionConfig", mock.Anything, s.def).Return(nil).Once()

		ownerUUID := uuid.New()
		aExec := &asyncExecutor{
			ownerUUID: ownerUUID,
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),

			definitions: &database.DBService{DB: s.db},
			tasks:       &database.DBService{DB: s.db},

			definitionsCfg: mDefinitions,
		}

		// Advance time.
		clock.FakeClock.Set(clock.FakeClock.Now().Add(5 * time.Second))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		aExec.processTasks(ctx)

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:                  s.task.ID,
				OwnerUUID:           ownerUUID,
				AlertDefinitionUUID: s.task.AlertDefinitionUUID,
				State:               models.TaskApplied,
				Version:             s.task.Version,
				CreationDate:        s.task.CreationDate,
				StartDate:           clock.FakeClock.Now().UTC(),
				CompletionDate:      clock.FakeClock.Now().UTC(),
				TenantID:            s.task.TenantID,
			},
		}, res)

		s.Require().True(mDefinitions.AssertExpectations(s.T()))
	})

	// 8. Check if errored out task will be taken and retried and it's retry count has been incremented.
	s.Run("Failed to process new task", func() {
		mDefinitions := &DefConfigMock{}
		mDefinitions.On("UpdateDefinitionConfig", mock.Anything, s.def).Return(errors.New("mock error")).Once()

		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),

			definitions: &database.DBService{DB: s.db},
			tasks:       &database.DBService{DB: s.db},

			definitionsCfg: mDefinitions,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Advance time.
		clock.FakeClock.Set(clock.FakeClock.Now().Add(5 * time.Second))

		aExec.processTasks(ctx)

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:                  s.task.ID,
				AlertDefinitionUUID: s.task.AlertDefinitionUUID,
				State:               models.TaskError,
				Version:             s.task.Version,
				RetryCount:          s.task.RetryCount + 1,
				CreationDate:        s.task.CreationDate,
				StartDate:           clock.FakeClock.Now().UTC(),
				TenantID:            s.task.TenantID,
			},
		}, res)

		s.Require().True(mDefinitions.AssertExpectations(s.T()))
	})

	// 2. Test that checks when we insert two of the same type of tasks with different version.
	// Check if the older one is invalidated.
	s.Run("Process only the latest version of a task", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		olderTask := models.Task{
			ID:                  s.task.ID + 1,
			AlertDefinitionUUID: s.task.AlertDefinitionUUID,
			State:               models.TaskNew,
			Version:             1,
			CreationDate:        clock.FakeClock.Now(),
			TenantID:            s.task.TenantID,
		}
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(&olderTask).Error)

		mDefinitions := &DefConfigMock{}
		mDefinitions.On("UpdateDefinitionConfig", mock.Anything, s.def).Return(nil).Once()

		ownerUUID := uuid.New()
		aExec := &asyncExecutor{
			ownerUUID: ownerUUID,
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),

			definitions: &database.DBService{DB: s.db},
			tasks:       &database.DBService{DB: s.db},

			definitionsCfg: mDefinitions,
		}

		// Advance time.
		clock.FakeClock.Set(clock.FakeClock.Now().Add(5 * time.Second))

		aExec.processTasks(ctx)

		var res []models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Find(&res).Error)
		s.Require().Equal([]models.Task{
			{
				ID:                  s.task.ID,
				OwnerUUID:           ownerUUID,
				AlertDefinitionUUID: s.task.AlertDefinitionUUID,
				State:               models.TaskApplied,
				Version:             s.task.Version,
				CreationDate:        s.task.CreationDate,
				StartDate:           clock.FakeClock.Now().UTC(),
				CompletionDate:      clock.FakeClock.Now().UTC(),
				TenantID:            s.task.TenantID,
			},
			{
				ID:                  olderTask.ID,
				AlertDefinitionUUID: olderTask.AlertDefinitionUUID,
				State:               models.TaskInvalid,
				Version:             olderTask.Version,
				CreationDate:        olderTask.CreationDate,
				CompletionDate:      clock.FakeClock.Now().UTC(),
				TenantID:            s.task.TenantID,
			},
		}, res)

		s.Require().True(mDefinitions.AssertExpectations(s.T()))
	})
}

func (s *ExecuteDefinitionTaskTestSuite) TestExecuteTask() {
	s.Run("Definition update task failed - alert definition not found in DB", func() {
		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),

			tasks:       &database.DBService{DB: s.db},
			definitions: &database.DBService{DB: s.db},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		defTask := &models.Task{
			ID:                  99,
			State:               models.TaskNew,
			AlertDefinitionUUID: uuidPtr(uuid.New()),
			Version:             2,
			CreationDate:        clock.FakeClock.Now().UTC(),
			TenantID:            "edgenode",
		}
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).Create(defTask).Error)

		clock.FakeClock.Set(clock.FakeClock.Now().Add(1 * time.Second))

		// Task gets stored into db with StateError
		s.Require().NoError(aExec.executeTask(ctx, defTask))

		var updatedTask models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).First(&updatedTask, defTask.ID).Error)
		s.Require().Equal(models.Task{
			ID:                  defTask.ID,
			State:               models.TaskInvalid,
			AlertDefinitionUUID: defTask.AlertDefinitionUUID,
			Version:             defTask.Version,
			CreationDate:        defTask.CreationDate,
			CompletionDate:      clock.FakeClock.Now().UTC(),
			RetryCount:          defTask.RetryCount,
			TenantID:            defTask.TenantID,
		}, updatedTask)
	})

	s.Run("Definition update task error - UpdateDefinitionConfig returns error", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		mDefinitions := &DefConfigMock{}
		mDefinitions.On("UpdateDefinitionConfig", mock.Anything, s.def).Return(errors.New("mock error")).Once()

		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),

			tasks:          &database.DBService{DB: s.db},
			definitions:    &database.DBService{DB: s.db},
			definitionsCfg: mDefinitions,
		}

		// Task gets stored into db with StateError
		s.Require().NoError(aExec.executeTask(ctx, s.task))

		var updatedTask models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).First(&updatedTask, s.task.ID).Error)
		s.Require().Equal(models.Task{
			ID:                  s.task.ID,
			AlertDefinitionUUID: s.task.AlertDefinitionUUID,
			Version:             s.task.Version,
			State:               models.TaskError,
			CreationDate:        s.task.CreationDate,
			RetryCount:          1,
			TenantID:            s.task.TenantID,
		}, updatedTask)

		defInfoOut, err := aExec.definitions.GetAlertDefinition(ctx, s.def.TenantID, s.def.ID, s.def.Version)
		s.Require().NoError(err)
		s.Require().Equal(&models.DBAlertDefinition{
			ID:       s.def.ID,
			Name:     s.def.Name,
			State:    models.DefinitionError,
			Template: s.def.Template,
			Category: s.def.Category,
			Values:   s.def.Values,
			Interval: s.def.Interval,
			Version:  s.def.Version,
			TenantID: s.def.TenantID,
		}, defInfoOut)

		s.Require().True(mDefinitions.AssertExpectations(s.T()))
	})

	// Shows how fields from task change. Task status is StatusApplied, CompletionTime is set.
	s.Run("Definition update task succeeded", func() {
		mDefinitions := &DefConfigMock{}
		mDefinitions.On("UpdateDefinitionConfig", mock.Anything, s.def).Return(nil).Once()

		aExec := &asyncExecutor{
			executorConfig: config.TaskExecutorConfig{
				UUIDLimit:   2,
				RetryLimit:  5,
				TaskTimeout: 90 * time.Second,
			},
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),

			tasks:          &database.DBService{DB: s.db},
			definitions:    &database.DBService{DB: s.db},
			definitionsCfg: mDefinitions,
		}

		clock.FakeClock.Set(clock.FakeClock.Now().Add(10 * time.Second))
		completionDate := clock.FakeClock.Now().UTC()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Task gets stored into db with StateApplied
		s.Require().NoError(aExec.executeTask(ctx, s.task))

		var updatedTask models.Task
		s.Require().NoError(s.dbSrv.DB.WithContext(ctx).First(&updatedTask, s.task.ID).Error)
		s.Require().Equal(models.Task{
			ID:                  s.task.ID,
			State:               models.TaskApplied,
			AlertDefinitionUUID: s.task.AlertDefinitionUUID,
			Version:             s.task.Version,
			CreationDate:        s.task.CreationDate,
			CompletionDate:      completionDate,
			TenantID:            s.task.TenantID,
		}, updatedTask)

		defInfoOut, err := aExec.definitions.GetAlertDefinition(ctx, s.def.TenantID, s.def.ID, s.def.Version)
		s.Require().NoError(err)
		s.Require().Equal(&models.DBAlertDefinition{
			ID:       s.def.ID,
			Name:     s.def.Name,
			State:    models.DefinitionApplied,
			Template: s.def.Template,
			Category: s.def.Category,
			Values:   s.def.Values,
			Interval: s.def.Interval,
			Version:  s.def.Version,
			TenantID: s.def.TenantID,
		}, defInfoOut)

		s.Require().True(mDefinitions.AssertExpectations(s.T()))
	})
}
