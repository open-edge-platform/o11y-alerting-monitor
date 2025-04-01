// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package database_test

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/internal/clock"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

const (
	dbQueryTimeout = 5 * time.Second
)

var db *database.DBService

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

var _ = Describe("Database", func() {
	BeforeEach(func() {
		dbConn, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"))
		Expect(err).ToNot(HaveOccurred())
		db = &database.DBService{dbConn}

		clock.SetFakeClock()
		clock.FakeClock.Set(time.Now())
	})

	AfterEach(func() {
		clock.UnsetFakeClock()

		if db == nil {
			return
		}
		dbConn, err := db.DB.DB()
		Expect(err).ToNot(HaveOccurred())
		Expect(dbConn.Close()).To(Succeed())
	})

	Describe("Alert definitions", func() {
		BeforeEach(func() {
			Expect(db.DB.AutoMigrate(
				&models.AlertDuration{},
				&models.AlertThreshold{},
				&models.AlertDefinition{},
				&models.Task{},
			)).ShouldNot(HaveOccurred())
		})

		Context("With no alert definitions", func() {
			It("Get empty list with latest versions of successfully applied alert definitions because alert_definitions table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				tenantID := "edgenode"
				alertDefinitions, err := db.GetLatestAlertDefinitionList(ctx, tenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(alertDefinitions).To(BeEmpty())
			})

			It("Fail to get the latest version of a successfully applied alert definition because alert_definitions table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				tenantID := "edgenode"
				alertDefinition, err := db.GetLatestAlertDefinition(ctx, tenantID, uuid.New())
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(alertDefinition).To(BeNil())
			})

			It("Fail to get specific version of an alert definition because alert_definitions table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				tenantID := "edgenode"
				res, err := db.GetAlertDefinition(ctx, tenantID, uuid.New(), 1)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(res).To(BeNil())
			})

			It("Fail to set the values of an alert definition because alert_definitions table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()
				tenantID := "edgenode"

				err := db.SetAlertDefinitionValues(ctx, tenantID, uuid.New(), models.DBAlertDefinitionValues{})
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
			})

			It("Fail to set the state of an alert definition because alert_definitions table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()
				tenantID := "edgenode"

				err := db.SetAlertDefinitionState(ctx, tenantID, uuid.New(), 1, models.DefinitionNew)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
			})
		})

		Context("With alert definitions stored", func() {
			var (
				defInfoInitial  *models.DBAlertDefinition // Represents the first version of an alert definition.
				defInfoModified *models.DBAlertDefinition // Represents a modified version of the previous alert definition that was successfully applied.
				defInfoError    *models.DBAlertDefinition // Represents a new version of the previously modified alert definition that could not be applied.
			)

			defUUID := uuid.New()
			defTenantID := "edgenode"

			// This closure sets the table alert_definitions to have three different versions of the same alert definition.
			// It pictures how versions of the same alert definition are stored in a table with their corresponding state, based on their successful or
			// unsuccessful application.
			BeforeEach(func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating an applied alert definition")
				def := models.AlertDefinition{
					ID:   1,
					UUID: defUUID,
					Name: "alert-definition1",
					Template: `alert: HighCPUUsage
expr: cpu_usage > 10
for: 1m
annotations:
  description: CPU usage has exceeded
  summary: High CPU usage detected
labels:
  alert_category: performance
  alert_context: host
  duration: 8s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "10"
`,
					State:    models.DefinitionApplied,
					Category: models.CategoryHealth,
					Severity: "high",
					Enabled:  true,
					Version:  1,
					TenantID: defTenantID,
				}
				Expect(db.DB.WithContext(ctx).Create(&def).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's duration")
				dur := int64(8)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertDuration{
					ID:                10,
					Name:              "duration",
					Duration:          dur,
					DurationMin:       2,
					DurationMax:       20,
					AlertDefinitionID: def.ID,
				}).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's threshold")
				thres := int64(10)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertThreshold{
					ID:                100,
					Name:              "threshold",
					Threshold:         thres,
					ThresholdMin:      10,
					ThresholdMax:      100,
					AlertDefinitionID: def.ID,
				}).Error).ShouldNot(HaveOccurred())

				defInfoInitial = &models.DBAlertDefinition{
					ID:       def.UUID,
					Name:     def.Name,
					State:    def.State,
					Template: def.Template,
					Values: models.DBAlertDefinitionValues{
						Duration:  &dur,
						Threshold: &thres,
						Enabled:   &def.Enabled,
					},
					Version:  def.Version,
					Category: def.Category,
					TenantID: def.TenantID,
				}

				By("creating a newer version of the alert definition which was successfully applied")
				latestDef := def
				latestDef.ID = 2
				latestDef.Version = 2
				latestDef.State = models.DefinitionModified
				Expect(db.DB.WithContext(ctx).Create(&latestDef).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's duration")
				latestDur := int64(10)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertDuration{
					ID:                20,
					Name:              "duration",
					Duration:          latestDur,
					DurationMin:       3,
					DurationMax:       30,
					AlertDefinitionID: latestDef.ID,
				}).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's threshold")
				latestThres := int64(100)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertThreshold{
					ID:                200,
					Name:              "threshold",
					Threshold:         latestThres,
					ThresholdMin:      20,
					ThresholdMax:      200,
					AlertDefinitionID: latestDef.ID,
				}).Error).ShouldNot(HaveOccurred())

				defInfoModified = &models.DBAlertDefinition{
					ID:       latestDef.UUID,
					Name:     latestDef.Name,
					State:    latestDef.State,
					Template: latestDef.Template,
					Values: models.DBAlertDefinitionValues{
						Duration:  &latestDur,
						Threshold: &latestThres,
						Enabled:   &latestDef.Enabled,
					},
					Version:  latestDef.Version,
					Category: latestDef.Category,
					TenantID: latestDef.TenantID,
				}

				By("creating a newer version of the alert definition which was not successfully applied")
				latestDefError := latestDef
				latestDefError.ID = 3
				latestDefError.Version = 3
				latestDefError.State = models.DefinitionError
				Expect(db.DB.WithContext(ctx).Create(&latestDefError).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's duration")
				Expect(db.DB.WithContext(ctx).Create(&models.AlertDuration{
					ID:                21,
					Name:              "duration",
					Duration:          latestDur,
					DurationMin:       3,
					DurationMax:       30,
					AlertDefinitionID: latestDefError.ID,
				}).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's threshold")
				Expect(db.DB.WithContext(ctx).Create(&models.AlertThreshold{
					ID:                201,
					Name:              "threshold",
					Threshold:         latestThres,
					ThresholdMin:      20,
					ThresholdMax:      200,
					AlertDefinitionID: latestDefError.ID,
				}).Error).ShouldNot(HaveOccurred())

				defInfoError = &models.DBAlertDefinition{
					ID:       latestDefError.UUID,
					Name:     latestDefError.Name,
					State:    latestDefError.State,
					Version:  latestDefError.Version,
					TenantID: latestDefError.TenantID,
				}

				By("checking that there are three alert definitions")
				var defs []models.AlertDefinition
				Expect(db.DB.WithContext(ctx).Find(&defs).Error).ShouldNot(HaveOccurred())
				Expect(defs).To(HaveLen(3))
			})

			It("Get the list with the latest versions of successfully applied alert definitions", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				resList, err := db.GetLatestAlertDefinitionList(ctx, defTenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(resList).To(HaveLen(1))
				Expect(resList[0]).To(Equal(defInfoModified))
			})

			It("Get empty list with latest versions of successfully applied alert definitions because there are no alert definitions matching the tenant ID",
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
					defer cancel()

					resList, err := db.GetLatestAlertDefinitionList(ctx, "wrong_tenant")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(resList).To(BeEmpty())
				})

			It("Get the latest version of a successfully applied alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				res, err := db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoModified))
			})

			It("Fail to get the latest version of a successfully applied alert definition because there is no alert definition matching the tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				alertDefinition, err := db.GetLatestAlertDefinition(ctx, "wrong_tenant", defUUID)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(alertDefinition).To(BeNil())
			})

			It("Get a specific version of an alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("getting the first version of the applied alert definition")
				res, err := db.GetAlertDefinition(ctx, defTenantID, defUUID, defInfoInitial.Version)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoInitial))

				By("getting the modified version of the alert definition")
				res, err = db.GetAlertDefinition(ctx, defTenantID, defUUID, defInfoModified.Version)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoModified))
			})

			It("Fail to get a specific version of an alert definition because there is no alert definition with that version", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				res, err := db.GetAlertDefinition(ctx, defTenantID, defUUID, 99)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(res).To(BeNil())
			})

			It("Fail to get a specific version of an alert definition because there is no alert definition matching the tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				res, err := db.GetAlertDefinition(ctx, "wrong_tenant", defUUID, defInfoInitial.Version)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(res).To(BeNil())
			})

			It("Set the duration value of an alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("setting the duration value of the definition")
				newDuration := int64(12)
				Expect(db.SetAlertDefinitionValues(ctx, defTenantID, defUUID, models.DBAlertDefinitionValues{
					Duration: &newDuration,
				})).ShouldNot(HaveOccurred())

				newDefInfo := *defInfoModified
				newDefInfo.Version = defInfoError.Version + 1
				newDefInfo.Values.Duration = &newDuration
				newDefInfo.Template = `alert: HighCPUUsage
expr: cpu_usage > 10
for: 1m
annotations:
  description: CPU usage has exceeded
  summary: High CPU usage detected
labels:
  alert_category: performance
  alert_context: host
  duration: 12s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "10"
`
				By("getting the alert definition")
				res, err := db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(&newDefInfo))

				By("getting alert definition list")
				resList, err := db.GetLatestAlertDefinitionList(ctx, defTenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(resList).To(Equal([]*models.DBAlertDefinition{&newDefInfo}))

				By("getting task for updated alert definition after changing the duration value")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"AlertDefinitionUUID": Equal(&newDefInfo.ID),
					"Version":             Equal(newDefInfo.Version),
					"CreationDate":        BeTemporally("==", clock.FakeClock.Now()),
					"State":               Equal(models.TaskNew),
					"RetryCount":          Equal(int64(0)),
				}))
			})

			It("Set the threshold value of an alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("setting the threshold value of the definition")
				newThreshold := int64(20)
				Expect(db.SetAlertDefinitionValues(ctx, defTenantID, defUUID, models.DBAlertDefinitionValues{
					Threshold: &newThreshold,
				})).ShouldNot(HaveOccurred())

				newDefInfo := *defInfoModified
				newDefInfo.Version = defInfoError.Version + 1
				newDefInfo.Values.Threshold = &newThreshold
				newDefInfo.Template = `alert: HighCPUUsage
expr: cpu_usage > 10
for: 1m
annotations:
  description: CPU usage has exceeded
  summary: High CPU usage detected
labels:
  alert_category: performance
  alert_context: host
  duration: 8s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "20"
`

				By("getting the alert definition")
				res, err := db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(&newDefInfo))

				By("getting the alert definition list")
				resList, err := db.GetLatestAlertDefinitionList(ctx, defTenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(resList).To(Equal([]*models.DBAlertDefinition{&newDefInfo}))

				By("getting task for updated alert definition after changing the threshold value")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"AlertDefinitionUUID": Equal(&newDefInfo.ID),
					"Version":             Equal(newDefInfo.Version),
					"CreationDate":        BeTemporally("==", clock.FakeClock.Now()),
					"State":               Equal(models.TaskNew),
					"RetryCount":          Equal(int64(0)),
				}))
			})

			It("Set the enabled value of an alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("setting the enabled value of the definition")
				newEnabled := false
				Expect(db.SetAlertDefinitionValues(ctx, defTenantID, defUUID, models.DBAlertDefinitionValues{
					Enabled: &newEnabled,
				})).ShouldNot(HaveOccurred())

				newDefInfo := *defInfoModified
				newDefInfo.Version = defInfoError.Version + 1
				newDefInfo.Values.Enabled = &newEnabled

				By("getting the alert definition")
				res, err := db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(&newDefInfo))

				By("getting the alert definition list")
				resList, err := db.GetLatestAlertDefinitionList(ctx, defTenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(resList).To(Equal([]*models.DBAlertDefinition{&newDefInfo}))

				By("getting task for updated alert definition after changing enabled value")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"AlertDefinitionUUID": Equal(&newDefInfo.ID),
					"Version":             Equal(newDefInfo.Version),
					"CreationDate":        BeTemporally("==", clock.FakeClock.Now()),
					"State":               Equal(models.TaskNew),
					"RetryCount":          Equal(int64(0)),
				}))
			})

			It("Fail to set the duration value of an alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("failing to set a duration value greater than the max allowed to the definition")
				newDuration := int64(45)
				err := db.SetAlertDefinitionValues(ctx, defTenantID, defUUID, models.DBAlertDefinitionValues{
					Duration: &newDuration,
				})
				Expect(err).To(MatchError(ContainSubstring("failed to set duration to new alert definition")))
				Expect(err).To(MatchError(ContainSubstring("duration value out of valid range [3, 30]")))
				Expect(err).To(MatchError(database.ErrValueOutOfBounds))

				By("checking that the alert definition was not modified")
				res, err := db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoModified))

				By("failing to set threshold value smaller than the min allowed")
				newDuration = int64(1)
				err = db.SetAlertDefinitionValues(ctx, defTenantID, defUUID, models.DBAlertDefinitionValues{
					Duration: &newDuration,
				})
				Expect(err).To(MatchError(ContainSubstring("failed to set duration to new alert definition")))
				Expect(err).To(MatchError(ContainSubstring("duration value out of valid range [3, 30]")))

				By("checking that the alert definition was not modified")
				res, err = db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoModified))

				By("failing to set a duration value for an alert definition that does not exist for the given tenant ID")
				newDuration = int64(10)
				err = db.SetAlertDefinitionValues(ctx, "wrong_tenant", defUUID, models.DBAlertDefinitionValues{
					Duration: &newDuration,
				})
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve latest version of alert definition for tenant")))

				By("checking that the alert definition was not modified")
				res, err = db.GetLatestAlertDefinition(ctx, "wrong_tenant", defUUID)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(res).To(BeNil())

				By("checking that no new tasks are created when failed to set new duration value")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(BeEmpty())
			})

			It("Fail to set the threshold value of an alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("failing to set a threshold value greater than the max allowed to the definition")
				newThreshold := int64(210)
				err := db.SetAlertDefinitionValues(ctx, defTenantID, defUUID, models.DBAlertDefinitionValues{
					Threshold: &newThreshold,
				})
				Expect(err).To(MatchError(ContainSubstring("failed to set threshold to new alert definition")))
				Expect(err).To(MatchError(ContainSubstring("threshold value out of valid range [20, 200]")))
				Expect(err).To(MatchError(database.ErrValueOutOfBounds))

				By("checking that the alert definition was not modified")
				res, err := db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoModified))

				By("failing to set a duration value smaller than the min allowed to the definition")
				newThreshold = int64(1)
				err = db.SetAlertDefinitionValues(ctx, defTenantID, defUUID, models.DBAlertDefinitionValues{
					Threshold: &newThreshold,
				})
				Expect(err).To(MatchError(ContainSubstring("failed to set threshold to new alert definition")))
				Expect(err).To(MatchError(ContainSubstring("threshold value out of valid range [20, 200]")))

				By("checking that the alert definition was not modified")
				res, err = db.GetLatestAlertDefinition(ctx, defTenantID, defUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoModified))

				By("checking that no new tasks are created when failed to set new threshold value")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tasks).To(BeEmpty())
			})

			DescribeTable("Set the state of the specific version of an alert definition",
				func(newState models.AlertDefinitionState) {
					ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
					defer cancel()

					By("setting the state of the alert definition")
					err := db.SetAlertDefinitionState(ctx, defTenantID, defUUID, defInfoInitial.Version, newState)
					Expect(err).ShouldNot(HaveOccurred())

					By("checking that the alert definition state was set")
					res, err := db.GetAlertDefinition(ctx, defTenantID, defUUID, defInfoInitial.Version)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(*res).To(MatchFields(IgnoreExtras, Fields{
						"ID":       Equal(defInfoInitial.ID),
						"State":    Equal(newState),
						"Version":  Equal(defInfoInitial.Version),
						"TenantID": Equal(defInfoInitial.TenantID),
					}))
				},

				Entry("to 'New'", models.DefinitionNew),
				Entry("to 'Modified'", models.DefinitionModified),
				Entry("to 'Pending'", models.DefinitionPending),
				Entry("to 'Applied'", models.DefinitionApplied),
				Entry("to 'Error'", models.DefinitionError),
			)

			It("Fail to set the state of a specific version of an alert definition because the state is unknown", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("failing to set an unknown definition state")
				err := db.SetAlertDefinitionState(ctx, defTenantID, defUUID, defInfoInitial.Version, models.AlertDefinitionState("invalid state"))
				Expect(err).To(MatchError(ContainSubstring("failed to update alert definition state")))
				Expect(err).To(MatchError(ContainSubstring("unknown alert definition state")))

				By("checking that the definition state was not modified")
				res, err := db.GetAlertDefinition(ctx, defTenantID, defUUID, defInfoInitial.Version)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoInitial))
			})

			It("Fail to set the state of a specific version of an alert definition because there is no alert definition matching the tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("failing to set the alert definition state")
				err := db.SetAlertDefinitionState(ctx, "wrong_tenant", defUUID, defInfoInitial.Version, models.DefinitionNew)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))

				By("checking that the definition state was not modified")
				res, err := db.GetAlertDefinition(ctx, defTenantID, defUUID, defInfoInitial.Version)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfoInitial))
			})
		})

		Context("With different-tenant alert definitions stored", func() {
			var defInfo1 *models.DBAlertDefinition
			var defInfo2 *models.DBAlertDefinition
			BeforeEach(func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating first alert definition")
				def1 := models.AlertDefinition{
					ID:   1,
					UUID: uuid.New(),
					Name: "alert-definition1",
					Template: `alert: HighCPUUsage
expr: cpu_usage > 10
for: 1m
annotations:
  description: CPU usage has exceeded
  summary: High CPU usage detected
labels:
  alert_category: performance
  alert_context: host
  duration: 8s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "10"
`,
					State:    models.DefinitionModified,
					Category: models.CategoryHealth,
					Severity: "high",
					Enabled:  true,
					Version:  1,
					TenantID: "tenant1",
				}
				Expect(db.DB.WithContext(ctx).Create(&def1).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's duration")
				dur1 := int64(8)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertDuration{
					ID:                10,
					Name:              "duration",
					Duration:          dur1,
					DurationMin:       2,
					DurationMax:       20,
					AlertDefinitionID: def1.ID,
				}).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's threshold")
				thres1 := int64(10)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertThreshold{
					ID:                100,
					Name:              "threshold",
					Threshold:         thres1,
					ThresholdMin:      10,
					ThresholdMax:      100,
					AlertDefinitionID: def1.ID,
				}).Error).ShouldNot(HaveOccurred())

				defInfo1 = &models.DBAlertDefinition{
					ID:       def1.UUID,
					Name:     def1.Name,
					State:    def1.State,
					Template: def1.Template,
					Values: models.DBAlertDefinitionValues{
						Duration:  &dur1,
						Threshold: &thres1,
						Enabled:   &def1.Enabled,
					},
					Version:  def1.Version,
					Category: def1.Category,
					TenantID: def1.TenantID,
				}

				By("creating second alert definition")
				def2 := models.AlertDefinition{
					ID:   2,
					UUID: uuid.New(),
					Name: "alert-definition2",
					Template: `alert: HighCPUUsage
expr: cpu_usage > 10
for: 1m
annotations:
  description: CPU usage has exceeded
  summary: High CPU usage detected
labels:
  alert_category: performance
  alert_context: host
  duration: 8s
  host_uuid: '{{$labels.hostGuid}}'
  threshold: "10"
`,
					State:    models.DefinitionModified,
					Category: models.CategoryHealth,
					Severity: "high",
					Enabled:  true,
					Version:  1,
					TenantID: "tenant2",
				}
				Expect(db.DB.WithContext(ctx).Create(&def2).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's duration")
				dur2 := int64(8)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertDuration{
					ID:                20,
					Name:              "duration",
					Duration:          dur2,
					DurationMin:       2,
					DurationMax:       20,
					AlertDefinitionID: def2.ID,
				}).Error).ShouldNot(HaveOccurred())

				By("setting the alert definition's threshold")
				thres2 := int64(10)
				Expect(db.DB.WithContext(ctx).Create(&models.AlertThreshold{
					ID:                200,
					Name:              "threshold",
					Threshold:         thres2,
					ThresholdMin:      10,
					ThresholdMax:      100,
					AlertDefinitionID: def2.ID,
				}).Error).ShouldNot(HaveOccurred())

				defInfo2 = &models.DBAlertDefinition{
					ID:       def2.UUID,
					Name:     def2.Name,
					State:    def2.State,
					Template: def2.Template,
					Values: models.DBAlertDefinitionValues{
						Duration:  &dur2,
						Threshold: &thres2,
						Enabled:   &def2.Enabled,
					},
					Version:  def2.Version,
					Category: def2.Category,
					TenantID: def2.TenantID,
				}
			})

			It("Get latest version of alert definition for first tenant", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				resList, err := db.GetLatestAlertDefinitionList(ctx, defInfo1.TenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(resList).To(HaveLen(1))
				Expect(resList[0]).To(Equal(defInfo1))
			})

			It("Get latest version of alert definition for second tenant", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				resList, err := db.GetLatestAlertDefinitionList(ctx, defInfo2.TenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(resList).To(HaveLen(1))
				Expect(resList[0]).To(Equal(defInfo2))
			})

			It("Get empty alert definition list because of wrong tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				resList, err := db.GetLatestAlertDefinitionList(ctx, "wrong_tenant")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(resList).To(BeEmpty())
			})

			It("Get latest version of first alert definition by UUID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				res, err := db.GetLatestAlertDefinition(ctx, defInfo1.TenantID, defInfo1.ID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfo1))
			})

			It("Get latest version of second alert definition by UUID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				res, err := db.GetLatestAlertDefinition(ctx, defInfo2.TenantID, defInfo2.ID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(defInfo2))
			})

			It("Failed to get alert definition - UUID not from tenant", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				res, err := db.GetLatestAlertDefinition(ctx, defInfo1.TenantID, defInfo2.ID)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})
		})

		Context("Alert definition helpers", func() {
			It("Get alert definition UUIDs", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				def := models.AlertDefinition{
					ID:       1,
					UUID:     uuid.New(),
					Name:     "alert-definition1",
					State:    models.DefinitionApplied,
					Template: "template",
					Category: models.CategoryHealth,
					Severity: "high",
					Enabled:  true,
					Version:  1,
					TenantID: "edgenode",
				}

				By("creating an alert definition")
				Expect(db.DB.WithContext(ctx).Create(&def).Error).ShouldNot(HaveOccurred())

				newDef := def
				newDef.ID = 3
				newDef.Version++

				By("creating a new alert definition with bumped version")
				Expect(db.DB.WithContext(ctx).Create(&newDef).Error).ShouldNot(HaveOccurred())

				By("getting the list of UUIDs")
				uuids, err := database.GetAlertDefinitionUUIDs(db.DB.WithContext(ctx), "edgenode")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(uuids).To(HaveLen(1))

				By("failing getting the list of UUIDs")
				uuids, err = database.GetAlertDefinitionUUIDs(db.DB.WithContext(ctx), "wrong_tenant")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(uuids).To(BeEmpty())
			})
		})
	})

	Describe("Alert receivers", func() {
		BeforeEach(func() {
			Expect(db.DB.AutoMigrate(
				&models.EmailAddress{},
				&models.EmailConfig{},
				&models.Receiver{},
				&models.EmailRecipient{},
				&models.Task{},
			)).ShouldNot(HaveOccurred())
		})

		Context("With no alert receivers", func() {
			It("Get empty list with latest versions of successfully applied receivers with email config because the receivers table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				receivers, err := db.GetLatestReceiverListWithEmailConfig(ctx, "edgenode")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(receivers).To(BeEmpty())
			})

			It("Fail to get latest version of a successfully applied alert receiver because the receivers table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				receivers, err := db.GetLatestReceiverWithEmailConfig(ctx, "edgenode", uuid.New())
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(receivers).To(BeNil())
			})

			It("Fail to get specific version of alert receiver because the receivers table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				receivers, err := db.GetReceiverWithEmailConfig(ctx, "edgenode", uuid.New(), 999)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(receivers).To(BeNil())
			})

			It("Fail to set email recipients of an alert receiver because the receivers table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("failing to set email recipients")
				Expect(db.SetReceiverEmailRecipients(ctx, "edgenode", uuid.New(), []models.EmailAddress{})).To(MatchError(gorm.ErrRecordNotFound))

				By("getting tasks for receiver when failed to set email recipients")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(BeEmpty())
			})

			It("Fail to set the state of a specific version of an alert receiver by UUID because the receivers table is empty", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				Expect(db.SetReceiverState(ctx, "edgenode", uuid.New(), 999, models.ReceiverModified)).Should(
					MatchError(gorm.ErrRecordNotFound),
				)
			})
		})

		Context("With alert receiver stored", func() {
			var (
				recvInfoInitial  *models.DBReceiver // Represents the first applied version of a receiver.
				recvInfoModified *models.DBReceiver // Represents a modified version of the previous receiver that was successfully applied.
				recvInfoError    *models.DBReceiver // Represents a modified version of the previously modified receiver that could not be applied.
			)

			recvUUID := uuid.New()
			recvTenantID := "edgenode"

			// This closure sets the table receivers to have three different versions of the same receiver.
			// It pictures how versions of the same receiver are stored in a table with their corresponding state, based on their successful or
			// unsuccessful application.
			BeforeEach(func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating the email address of the sender.")
				fromEmailID := int64(10)
				sender := &models.EmailAddress{
					ID:        fromEmailID,
					FirstName: "testOrg",
					LastName:  "testSubOrg",
					Email:     "test_org@email.com",
				}
				Expect(db.DB.WithContext(ctx).Create(sender).Error).ShouldNot(HaveOccurred())

				By("creating the email config.")
				emailConfigID := int64(100)
				mailServer := "smtp.server.com"
				Expect(db.DB.WithContext(ctx).Create(&models.EmailConfig{
					ID:         emailConfigID,
					MailServer: mailServer,
					From:       fromEmailID,
				}).Error).ShouldNot(HaveOccurred())

				By("creating a receiver with associated email config")
				recv := models.Receiver{
					ID:            10,
					UUID:          recvUUID,
					Name:          "test-receiver",
					State:         models.ReceiverNew,
					Version:       1,
					EmailConfigID: emailConfigID,
					TenantID:      recvTenantID,
				}
				Expect(db.DB.WithContext(ctx).Create(&recv).Error).ShouldNot(HaveOccurred())

				By("creating a recipient email address.")
				toEmailID1 := int64(100)
				recipient1 := &models.EmailAddress{
					ID:        toEmailID1,
					FirstName: "first",
					LastName:  "user",
					Email:     "first.user@email.com",
				}
				Expect(db.DB.WithContext(ctx).Create(recipient1).Error).ShouldNot(HaveOccurred())

				By("setting email recipient to the receiver.")
				Expect(db.DB.WithContext(ctx).Create(&models.EmailRecipient{
					ReceiverID:     recv.ID,
					EmailAddressID: toEmailID1,
				}).Error).ShouldNot(HaveOccurred())

				recvInfoInitial = &models.DBReceiver{
					UUID:       recv.UUID,
					State:      recv.State,
					Name:       recv.Name,
					Version:    int(recv.Version),
					MailServer: mailServer,
					From:       sender.String(),
					To:         []string{recipient1.String()},
					TenantID:   recv.TenantID,
				}

				By("creating a newer version of the receiver")
				latestRecv := recv
				latestRecv.ID = 20
				latestRecv.Version = 2
				Expect(db.DB.WithContext(ctx).Create(&latestRecv).Error).ShouldNot(HaveOccurred())

				By("creating a recipient email address.")
				toEmailID2 := int64(200)
				recipient2 := &models.EmailAddress{
					ID:        toEmailID2,
					FirstName: "second",
					LastName:  "user",
					Email:     "second.user@email.com",
				}
				Expect(db.DB.WithContext(ctx).Create(recipient2).Error).ShouldNot(HaveOccurred())

				By("setting email recipient to the receiver.")
				Expect(db.DB.WithContext(ctx).Create(&models.EmailRecipient{
					ReceiverID:     latestRecv.ID,
					EmailAddressID: toEmailID2,
				}).Error).ShouldNot(HaveOccurred())

				recvInfoModified = &models.DBReceiver{
					UUID:       latestRecv.UUID,
					State:      latestRecv.State,
					Name:       latestRecv.Name,
					Version:    int(latestRecv.Version),
					MailServer: mailServer,
					From:       sender.String(),
					To:         []string{recipient2.String()},
					TenantID:   latestRecv.TenantID,
				}

				By("creating a newer version of the receiver with 'Error' state")
				latestRecvError := latestRecv
				latestRecvError.ID = 30
				latestRecvError.Version = 3
				latestRecvError.State = models.ReceiverError
				Expect(db.DB.WithContext(ctx).Create(&latestRecvError).Error).ShouldNot(HaveOccurred())

				recvInfoError = &models.DBReceiver{
					UUID:     latestRecvError.UUID,
					Name:     latestRecvError.Name,
					State:    latestRecvError.State,
					Version:  int(latestRecvError.Version),
					TenantID: latestRecvError.TenantID,
				}

				By("checking that there are three receivers")
				var receivers []models.Receiver
				Expect(db.DB.WithContext(ctx).Find(&receivers).Error).ShouldNot(HaveOccurred())
				Expect(receivers).To(HaveLen(3))
			})

			It("Get the list with the latest versions of successfully applied receivers with email config", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recvs, err := db.GetLatestReceiverListWithEmailConfig(ctx, recvTenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recvs).To(HaveLen(1))
				Expect(recvs).To(Equal([]*models.DBReceiver{recvInfoModified}))
			})

			It("Get empty list with latest versions of successfully applied receivers with email config because there are no receivers matching the tenant ID",
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
					defer cancel()

					recvs, err := db.GetLatestReceiverListWithEmailConfig(ctx, "wrong_tenant")
					Expect(err).ShouldNot(HaveOccurred())
					Expect(recvs).To(BeEmpty())
				})

			It("Get the latest version of a successfully applied alert receiver with email config", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recv, err := db.GetLatestReceiverWithEmailConfig(ctx, recvTenantID, recvUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recv).To(Equal(recvInfoModified))
			})

			It("Fail to get latest version of a successfully applied alert receiver because there is no alert receiver matching the tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				receivers, err := db.GetLatestReceiverWithEmailConfig(ctx, "wrong_tenant", uuid.New())
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(receivers).To(BeNil())
			})

			It("Get a specific version of an alert receiver with email config", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recv, err := db.GetReceiverWithEmailConfig(ctx, recvTenantID, recvUUID, int64(recvInfoInitial.Version))
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recv).To(Equal(recvInfoInitial))

				recv, err = db.GetReceiverWithEmailConfig(ctx, recvTenantID, recvUUID, int64(recvInfoModified.Version))
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recv).To(Equal(recvInfoModified))
			})

			It("Fail to get a specific version of an alert receiver because there is no alert definition with that version", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				res, err := db.GetReceiverWithEmailConfig(ctx, recvTenantID, recvUUID, 99)
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(res).To(BeNil())
			})

			It("Fail to get specific version of an alert receiver because there is no alert definition matching the tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				receivers, err := db.GetReceiverWithEmailConfig(ctx, "wrong_tenant", recvUUID, int64(recvInfoInitial.Version))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))
				Expect(receivers).To(BeNil())
			})

			It("Add the email recipients to an alert receiver", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("updating the email recipient list of alert receiver.")
				newRecipients := []models.EmailAddress{
					// First email address already exists in database.
					{
						FirstName: "first",
						LastName:  "user",
						Email:     "first.user@email.com",
					},
					{
						FirstName: "second",
						LastName:  "user",
						Email:     "second.user@email.com",
					},
					{
						FirstName: "third",
						LastName:  "user",
						Email:     "third.user@email.com",
					},
				}
				Expect(db.SetReceiverEmailRecipients(ctx, recvTenantID, recvUUID, newRecipients)).ShouldNot(HaveOccurred())

				newRecvInfo := *recvInfoModified
				newRecvInfo.Version = recvInfoError.Version + 1
				newRecvInfo.State = models.ReceiverModified
				recipients := make([]string, len(newRecipients))
				for i, r := range newRecipients {
					recipients[i] = r.String()
				}
				newRecvInfo.To = recipients

				By("getting updated alert receiver with email recipient list")
				recv, err := db.GetLatestReceiverWithEmailConfig(ctx, recvTenantID, recvUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(*recv).To(Equal(newRecvInfo))

				By("getting the tasks related to new receiver")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"ReceiverUUID": Equal(&recv.UUID),
					"Version":      Equal(int64(recv.Version)),
					"CreationDate": BeTemporally("==", clock.FakeClock.Now()),
					"State":        Equal(models.TaskNew),
					"RetryCount":   Equal(int64(0)),
				}))
			})

			It("Set empty recipient list to alert receiver", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("setting empty recipient list")
				Expect(db.SetReceiverEmailRecipients(ctx, recvTenantID, recvUUID, []models.EmailAddress{})).ShouldNot(HaveOccurred())

				newRecvInfo := *recvInfoModified
				newRecvInfo.Version = recvInfoError.Version + 1
				newRecvInfo.State = models.ReceiverModified
				newRecvInfo.To = []string{}

				By("getting updated alert receiver with empty recipient list")
				recv, err := db.GetLatestReceiverWithEmailConfig(ctx, recvTenantID, recvUUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(*recv).To(Equal(newRecvInfo))

				By("getting the tasks related to new receiver with empty recipient list")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"ReceiverUUID": Equal(&recv.UUID),
					"Version":      Equal(int64(recv.Version)),
					"CreationDate": BeTemporally("==", clock.FakeClock.Now()),
					"State":        Equal(models.TaskNew),
					"RetryCount":   Equal(int64(0)),
				}))
			})

			It("Fail to set email recipients by UUID because non existing tenantID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("failing to set email recipients")
				Expect(db.SetReceiverEmailRecipients(ctx, "wrong_tenant", recvUUID, []models.EmailAddress{})).To(MatchError(gorm.ErrRecordNotFound))

				By("getting tasks for receiver when failed to set email recipients")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(BeEmpty())
			})

			DescribeTable("Set the state of the specific version of an alert receiver",
				func(newState models.ReceiverState) {
					ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
					defer cancel()

					By("setting the state of the receiver")
					err := db.SetReceiverState(ctx, recvTenantID, recvUUID, int64(recvInfoInitial.Version), newState)
					Expect(err).ShouldNot(HaveOccurred())

					By("checking that the receiver state was set")
					res, err := db.GetReceiverWithEmailConfig(ctx, recvTenantID, recvUUID, int64(recvInfoInitial.Version))
					Expect(err).ShouldNot(HaveOccurred())
					Expect(*res).To(MatchFields(IgnoreExtras, Fields{
						"UUID":     Equal(recvInfoInitial.UUID),
						"State":    Equal(newState),
						"Version":  Equal(recvInfoInitial.Version),
						"TenantID": Equal(recvTenantID),
					}))
				},

				Entry("to 'New'", models.ReceiverNew),
				Entry("to 'Modified'", models.ReceiverModified),
				Entry("to 'Pending'", models.ReceiverPending),
				Entry("to 'Applied'", models.ReceiverApplied),
				Entry("to 'Error'", models.ReceiverError),
			)

			It("Fail to set the state of a specific version of an alert receiver because the state is unknown", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("failing to set an unknown receiver state")
				invalidState := models.ReceiverState("In Progress")
				err := db.SetReceiverState(ctx, recvTenantID, recvUUID, int64(recvInfoInitial.Version), invalidState)
				Expect(err).To(MatchError(ContainSubstring("failed to update receiver state")))
				Expect(err).To(MatchError(ContainSubstring("unknown receiver state")))

				By("checking that the receiver state was not modified")
				res, err := db.GetReceiverWithEmailConfig(ctx, recvTenantID, recvUUID, int64(recvInfoInitial.Version))
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(Equal(recvInfoInitial))
			})

			It("Fail to set a state of a specific version of an alert receiver because there is no alert receiver matching the tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("setting the state of the alert receiver")
				modified := models.ReceiverModified
				Expect(db.SetReceiverState(ctx, "wrong_tenant", recvUUID, int64(recvInfoInitial.Version), modified)).To(
					MatchError(gorm.ErrRecordNotFound),
				)
			})
		})

		Context("With different-tenant alert reveivers stored", func() {
			var recvInfo1 *models.DBReceiver
			var recvInfo2 *models.DBReceiver
			BeforeEach(func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating first alert receiver")
				recvInfo1 = &models.DBReceiver{}

				By("creating the email address of the sender.")
				fromEmailID1 := int64(10)
				sender1 := &models.EmailAddress{
					ID:        fromEmailID1,
					FirstName: "testOrg",
					LastName:  "testSubOrg",
					Email:     "test_org@email.com",
				}
				Expect(db.DB.WithContext(ctx).Create(sender1).Error).ShouldNot(HaveOccurred())
				recvInfo1.From = sender1.String()

				By("creating the email config.")
				emailConfigID1 := int64(100)
				mailServer1 := "smtp.server.com"
				Expect(db.DB.WithContext(ctx).Create(&models.EmailConfig{
					ID:         emailConfigID1,
					MailServer: mailServer1,
					From:       fromEmailID1,
				}).Error).ShouldNot(HaveOccurred())
				recvInfo1.MailServer = mailServer1

				By("creating a new receiver with associated email config")
				recvInfo1.UUID = uuid.New()
				recvInfo1.State = models.ReceiverNew
				recvInfo1.Name = "test-receiver"
				recvInfo1.Version = 1
				receiverID := int64(10)
				recvInfo1.TenantID = "tenant1"
				Expect(db.DB.WithContext(ctx).Create(&models.Receiver{
					ID:            receiverID,
					UUID:          recvInfo1.UUID,
					Name:          recvInfo1.Name,
					State:         recvInfo1.State,
					Version:       int64(recvInfo1.Version),
					EmailConfigID: emailConfigID1,
					TenantID:      recvInfo1.TenantID,
				}).Error).ShouldNot(HaveOccurred())

				By("creating a recipient email address.")
				toEmailID1 := int64(100)
				recipient1 := &models.EmailAddress{
					ID:        toEmailID1,
					FirstName: "first",
					LastName:  "user",
					Email:     "first.user@email.com",
				}
				Expect(db.DB.WithContext(ctx).Create(recipient1).Error).ShouldNot(HaveOccurred())
				recvInfo1.To = append(recvInfo1.To, recipient1.String())

				By("setting email recipient to the receiver.")
				Expect(db.DB.WithContext(ctx).Create(&models.EmailRecipient{
					ReceiverID:     receiverID,
					EmailAddressID: toEmailID1,
				}).Error).ShouldNot(HaveOccurred())

				By("creating second alert reveiver")
				recvInfo2 = &models.DBReceiver{}

				By("creating the email address of the sender.")
				fromEmailID2 := int64(20)
				sender2 := &models.EmailAddress{
					ID:        fromEmailID2,
					FirstName: "testOrg",
					LastName:  "testSubOrg",
					Email:     "test_org_2@email.com",
				}
				Expect(db.DB.WithContext(ctx).Create(sender2).Error).ShouldNot(HaveOccurred())
				recvInfo2.From = sender2.String()

				By("creating the email config.")
				emailConfigID2 := int64(200)
				mailServer2 := "smtp.server.com"
				Expect(db.DB.WithContext(ctx).Create(&models.EmailConfig{
					ID:         emailConfigID2,
					MailServer: mailServer2,
					From:       fromEmailID2,
				}).Error).ShouldNot(HaveOccurred())
				recvInfo2.MailServer = mailServer2

				By("creating a new receiver with associated email config")
				recvInfo2.UUID = uuid.New()
				recvInfo2.State = models.ReceiverNew
				recvInfo2.Name = "test-receiver"
				recvInfo2.Version = 1
				receiverID2 := int64(20)
				recvInfo2.TenantID = "tenant2"
				Expect(db.DB.WithContext(ctx).Create(&models.Receiver{
					ID:            receiverID2,
					UUID:          recvInfo2.UUID,
					Name:          recvInfo2.Name,
					State:         recvInfo2.State,
					Version:       int64(recvInfo2.Version),
					EmailConfigID: emailConfigID2,
					TenantID:      recvInfo2.TenantID,
				}).Error).ShouldNot(HaveOccurred())

				By("creating a recipient email address.")
				toEmailID2 := int64(200)
				recipient2 := &models.EmailAddress{
					ID:        toEmailID2,
					FirstName: "first",
					LastName:  "user",
					Email:     "first.user_2@email.com",
				}
				Expect(db.DB.WithContext(ctx).Create(recipient2).Error).ShouldNot(HaveOccurred())
				recvInfo2.To = append(recvInfo2.To, recipient2.String())

				By("setting email recipient to the receiver.")
				Expect(db.DB.WithContext(ctx).Create(&models.EmailRecipient{
					ReceiverID:     receiverID2,
					EmailAddressID: toEmailID2,
				}).Error).ShouldNot(HaveOccurred())
			})

			It("Get latest version of first alert receiver by UUID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recv, err := db.GetLatestReceiverWithEmailConfig(ctx, recvInfo1.TenantID, recvInfo1.UUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recv).To(Equal(recvInfo1))
			})

			It("Get latest version of second alert receiver by UUID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recv, err := db.GetLatestReceiverWithEmailConfig(ctx, recvInfo2.TenantID, recvInfo2.UUID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recv).To(Equal(recvInfo2))
			})

			It("Failed to get receiver - UUID not from tenant", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recv, err := db.GetLatestReceiverWithEmailConfig(ctx, recvInfo2.TenantID, recvInfo1.UUID)
				Expect(err).To(HaveOccurred())
				Expect(recv).To(BeNil())
			})

			It("Get latest version of receiver for first tenant", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recvs, err := db.GetLatestReceiverListWithEmailConfig(ctx, recvInfo1.TenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recvs).To(HaveLen(1))
				Expect(recvs).To(Equal([]*models.DBReceiver{recvInfo1}))
			})

			It("Get latest version of receiver for second tenant", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recvs, err := db.GetLatestReceiverListWithEmailConfig(ctx, recvInfo2.TenantID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recvs).To(HaveLen(1))
				Expect(recvs).To(Equal([]*models.DBReceiver{recvInfo2}))
			})

			It("Get empty receiver list with email config because of wrong tenant ID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				recvs, err := db.GetLatestReceiverListWithEmailConfig(ctx, "wrong_tenant")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(recvs).To(BeEmpty())
			})
		})
	})

	Describe("Tasks", func() {
		BeforeEach(func() {
			Expect(db.DB.AutoMigrate(
				&models.AlertDefinition{},
				&models.Receiver{},
				&models.Task{},
			)).ShouldNot(HaveOccurred())

			clock.SetFakeClock()
			clock.FakeClock.Set(time.Now())
		})

		When("Deleting processed tasks which exceed a specific duration", func() {
			It("There are no tasks with Applied or Invalid state to delete", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating pending tasks into database")
				newTask := models.Task{
					ID:           1,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					State:        models.TaskNew,
				}
				Expect(db.DB.WithContext(ctx).Create(&newTask).Error).ShouldNot(HaveOccurred())

				takenTask := models.Task{
					ID:                  2,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskTaken,
				}
				Expect(db.DB.WithContext(ctx).Create(&takenTask).Error).ShouldNot(HaveOccurred())

				errorTask := models.Task{
					ID:                  3,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskError,
				}
				Expect(db.DB.WithContext(ctx).Create(&errorTask).Error).ShouldNot(HaveOccurred())

				By("setting time to exceed the duration")
				clock.FakeClock.Set(clock.FakeClock.Now().Add(11 * time.Second))

				By("deleting not pending tasks exceeding the duration")
				Expect(db.DeleteNotPendingTasksExceedingDuration(ctx, 10*time.Second)).ShouldNot(HaveOccurred())

				By("getting pending tasks from database")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(Equal([]models.Task{
					newTask,
					takenTask,
					errorTask,
				}))
			})

			It("Tasks with Applied and Invalid state do not exceed retention period", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating Applied and Invalid tasks into database")
				invalidTask := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskInvalid,
					CreationDate:        clock.FakeClock.Now(),
					CompletionDate:      clock.FakeClock.Now().Add(5 * time.Second),
				}
				Expect(db.DB.WithContext(ctx).Create(&invalidTask).Error).ShouldNot(HaveOccurred())

				appliedTask := models.Task{
					ID:             2,
					ReceiverUUID:   uuidPtr(uuid.New()),
					TenantID:       "edgenode",
					State:          models.TaskApplied,
					CreationDate:   clock.FakeClock.Now(),
					CompletionDate: clock.FakeClock.Now().Add(5 * time.Second),
				}
				Expect(db.DB.WithContext(ctx).Create(&appliedTask).Error).ShouldNot(HaveOccurred())

				By("setting time which does not make completion date of tasks to exceed duration")
				clock.FakeClock.Set(clock.FakeClock.Now().Add(10 * time.Second))

				By("deleting not pending tasks which exceed duration")
				Expect(db.DeleteNotPendingTasksExceedingDuration(ctx, 10*time.Second)).ShouldNot(HaveOccurred())

				By("getting not pending tasks from database")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(2))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"AlertDefinitionUUID": Equal(invalidTask.AlertDefinitionUUID),
					"Version":             Equal(invalidTask.Version),
					"CreationDate":        BeTemporally("==", invalidTask.CreationDate),
					"CompletionDate":      BeTemporally("==", invalidTask.CompletionDate),
					"State":               Equal(invalidTask.State),
				}))
				Expect(tasks[1]).To(MatchFields(IgnoreExtras, Fields{
					"ReceiverUUID":   Equal(appliedTask.ReceiverUUID),
					"Version":        Equal(appliedTask.Version),
					"CreationDate":   BeTemporally("==", appliedTask.CreationDate),
					"CompletionDate": BeTemporally("==", appliedTask.CompletionDate),
					"State":          Equal(appliedTask.State),
				}))
			})

			It("Delete tasks with Applied and Invalid state that exceed duration", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				timeNow := clock.TimeNowFn()

				By("creating Applied and Invalid tasks into database")
				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskApplied,
					StartDate:           timeNow,
					CompletionDate:      timeNow.Add(5 * time.Second),
				}).Error).ShouldNot(HaveOccurred())
				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  6,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskInvalid,
					StartDate:           timeNow,
					CompletionDate:      timeNow.Add(10 * time.Second),
				}).Error).ShouldNot(HaveOccurred())
				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  7,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskApplied,
					StartDate:           timeNow,
					CompletionDate:      timeNow.Add(15 * time.Second),
				}).Error).ShouldNot(HaveOccurred())

				By("setting time which makes completion date of tasks to exceed duration")
				clock.FakeClock.Set(timeNow.Add(30 * time.Second))

				By("deleting not pending tasks which exceed duration")
				Expect(db.DeleteNotPendingTasksExceedingDuration(ctx, 10*time.Second)).ShouldNot(HaveOccurred())

				By("getting empty slice of not pending tasks from database")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(BeEmpty())
			})
		})

		When("Failing tasks which are taken and exceeded timeout duration", func() {
			It("There are no tasks in Taken state", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a task which is not in Taken state")
				newTask := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskNew,
					StartDate:           clock.FakeClock.Now(),
					RetryCount:          0,
				}
				Expect(db.DB.WithContext(ctx).Create(&newTask).Error).ShouldNot(HaveOccurred())

				By("setting time to exceed the duration")
				clock.FakeClock.Set(clock.FakeClock.Now().Add(20 * time.Second))
				timeout := 5 * time.Second

				By("setting taken tasks which exceed duration as failed")
				Expect(db.SetTakenTasksExceedingDurationAsFailed(ctx, timeout, 5)).ShouldNot(HaveOccurred())

				By("checking that the task was not set to failed")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"AlertDefinitionUUID": Equal(newTask.AlertDefinitionUUID),
					"State":               Equal(newTask.State),
					"StartDate":           BeTemporally("==", newTask.StartDate),
					"RetryCount":          Equal(newTask.RetryCount),
				}))
			})

			It("Task with Taken state does not exceed timeout duration", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a task which with Taken state and not exceeding timeout duration")
				takenTask := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskTaken,
					StartDate:           clock.FakeClock.Now(),
					RetryCount:          0,
				}
				Expect(db.DB.WithContext(ctx).Create(&takenTask).Error).ShouldNot(HaveOccurred())

				By("setting time that does not exceed the duration")
				clock.FakeClock.Set(clock.FakeClock.Now().Add(5 * time.Second))
				timeout := 10 * time.Second

				By("setting taken tasks which exceed duration as failed")
				Expect(db.SetTakenTasksExceedingDurationAsFailed(ctx, timeout, 5)).ShouldNot(HaveOccurred())

				By("checking that the task was not set to failed")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"AlertDefinitionUUID": Equal(takenTask.AlertDefinitionUUID),
					"State":               Equal(takenTask.State),
					"StartDate":           BeTemporally("==", takenTask.StartDate),
					"RetryCount":          Equal(takenTask.RetryCount),
				}))
			})

			It("Task with Taken state exceeds timeout duration", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating receiver")
				recv := &models.Receiver{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.ReceiverModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(recv).Error).ShouldNot(HaveOccurred())

				By("creating a task which with Taken state which exceeds timeout duration")
				takenTask := models.Task{
					ID:           1,
					ReceiverUUID: &recv.UUID,
					TenantID:     recv.TenantID,
					Version:      recv.Version,
					State:        models.TaskTaken,
					StartDate:    clock.FakeClock.Now(),
					RetryCount:   0,
				}
				Expect(db.DB.WithContext(ctx).Create(&takenTask).Error).ShouldNot(HaveOccurred())

				By("setting time to exceed the duration")
				clock.FakeClock.Set(clock.FakeClock.Now().Add(20 * time.Second))
				timeout := 5 * time.Second

				By("setting taken tasks which exceed duration as failed")
				Expect(db.SetTakenTasksExceedingDurationAsFailed(ctx, timeout, 5)).ShouldNot(HaveOccurred())

				By("checking that the task was set to Error state")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"ReceiverUUID": Equal(takenTask.ReceiverUUID),
					"Version":      Equal(takenTask.Version),
					"State":        Equal(models.TaskError),
					"RetryCount":   Equal(takenTask.RetryCount + 1),
				}))

				By("checking that the receiver state is Error")
				var recvOut models.Receiver
				Expect(db.DB.WithContext(ctx).First(&recvOut, recv.ID).Error).ShouldNot(HaveOccurred())
				Expect(recvOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(recv.ID),
					"UUID":    Equal(recv.UUID),
					"State":   Equal(models.ReceiverError),
					"Version": Equal(recv.Version),
				}))
			})

			It("Task with Taken state exceeds timeout duration and retry limit", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				retryLimit := 5

				By("creating receiver")
				recv := &models.Receiver{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.ReceiverModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(recv).Error).ShouldNot(HaveOccurred())

				By("creating a task which with Taken state which exceeds timeout duration")
				takenTask := models.Task{
					ID:           1,
					ReceiverUUID: &recv.UUID,
					TenantID:     recv.TenantID,
					Version:      recv.Version,
					State:        models.TaskTaken,
					StartDate:    clock.FakeClock.Now(),
					RetryCount:   int64(retryLimit),
				}
				Expect(db.DB.WithContext(ctx).Create(&takenTask).Error).ShouldNot(HaveOccurred())

				By("setting time to exceed the duration")
				clock.FakeClock.Set(clock.FakeClock.Now().Add(20 * time.Second))
				timeout := 5 * time.Second

				By("setting taken tasks which exceed duration as failed")
				Expect(db.SetTakenTasksExceedingDurationAsFailed(ctx, timeout, retryLimit)).ShouldNot(HaveOccurred())

				By("checking that the task was set to Invalid state")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"ReceiverUUID":   Equal(takenTask.ReceiverUUID),
					"Version":        Equal(takenTask.Version),
					"State":          Equal(models.TaskInvalid),
					"CompletionDate": BeTemporally("~", clock.FakeClock.Now()),
					"RetryCount":     Equal(takenTask.RetryCount),
				}))

				By("checking that the receiver state is Error")
				var recvOut models.Receiver
				Expect(db.DB.WithContext(ctx).First(&recvOut, recv.ID).Error).ShouldNot(HaveOccurred())
				Expect(recvOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(recv.ID),
					"UUID":    Equal(recv.UUID),
					"State":   Equal(models.ReceiverError),
					"Version": Equal(recv.Version),
				}))
			})
		})

		When("Getting pending tasks", func() {
			It("There are no tasks with New or Error state", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating tasks which are not in a pending state")
				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskApplied,
				}).Error).ShouldNot(HaveOccurred())

				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  2,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskInvalid,
				}).Error).ShouldNot(HaveOccurred())

				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  3,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskTaken,
				}).Error).ShouldNot(HaveOccurred())

				By("getting empty slice of pending tasks")
				tasks, err := db.GetPendingTasks(ctx, uuid.New(), 100)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tasks).To(BeEmpty())
			})

			It("Number of tasks with New or Error state exceeds the count limit", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating tasks which are in a pending state exceeding the count limit")
				taskError := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskError,
					Version:             20,
					CreationDate:        clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&taskError).Error).ShouldNot(HaveOccurred())

				newTask := models.Task{
					ID:           2,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					State:        models.TaskNew,
					Version:      1,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&newTask).Error).ShouldNot(HaveOccurred())

				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  3,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					State:               models.TaskNew,
					Version:             1,
					CreationDate:        clock.FakeClock.Now(),
				}).Error).ShouldNot(HaveOccurred())

				By("getting only pending tasks according to count limit")
				ownerUUID := uuid.New()
				res, err := db.GetPendingTasks(ctx, ownerUUID, 2)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(HaveLen(2))
				Expect(res[0]).To(MatchFields(IgnoreExtras, Fields{
					"OwnerUUID":           Equal(ownerUUID),
					"AlertDefinitionUUID": Equal(taskError.AlertDefinitionUUID),
					"Version":             Equal(taskError.Version),
					"CreationDate":        BeTemporally("==", taskError.CreationDate),
					"StartDate":           BeTemporally("==", clock.FakeClock.Now()),
					"State":               Equal(models.TaskTaken),
				}))
				Expect(res[1]).To(MatchFields(IgnoreExtras, Fields{
					"OwnerUUID":    Equal(ownerUUID),
					"ReceiverUUID": Equal(newTask.ReceiverUUID),
					"Version":      Equal(newTask.Version),
					"CreationDate": BeTemporally("==", newTask.CreationDate),
					"StartDate":    BeTemporally("==", clock.FakeClock.Now()),
					"State":        Equal(models.TaskTaken),
				}))
			})

			It("Take only the latest version of a task", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating several versions of a task UUID with pending state")
				task := models.Task{
					ID:           10,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					State:        models.TaskNew,
					Version:      20,
				}

				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:           1,
					ReceiverUUID: task.ReceiverUUID,
					TenantID:     task.TenantID,
					State:        models.TaskError,
					Version:      1,
				}).Error).ShouldNot(HaveOccurred())

				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:           2,
					ReceiverUUID: task.ReceiverUUID,
					TenantID:     task.TenantID,
					State:        models.TaskError,
					Version:      5,
				}).Error).ShouldNot(HaveOccurred())

				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				By("getting only the latest version of the task")
				ownerUUID := uuid.New()
				res, err := db.GetPendingTasks(ctx, ownerUUID, 100)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res).To(HaveLen(1))
				Expect(res[0]).To(MatchFields(IgnoreExtras, Fields{
					"OwnerUUID":    Equal(ownerUUID),
					"ReceiverUUID": Equal(task.ReceiverUUID),
					"Version":      Equal(task.Version),
					"StartDate":    BeTemporally("==", clock.FakeClock.Now()),
					"State":        Equal(models.TaskTaken),
				}))
			})

			It("Take only tasks whose UUID does not belong to a task in Taken state", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating two tasks with same UUID where one of them has Taken state")
				taskUUID := uuid.New()
				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  1,
					AlertDefinitionUUID: &taskUUID,
					TenantID:            "edgenode",
					State:               models.TaskError,
					Version:             1,
				}).Error).ShouldNot(HaveOccurred())

				Expect(db.DB.WithContext(ctx).Create(&models.Task{
					ID:                  2,
					AlertDefinitionUUID: &taskUUID,
					TenantID:            "edgenode",
					State:               models.TaskTaken,
					Version:             2,
				}).Error).ShouldNot(HaveOccurred())

				By("getting no pending tasks")
				tasks, err := db.GetPendingTasks(ctx, uuid.New(), 100)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tasks).To(BeEmpty())
			})
		})

		When("Setting tasks with same UUID and older version to invalid", func() {
			It("There are no tasks with same UUID", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a task with New state")
				newTask := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					Version:             1,
					State:               models.TaskNew,
					CreationDate:        clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&newTask).Error).ShouldNot(HaveOccurred())

				By("setting no task to invalid since the task in the argument has different UUID")
				Expect(db.SetOlderVersionsToInvalidState(ctx, []models.Task{
					{
						ID:                  2,
						AlertDefinitionUUID: uuidPtr(uuid.New()),
						TenantID:            "edgenode",
						Version:             10,
					},
				})).ShouldNot(HaveOccurred())

				By("checking that the task was not modified")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(Equal(newTask))
			})

			It("An older tasks with Taken state is not set to Invalid", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a task with Taken state")
				takenTask := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					Version:             1,
					State:               models.TaskTaken,
					CreationDate:        clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&takenTask).Error).ShouldNot(HaveOccurred())

				By("setting no task to invalid since its state is Taken")
				Expect(db.SetOlderVersionsToInvalidState(ctx, []models.Task{
					{
						ID:                  2,
						AlertDefinitionUUID: takenTask.AlertDefinitionUUID,
						TenantID:            "edgenode",
						Version:             10,
					},
				})).ShouldNot(HaveOccurred())

				By("checking that the task was not modified")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(Equal(takenTask))
			})

			It("An older task with Applied state is not set to Invalid", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a task with Applied state")
				appliedTask := models.Task{
					ID:           1,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					Version:      1,
					State:        models.TaskApplied,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&appliedTask).Error).ShouldNot(HaveOccurred())

				By("setting no task to invalid since its state is Applied")
				Expect(db.SetOlderVersionsToInvalidState(ctx, []models.Task{
					{
						ID:           2,
						ReceiverUUID: appliedTask.ReceiverUUID,
						TenantID:     "edgenode",
						Version:      100,
					},
				})).ShouldNot(HaveOccurred())

				By("checking that the task was not modified")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(1))
				Expect(tasks[0]).To(Equal(appliedTask))
			})

			It("The tasks with same UUID and newer versions are not set to Invalid", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating tasks with same UUID")
				recvUUID := uuid.New()
				task1 := models.Task{
					ID:           1,
					ReceiverUUID: &recvUUID,
					TenantID:     "edgenode",
					Version:      5,
					State:        models.TaskNew,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task1).Error).ShouldNot(HaveOccurred())

				task2 := models.Task{
					ID:           2,
					ReceiverUUID: &recvUUID,
					TenantID:     "edgenode",
					Version:      10,
					State:        models.TaskError,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task2).Error).ShouldNot(HaveOccurred())

				By("setting no task to invalid since their version is not older")
				Expect(db.SetOlderVersionsToInvalidState(ctx, []models.Task{
					{
						ID:           3,
						ReceiverUUID: &recvUUID,
						TenantID:     "edgenode",
						Version:      1,
					},
				})).ShouldNot(HaveOccurred())

				By("checking that the tasks were not modified")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				id := func(element interface{}) string {
					task := element.(models.Task)
					return strconv.FormatInt(task.ID, 10)
				}
				Expect(tasks).To(MatchAllElements(id, Elements{
					"1": Equal(task1),
					"2": Equal(task2),
				}))
			})

			It("Older version of tasks with New and Error state are set to invalid", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating definition and receiver tasks with different versions")
				defUUID := uuid.New()
				task1 := models.Task{
					ID:                  1,
					AlertDefinitionUUID: &defUUID,
					TenantID:            "edgenode",
					Version:             5,
					State:               models.TaskNew,
					CreationDate:        clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task1).Error).ShouldNot(HaveOccurred())

				task2 := models.Task{
					ID:                  2,
					AlertDefinitionUUID: &defUUID,
					TenantID:            "edgenode",
					Version:             10,
					State:               models.TaskError,
					CreationDate:        clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task2).Error).ShouldNot(HaveOccurred())

				recvUUID := uuid.New()
				task3 := models.Task{
					ID:           3,
					ReceiverUUID: &recvUUID,
					TenantID:     "edgenode",
					Version:      2,
					State:        models.TaskNew,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task3).Error).ShouldNot(HaveOccurred())

				task4 := models.Task{
					ID:           4,
					ReceiverUUID: &recvUUID,
					TenantID:     "edgenode",
					Version:      9,
					State:        models.TaskError,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task4).Error).ShouldNot(HaveOccurred())

				// Set time to later be compared with CompletionDate field of tasks set to invalid.
				completionDate := clock.FakeClock.Now().Add(10 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting tasks with older version to invalid")
				Expect(db.SetOlderVersionsToInvalidState(ctx, []models.Task{
					{
						ID:                  5,
						AlertDefinitionUUID: &defUUID,
						Version:             100,
						TenantID:            "edgenode",
					},
					{
						ID:           6,
						ReceiverUUID: &recvUUID,
						Version:      20,
						TenantID:     "edgenode",
					},
				})).ShouldNot(HaveOccurred())

				By("checking that tasks with same UUID and older version were set to invalid")
				var tasks []models.Task
				Expect(db.DB.WithContext(ctx).Find(&tasks).Error).ShouldNot(HaveOccurred())
				Expect(tasks).To(HaveLen(4))
				Expect(tasks[0]).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task1.ID),
					"AlertDefinitionUUID": Equal(task1.AlertDefinitionUUID),
					"Version":             Equal(task1.Version),
					"State":               Equal(models.TaskInvalid),
					"CreationDate":        BeTemporally("==", task1.CreationDate),
					"CompletionDate":      BeTemporally("~", completionDate),
				}))
				Expect(tasks[1]).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task2.ID),
					"AlertDefinitionUUID": Equal(task2.AlertDefinitionUUID),
					"Version":             Equal(task2.Version),
					"CreationDate":        BeTemporally("==", task2.CreationDate),
					"CompletionDate":      BeTemporally("==", completionDate),
				}))
				Expect(tasks[2]).To(MatchFields(IgnoreExtras, Fields{
					"ID":             Equal(task3.ID),
					"ReceiverUUID":   Equal(task3.ReceiverUUID),
					"Version":        Equal(task3.Version),
					"CreationDate":   BeTemporally("==", task3.CreationDate),
					"CompletionDate": BeTemporally("==", completionDate),
				}))
				Expect(tasks[3]).To(MatchFields(IgnoreExtras, Fields{
					"ID":             Equal(task4.ID),
					"ReceiverUUID":   Equal(task4.ReceiverUUID),
					"Version":        Equal(task4.Version),
					"CreationDate":   BeTemporally("==", task4.CreationDate),
					"CompletionDate": BeTemporally("==", completionDate),
				}))
			})
		})

		When("Setting a task as applied", func() {
			It("Set receiver task Applied state and completion date", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver")
				recv := &models.Receiver{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.ReceiverModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(recv).Error).ShouldNot(HaveOccurred())

				By("creating associated receiver task with New state")
				task := models.Task{
					ID:           1,
					ReceiverUUID: &recv.UUID,
					TenantID:     recv.TenantID,
					Version:      recv.Version,
					State:        models.TaskNew,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as applied")
				Expect(db.SetTaskAsApplied(ctx, task)).ShouldNot(HaveOccurred())

				By("checking that the task state is Applied and completion time is set")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":             Equal(task.ID),
					"ReceiverUUID":   Equal(task.ReceiverUUID),
					"State":          Equal(models.TaskApplied),
					"Version":        Equal(task.Version),
					"CreationDate":   BeTemporally("==", task.CreationDate),
					"CompletionDate": BeTemporally("==", completionDate),
				}))

				By("checking that the receiver state is Applied")
				var recvOut models.Receiver
				Expect(db.DB.WithContext(ctx).First(&recvOut, recv.ID).Error).ShouldNot(HaveOccurred())
				Expect(recvOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(recv.ID),
					"UUID":    Equal(recv.UUID),
					"State":   Equal(models.ReceiverApplied),
					"Version": Equal(recv.Version),
				}))
			})

			It("Fail to set receiver task Applied state and completion date because there is no associated receiver record", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver task with New state")
				task := models.Task{
					ID:           1,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					Version:      1,
					State:        models.TaskNew,
					CreationDate: clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				By("failing to set the task as applied")
				err := db.SetTaskAsApplied(ctx, task)
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve receiver")))
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))

				By("checking that the task state and completion time are not modified")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":           Equal(task.ID),
					"ReceiverUUID": Equal(task.ReceiverUUID),
					"State":        Equal(task.State),
					"Version":      Equal(task.Version),
					"CreationDate": BeTemporally("==", task.CreationDate),
				}))
			})

			It("Set definition task Applied state and completion date", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating an alert definition")
				def := &models.AlertDefinition{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.DefinitionNew,
					Version:  1,
					Category: models.CategoryHealth,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(&def).Error).ShouldNot(HaveOccurred())

				By("creating associated definitiontask with New state")
				task := models.Task{
					ID:                  1,
					AlertDefinitionUUID: &def.UUID,
					TenantID:            def.TenantID,
					Version:             def.Version,
					State:               models.TaskNew,
					CreationDate:        clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as applied")
				Expect(db.SetTaskAsApplied(ctx, task)).ShouldNot(HaveOccurred())

				By("checking that the task state is Applied and completion time is set")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":             Equal(task.ID),
					"ReceiverUUID":   Equal(task.ReceiverUUID),
					"State":          Equal(models.TaskApplied),
					"Version":        Equal(task.Version),
					"CreationDate":   BeTemporally("==", task.CreationDate),
					"CompletionDate": BeTemporally("==", completionDate),
				}))

				By("checking that the alert definition state is Applied")
				var defOut models.AlertDefinition
				Expect(db.DB.WithContext(ctx).First(&defOut, def.ID).Error).ShouldNot(HaveOccurred())
				Expect(defOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(def.ID),
					"UUID":    Equal(def.UUID),
					"State":   Equal(models.DefinitionApplied),
					"Version": Equal(def.Version),
				}))
			})

			It("Fail to set definition task Applied state and completion date because there is no associated definition record", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()

				By("creating a definition task with New state")
				task := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					Version:             1,
					State:               models.TaskNew,
					CreationDate:        clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				By("failing to set the task as applied")
				err := db.SetTaskAsApplied(ctx, task)
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve alert definition")))
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))

				By("checking that the task state and completion time are not modified")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task.ID),
					"AlertDefinitionUUID": Equal(task.AlertDefinitionUUID),
					"State":               Equal(task.State),
					"Version":             Equal(task.Version),
					"CreationDate":        BeTemporally("==", task.CreationDate),
				}))
			})
		})

		When("Setting a task as failed", func() {
			It("Fail to set receiver task as failed because there is no associated receiver record", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver task")
				task := models.Task{
					ID:           1,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					Version:      1,
					State:        models.TaskTaken,
					CreationDate: clock.FakeClock.Now(),
					StartDate:    clock.FakeClock.Now().Add(5 * time.Second),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				By("failing to set the task as failed")
				err := db.SetTaskAsFailed(ctx, task, 10)
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve receiver")))
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))

				By("checking that the task is not modified")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":           Equal(task.ID),
					"ReceiverUUID": Equal(task.ReceiverUUID),
					"Version":      Equal(task.Version),
					"State":        Equal(task.State),
					"CreationDate": BeTemporally("==", task.CreationDate),
					"StartDate":    BeTemporally("==", task.StartDate),
				}))
			})

			It("Set receiver task to Error state because the retry count does not reach the limit", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver")
				recv := &models.Receiver{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.ReceiverModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(recv).Error).ShouldNot(HaveOccurred())

				By("creating an associated receiver task with a retry count")
				task := models.Task{
					ID:           1,
					ReceiverUUID: &recv.UUID,
					TenantID:     recv.TenantID,
					Version:      recv.Version,
					State:        models.TaskTaken,
					CreationDate: clock.FakeClock.Now(),
					StartDate:    clock.FakeClock.Now().Add(5 * time.Second),
					RetryCount:   5,
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as failed")
				Expect(db.SetTaskAsFailed(ctx, task, 10)).ShouldNot(HaveOccurred())

				By("checking that the task state is Error since its retry count does not exceed the retry limit")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":           Equal(task.ID),
					"ReceiverUUID": Equal(task.ReceiverUUID),
					"State":        Equal(models.TaskError),
					"RetryCount":   Equal(task.RetryCount + 1),
					"CreationDate": BeTemporally("==", task.CreationDate),
					"StartDate":    BeTemporally("==", task.StartDate),
				}))

				By("checking that the receiver state is Error")
				var recvOut models.Receiver
				Expect(db.DB.WithContext(ctx).First(&recvOut, recv.ID).Error).ShouldNot(HaveOccurred())
				Expect(recvOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(recv.ID),
					"UUID":    Equal(recv.UUID),
					"State":   Equal(models.ReceiverError),
					"Version": Equal(recv.Version),
				}))
			})

			It("Set receiver task to Error state because the retry count does not reach the limit", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver")
				recv := &models.Receiver{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.ReceiverModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(recv).Error).ShouldNot(HaveOccurred())

				By("creating an associated receiver task with a retry count")
				task := models.Task{
					ID:           1,
					ReceiverUUID: &recv.UUID,
					TenantID:     recv.TenantID,
					Version:      recv.Version,
					State:        models.TaskTaken,
					CreationDate: clock.FakeClock.Now(),
					StartDate:    clock.FakeClock.Now().Add(5 * time.Second),
					RetryCount:   5,
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as failed")
				Expect(db.SetTaskAsFailed(ctx, task, 10)).ShouldNot(HaveOccurred())

				By("checking that the task state is Error since its retry count does not exceed the retry limit")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":           Equal(task.ID),
					"ReceiverUUID": Equal(task.ReceiverUUID),
					"State":        Equal(models.TaskError),
					"RetryCount":   Equal(task.RetryCount + 1),
					"CreationDate": BeTemporally("==", task.CreationDate),
					"StartDate":    BeTemporally("==", task.StartDate),
				}))

				By("checking that the receiver state is Error")
				var recvOut models.Receiver
				Expect(db.DB.WithContext(ctx).First(&recvOut, recv.ID).Error).ShouldNot(HaveOccurred())
				Expect(recvOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(recv.ID),
					"UUID":    Equal(recv.UUID),
					"State":   Equal(models.ReceiverError),
					"Version": Equal(recv.Version),
				}))
			})

			It("Fail to set definition task as failed because there is no associated definition record", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating an alert definition task")
				task := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					Version:             1,
					State:               models.TaskTaken,
					CreationDate:        clock.FakeClock.Now(),
					StartDate:           clock.FakeClock.Now().Add(5 * time.Second),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				By("failing to set the task as failed")
				err := db.SetTaskAsFailed(ctx, task, 10)
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve alert definition")))
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))

				By("checking that the task has not been modified")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task.ID),
					"AlertDefinitionUUID": Equal(task.AlertDefinitionUUID),
					"Version":             Equal(task.Version),
					"State":               Equal(task.State),
					"CreationDate":        BeTemporally("==", task.CreationDate),
					"StartDate":           BeTemporally("==", task.StartDate),
				}))
			})

			It("Set definition task to Error state because the retry count does not reach the limit", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver")
				def := &models.AlertDefinition{
					ID:       1,
					UUID:     uuid.New(),
					Category: models.CategoryHealth,
					State:    models.DefinitionModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(def).Error).ShouldNot(HaveOccurred())

				By("creating an associated receiver task with a retry count")
				task := models.Task{
					ID:                  1,
					AlertDefinitionUUID: &def.UUID,
					TenantID:            def.TenantID,
					Version:             def.Version,
					State:               models.TaskTaken,
					CreationDate:        clock.FakeClock.Now(),
					StartDate:           clock.FakeClock.Now().Add(5 * time.Second),
					RetryCount:          5,
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as failed")
				Expect(db.SetTaskAsFailed(ctx, task, 10)).ShouldNot(HaveOccurred())

				By("checking that the task state is Error since its retry count does not exceed the retry limit")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task.ID),
					"AlertDefinitionUUID": Equal(task.AlertDefinitionUUID),
					"State":               Equal(models.TaskError),
					"RetryCount":          Equal(task.RetryCount + 1),
					"CreationDate":        BeTemporally("==", task.CreationDate),
					"StartDate":           BeTemporally("==", task.StartDate),
				}))

				By("checking that the receiver state is Error")
				var defOut models.AlertDefinition
				Expect(db.DB.WithContext(ctx).First(&defOut, def.ID).Error).ShouldNot(HaveOccurred())
				Expect(defOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(def.ID),
					"UUID":    Equal(def.UUID),
					"State":   Equal(models.DefinitionError),
					"Version": Equal(def.Version),
				}))
			})

			It("Set receiver task to invalid state because the retry count reaches the limit", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver")
				recv := &models.Receiver{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.ReceiverModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(recv).Error).ShouldNot(HaveOccurred())

				By("creating an associated receiber task with a retry count")
				retryLimit := 10
				task := models.Task{
					ID:           1,
					ReceiverUUID: &recv.UUID,
					TenantID:     recv.TenantID,
					Version:      recv.Version,
					State:        models.TaskTaken,
					CreationDate: clock.FakeClock.Now(),
					StartDate:    clock.FakeClock.Now().Add(5 * time.Second),
					RetryCount:   int64(retryLimit),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as failed")
				Expect(db.SetTaskAsFailed(ctx, task, retryLimit)).ShouldNot(HaveOccurred())

				By("checking that the task state is Invalid since its retry count exceeds the retry limit")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":           Equal(task.ID),
					"ReceiverUUID": Equal(task.ReceiverUUID),
					"State":        Equal(models.TaskInvalid),
					"RetryCount":   Equal(task.RetryCount),
					"CreationDate": BeTemporally("==", task.CreationDate),
					"StartDate":    BeTemporally("==", task.StartDate),
				}))

				By("checking that the receiver state is Error")
				var recvOut models.Receiver
				Expect(db.DB.WithContext(ctx).First(&recvOut, recv.ID).Error).ShouldNot(HaveOccurred())
				Expect(recvOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(recv.ID),
					"UUID":    Equal(recv.UUID),
					"State":   Equal(models.ReceiverError),
					"Version": Equal(recv.Version),
				}))
			})

			It("Set definition task to invalid state because the retry count reaches the limit", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating an alert definition")
				def := &models.AlertDefinition{
					ID:       1,
					UUID:     uuid.New(),
					Category: models.CategoryHealth,
					State:    models.DefinitionModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(def).Error).ShouldNot(HaveOccurred())

				By("creating an associated alert definition task with a retry count")
				retryLimit := 10
				task := models.Task{
					ID:                  1,
					AlertDefinitionUUID: &def.UUID,
					TenantID:            def.TenantID,
					Version:             def.Version,
					State:               models.TaskTaken,
					CreationDate:        clock.FakeClock.Now(),
					StartDate:           clock.FakeClock.Now().Add(5 * time.Second),
					RetryCount:          int64(retryLimit),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as failed")
				Expect(db.SetTaskAsFailed(ctx, task, retryLimit)).ShouldNot(HaveOccurred())

				By("checking that the task state is Invalid since its retry count exceeds the retry limit")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task.ID),
					"AlertDefinitionUUID": Equal(task.AlertDefinitionUUID),
					"State":               Equal(models.TaskInvalid),
					"RetryCount":          Equal(task.RetryCount),
					"CreationDate":        BeTemporally("==", task.CreationDate),
					"StartDate":           BeTemporally("==", task.StartDate),
				}))

				By("checking that the receiver state is Error")
				var defOut models.AlertDefinition
				Expect(db.DB.WithContext(ctx).First(&defOut, def.ID).Error).ShouldNot(HaveOccurred())
				Expect(defOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(def.ID),
					"UUID":    Equal(def.UUID),
					"State":   Equal(models.DefinitionError),
					"Version": Equal(def.Version),
				}))
			})
		})

		When("Setting a task as invalid", func() {
			It("Fail to set an alert definition task to Invalid because there is no associated alert definition", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating an alert definition task with Taken state")
				task := models.Task{
					ID:                  1,
					AlertDefinitionUUID: uuidPtr(uuid.New()),
					TenantID:            "edgenode",
					Version:             1,
					State:               models.TaskTaken,
					CreationDate:        clock.FakeClock.Now(),
					StartDate:           clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				By("failing to set the task as invalid")
				err := db.SetTaskAsInvalid(ctx, task)
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve alert definition")))
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))

				By("checking that the task has not been modified")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task.ID),
					"AlertDefinitionUUID": Equal(task.AlertDefinitionUUID),
					"Version":             Equal(task.Version),
					"State":               Equal(task.State),
					"CreationDate":        BeTemporally("==", task.CreationDate),
					"StartDate":           BeTemporally("==", task.StartDate),
				}))
			})

			It("Success to set an alert definition task as invalid", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating an alert definition")
				def := models.AlertDefinition{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.DefinitionModified,
					Category: models.CategoryHealth,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(&def).Error).ShouldNot(HaveOccurred())

				By("creating an associated alert definition task with Taken state")
				task := models.Task{
					ID:                  1,
					AlertDefinitionUUID: &def.UUID,
					TenantID:            def.TenantID,
					Version:             def.Version,
					State:               models.TaskTaken,
					CreationDate:        clock.FakeClock.Now(),
					StartDate:           clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as invalid")
				Expect(db.SetTaskAsInvalid(ctx, task)).ShouldNot(HaveOccurred())

				By("checking that the task state is set to Invalid with completion_date")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":                  Equal(task.ID),
					"AlertDefinitionUUID": Equal(task.AlertDefinitionUUID),
					"State":               Equal(models.TaskInvalid),
					"CreationDate":        BeTemporally("==", task.CreationDate),
					"StartDate":           BeTemporally("==", task.StartDate),
					"CompletionDate":      BeTemporally("==", completionDate),
				}))

				By("checking that the alert definition state is Error")
				var defOut models.AlertDefinition
				Expect(db.DB.WithContext(ctx).First(&defOut, def.ID).Error).ShouldNot(HaveOccurred())
				Expect(defOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(def.ID),
					"UUID":    Equal(def.UUID),
					"State":   Equal(models.DefinitionError),
					"Version": Equal(def.Version),
				}))
			})

			It("Fail to set a receiver task to Invalid because there is no associated receiver", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver task with Taken state")
				task := models.Task{
					ID:           1,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					Version:      1,
					State:        models.TaskTaken,
					CreationDate: clock.FakeClock.Now(),
					StartDate:    clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				By("failing to set the task as invalid")
				err := db.SetTaskAsInvalid(ctx, task)
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve receiver")))
				Expect(err).To(MatchError(gorm.ErrRecordNotFound))

				By("checking that the task has not been modified")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":           Equal(task.ID),
					"ReceiverUUID": Equal(task.ReceiverUUID),
					"Version":      Equal(task.Version),
					"State":        Equal(task.State),
					"CreationDate": BeTemporally("==", task.CreationDate),
					"StartDate":    BeTemporally("==", task.StartDate),
				}))
			})

			It("Success to set a receiver task as invalid", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a receiver")
				recv := models.Receiver{
					ID:       1,
					UUID:     uuid.New(),
					State:    models.ReceiverModified,
					Version:  1,
					TenantID: "edgenode",
				}
				Expect(db.DB.WithContext(ctx).Create(&recv).Error).ShouldNot(HaveOccurred())

				By("creating an associated receiver task with Taken state")
				task := models.Task{
					ID:           1,
					ReceiverUUID: &recv.UUID,
					TenantID:     recv.TenantID,
					Version:      recv.Version,
					State:        models.TaskTaken,
					CreationDate: clock.FakeClock.Now(),
					StartDate:    clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task as invalid")
				Expect(db.SetTaskAsInvalid(ctx, task)).ShouldNot(HaveOccurred())

				By("checking that the task state is set to Invalid with completion_date")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":             Equal(task.ID),
					"ReceiverUUID":   Equal(task.ReceiverUUID),
					"State":          Equal(models.TaskInvalid),
					"CreationDate":   BeTemporally("==", task.CreationDate),
					"StartDate":      BeTemporally("==", task.StartDate),
					"CompletionDate": BeTemporally("==", completionDate),
				}))

				By("checking that the receiver state is Error")
				var recvOut models.Receiver
				Expect(db.DB.WithContext(ctx).First(&recvOut, recv.ID).Error).ShouldNot(HaveOccurred())
				Expect(recvOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":      Equal(recv.ID),
					"UUID":    Equal(recv.UUID),
					"State":   Equal(models.ReceiverError),
					"Version": Equal(recv.Version),
				}))
			})
		})

		When("Setting only the state of a task to Invalid", func() {
			It("Set a task state to Invalid", func() {
				ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
				defer cancel()

				By("creating a task with Taken state")
				task := models.Task{
					ID:           1,
					ReceiverUUID: uuidPtr(uuid.New()),
					TenantID:     "edgenode",
					Version:      1,
					State:        models.TaskTaken,
					CreationDate: clock.FakeClock.Now(),
					StartDate:    clock.FakeClock.Now(),
				}
				Expect(db.DB.WithContext(ctx).Create(&task).Error).ShouldNot(HaveOccurred())

				// Set time to make CompletionDate different from CreationDate.
				completionDate := clock.FakeClock.Now().Add(30 * time.Second)
				clock.FakeClock.Set(completionDate)

				By("setting the task state to Invalid")
				Expect(db.SetTaskStateToInvalid(ctx, task)).ShouldNot(HaveOccurred())

				By("checking that the task state is Invalid and the completion time is set")
				var taskOut models.Task
				Expect(db.DB.WithContext(ctx).First(&taskOut, task.ID).Error).ShouldNot(HaveOccurred())
				Expect(taskOut).To(MatchFields(IgnoreExtras, Fields{
					"ID":             Equal(task.ID),
					"ReceiverUUID":   Equal(task.ReceiverUUID),
					"Version":        Equal(task.Version),
					"State":          Equal(models.TaskInvalid),
					"CreationDate":   BeTemporally("==", task.CreationDate),
					"StartDate":      BeTemporally("==", task.StartDate),
					"CompletionDate": BeTemporally("~", clock.FakeClock.Now()),
				}))
			})
		})
	})
})
