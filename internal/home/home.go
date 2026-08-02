/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package home resolves the single active data directory of informer and
// migrates the legacy executable-directory layout into it.
package home

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vogo/informer/internal"
)

const (
	// EnvHome is the environment variable overriding the active data directory.
	EnvHome = "INFORMER_HOME"

	// DefaultDirName is the directory used under the user home dir when EnvHome is unset or empty.
	DefaultDirName = ".informer"

	// MigratedMarker marks a finished legacy migration so that it never runs a second time.
	MigratedMarker = ".migrated"

	dirPermission = 0o700
	probeFileName = ".informer-write-probe"
)

// legacyEntries are the application data entries copied from the legacy executable directory.
// The list is explicit on purpose: the executable may live in a shared location such as
// /usr/local/bin or $GOBIN, where copying the whole directory would drag in unrelated files.
//
//nolint:gochecknoglobals //ignore this.
var legacyEntries = []string{
	"informer.json",
	"feed.db",
	"feed.db-wal",
	"feed.db-shm",
	"foodorder.db",
	"foodorder.db-wal",
	"foodorder.db-shm",
	"previous_chosen.json",
	"feed_data.json",
	"data",
}

// Resolve returns the active data directory without creating it.
// INFORMER_HOME wins when it is set to a non-blank value, otherwise the
// directory is <user home>/.informer. It never falls back to a second location.
func Resolve() (string, error) {
	if env := strings.TrimSpace(os.Getenv(EnvHome)); env != "" {
		dir, err := filepath.Abs(filepath.Clean(env))
		if err != nil {
			return "", fmt.Errorf("resolve %s %q: %w", EnvHome, env, err)
		}

		return dir, nil
	}

	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}

	return filepath.Join(userHome, DefaultDirName), nil
}

// Init resolves the active data directory, makes sure it exists and is writable,
// and copies the legacy layout from legacyDir on the first successful run.
// Every failure is returned so that the caller can abort before touching business data.
func Init(legacyDir string) (string, error) {
	dir, err := Resolve()
	if err != nil {
		return "", err
	}

	if err = os.MkdirAll(dir, dirPermission); err != nil {
		return "", fmt.Errorf("create informer home %q: %w", dir, err)
	}

	if err = ensureWritable(dir); err != nil {
		return "", err
	}

	if err = Migrate(legacyDir, dir); err != nil {
		return "", err
	}

	return dir, nil
}

// Migrate copies the legacy application data from legacyDir into activeDir.
// Source files are never removed, existing targets are always kept and skipped,
// and a completed migration is recorded so repeated startups are a no-op.
func Migrate(legacyDir, activeDir string) error {
	if legacyDir == "" {
		return nil
	}

	legacy, err := filepath.Abs(legacyDir)
	if err != nil {
		return fmt.Errorf("resolve legacy dir %q: %w", legacyDir, err)
	}

	if legacy == activeDir {
		return nil
	}

	marker := filepath.Join(activeDir, MigratedMarker)
	if _, statErr := os.Stat(marker); statErr == nil {
		return nil
	}

	if !isLegacyDataDir(legacy) {
		return nil
	}

	for _, name := range legacyEntries {
		if err = copyEntry(filepath.Join(legacy, name), filepath.Join(activeDir, name)); err != nil {
			return err
		}
	}

	if err = os.WriteFile(marker, []byte(legacy+"\n"), internal.DefaultDataFilePermission); err != nil {
		return fmt.Errorf("write migration marker %q: %w", marker, err)
	}

	return nil
}

// isLegacyDataDir reports whether the directory actually holds informer data,
// so that an executable placed in an unrelated directory triggers no migration.
func isLegacyDataDir(dir string) bool {
	for _, name := range []string{"informer.json", "feed.db", "foodorder.db", "data"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}

	return false
}

func copyEntry(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("stat legacy entry %q: %w", src, err)
	}

	switch {
	case info.IsDir():
		return copyDir(src, dst)
	case info.Mode().IsRegular():
		return copyFile(src, dst, info.Mode())
	default:
		// symlinks, sockets and devices are not informer data.
		return nil
	}
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, dirPermission); err != nil {
		return fmt.Errorf("create directory %q: %w", dst, err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read legacy directory %q: %w", src, err)
	}

	for _, entry := range entries {
		if err = copyEntry(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if _, err := os.Lstat(dst); err == nil {
		// keep whatever the user already has in the active directory.
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target %q: %w", dst, err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open legacy file %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return fmt.Errorf("create target file %q: %w", dst, err)
	}

	if _, err = io.Copy(out, in); err != nil {
		out.Close()

		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}

	if err = out.Close(); err != nil {
		return fmt.Errorf("close target file %q: %w", dst, err)
	}

	return nil
}

func ensureWritable(dir string) error {
	probe := filepath.Join(dir, probeFileName)

	file, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, internal.DefaultDataFilePermission)
	if err != nil {
		return fmt.Errorf("informer home %q is not writable: %w", dir, err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("informer home %q is not writable: %w", dir, err)
	}

	if err = os.Remove(probe); err != nil {
		return fmt.Errorf("informer home %q is not writable: %w", dir, err)
	}

	return nil
}
