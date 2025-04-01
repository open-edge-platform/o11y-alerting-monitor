// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"

	"github.com/open-edge-platform/o11y-alerting-monitor/api/v1"
	pb "github.com/open-edge-platform/o11y-alerting-monitor/api/v1/management"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/app"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/mimir"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

var (
	tenantIDNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9!_\-.*'()]+$`)
)

type server struct {
	pb.UnimplementedManagementServer

	rulesCfg rules.RulesConfig

	grpcServer *grpc.Server
	dbService  *database.DBService
	port       int
}

func main() {
	port := flag.Int("port", 51001, "gRPC server port")
	flag.Parse()

	rulesCfg, err := rules.LoadRulesConfig("/config/rules.yaml")
	if err != nil {
		log.Panicf("Failed to load alert definitions: %v", err)
	}

	dbConn, err := database.ConnectDB()
	if err != nil {
		log.Panic(err)
	}

	sqlDB, err := dbConn.DB()
	if err != nil {
		log.Panic(err)
	}
	defer func() {
		err := sqlDB.Close()
		if err != nil {
			log.Printf("Error appeared when closing database connection: %v", err)
		}
	}()

	s := server{
		rulesCfg:   *rulesCfg,
		grpcServer: grpc.NewServer(),
		dbService:  &database.DBService{DB: dbConn},
		port:       *port,
	}

	if err = s.initializeDefaults(context.Background(), app.DefaultTenantID); err != nil {
		log.Panicf("Failed to initialized defaults: %v", err)
	}
	log.Printf("Data for default tenant %q initialized successfully.", app.DefaultTenantID)

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(s.port))
	if err != nil {
		log.Panicf("Failed to listen: %v", err)
	}
	log.Printf("Server listening on :%v", s.port)

	// Register the health service
	healthCheck := health.NewServer()
	healthCheck.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s.grpcServer, healthCheck)

	pb.RegisterManagementServer(s.grpcServer, &s)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		<-ctx.Done()
		stop()

		log.Println("Got termination/interruption signal, attempting graceful shutdown.")
		stopped := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(stopped)
		}()

		dur := 5 * time.Second
		t := time.NewTimer(dur)
		select {
		case <-t.C:
			log.Printf("Graceful shutdown could not be completed within %q, attempting ungraceful shutdown.", dur)
			s.grpcServer.Stop()
		case <-stopped:
			t.Stop()
		}

		wg.Done()
	}()

	log.Println("Starting grpc server.")
	if err := s.grpcServer.Serve(lis); err != nil {
		log.Panicf("Failed to serve: %v", err)
	}
	wg.Wait()
	log.Println("Shutdown completed.")
}

// InitializeTenant ensures that all data for given tenant are in DB.
func (s *server) InitializeTenant(ctx context.Context, req *pb.TenantRequest) (*emptypb.Empty, error) {
	log.Printf("Received initialization request for tenant: %q", req.GetTenant())
	if err := validateTenantID(req.GetTenant()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	rowsAffected, err := s.initDataForTenant(ctx, req.GetTenant())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "initialization of tenant %q failed: %v", req.GetTenant(), err)
	}

	if rowsAffected == 0 {
		return nil, status.Errorf(codes.AlreadyExists, "tenant %q had already been initialized before", req.GetTenant())
	}
	return nil, nil
}

// CleanupTenant ensures that all existing data for given tenant are removed from DB.
func (s *server) CleanupTenant(ctx context.Context, req *pb.TenantRequest) (*emptypb.Empty, error) {
	log.Printf("Received cleanup request for tenant: %q", req.GetTenant())
	if err := validateTenantID(req.GetTenant()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	rowsAffected, err := s.cleanupDataForTenant(ctx, req.GetTenant())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cleanup of tenant %q failed: %v", req.GetTenant(), err)
	}

	if rowsAffected == 0 {
		return nil, status.Errorf(codes.NotFound, "tenant %q had already been cleanedup before", req.GetTenant())
	}
	return nil, nil
}

func (s *server) initializeDefaults(ctx context.Context, tenant string) error {
	err := s.initializeEmailCfg(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize email configuration: %w", err)
	}

	_, err = s.initDataForTenant(ctx, tenant)
	if err != nil {
		return fmt.Errorf("failed to initialize data for tenant %q: %w", tenant, err)
	}

	return nil
}

func (s *server) initializeEmailCfg(ctx context.Context) error {
	from := os.Getenv("FROM_MAIL")
	host := os.Getenv("SMART_HOST")
	port := os.Getenv("SMART_PORT")

	if from == "" || host == "" || port == "" {
		log.Println("SMTP configuration is not set, not initializing email configuration.")
		return nil
	}
	firstName, lastName, email, err := app.GetEmailSender(from)
	if err != nil {
		return err
	}

	tx := s.dbService.DB.Begin().WithContext(ctx)
	defer tx.Rollback()

	sender := models.EmailAddress{
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
	}
	if err := tx.Where(models.EmailAddress{
		FirstName: sender.FirstName,
		LastName:  sender.LastName,
		Email:     sender.Email,
	}).FirstOrCreate(&sender).Error; err != nil {
		return err
	}

	emailConfig := models.EmailConfig{
		MailServer: fmt.Sprintf("%s:%s", host, port),
		From:       sender.ID,
	}
	if err := tx.Where(models.EmailConfig{
		From: sender.ID,
	}).FirstOrCreate(&emailConfig).Error; err != nil {
		return err
	}

	return tx.Commit().Error
}

func (s *server) initDataForTenant(ctx context.Context, tenant string) (int64, error) {
	rowsAffected := int64(0)

	tx := s.dbService.DB.Begin().WithContext(ctx)
	defer tx.Rollback()

	for _, group := range s.rulesCfg.Groups {
		alertInterval, err := mimir.ParseDurationToSeconds(group.Interval)
		if err != nil {
			return rowsAffected, fmt.Errorf("failed to parse interval: %w", err)
		}
		for _, rule := range group.Rules {
			rows, err := insertAlertDefinition(tx, alertInterval, rule, tenant)
			if err != nil {
				return rowsAffected, fmt.Errorf("failed to insert Alert Definition %q: %w", rule.Alert, err)
			}
			rowsAffected += rows
		}
	}

	rows, err := insertEmailReceiver(tx, tenant)
	if err != nil {
		return rowsAffected, fmt.Errorf("failed to insert Email Receiver: %w", err)
	}
	rowsAffected += rows

	return rowsAffected, tx.Commit().Error
}

func insertAlertDefinition(tx *gorm.DB, interval int64, r rules.Rule, tenant string) (int64, error) {
	rowsAffected := int64(0)
	ruleUUID, err := uuid.Parse(r.Annotations["am_uuid"])
	if err != nil {
		return rowsAffected, fmt.Errorf("invalid UUID %q for alert %q: %w", r.Annotations["am_uuid"], r.Alert, err)
	}

	template, err := r.ConstructTemplate()
	if err != nil {
		return rowsAffected, fmt.Errorf("failed to construct template: %w", err)
	}

	ad := &models.AlertDefinition{
		Enabled:       true,
		UUID:          ruleUUID,
		Version:       1,
		Name:          r.Alert,
		State:         models.DefinitionNew,
		Template:      template,
		Category:      models.AlertDefinitionCategory(r.Labels["alert_category"]),
		Context:       r.Labels["alert_context"],
		AlertInterval: interval,
		TenantID:      tenant,
	}

	res := tx.Where(models.AlertDefinition{
		UUID:     ad.UUID,
		Version:  1,
		TenantID: ad.TenantID,
	}).FirstOrCreate(&ad)
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	threshold, err := strconv.ParseInt(r.Annotations["am_threshold"], 10, 64)
	if err != nil {
		return rowsAffected, err
	}
	thresholdMin, err := strconv.ParseInt(r.Annotations["am_threshold_min"], 10, 64)
	if err != nil {
		return rowsAffected, err
	}
	thresholdMax, err := strconv.ParseInt(r.Annotations["am_threshold_max"], 10, 64)
	if err != nil {
		return rowsAffected, err
	}

	at := &models.AlertThreshold{
		Name:              "Threshold",
		Threshold:         threshold,
		ThresholdMin:      thresholdMin,
		ThresholdMax:      thresholdMax,
		ThresholdType:     r.Annotations["am_definition_type"],
		ThresholdUnit:     r.Annotations["am_threshold_unit"],
		AlertDefinitionID: ad.ID,
	}
	res = tx.Where(models.AlertThreshold{
		AlertDefinitionID: ad.ID,
		Name:              at.Name,
	}).FirstOrCreate(&at)
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	duration, err := mimir.ParseDurationToSeconds(r.Annotations["am_duration"])
	if err != nil {
		return rowsAffected, err
	}
	durationMin, err := mimir.ParseDurationToSeconds(r.Annotations["am_duration_min"])
	if err != nil {
		return rowsAffected, err
	}
	durationMax, err := mimir.ParseDurationToSeconds(r.Annotations["am_duration_max"])
	if err != nil {
		return rowsAffected, err
	}

	aDur := &models.AlertDuration{
		Name:              "Duration",
		Duration:          duration,
		DurationMin:       durationMin,
		DurationMax:       durationMax,
		AlertDefinitionID: ad.ID,
	}
	res = tx.Where(models.AlertDuration{
		AlertDefinitionID: ad.ID,
		Name:              aDur.Name,
	}).FirstOrCreate(&aDur)
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	task := models.Task{
		State:               models.TaskNew,
		AlertDefinitionUUID: &ad.UUID,
		TenantID:            ad.TenantID,
		Version:             1,
		CreationDate:        time.Now(),
	}

	res = tx.Where(models.Task{
		AlertDefinitionUUID: &ad.UUID,
		Version:             1,
		TenantID:            ad.TenantID,
	}).FirstOrCreate(&task)
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	return rowsAffected, nil
}

func insertEmailReceiver(tx *gorm.DB, tenant string) (int64, error) {
	rowsAffected := int64(0)

	from := os.Getenv("FROM_MAIL")
	host := os.Getenv("SMART_HOST")
	port := os.Getenv("SMART_PORT")

	if from == "" || host == "" || port == "" {
		log.Println("SMTP configuration is not set, not setting email receiver.")
		return rowsAffected, nil
	}

	var emailConfig models.EmailConfig
	if err := tx.Last(&emailConfig).Error; err != nil {
		return rowsAffected, fmt.Errorf("failed to query email config: %w", err)
	}

	// Create new UUID, so we don't have to store it anywhere,
	// and base the uniqueness on name and version.
	recvUUID := uuid.New()
	recv := models.Receiver{
		UUID:          recvUUID,
		Name:          "alert-monitor-config",
		State:         "New",
		Version:       1,
		EmailConfigID: emailConfig.ID,
		TenantID:      tenant,
	}
	res := tx.Where(models.Receiver{
		Version:  recv.Version,
		Name:     recv.Name,
		TenantID: recv.TenantID,
	}).FirstOrCreate(&recv)
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	task := models.Task{
		State:        models.TaskNew,
		ReceiverUUID: &recv.UUID,
		TenantID:     recv.TenantID,
		Version:      1,
		CreationDate: time.Now(),
	}
	res = tx.Where(models.Task{
		ReceiverUUID: &recv.UUID,
		TenantID:     recv.TenantID,
		Version:      1,
	}).FirstOrCreate(&task)
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	return rowsAffected, nil
}

func (s *server) cleanupDataForTenant(ctx context.Context, tenant string) (int64, error) {
	rowsAffected := int64(0)

	tx := s.dbService.DB.Begin().WithContext(ctx)
	defer tx.Rollback()

	// First, let's find all AlertDefinitions IDs associated with the given tenant, they will be needed for subsequent operations.
	var alertDefinitionIDs []int64
	if err := tx.Model(&models.AlertDefinition{}).Where("tenant_id = ?", tenant).Pluck("ID", &alertDefinitionIDs).Error; err != nil {
		return 0, fmt.Errorf("failed to get alert definition IDs for tenant %q: %w", tenant, err)
	}

	// Delete all Tasks with the given tenant.
	res := tx.Where("tenant_id = ?", tenant).Delete(&models.Task{})
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	// Delete all Receivers with the given tenant.
	res = tx.Where("tenant_id = ?", tenant).Delete(&models.Receiver{})
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	// Delete all AlertDurations associated (by id) with the previously found AlertDefinitions.
	res = tx.Where("alert_definition_id IN ?", alertDefinitionIDs).Delete(&models.AlertDuration{})
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	// Delete all AlertThresholds associated (by id) with the previously found AlertDefinitions.
	res = tx.Where("alert_definition_id IN ?", alertDefinitionIDs).Delete(&models.AlertThreshold{})
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	// Delete all AlertDefinitions by previously found IDs.
	res = tx.Where("tenant_id = ?", tenant).Delete(&models.AlertDefinition{})
	if res.Error != nil {
		return rowsAffected, res.Error
	}
	rowsAffected += res.RowsAffected

	return rowsAffected, tx.Commit().Error
}

// validateTenantID validates the tenantID based on Mimir restrictions.
// https://grafana.com/docs/mimir/latest/configure/about-tenant-ids/
func validateTenantID(tenantID api.TenantID) error {
	// Check if tenantID is empty.
	if len(strings.TrimSpace(tenantID)) == 0 {
		return errors.New("tenantID cannot be empty")
	}

	// tenantID must be <= 150 bytes or characters in length.
	if len(tenantID) > 150 {
		return errors.New("tenantID exceeds 150 characters")
	}

	// Forbidden tenantID patterns.
	if tenantID == "." || tenantID == ".." || tenantID == "__mimir_cluster" {
		return errors.New("tenantID cannot be '.' or '..' or '__mimir_cluster'")
	}

	// Only allowed characters: Alphanumeric + special characters defined.
	matches := tenantIDNameRegexp.MatchString(tenantID)
	if !matches {
		return errors.New("tenantID contains unsupported characters")
	}

	return nil
}
