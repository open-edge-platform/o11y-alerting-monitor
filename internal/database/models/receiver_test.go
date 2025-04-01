// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type ReceiverSuite struct {
	suite.Suite

	db *gorm.DB
}

func (s *ReceiverSuite) SetupSubTest() {
	var err error
	s.db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	s.Require().NoError(err)
	s.Require().NoError(s.db.AutoMigrate(&Receiver{}))
}

func (s *ReceiverSuite) TearDownSubTest() {
	s.db.Exec("DELETE FROM receivers")

	dbConn, err := s.db.DB()
	s.Require().NoError(err)
	dbConn.Close()
}

func TestReceivers(t *testing.T) {
	suite.Run(t, new(ReceiverSuite))
}

func (s *ReceiverSuite) TestBeforeCreate() {
	s.Run("InvalidState", func() {
		invalidState := ReceiverState("In Progress")
		s.Require().ErrorContains(s.db.Create(&Receiver{
			UUID:     uuid.New(),
			State:    invalidState,
			TenantID: "edgenode",
		}).Error, fmt.Sprintf("unknown receiver state: %q", invalidState))
	})

	s.Run("Succeeded", func() {
		states := []ReceiverState{ReceiverNew, ReceiverModified, ReceiverPending, ReceiverApplied, ReceiverError}

		for _, state := range states {
			name := fmt.Sprintf("alert-%s", state)
			recv := Receiver{
				UUID:     uuid.New(),
				State:    state,
				Name:     name,
				TenantID: "edgenode",
			}
			s.Require().NoError(s.db.Create(&recv).Error)

			var recvOut Receiver
			s.Require().NoError(s.db.Find(&recvOut, recv.ID).Error)
			s.Require().Equal(recv, recvOut)
		}
	})
}

func (s *ReceiverSuite) TestAfterUpdate() {
	s.Run("InvalidState", func() {
		recv := Receiver{
			ID:       1,
			UUID:     uuid.New(),
			State:    ReceiverNew,
			Version:  1,
			TenantID: "edgenode",
		}
		s.Require().NoError(s.db.Create(&recv).Error)

		invalidState := AlertDefinitionState("In Progress")
		err := s.db.Model(&recv).Update("state", invalidState).Error
		s.Require().ErrorContains(err, fmt.Sprintf("unknown receiver state: %q", invalidState))
	})

	s.Run("Succeeded", func() {
		recv := Receiver{
			ID:       1,
			UUID:     uuid.New(),
			State:    ReceiverModified,
			TenantID: "edgenode",
		}
		s.Require().NoError(s.db.Create(&recv).Error)

		states := []ReceiverState{ReceiverNew, ReceiverModified, ReceiverPending, ReceiverApplied, ReceiverError}

		for _, state := range states {
			s.Require().NoError(s.db.Model(&recv).Update("state", state).Error)

			var recvOut Receiver
			s.Require().NoError(s.db.Find(&recvOut, recv.ID).Error)
			s.Require().Equal(Receiver{
				ID:       recv.ID,
				UUID:     recv.UUID,
				State:    state,
				TenantID: recv.TenantID,
			}, recvOut)
		}
	})
}
