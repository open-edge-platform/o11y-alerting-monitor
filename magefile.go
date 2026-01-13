// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

//go:build mage

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"

	"github.com/open-edge-platform/o11y-alerting-monitor/tools/parser"
)

type (
	// Lint is the Mage namespace for linting targets.
	Lint mg.Namespace

	// Gen is the Mage namespace for generation targets.
	Gen mg.Namespace

	// Test is the Mage namespace for testing targets.
	Test mg.Namespace

	// Migrate is the Mage namespace for database migration targets.
	Migrate mg.Namespace
)

var (
	licenseCommonArgs = []string{
		"--copyright-style=spdx-c",
		"--copyright=Intel Corporation",
		"--license=Apache-2.0",
		"--template=intel",
		"--skip-unrecognised",
		"--merge-copyrights",
	}

	skipLicenseDirs = []string{
		".git",
		".reuse",
		"api/boilerplate",
		"LICENSES",
	}

	anyFileRegex = regexp.MustCompile(".*")
)

// Ensures all files have copyright and license set.
func (Lint) License() error {
	return sh.Run("reuse", "lint")
}

// Generates copyright and license headers.
func (Gen) License() error {
	f, err := os.Open(".reuse/dep5")
	if err != nil {
		return err
	}
	defer f.Close()

	p := parser.NewParser(bufio.NewReader(f))

	// Ensures one header stanza and one files stanza.
	dep5File, err := p.Parse()
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	excludedFiles := make([]string, 0) //nolint:prealloc // Keep current configuration
	patterns := dep5File.Files[0].Files
	for _, p := range patterns {
		p = strings.ReplaceAll(p, ".", "\\.")
		p = strings.ReplaceAll(p, "*", ".*")
		p = strings.ReplaceAll(p, "?", ".")
		r, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("error occurred: %w", err)
		}

		matches, err := find(".", r, skipLicenseDirs)
		if err != nil {
			return fmt.Errorf("failed to find exluded files: %w", err)
		}

		excludedFiles = append(excludedFiles, matches...)
	}

	files, err := find(".", anyFileRegex, skipLicenseDirs)
	if err != nil {
		return fmt.Errorf("failed to find included files: %w", err)
	}

	files = slices.DeleteFunc(files, func(f string) bool {
		return slices.Contains(excludedFiles, f)
	})

	args := []string{"annotate"} //nolint:prealloc // Keep current configuration
	args = append(args, licenseCommonArgs...)
	args = append(args, files...)
	return sh.Run("reuse", args...)
}

func find(dir string, re *regexp.Regexp, skipDirs []string) ([]string, error) {
	if re == nil {
		return nil, errors.New("no regex was provided")
	}

	found := make([]string, 0)
	if err := filepath.WalkDir(dir, func(fpath string, d fs.DirEntry, _ error) error {
		if d.IsDir() && slices.Contains(skipDirs, d.Name()) {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			if re.MatchString(fpath) {
				found = append(found, fpath)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to walk path %q: %w", dir, err)
	}

	return found, nil
}

// Runs fuzz tests.
func (Test) Fuzz(fuzzMinutes string) error {
	fuzzTestList := []string{
		"FuzzPatchAlertDefinitionRandomInput",
		"FuzzPatchAlertDefinitionDuration",
		"FuzzPatchAlertDefinitionEnabled",
		"FuzzPatchAlertDefinitionThreshold",
		"FuzzPatchAlertDefinitionAllInputs",
		"FuzzPatchAlertReceiverRandomInput",
		"FuzzPatchAlertReceiverAddress",
	}

	outputDir := filepath.Join("internal", "app", "fuzz-output")

	// Create the directory if it doesn't exist
	err := os.MkdirAll(outputDir, 0750)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fuzzSeconds, err := parseMinutesToSeconds(fuzzMinutes)
	if err != nil {
		return err
	}

	for _, fuzzTest := range fuzzTestList {
		outputFile := filepath.Join(outputDir, "fuzz_output.txt")
		cmd := fmt.Sprintf("nohup go test ./internal/app/ -fuzz=%s -run=%s -fuzztime=%ds >> %s 2>&1 &", fuzzTest, fuzzTest, fuzzSeconds, outputFile)
		fmt.Println("Running command:", cmd)

		err := sh.Run("sh", "-c", cmd)
		if err != nil {
			return err
		}
	}
	return nil
}

// parseMinutesToSeconds converts a duration in minutes to seconds.
func parseMinutesToSeconds(minutes string) (int, error) {
	if minutes == "" {
		return 60, nil
	}

	minValue, err := strconv.Atoi(minutes)
	if err != nil {
		return 0, fmt.Errorf("invalid minutes format: %w", err)
	}

	return minValue * 60, nil
}

func createDiff(name string) (string, error) {
	originalDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			log.Printf("failed to change back to original directory: %v", err)
		}
	}()

	err = os.Chdir("deployments/alerting-monitor/files/atlas")
	if err != nil {
		return "", fmt.Errorf("failed to change to atlas migrate files directory: %w", err)
	}

	out, err := exec.Command("atlas", "migrate", "diff", "--env", "local", name).Output()
	if err != nil {
		return "", fmt.Errorf("failed to run atlas command, out: %v: %w", out, err)
	}

	return string(out), nil
}

// Creates migration files with atlas.
func (Migrate) Schema(name string) error {
	migrationName := "updated"
	if len(name) > 0 {
		migrationName = name
	}
	_, err := createDiff(migrationName)
	return err
}

// Validates whether migration files match the desired database schema.
func (Migrate) Verify() error {
	out, err := createDiff("updated")
	if err != nil {
		return err
	}

	// If migration files are valid, there should be a stdout from atlas.
	if out == "" {
		return errors.New("migration files do not match the desired database schema")
	}

	return nil
}
