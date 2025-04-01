// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"gorm.io/gorm"
)

type DBService struct {
	DB *gorm.DB
}
