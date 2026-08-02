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

package configstore

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	// LockSuffix is appended to a configuration path to name its lock file.
	LockSuffix = ".lock"

	// DefaultLockTimeout bounds how long a writer waits for its turn. It is short
	// enough that the desktop app reports a busy file instead of appearing frozen.
	DefaultLockTimeout = 5 * time.Second

	// staleLockAge is how old a lock file has to be before it is treated as the
	// leftover of a killed process and taken over. Every guarded section is a few
	// file operations long, so a lock this old cannot belong to a live writer.
	staleLockAge = 60 * time.Second

	// lockRetryInterval is the wait between two acquisition attempts.
	lockRetryInterval = 20 * time.Millisecond
)

// WithLock runs fn while holding the advisory write lock of path.
//
// The lock is an exclusively created sibling file, which is the one primitive that
// behaves the same on every platform informer ships on. Waiting is bounded by timeout
// and a lock left behind by a killed process is taken over once it is clearly stale,
// so neither a crash nor a busy peer can block a writer forever.
//
// Readers do not take the lock: WriteAtomic replaces the file in one rename, so a read
// always observes a complete document. The lock exists to keep two read-modify-write
// sequences from overwriting each other.
func WithLock(path string, timeout time.Duration, fn func() error) error {
	lockPath := path + LockSuffix

	err := acquireLock(lockPath, timeout)
	if err != nil {
		return err
	}

	defer func() {
		// releasing is best effort: a failed remove leaves a stale lock that the
		// next writer takes over, which is strictly better than aborting the save.
		_ = os.Remove(lockPath)
	}()

	return fn()
}

// acquireLock blocks until the lock file could be created exclusively.
func acquireLock(lockPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, PermSecret)
		if err == nil {
			_, _ = file.WriteString(strconv.Itoa(os.Getpid()))

			return file.Close()
		}

		if !os.IsExist(err) {
			return fmt.Errorf("create configuration lock %q: %w", lockPath, err)
		}

		if takeOverStaleLock(lockPath) {
			continue
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %q is held by another informer process", ErrLockTimeout, lockPath)
		}

		time.Sleep(lockRetryInterval)
	}
}

// takeOverStaleLock removes a lock file older than staleLockAge and reports whether
// it did, so the caller can retry immediately.
func takeOverStaleLock(lockPath string) bool {
	info, err := os.Stat(lockPath)
	if err != nil {
		// the holder released it between the failed create and this stat.
		return os.IsNotExist(err)
	}

	if time.Since(info.ModTime()) < staleLockAge {
		return false
	}

	return os.Remove(lockPath) == nil
}
