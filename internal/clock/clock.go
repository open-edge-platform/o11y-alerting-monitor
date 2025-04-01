// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package clock

import (
	"time"

	"github.com/jmhodges/clock"
)

var (
	// holds function definition to retrieve the current local time.
	TimeNowFn func() time.Time

	// holds a fake clock used to test time-sensitive code.
	FakeClock clock.FakeClock
)

// SetFakeClock gates the use of a fake clock for unit tests to retrieve
// the current local time.
func SetFakeClock() {
	TimeNowFn = FakeClock.Now
}

// UnsetFakeClock restores TimeNowFn function to retrieve the current time from the host.
func UnsetFakeClock() {
	TimeNowFn = time.Now
}

func init() {
	TimeNowFn = time.Now
	FakeClock = clock.NewFake()
}
