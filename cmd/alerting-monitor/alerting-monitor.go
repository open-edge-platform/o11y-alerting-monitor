// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"

	am "github.com/open-edge-platform/o11y-alerting-monitor/internal/alertmanager"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/app"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/executor"
)

func validateLogLevel(value string) error {
	switch value {
	case "debug":
	case "info":
	case "warn":
	case "error":
	default:
		return fmt.Errorf("invalid log level %q", value)
	}
	return nil
}

func main() {
	configFile := flag.String("config", "", "config file path")
	apiPort := flag.Int("port", 8080, "API service port")
	logLevel := flag.String("log-level", "debug", "API server log level")

	flag.Parse()

	configuration, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	err = validateLogLevel(*logLevel)
	if err != nil {
		log.Fatal(err.Error())
	}

	alertManager, err := am.New(configuration.AlertManager)
	if err != nil {
		log.Fatalf("Failed to create alertmanager client: %v", err)
	}

	db, err := database.ConnectDB()
	if err != nil {
		log.Fatal(err.Error())
	}

	// Get pod uuid for executor
	podUUIDstring := os.Getenv("POD_UID")
	podUUID, err := uuid.Parse(podUUIDstring)
	if err != nil {
		log.Fatal(err.Error())
	}

	// TODO: Unify signal catching logic for both async executor and web server.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	aEx := executor.NewAsyncExecutor(podUUID, configuration, db, *logLevel, alertManager)
	aEx.Start(context.Background())

	app.StartServer(*apiPort, configuration, *logLevel, db)

	<-done
	aEx.Stop()
}
