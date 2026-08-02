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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Errors returned by this package.
var (
	// ErrInvalidConfig marks a file that is not the expected json document.
	ErrInvalidConfig = errors.New("invalid configuration file")

	// ErrLockTimeout marks a write that gave up waiting for the configuration lock.
	// It is returned instead of blocking forever, so a stuck peer can never deadlock
	// the desktop app or a scheduled run.
	ErrLockTimeout = errors.New("timed out waiting for the configuration lock")

	// ErrInsecurePermission marks a sensitive file whose permission bits could not be
	// brought to the required value. The write is aborted rather than completed in an
	// insecure state.
	ErrInsecurePermission = errors.New("sensitive file permission could not be enforced")
)

const (
	// PermConfig is the permission of the ordinary, non sensitive configuration file.
	PermConfig os.FileMode = 0o644

	// PermSecret is the required permission of a file holding credentials.
	PermSecret os.FileMode = 0o600

	// windowsGOOS is the one platform that does not model unix permission bits.
	windowsGOOS = "windows"
)

// WriteAtomic replaces path with data in one step.
//
// The bytes are written to a temporary file in the same directory, flushed, given the
// requested permission and only then renamed onto path. A concurrent reader therefore
// sees either the previous complete file or the new complete file, never a truncated
// one, and a crash in the middle leaves the previous file intact.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file next to %q: %w", path, err)
	}

	tmpName := tmp.Name()

	err = replaceWithTemp(tmp, tmpName, path, data, perm)
	if err != nil {
		// a failed save must not leave a stray temporary file behind.
		_ = os.Remove(tmpName)

		return err
	}

	return nil
}

// replaceWithTemp fills the already created temporary file and moves it onto path.
func replaceWithTemp(tmp *os.File, tmpName, path string, data []byte, perm os.FileMode) error {
	err := writeAndSync(tmp, data)
	if err != nil {
		return fmt.Errorf("write temporary file %q: %w", tmpName, err)
	}

	err = enforcePerm(tmpName, perm)
	if err != nil {
		return err
	}

	err = os.Rename(tmpName, path)
	if err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}

	// the rename carries the verified permission over, but an existing target that was
	// replaced on a platform without atomic permission transfer is checked again.
	return enforcePerm(path, perm)
}

// writeAndSync writes the payload and flushes it to disk before closing.
func writeAndSync(file *os.File, data []byte) error {
	_, err := file.Write(data)
	if err != nil {
		_ = file.Close()

		return err
	}

	err = file.Sync()
	if err != nil {
		_ = file.Close()

		return err
	}

	return file.Close()
}

// enforcePerm sets the permission of path and verifies the result.
//
// Windows does not model unix permission bits - its file system only exposes a
// read only flag - so there the chmod result cannot be verified and is trusted.
// On every other platform a mismatch aborts the write with ErrInsecurePermission,
// so a secret file is never left readable by other users.
func enforcePerm(path string, perm os.FileMode) error {
	err := os.Chmod(path, perm)
	if err != nil {
		return fmt.Errorf("%w: chmod %q to %#o: %w", ErrInsecurePermission, path, perm, err)
	}

	if runtime.GOOS == windowsGOOS {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%w: stat %q: %w", ErrInsecurePermission, path, err)
	}

	if info.Mode().Perm() != perm {
		return fmt.Errorf("%w: %q is %#o, expected %#o",
			ErrInsecurePermission, path, info.Mode().Perm(), perm)
	}

	return nil
}
