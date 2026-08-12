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

package configstore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/configstore"
)

// Keys the fixtures below reuse.
const (
	feedKey   = "feed"
	futureKey = "future_section"
	noteKey   = "note"
)

func TestLoadMissingFileIsEmptyDocument(t *testing.T) {
	t.Parallel()

	doc, err := configstore.Load(filepath.Join(t.TempDir(), "informer.json"))
	require.NoError(t, err)

	assert.False(t, doc.Exists())
	assert.Empty(t, doc.Keys())
}

func TestDocKeepsUnknownFieldsAndOrder(t *testing.T) {
	t.Parallel()

	path := writeFile(t, `{
  "note": "kept by hand",
  "feed": {"max_fetch_num": 1},
  "future_section": {"nested": [1, 2]}
}`)

	doc, err := configstore.Load(path)
	require.NoError(t, err)
	assert.True(t, doc.Exists())
	assert.Equal(t, []string{noteKey, feedKey, futureKey}, doc.Keys())

	require.NoError(t, doc.Set(feedKey, map[string]int{"max_fetch_num": 3}))

	data, err := doc.Bytes()
	require.NoError(t, err)
	require.NoError(t, configstore.WriteAtomic(path, data, configstore.PermConfig))

	reloaded, err := configstore.Load(path)
	require.NoError(t, err)

	// the edited section changed, every other field survived in place.
	assert.Equal(t, []string{noteKey, feedKey, futureKey}, reloaded.Keys())

	var note string

	found, err := reloaded.Unmarshal(noteKey, &note)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "kept by hand", note)

	var future struct {
		Nested []int `json:"nested"`
	}

	_, err = reloaded.Unmarshal(futureKey, &future)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, future.Nested)
}

func TestDocDeleteRemovesAKey(t *testing.T) {
	t.Parallel()

	path := writeFile(t, `{
  "note": "kept",
  "webhook": "https://example.com/hook",
  "feed": {"max_fetch_num": 1}
}`)

	doc, err := configstore.Load(path)
	require.NoError(t, err)

	doc.Delete("webhook")
	doc.Delete("missing")

	assert.Equal(t, []string{noteKey, feedKey}, doc.Keys())

	data, err := doc.Bytes()
	require.NoError(t, err)
	require.NoError(t, configstore.WriteAtomic(path, data, configstore.PermConfig))

	reloaded, err := configstore.Load(path)
	require.NoError(t, err)
	assert.Equal(t, []string{noteKey, feedKey}, reloaded.Keys())
}

func TestDocUnmarshalReportsMissingAndBrokenFields(t *testing.T) {
	t.Parallel()

	path := writeFile(t, `{"feed": "not an object"}`)

	doc, err := configstore.Load(path)
	require.NoError(t, err)

	var absent int

	found, err := doc.Unmarshal("nothing", &absent)
	require.NoError(t, err)
	assert.False(t, found)

	var section struct{}

	_, err = doc.Unmarshal("feed", &section)
	require.ErrorContains(t, err, `parse config field "feed"`)
}

func TestLoadRejectsNonObjectDocuments(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"array":    `[1, 2]`,
		"trailing": `{"a": 1} {"b": 2}`,
		"garbage":  `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := configstore.Load(writeFile(t, content))
			require.Error(t, err)
		})
	}
}

func TestWriteAtomicKeepsPreviousFileOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "informer.json")

	require.NoError(t, configstore.WriteAtomic(path, []byte("{\"a\":1}\n"), configstore.PermConfig))

	// a directory in place of the target makes the rename fail; the original bytes
	// have to survive and no temporary file may be left behind.
	target := filepath.Join(dir, "blocked")
	require.NoError(t, os.Mkdir(target, 0o750))
	require.Error(t, configstore.WriteAtomic(target, []byte("{}"), configstore.PermConfig))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no temporary file should remain")

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(raw))
}

func TestWriteAtomicEnforcesSecretPermission(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("windows does not model unix permission bits")
	}

	path := filepath.Join(t.TempDir(), "informer.secret.json")

	// a pre-existing world readable file must end up locked down after the save.
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644)) //nolint:gosec //that is the point of the case.
	require.NoError(t, configstore.WriteAtomic(path, []byte(`{"webhook":"x"}`), configstore.PermSecret))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, configstore.PermSecret, info.Mode().Perm())

	// and rewriting it keeps the permission.
	require.NoError(t, configstore.WriteAtomic(path, []byte(`{"webhook":"y"}`), configstore.PermSecret))

	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, configstore.PermSecret, info.Mode().Perm())
}

func TestWithLockSerialisesReadModifyWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "informer.json")
	require.NoError(t, configstore.WriteAtomic(path, []byte(`{"counter":0}`+"\n"), configstore.PermConfig))

	const writers = 12

	var wait sync.WaitGroup

	for range writers {
		wait.Go(func() {
			err := configstore.WithLock(path, 20*time.Second, func() error {
				doc, err := configstore.Load(path)
				if err != nil {
					return err
				}

				var counter int

				_, err = doc.Unmarshal("counter", &counter)
				if err != nil {
					return err
				}

				err = doc.Set("counter", counter+1)
				if err != nil {
					return err
				}

				data, err := doc.Bytes()
				if err != nil {
					return err
				}

				return configstore.WriteAtomic(path, data, configstore.PermConfig)
			})
			// assert, not require: a require inside a goroutine cannot fail the test.
			assert.NoError(t, err)
		})
	}

	wait.Wait()

	doc, err := configstore.Load(path)
	require.NoError(t, err)

	var counter int

	_, err = doc.Unmarshal("counter", &counter)
	require.NoError(t, err)
	assert.Equal(t, writers, counter, "every increment must survive, none may be lost")
}

func TestConcurrentReadersNeverSeeATruncatedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "informer.json")
	require.NoError(t, configstore.WriteAtomic(path, []byte(`{"round":0}`+"\n"), configstore.PermConfig))

	var (
		stop    atomic.Bool
		wait    sync.WaitGroup
		payload = make(map[string]string, 200)
	)

	for i := range 200 {
		payload["key"+string(rune('a'+i%26))+string(rune('0'+i/26))] = "a fairly long value so the document does not fit in one page"
	}

	wait.Go(func() {
		for round := 1; round <= 40; round++ {
			err := configstore.WithLock(path, 20*time.Second, func() error {
				doc := configstore.NewDoc()

				err := doc.Set("round", round)
				if err != nil {
					return err
				}

				err = doc.Set("bulk", payload)
				if err != nil {
					return err
				}

				data, err := doc.Bytes()
				if err != nil {
					return err
				}

				return configstore.WriteAtomic(path, data, configstore.PermConfig)
			})
			// assert, not require: a require inside a goroutine cannot fail the test.
			assert.NoError(t, err) //nolint:testifylint //see above.
		}

		stop.Store(true)
	})

	reads := 0

	for !stop.Load() {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		require.True(t, json.Valid(raw), "a reader observed a partially written document")

		reads++
	}

	wait.Wait()
	assert.Positive(t, reads)
}

func TestWithLockTimesOutInsteadOfDeadlocking(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "informer.json")

	// simulate a peer holding the lock: the file exists and is fresh, so it is not
	// stale, and a second writer has to give up instead of waiting forever.
	require.NoError(t, os.WriteFile(path+configstore.LockSuffix, []byte("1"), configstore.PermSecret))

	start := time.Now()
	err := configstore.WithLock(path, 100*time.Millisecond, func() error {
		t.Fatal("the guarded section must not run while the lock is held")

		return nil
	})

	require.ErrorIs(t, err, configstore.ErrLockTimeout)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestWithLockTakesOverAStaleLock(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "informer.json")
	lockPath := path + configstore.LockSuffix

	require.NoError(t, os.WriteFile(lockPath, []byte("999999"), configstore.PermSecret))

	// a lock left behind by a killed process is old; aging it makes the takeover
	// deterministic instead of waiting a real minute.
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(lockPath, old, old))

	ran := false

	require.NoError(t, configstore.WithLock(path, 100*time.Millisecond, func() error {
		ran = true

		return nil
	}))
	assert.True(t, ran)

	_, err := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(err), "the lock must be released again")
}

func writeFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "informer.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
