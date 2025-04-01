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

type AlertDefinitionSuite struct {
	suite.Suite

	db *gorm.DB
}

func (s *AlertDefinitionSuite) SetupSubTest() {
	var err error
	s.db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	s.Require().NoError(err)
	s.Require().NoError(s.db.AutoMigrate(&AlertDefinition{}))
}

func (s *AlertDefinitionSuite) TearDownSubTest() {
	s.db.Exec("DELETE FROM alert_definitions")

	dbConn, err := s.db.DB()
	s.Require().NoError(err)
	dbConn.Close()
}

func TestAlertDefinitions(t *testing.T) {
	suite.Run(t, new(AlertDefinitionSuite))
}

func (s *AlertDefinitionSuite) TestBeforeCreate() {
	s.Run("InvalidState", func() {
		invalidState := AlertDefinitionState("In Progress")
		s.Require().ErrorContains(s.db.Create(&AlertDefinition{
			UUID:     uuid.New(),
			Category: CategoryPerformance,
			State:    invalidState,
			TenantID: "edgenode",
		}).Error, fmt.Sprintf("unknown alert definition state: %q", invalidState))
	})

	s.Run("InvalidCategory", func() {
		invalidCategory := AlertDefinitionCategory("testing")
		s.Require().ErrorContains(s.db.Create(&AlertDefinition{
			UUID:     uuid.New(),
			State:    DefinitionNew,
			Category: invalidCategory,
			TenantID: "edgenode",
		}).Error, fmt.Sprintf("unknown alert definition category: %q", invalidCategory))
	})

	s.Run("Succeeded", func() {
		states := []AlertDefinitionState{DefinitionNew, DefinitionPending, DefinitionModified, DefinitionError, DefinitionApplied}

		for _, state := range states {
			name := fmt.Sprintf("alert-%s", state)
			ad := AlertDefinition{
				UUID:     uuid.New(),
				State:    state,
				Name:     name,
				Version:  1,
				Category: CategoryPerformance,
				TenantID: "edgenode",
			}
			s.Require().NoError(s.db.Create(&ad).Error)

			var adOut AlertDefinition
			s.Require().NoError(s.db.Find(&adOut, ad.ID).Error)
			s.Require().Equal(ad, adOut)
		}

		categories := []AlertDefinitionCategory{CategoryPerformance, CategoryHealth, CategoryMaintenance}

		for _, category := range categories {
			name := fmt.Sprintf("alert-%s", category)
			ad := AlertDefinition{
				UUID:     uuid.New(),
				State:    DefinitionNew,
				Name:     name,
				Category: category,
				Version:  1,
				TenantID: "edgenode",
			}
			s.Require().NoError(s.db.Create(&ad).Error)

			var adOut AlertDefinition
			s.Require().NoError(s.db.Find(&adOut, ad.ID).Error)
			s.Require().Equal(ad, adOut)
		}
	})
}

func (s *AlertDefinitionSuite) TestAfterUpdate() {
	s.Run("InvalidCategory", func() {
		ad := AlertDefinition{
			ID:       1,
			UUID:     uuid.New(),
			State:    DefinitionApplied,
			Category: CategoryPerformance,
			Version:  1,
			TenantID: "edgenode",
		}
		s.Require().NoError(s.db.Create(&ad).Error)

		invalidCategory := AlertDefinitionCategory("testing")
		err := s.db.Model(&ad).Update("category", invalidCategory).Error
		s.Require().ErrorContains(err, fmt.Sprintf("unknown alert definition category: %q", invalidCategory))
	})

	s.Run("InvalidState", func() {
		ad := AlertDefinition{
			ID:       1,
			UUID:     uuid.New(),
			State:    DefinitionApplied,
			Category: CategoryPerformance,
			Version:  1,
			TenantID: "edgenode",
		}
		s.Require().NoError(s.db.Create(&ad).Error)

		invalidState := AlertDefinitionState("In Progress")
		err := s.db.Model(&ad).Update("state", invalidState).Error
		s.Require().ErrorContains(err, fmt.Sprintf("unknown alert definition state: %q", invalidState))
	})

	s.Run("Succeeded", func() {
		ad := AlertDefinition{
			ID:       1,
			UUID:     uuid.New(),
			State:    DefinitionApplied,
			Category: CategoryPerformance,
			Version:  1,
			TenantID: "edgenode",
		}
		s.Require().NoError(s.db.Create(&ad).Error)

		states := []AlertDefinitionState{DefinitionNew, DefinitionPending, DefinitionModified, DefinitionError, DefinitionApplied}

		for _, state := range states {
			s.Require().NoError(s.db.Model(&ad).Update("state", state).Error)

			var adOut AlertDefinition
			s.Require().NoError(s.db.Find(&adOut, ad.ID).Error)
			s.Require().Equal(AlertDefinition{
				ID:       ad.ID,
				UUID:     ad.UUID,
				Category: ad.Category,
				State:    state,
				Version:  ad.Version,
				TenantID: ad.TenantID,
			}, adOut)
		}

		categories := []AlertDefinitionCategory{CategoryPerformance, CategoryHealth, CategoryMaintenance}

		for _, category := range categories {
			s.Require().NoError(s.db.Model(&ad).Update("category", category).Error)

			var adOut AlertDefinition
			s.Require().NoError(s.db.Find(&adOut, ad.ID).Error)
			s.Require().Equal(AlertDefinition{
				ID:       ad.ID,
				UUID:     ad.UUID,
				Category: category,
				State:    ad.State,
				Version:  ad.Version,
				TenantID: ad.TenantID,
			}, adOut)
		}
	})
}
