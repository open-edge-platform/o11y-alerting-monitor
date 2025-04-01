// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailAddress struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Email     string `gorm:"not null;unique"`
	FirstName string `gorm:"not null"`
	LastName  string `gorm:"not null"`
}

func (e EmailAddress) String() string {
	return fmt.Sprintf("%s %s <%s>", e.FirstName, e.LastName, e.Email)
}

type EmailConfig struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	MailServer string `gorm:"not null"`
	From       int64  `gorm:"not null"` // This references an EmailAddress.ID
}

type ReceiverState string

const (
	ReceiverNew      ReceiverState = "New"
	ReceiverModified ReceiverState = "Modified"
	ReceiverPending  ReceiverState = "Pending"
	ReceiverApplied  ReceiverState = "Applied"
	ReceiverError    ReceiverState = "Error"
)

func (rs ReceiverState) Validate() error {
	switch rs {
	case ReceiverNew:
	case ReceiverModified:
	case ReceiverPending:
	case ReceiverApplied:
	case ReceiverError:
	default:
		return fmt.Errorf("unknown receiver state: %q", rs)
	}
	return nil
}

type Receiver struct {
	ID            int64         `gorm:"primaryKey;autoIncrement"`
	UUID          uuid.UUID     `gorm:"type:uuid;not null;uniqueIndex:idx_recv_uuid_version_tenant"`
	Name          string        `gorm:"not null;uniqueIndex:idx_name_version_tenant"`
	State         ReceiverState `gorm:"not null,type:enum('New','Modified','Pending','Applied','Error'),default:New"`
	Version       int64         `gorm:"not null;uniqueIndex:idx_recv_uuid_version_tenant;uniqueIndex:idx_name_version_tenant"`
	EmailConfigID int64         `gorm:"not null"`
	TenantID      string        `gorm:"not null;default:edgenode;uniqueIndex:idx_recv_uuid_version_tenant;uniqueIndex:idx_name_version_tenant"`
}

func (r *Receiver) BeforeCreate(*gorm.DB) error {
	return r.State.Validate()
}

func (r *Receiver) AfterUpdate(*gorm.DB) error {
	return r.State.Validate()
}

// DBReceiver represents info of an alert receiver, including mail server, sender address,
// and the list of email recipients.
type DBReceiver struct {
	UUID       uuid.UUID
	State      ReceiverState
	Name       string
	Version    int
	MailServer string
	From       string
	To         []string
	TenantID   string
}

type EmailRecipient struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	ReceiverID     int64 `gorm:"uniqueIndex:idx_receiver_email_enabled"`
	EmailAddressID int64 `gorm:"uniqueIndex:idx_receiver_email_enabled"`
}
