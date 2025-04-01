// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/clock"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

var (
	ErrValueOutOfBounds = errors.New("value out of bounds")
)

// GetLatestAlertDefinitionList gets the list with the info on the latest version of alert definitions including their duration, threshold,
// and a flag specifying if the alerts are enabled. Alert definitions with state 'Error' are excluded.
func (d *DBService) GetLatestAlertDefinitionList(ctx context.Context, tenantID api.TenantID) ([]*models.DBAlertDefinition, error) {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	definitionUUIDs, err := GetAlertDefinitionUUIDs(tx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get list of alert definition UUIDs for tenant %q: %w", tenantID, err)
	}

	definitions := make([]*models.DBAlertDefinition, len(definitionUUIDs))
	for i, definitionUUID := range definitionUUIDs {
		d, err := d.GetLatestAlertDefinition(ctx, tenantID, definitionUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to get alert definition %q for tenant %q: %w", definitionUUID, tenantID, err)
		}
		definitions[i] = d
	}

	return definitions, nil
}

// GetAlertDefinitionUUIDs is a helper function that gets the list with unique alert definition UUIDs.
func GetAlertDefinitionUUIDs(tx *gorm.DB, tenantID api.TenantID) ([]uuid.UUID, error) {
	var ids []uuid.UUID

	txx := tx.Model(&models.AlertDefinition{}).Where("tenant_id = ?", tenantID).Distinct().Pluck("uuid", &ids)
	if err := txx.Error; err != nil {
		return nil, err
	}

	return ids, nil
}

// GetLatestAlertDefinition gets the info on the latest version of an alert definition, including its duration, threshold, and a flag specifying
// if the alert is enabled. Alert definitions with state 'Error' are excluded.
func (d *DBService) GetLatestAlertDefinition(ctx context.Context, tenantID api.TenantID, id uuid.UUID) (*models.DBAlertDefinition, error) {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	var ad models.AlertDefinition
	if err := tx.
		Where("tenant_id = ?", tenantID).
		Where("uuid = ?", id).
		Where("state != ?", models.DefinitionError).
		Order("version desc").
		First(&ad).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve latest version of alert definition for tenant %q: %w", tenantID, err)
	}

	return getDBAlertDefinition(tx, id, ad)
}

// GetAlertDefinition gets the info of a specific version of alert definition, including its duration, threshold,
// and a flag specifying if the alert is enabled.
func (d *DBService) GetAlertDefinition(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64) (*models.DBAlertDefinition, error) {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	var ad models.AlertDefinition
	if err := tx.Where("tenant_id = ?", tenantID).Where("uuid = ?", id).Where("version = ?", version).Take(&ad).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve alert definition %q version %d for tenant %q: %w", id, version, tenantID, err)
	}

	return getDBAlertDefinition(tx, id, ad)
}

func getDBAlertDefinition(tx *gorm.DB, id uuid.UUID, ad models.AlertDefinition) (*models.DBAlertDefinition, error) {
	res := &models.DBAlertDefinition{
		ID:       ad.UUID,
		Name:     ad.Name,
		State:    ad.State,
		Template: ad.Template,
		Interval: ad.AlertInterval,
		Version:  ad.Version,
		Category: ad.Category,
		TenantID: ad.TenantID,
	}

	row := tx.
		Table("alert_definitions adef").
		Joins("INNER JOIN alert_durations adur ON adur.alert_definition_id = adef.id").
		Joins("INNER JOIN alert_thresholds athr ON athr.alert_definition_id = adef.id").
		Select("adur.duration, athr.threshold, adef.enabled").
		Where("adef.tenant_id = ?", ad.TenantID).
		Where("adef.uuid = ?", id).
		Where("adef.version = ?", ad.Version).
		Row()

	if err := row.Scan(
		&res.Values.Duration,
		&res.Values.Threshold,
		&res.Values.Enabled,
	); err != nil {
		return nil, err
	}

	return res, nil
}

// SetAlertDefinitionValues sets values such as duration, threshold, and enabled state of an alert definition given its UUID.
// It also creates a new task for task executor, linked to the newly created definition.
func (d *DBService) SetAlertDefinitionValues(ctx context.Context, tenantID api.TenantID, id uuid.UUID, values models.DBAlertDefinitionValues) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	// Get the latest version of the alert definition by UUID and tenantID, if exists.
	var definition models.AlertDefinition
	if err := tx.Where("tenant_id = ?", tenantID).Where("uuid = ?", id).Order("version desc").First(&definition).Error; err != nil {
		return fmt.Errorf("failed to retrieve latest version of alert definition for tenant %q: %w", tenantID, err)
	}

	// Set enabled field for the new alert definition.
	var enabledValue bool
	if values.Enabled != nil {
		enabledValue = *values.Enabled
	} else {
		enabledValue = definition.Enabled
	}

	tmpl, err := rules.UpdateTemplateWithValues(definition.Template, values.Duration, values.Threshold)
	if err != nil {
		return fmt.Errorf("failed to update alert definition template: %w", err)
	}

	// Create new alert definition with enabled field set and bumped version.
	newDefinition := models.AlertDefinition{
		UUID:          definition.UUID,
		Name:          definition.Name,
		State:         models.DefinitionModified,
		Template:      tmpl,
		Category:      definition.Category,
		Context:       definition.Context,
		Severity:      definition.Severity,
		AlertInterval: definition.AlertInterval,
		Enabled:       enabledValue,
		Version:       definition.Version + 1,
		TenantID:      definition.TenantID,
	}
	if err := tx.Create(&newDefinition).Error; err != nil {
		return fmt.Errorf("failed to create new alert definition with bumped version %v: %w", newDefinition.Version, err)
	}

	// Create new alert duration and associate it to the new alert definition.
	if err := setAlertDefinitionDuration(tx, definition.ID, newDefinition.ID, values.Duration); err != nil {
		return fmt.Errorf("failed to set duration to new alert definition ID %v: %w", newDefinition.ID, err)
	}

	// Create new alert threshold and associate it to the new alert definition.
	if err := setAlertDefinitionThreshold(tx, definition.ID, newDefinition.ID, values.Threshold); err != nil {
		return fmt.Errorf("failed to set threshold to new alert definition ID %v: %w", newDefinition.ID, err)
	}

	task := models.Task{
		State:               models.TaskNew,
		AlertDefinitionUUID: &newDefinition.UUID,
		TenantID:            newDefinition.TenantID,
		Version:             newDefinition.Version,
		CreationDate:        clock.TimeNowFn(),
	}

	if err := tx.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to create a new task for alert definition ID %v version %v: %w", newDefinition.ID, newDefinition.Version, err)
	}

	return tx.Commit().Error
}

// SetAlertDefinitionState updates the `State` column of specific alert definition version.
func (d *DBService) SetAlertDefinitionState(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64, state models.AlertDefinitionState) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	if err := setAlertDefinitionState(tx, tenantID, id, version, state); err != nil {
		return err
	}

	return tx.Commit().Error
}

func setAlertDefinitionState(tx *gorm.DB, tenantID api.TenantID, id uuid.UUID, version int64, state models.AlertDefinitionState) error {
	var definition models.AlertDefinition

	if err := tx.Where("tenant_id = ?", tenantID).Where("uuid = ?", id).Where("version = ?", version).Take(&definition).Error; err != nil {
		return fmt.Errorf("failed to retrieve alert definition for tenant %q: %w", tenantID, err)
	}

	if err := tx.Model(&definition).Update("state", state).Error; err != nil {
		return fmt.Errorf("failed to update alert definition state: %w", err)
	}
	return nil
}

// setAlertDefinitionDuration is a helper function that creates a new alert duration. It populates its content with the alert duration
// associated to fromID foreign key. The duration value is set to the value argument, if not nil. Otherwise remains unchanged. Eventually
// it associates the newly created duration with the alert definition ID specified by toID argument. Additionally checks that the value to
// set as is within allowed minimum and maximum for the alert definition.
func setAlertDefinitionDuration(tx *gorm.DB, fromID, toID int64, value *int64) error {
	// Get duration corresponding to the original alert definition.
	var duration models.AlertDuration
	if err := tx.Where("alert_definition_id = ?", fromID).Find(&duration).Error; err != nil {
		return fmt.Errorf("failed to retrieve duration for alert definition ID %v: %w", fromID, err)
	}

	// Set duration value for the new alert duration.
	durationValue := duration.Duration
	if value != nil {
		durationValue = *value
	}

	if durationValue < duration.DurationMin || durationValue > duration.DurationMax {
		return fmt.Errorf("duration value out of valid range [%d, %d] seconds: %w", duration.DurationMin, duration.DurationMax, ErrValueOutOfBounds)
	}

	// Create new duration and associate it with the new alert definition's foreign key.
	newDuration := models.AlertDuration{
		Name:              duration.Name,
		Duration:          durationValue,
		DurationMin:       duration.DurationMin,
		DurationMax:       duration.DurationMax,
		AlertDefinitionID: toID,
	}
	if err := tx.Create(&newDuration).Error; err != nil {
		return errors.New("failed to create duration with new value set")
	}

	return nil
}

// setAlertDefinitionThreshold is a helper function that creates a new alert threshold. It populates its content with the alert threshold
// associated to fromID foreign key. The threshold value is set to the value argument, if not nil. Otherwise remains unchanged. Eventually
// it associates the newly created threshold with the alert definition ID specified by toID argument. Additionally checks that the value to
// set is within allowed minimum and maximum for the alert definition.
func setAlertDefinitionThreshold(tx *gorm.DB, fromID, toID int64, value *int64) error {
	// Get threshold corresponding to the original alert definition.
	var threshold models.AlertThreshold
	if err := tx.Where("alert_definition_id = ?", fromID).Find(&threshold).Error; err != nil {
		return fmt.Errorf("failed to retrieve threshold for alert definition ID %v: %w", fromID, err)
	}

	// Set threshold value for the new alert duration.
	thresholdValue := threshold.Threshold
	if value != nil {
		thresholdValue = *value
	}

	if thresholdValue < threshold.ThresholdMin || thresholdValue > threshold.ThresholdMax {
		return fmt.Errorf("threshold value out of valid range [%d, %d]: %w", threshold.ThresholdMin, threshold.ThresholdMax, ErrValueOutOfBounds)
	}

	// Create new threshold and associate it with the new alert definition's foreign key.
	newThreshold := models.AlertThreshold{
		Name:              threshold.Name,
		Threshold:         thresholdValue,
		ThresholdMin:      threshold.ThresholdMin,
		ThresholdMax:      threshold.ThresholdMax,
		AlertDefinitionID: toID,
	}
	if err := tx.Create(&newThreshold).Error; err != nil {
		return errors.New("failed to create threshold with new value set")
	}

	return nil
}
