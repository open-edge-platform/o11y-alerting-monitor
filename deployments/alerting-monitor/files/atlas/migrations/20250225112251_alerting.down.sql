-- SPDX-FileCopyrightText: (C) 2025 Intel Corporation
-- SPDX-License-Identifier: Apache-2.0

-- reverse: create "tasks" table
DROP TABLE "public"."tasks";
-- reverse: create "email_recipients" table
DROP TABLE "public"."email_recipients";
-- reverse: create "receivers" table
DROP TABLE "public"."receivers";
-- reverse: create "email_configs" table
DROP TABLE "public"."email_configs";
-- reverse: create "email_addresses" table
DROP TABLE "public"."email_addresses";
-- reverse: create "alert_thresholds" table
DROP TABLE "public"."alert_thresholds";
-- reverse: create "alert_durations" table
DROP TABLE "public"."alert_durations";
-- reverse: create "alert_definitions" table
DROP TABLE "public"."alert_definitions";
-- reverse: create enum type "task_state"
DROP TYPE "public"."task_state";
-- reverse: create enum type "receiver_state"
DROP TYPE "public"."receiver_state";
-- reverse: create enum type "alert_definition_state"
DROP TYPE "public"."alert_definition_state";
