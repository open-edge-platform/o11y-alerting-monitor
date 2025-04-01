// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/clock"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
)

// GetLatestReceiverListWithEmailConfig gets the list with the info of the latest version of alert receivers including their mail server,
// sender, and list of email recipients. Receivers with state 'Error' are excluded.
func (d *DBService) GetLatestReceiverListWithEmailConfig(ctx context.Context, tenantID api.TenantID) ([]*models.DBReceiver, error) {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	recvUUIDs, err := GetReceiverUUIDs(tx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get list of receiver UUIDs for tenant %q: %w", tenantID, err)
	}

	receivers := make([]*models.DBReceiver, len(recvUUIDs))
	for i, recvUUID := range recvUUIDs {
		// Get the receiver by UUID and tenantID, if exists, with the latest version.
		var recv models.Receiver
		if err := tx.
			Where("tenant_id = ?", tenantID).
			Where("uuid = ?", recvUUID).
			Where("state != ?", models.ReceiverError).
			Order("version desc").
			First(&recv).Error; err != nil {
			return nil, err
		}

		dbRecv, err := getReceiverWithEmailConfig(tx, recv)
		if err != nil {
			return nil, fmt.Errorf("failed to get receiver %q for tenant %q: %w", recvUUID, tenantID, err)
		}
		receivers[i] = dbRecv
	}

	return receivers, nil
}

// GetReceiverUUIDs is a helper function that gets the list with unique alert receiver UUIDs.
func GetReceiverUUIDs(tx *gorm.DB, tenantID api.TenantID) ([]uuid.UUID, error) {
	var ids []uuid.UUID

	txx := tx.Model(&models.Receiver{}).Where("tenant_id = ?", tenantID).Distinct().Pluck("uuid", &ids)
	if err := txx.Error; err != nil {
		return nil, err
	}

	return ids, nil
}

// GetLatestReceiverWithEmailConfig gets the info on the latest version of an alert receiver including its mail server, sender, and list of email
// recipients. Receivers with state 'Error' are excluded.
func (d *DBService) GetLatestReceiverWithEmailConfig(ctx context.Context, tenantID api.TenantID, id uuid.UUID) (*models.DBReceiver, error) {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	// Get the receiver by UUID and tenantID, if exists, with the latest version.
	var recv models.Receiver
	if err := tx.
		Where("tenant_id = ?", tenantID).
		Where("uuid = ?", id).
		Where("state != ?", models.ReceiverError).
		Order("version desc").
		First(&recv).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve latest version of receiver for tenant %q: %w", tenantID, err)
	}

	return getReceiverWithEmailConfig(tx, recv)
}

// GetReceiverWithEmailConfig gets the info of a specific version of an alert receiver including its mail server, sender, and
// list of email recipients.
func (d *DBService) GetReceiverWithEmailConfig(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64) (*models.DBReceiver, error) {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	// Get the receiver by UUID and tenantID, if exists, with the specified version.
	var recv models.Receiver
	if err := tx.
		Where("tenant_id = ?", tenantID).
		Where("uuid = ?", id).
		Where("version = ?", version).
		Take(&recv).Error; err != nil {
		return nil, err
	}

	return getReceiverWithEmailConfig(tx, recv)
}

// getReceiverWithEmailConfig is a helper function that gets the info of an alert receiver.
// It accepts a pointer to DB GORM definition to allow query executions within the same transaction.
func getReceiverWithEmailConfig(tx *gorm.DB, recv models.Receiver) (*models.DBReceiver, error) {
	var (
		mailServer string

		from struct {
			firstName string
			lastName  string
			email     string
		}
	)

	// Get server mail and the email address of the sender of the versioned alert receiver.
	row := tx.
		Table("email_addresses ea").
		Joins("INNER JOIN email_configs ec ON ec.\"from\" = ea.id").
		Joins("INNER JOIN receivers r ON r.email_config_id = ec.id").
		Select("ec.mail_server, ea.first_name, ea.last_name, ea.email").
		Where("r.tenant_id = ?", recv.TenantID).
		Where("r.uuid = ?", recv.UUID).
		Where("r.version = ?", recv.Version).
		Row()

	if err := row.Scan(
		&mailServer,
		&from.firstName,
		&from.lastName,
		&from.email,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to get sender email address and mail server for receiver for tenant %q: %w", recv.TenantID, err)
	} else if err != nil {
		return nil, err
	}

	// Get email recipients of the versioned alert receiver.
	var recipients []models.EmailAddress
	err := tx.
		Table("email_addresses ea").
		Joins("INNER JOIN email_recipients er ON ea.id = er.email_address_id").
		Joins("INNER JOIN receivers r ON er.receiver_id = r.id").
		Where("r.tenant_id = ?", recv.TenantID).
		Where("r.uuid = ?", recv.UUID).
		Where("r.version = ?", recv.Version).
		Find(&recipients).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get email recipients for receiver for tenant %q: %w", recv.TenantID, err)
	}

	to := make([]string, len(recipients))
	for i, r := range recipients {
		to[i] = r.String()
	}

	return &models.DBReceiver{
		UUID:       recv.UUID,
		State:      recv.State,
		Name:       recv.Name,
		Version:    int(recv.Version),
		MailServer: mailServer,
		From:       fmt.Sprintf("%s %s <%s>", from.firstName, from.lastName, from.email),
		To:         to,
		TenantID:   recv.TenantID,
	}, nil
}

// SetReceiverEmailRecipients sets the list of email recipients of an alert receiver.
// It also creates a new task for task executor, linked to the newly created receiver.
func (d *DBService) SetReceiverEmailRecipients(ctx context.Context, tenantID api.TenantID, id uuid.UUID, recipients []models.EmailAddress) error {
	tx := d.DB.Begin().WithContext(ctx)
	defer tx.Rollback()

	// Get the receiver by UUID and tenantID, if exists, with the latest version.
	var recv models.Receiver
	if err := tx.Where("tenant_id = ?", tenantID).Where("uuid = ?", id).Order("version desc").First(&recv).Error; err != nil {
		return err
	}

	// Create new receiver with bumped version.
	newRecv := models.Receiver{
		UUID:          recv.UUID,
		Name:          recv.Name,
		State:         models.ReceiverModified,
		EmailConfigID: recv.EmailConfigID,
		Version:       recv.Version + 1,
		TenantID:      recv.TenantID,
	}
	if err := tx.Create(&newRecv).Error; err != nil {
		return err
	}

	for _, r := range recipients {
		recipient := r

		// Check if email is within the email_addresses table, if not insert.
		if err := tx.Where(models.EmailAddress{
			Email: recipient.Email,
		}).FirstOrCreate(&recipient).Error; err != nil {
			return err
		}

		if err := tx.Create(&models.EmailRecipient{
			ReceiverID:     newRecv.ID,
			EmailAddressID: recipient.ID,
		}).Error; err != nil {
			return err
		}
	}

	task := models.Task{
		State:        models.TaskNew,
		ReceiverUUID: &newRecv.UUID,
		TenantID:     newRecv.TenantID,
		Version:      newRecv.Version,
		CreationDate: clock.TimeNowFn(),
	}
	if err := tx.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to create a new task for receiver with uuid %v version %v for tenant %q: %w", newRecv.UUID, newRecv.Version, tenantID, err)
	}

	return tx.Commit().Error
}

// SetReceiverState sets the state of the specific version of a given receiver.
func (d *DBService) SetReceiverState(ctx context.Context, tenantID api.TenantID, id uuid.UUID, version int64, state models.ReceiverState) error {
	tx := d.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	if err := setReceiverState(tx, tenantID, id, version, state); err != nil {
		return err
	}

	return tx.Commit().Error
}

func setReceiverState(tx *gorm.DB, tenantID api.TenantID, id uuid.UUID, version int64, state models.ReceiverState) error {
	// Get the receiver by UUID and tenantID, if exists, with the specified version.
	var recv models.Receiver
	if err := tx.
		Where("tenant_id = ?", tenantID).
		Where("uuid = ?", id).
		Where("version = ?", version).
		Take(&recv).Error; err != nil {
		return fmt.Errorf("failed to retrieve receiver for tenant %q: %w", tenantID, err)
	}

	if err := tx.
		Model(&recv).
		Update("state", state).Error; err != nil {
		return fmt.Errorf("failed to update receiver state: %w", err)
	}

	return nil
}
