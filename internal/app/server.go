// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/config"
)

var logger *slog.Logger

func StartServer(port int, conf config.Config, logLvl string, db *gorm.DB) {
	// Creating new Echo server
	e := echo.New()

	// Create a custom logger using slog
	opts := setLogLvl(e, logLvl)
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &opts))

	// Set slog logger as the default logger for Echo to use the same logger configuration without explicitly passing the logger instance around.
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	vault, err := newVault(conf.Vault)
	if err != nil {
		e.Logger.Panic(err)
	}
	go func() {
		if err = vault.renewToken(ctx, conf.Vault); err != nil && !errors.Is(err, context.Canceled) {
			e.Logger.Panic("Failed to renew token", slog.Any("error", err))
		}
	}()

	m2m, err := NewM2MAuthenticator(conf, vault)
	if err != nil {
		e.Logger.Panic(err)
	}

	serverInterface := NewServerInterfaceHandler(conf, db, m2m)

	sqlDB, err := db.DB()
	if err != nil {
		e.Logger.Panic(err)
	}
	defer sqlDB.Close()

	// Registering API call handlers
	api.RegisterHandlers(e, serverInterface)
	authenticationHandler := NewAuthenticationHandler(conf.Authentication.OidcServer, conf.Authentication.OidcServerRealm)

	// Midd
	e.Use(authorize)
	e.Use(authenticationHandler.authenticate)
	e.Use(middleware.Recover())
	// Use middleware to log requests with the custom logger
	e.Use(middleware.RequestLoggerWithConfig(
		middleware.RequestLoggerConfig{
			// NOTE: skipping GET requests from curl/kube-probe to /edgenode/api/v1/status
			// in order to not log incoming readiness/liveness probes requests
			Skipper:      skipLog,
			LogURI:       true,
			LogStatus:    true,
			LogError:     true,
			LogUserAgent: true,
			LogMethod:    true,
			LogValuesFunc: func(_ echo.Context, v middleware.RequestLoggerValues) error {
				if v.Error != nil {
					logger.LogAttrs(context.Background(), slog.LevelError, "REQUEST_ERROR",
						slog.String("uri", v.URI),
						slog.Int("status", v.Status),
						slog.String("user-agent", v.UserAgent),
						slog.String("method", v.Method),
						slog.String("error", v.Error.Error()),
					)
				} else {
					logger.LogAttrs(context.Background(), slog.LevelInfo, "REQUEST",
						slog.String("uri", v.URI),
						slog.Int("status", v.Status),
						slog.String("user-agent", v.UserAgent),
						slog.String("method", v.Method),
					)
				}
				return nil
			},
		},
	))

	// Print welcome message in logs
	welcomeMessage(e, conf, logLvl, port)

	// Start server
	go func() {
		if err := e.Start(fmt.Sprintf(":%v", port)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			e.Logger.Panic("Server shutdown")
		}
	}()

	// Graceful shutdown in 5 seconds after interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelTimeout()
	if err := e.Shutdown(ctxTimeout); err != nil {
		e.Logger.Panic(err)
	}
}

func setLogLvl(e *echo.Echo, logLvl string) slog.HandlerOptions {
	switch logLvl {
	case "debug":
		e.Logger.SetLevel(log.DEBUG)
		return slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
	case "info":
		e.Logger.SetLevel(log.INFO)
		return slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
	case "warn":
		e.Logger.SetLevel(log.WARN)
		return slog.HandlerOptions{
			Level: slog.LevelWarn,
		}
	case "error":
		e.Logger.SetLevel(log.ERROR)
		return slog.HandlerOptions{
			Level: slog.LevelError,
		}
	default:
		e.Logger.SetLevel(log.INFO)
		return slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
	}
}

func welcomeMessage(e *echo.Echo, cfg config.Config, logLvl string, port int) {
	e.HidePort = true
	e.HideBanner = true
	fmt.Println("Alerting Monitor")
	fmt.Printf("⇨ Log level: %s\n", logLvl)
	fmt.Printf("⇨ HTTP server port: %d\n", port)
	fmt.Println("Configuration:")
	printStruct("Alert Manager", cfg.AlertManager)
	printStruct("Auth", cfg.Authentication)
	printStruct("Keycloak", cfg.Keycloak)
	printStruct("Mimir", cfg.Mimir)
}

func printStruct(header string, obj any) {
	fmt.Printf("⇨ %s:\n", header)
	vals := reflect.ValueOf(obj)
	types := vals.Type()
	for i := 0; i < vals.NumField(); i++ {
		fmt.Printf("  %s: %v\n", types.Field(i).Name, vals.Field(i))
	}
}
