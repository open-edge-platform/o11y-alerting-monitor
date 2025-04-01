// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/clock"
)

type TaskSuite struct {
	suite.Suite

	db *gorm.DB
}

func (s *TaskSuite) SetupSubTest() {
	clock.SetFakeClock()
	clock.FakeClock.Set(time.Now())

	var err error
	s.db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	s.Require().NoError(err)
	s.Require().NoError(s.db.AutoMigrate(&Task{}))
}

func (s *TaskSuite) TearDownSubTest() {
	clock.UnsetFakeClock()

	s.db.Exec("DELETE FROM tasks")

	dbConn, err := s.db.DB()
	s.Require().NoError(err)
	dbConn.Close()
}

func TestTasks(t *testing.T) {
	suite.Run(t, new(TaskSuite))
}

func (s *TaskSuite) TestBeforeCreate() {
	s.Run("MissingUUID", func() {
		s.Require().ErrorContains(s.db.Create(&Task{}).Error, "alert definition and receiver UUID are nil")
	})

	s.Run("InvalidUUID", func() {
		id := uuid.New()
		s.Require().ErrorContains(s.db.Create(&Task{
			AlertDefinitionUUID: &id,
			ReceiverUUID:        &id,
		}).Error, "both alert definition and receiver UUID are populated")
	})

	s.Run("InvalidState", func() {
		id := uuid.New()
		invalidState := TaskState("In Progress")
		s.Require().ErrorContains(s.db.Create(&Task{
			AlertDefinitionUUID: &id,
			State:               invalidState,
		}).Error, fmt.Sprintf("unknown task state: %q", invalidState))
	})

	s.Run("Succeeded", func() {
		states := []TaskState{TaskNew, TaskTaken, TaskApplied, TaskError, TaskInvalid}

		for _, state := range states {
			id := uuid.New()
			task := Task{
				AlertDefinitionUUID: &id,
				TenantID:            "edgenode",
				State:               state,
				Version:             1,
				CreationDate:        clock.FakeClock.Now(),
			}
			s.Require().NoError(s.db.Create(&task).Error)

			var taskOut Task
			s.Require().NoError(s.db.Find(&taskOut, task.ID).Error)
			s.Require().Equal(task, taskOut)
		}
	})
}

func (s *TaskSuite) TestGetTaskType() {
	s.Run("Definition", func() {
		id := uuid.New()
		task := Task{
			ID:                  1,
			AlertDefinitionUUID: &id,
			TenantID:            "edgenode",
			State:               TaskNew,
			Version:             1,
			CreationDate:        clock.FakeClock.Now(),
		}
		s.Require().NoError(s.db.Create(&task).Error)
		s.Require().Equal(TypeAlertDefinition, task.GetTaskType())
	})

	s.Run("Receiver", func() {
		id := uuid.New()
		task := Task{
			ID:           1,
			ReceiverUUID: &id,
			TenantID:     "edgenode",
			State:        TaskNew,
			Version:      1,
			CreationDate: clock.FakeClock.Now(),
		}
		s.Require().NoError(s.db.Create(&task).Error)
		s.Require().Equal(TypeReceiver, task.GetTaskType())
	})
}

func (s *TaskSuite) TestGetTaskUUID() {
	s.Run("Definition", func() {
		id := uuid.New()
		task := Task{
			ID:                  1,
			AlertDefinitionUUID: &id,
			TenantID:            "edgenode",
			State:               TaskNew,
			Version:             1,
			CreationDate:        clock.FakeClock.Now(),
		}
		s.Require().NoError(s.db.Create(&task).Error)
		s.Require().Equal(id, task.GetTaskUUID())
	})

	s.Run("Receiver", func() {
		id := uuid.New()
		task := Task{
			ID:           1,
			ReceiverUUID: &id,
			TenantID:     "edgenode",
			State:        TaskNew,
			Version:      1,
			CreationDate: clock.FakeClock.Now(),
		}
		s.Require().NoError(s.db.Create(&task).Error)
		s.Require().Equal(id, task.GetTaskUUID())
	})
}
