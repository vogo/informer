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

package home_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/home"
)

// setUserHome points os.UserHomeDir at dir on every supported platform.
func setUserHome(t *testing.T, dir string) {
	t.Helper()

	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestResolveUsesUserHomeWhenEnvUnset(t *testing.T) {
	userHome := t.TempDir()
	setUserHome(t, userHome)
	t.Setenv(home.EnvHome, "")
	require.NoError(t, os.Unsetenv(home.EnvHome))

	dir, err := home.Resolve()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(userHome, home.DefaultDirName), dir)
}

func TestResolveUsesUserHomeWhenEnvBlank(t *testing.T) {
	userHome := t.TempDir()
	setUserHome(t, userHome)
	t.Setenv(home.EnvHome, "   ")

	dir, err := home.Resolve()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(userHome, home.DefaultDirName), dir)
}

func TestResolveUsesEnvWhenSet(t *testing.T) {
	setUserHome(t, t.TempDir())

	custom := t.TempDir()
	t.Setenv(home.EnvHome, filepath.Join(custom, "sub", ".."))

	dir, err := home.Resolve()
	require.NoError(t, err)
	assert.Equal(t, custom, dir)
}

func TestResolveMakesRelativeEnvAbsolute(t *testing.T) {
	setUserHome(t, t.TempDir())
	t.Setenv(home.EnvHome, "relative-informer-home")

	dir, err := home.Resolve()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(dir), "expected an absolute path, got %q", dir)
}

func TestResolveFailsWithoutUserHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.UserHomeDir on windows reads USERPROFILE differently")
	}

	require.NoError(t, os.Unsetenv(home.EnvHome))
	t.Setenv("HOME", "")

	_, err := home.Resolve()
	require.Error(t, err)
}

func TestInitCreatesDirectory(t *testing.T) {
	setUserHome(t, t.TempDir())

	target := filepath.Join(t.TempDir(), "nested", "informer")
	t.Setenv(home.EnvHome, target)

	dir, err := home.Init("")
	require.NoError(t, err)
	assert.Equal(t, target, dir)

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestInitFailsOnUnusableDirectory(t *testing.T) {
	setUserHome(t, t.TempDir())

	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	// a regular file on the path makes the directory impossible to create,
	// and Init must report it instead of silently using another location.
	t.Setenv(home.EnvHome, filepath.Join(blocker, "informer"))

	_, err := home.Init("")
	require.Error(t, err)
}

func TestInitFailsOnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	setUserHome(t, t.TempDir())

	target := filepath.Join(t.TempDir(), "readonly")
	require.NoError(t, os.MkdirAll(target, 0o500))

	t.Setenv(home.EnvHome, target)

	_, err := home.Init("")
	require.ErrorContains(t, err, "not writable")
}

// legacyLayout builds an executable directory holding the historical file layout.
func legacyLayout(t *testing.T) string {
	t.Helper()

	legacy := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "informer.json"), []byte(`{"feed":{}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "feed.db"), []byte("legacy-db"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "feed_data.json"), []byte("legacy-json"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(legacy, "data", "2024"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "data", "2024", "2024-01-02.md"), []byte("old day"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(legacy, "data", "2025"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "data", "2025", "2025-06-01.md"), []byte("new day"), 0o600))

	// the executable itself must never be copied into the data directory.
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "informer"), []byte("binary"), 0o700))

	return legacy
}

func TestInitCopiesLegacyLayout(t *testing.T) {
	setUserHome(t, t.TempDir())

	legacy := legacyLayout(t)
	target := filepath.Join(t.TempDir(), "informer-home")
	t.Setenv(home.EnvHome, target)

	dir, err := home.Init(legacy)
	require.NoError(t, err)
	require.Equal(t, target, dir)

	for name, want := range map[string]string{
		"informer.json":           `{"feed":{}}`,
		"feed.db":                 "legacy-db",
		"feed_data.json":          "legacy-json",
		"data/2024/2024-01-02.md": "old day",
		"data/2025/2025-06-01.md": "new day",
	} {
		got, readErr := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		require.NoError(t, readErr, name)
		assert.Equal(t, want, string(got), name)
	}

	// the source files are kept where they are.
	_, err = os.Stat(filepath.Join(legacy, "feed.db"))
	require.NoError(t, err)

	// unrelated files next to the executable stay out of the data directory.
	_, err = os.Stat(filepath.Join(target, "informer"))
	assert.True(t, os.IsNotExist(err), "the executable must not be copied")
}

func TestInitKeepsExistingTargetFiles(t *testing.T) {
	setUserHome(t, t.TempDir())

	legacy := legacyLayout(t)
	target := filepath.Join(t.TempDir(), "informer-home")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "data", "2024"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(target, "feed.db"), []byte("already migrated"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(target, "data", "2024", "2024-01-02.md"), []byte("edited"), 0o600))

	t.Setenv(home.EnvHome, target)

	_, err := home.Init(legacy)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(target, "feed.db"))
	require.NoError(t, err)
	assert.Equal(t, "already migrated", string(got), "an existing target file must be kept")

	got, err = os.ReadFile(filepath.Join(target, "data", "2024", "2024-01-02.md"))
	require.NoError(t, err)
	assert.Equal(t, "edited", string(got), "an existing nested target file must be kept")

	// the entries that were missing are still copied.
	got, err = os.ReadFile(filepath.Join(target, "data", "2025", "2025-06-01.md"))
	require.NoError(t, err)
	assert.Equal(t, "new day", string(got))
}

func TestInitIsIdempotent(t *testing.T) {
	setUserHome(t, t.TempDir())

	legacy := legacyLayout(t)
	target := filepath.Join(t.TempDir(), "informer-home")
	t.Setenv(home.EnvHome, target)

	_, err := home.Init(legacy)
	require.NoError(t, err)

	// a change made after the migration survives every later startup.
	dbPath := filepath.Join(target, "feed.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("changed after migration"), 0o600))

	_, err = home.Init(legacy)
	require.NoError(t, err)

	_, err = home.Init(legacy)
	require.NoError(t, err)

	got, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	assert.Equal(t, "changed after migration", string(got))

	_, err = os.Stat(filepath.Join(target, home.MigratedMarker))
	require.NoError(t, err, "a finished migration is recorded")
}

func TestInitSkipsUnrelatedExecutableDirectory(t *testing.T) {
	setUserHome(t, t.TempDir())

	// an executable installed into a shared bin directory holds no informer data.
	legacy := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "some-other-tool"), []byte("x"), 0o700))

	target := filepath.Join(t.TempDir(), "informer-home")
	t.Setenv(home.EnvHome, target)

	_, err := home.Init(legacy)
	require.NoError(t, err)

	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing may be copied from an unrelated directory")
}

func TestMigrateSkipsWhenLegacyEqualsActive(t *testing.T) {
	legacy := legacyLayout(t)

	require.NoError(t, home.Migrate(legacy, legacy))

	_, err := os.Stat(filepath.Join(legacy, home.MigratedMarker))
	assert.True(t, os.IsNotExist(err), "no marker is written when both directories are the same")
}
