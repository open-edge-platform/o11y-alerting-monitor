// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	pb "github.com/open-edge-platform/o11y-alerting-monitor/api/v1/management"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/database/models"
	"github.com/open-edge-platform/o11y-alerting-monitor/internal/rules"
)

const (
	dbQueryTimeout                            = 5 * time.Second
	expectedNumberOfAlertDefinitionsPerTenant = 18
	expectedNumberOfTasksPerTenant            = 19
)

type management struct {
	s      *server
	client pb.ManagementClient
	closer func()
}

var mgmt *management

var _ = Describe("Management", Ordered, func() {
	BeforeAll(func() {
		dbConn, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"))
		Expect(err).ToNot(HaveOccurred())

		rulesCfg, err := rules.LoadRulesConfig("testdata/rules.yaml")
		Expect(err).ToNot(HaveOccurred())

		lis := bufconn.Listen(1024 * 1024)
		s := &server{
			rulesCfg:   *rulesCfg,
			grpcServer: grpc.NewServer(),
			dbService:  &database.DBService{DB: dbConn},
		}

		pb.RegisterManagementServer(s.grpcServer, s)
		go func() {
			if err := s.grpcServer.Serve(lis); err != nil {
				log.Printf("Error serving server: %v", err)
			}
		}()

		conn, err := grpc.NewClient(
			"passthrough://bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return lis.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		Expect(err).ToNot(HaveOccurred())

		closer := func() {
			Expect(lis.Close()).To(Succeed())

			s.grpcServer.Stop()

			dbConn, err := s.dbService.DB.DB()
			Expect(err).ToNot(HaveOccurred())
			Expect(dbConn.Close()).To(Succeed())
		}
		client := pb.NewManagementClient(conn)

		mgmt = &management{
			s:      s,
			client: client,
			closer: closer,
		}

		Expect(mgmt.s.dbService.DB.AutoMigrate(
			&models.AlertDefinition{},
			&models.AlertThreshold{},
			&models.AlertDuration{},
			&models.Task{},
			&models.EmailAddress{},
			&models.EmailConfig{},
			&models.Receiver{},
		)).ShouldNot(HaveOccurred())

		GinkgoT().Setenv("FROM_MAIL", "Foo Bar <foo@bar.com>")
		GinkgoT().Setenv("SMART_HOST", "mailpit-svc.orch-infra.svc.cluster.local")
		GinkgoT().Setenv("SMART_PORT", "1025")
	})

	AfterAll(func() {
		if mgmt == nil {
			return
		}
		mgmt.closer()
	})

	It("Tables should empty at the beginning", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeZero())

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeZero())

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeZero())

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeZero())

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeZero())

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeZero())

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeZero())
	})

	It("Initialize defaults - tables should be initialized", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		err := mgmt.s.initializeDefaults(ctx, "default")
		Expect(err).ShouldNot(HaveOccurred())

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))
	})

	It("Initialize defaults with existing tenant - no new rows should be added to tables", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		err := mgmt.s.initializeDefaults(ctx, "default")
		Expect(err).ShouldNot(HaveOccurred())

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))
	})

	It("Create new tenant with valid name using gRPC endpoint - new rows should be added to tables", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		_, err := mgmt.client.InitializeTenant(ctx, &pb.TenantRequest{Tenant: "grpc_tenant"})
		Expect(err).ShouldNot(HaveOccurred())

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2))
	})

	It("Create new tenant with invalid name using gRPC endpoint - error should appear and no new rows should be added to tables", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		_, err := mgmt.client.InitializeTenant(ctx, &pb.TenantRequest{Tenant: " "})
		Expect(err).Should(HaveOccurred())
		Expect(err).To(MatchError(func(err error) bool {
			return status.Code(err) == codes.InvalidArgument
		}, "InvalidArgument"))

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2))
	})

	It("Create new tenant with existing name using gRPC endpoint - error should appear and no new rows should be added to tables", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		_, err := mgmt.client.InitializeTenant(ctx, &pb.TenantRequest{Tenant: "grpc_tenant"})
		Expect(err).To(MatchError(func(err error) bool {
			return status.Code(err) == codes.AlreadyExists
		}, "AlreadyExists"))

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2 * expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(2))
	})

	It("Cleanup existing tenant using gRPC endpoint - rows should be removed from tables", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		_, err := mgmt.client.CleanupTenant(ctx, &pb.TenantRequest{Tenant: "grpc_tenant"})
		Expect(err).ShouldNot(HaveOccurred())

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))
	})

	It("Cleanup tenant with invalid name using gRPC endpoint - error should appear and no rows should be removed from tables", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		_, err := mgmt.client.CleanupTenant(ctx, &pb.TenantRequest{Tenant: " "})
		Expect(err).To(MatchError(func(err error) bool {
			return status.Code(err) == codes.InvalidArgument
		}, "InvalidArgument"))

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))
	})

	It("Cleanup tenant with not existing name using gRPC endpoint - error should appear and no rows should be removed from tables", func() {
		ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
		defer cancel()

		_, err := mgmt.client.CleanupTenant(ctx, &pb.TenantRequest{Tenant: "does_not_exist"})
		Expect(err).To(MatchError(func(err error) bool {
			return status.Code(err) == codes.NotFound
		}, "NotFound"))

		count, err := mgmt.count(ctx, models.AlertDefinition{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertThreshold{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.AlertDuration{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfAlertDefinitionsPerTenant))

		count, err = mgmt.count(ctx, models.Task{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(expectedNumberOfTasksPerTenant))

		count, err = mgmt.count(ctx, models.EmailAddress{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.EmailConfig{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))

		count, err = mgmt.count(ctx, models.Receiver{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).To(BeEquivalentTo(1))
	})
})

func (m *management) count(ctx context.Context, dbType interface{}) (int64, error) {
	var count int64

	tx := m.s.dbService.DB.WithContext(ctx).Begin()
	defer tx.Rollback()

	if err := tx.Model(dbType).Count(&count).Error; err != nil {
		return 0, err
	}

	return count, tx.Commit().Error
}
