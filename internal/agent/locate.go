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

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// CodexCommand is the default executable of the Codex command line.
const CodexCommand = "codex"

// ErrCommandNotFound marks a locate that found no usable agent binary.
var ErrCommandNotFound = errors.New("agent command not found")

// loginShellLocateTimeout bounds how long a login-shell PATH probe may take.
// Desktop apps often inherit a stripped PATH; asking the user's shell is reliable
// but must not stall a fetch when the shell init is slow.
const loginShellLocateTimeout = 3 * time.Second

// DefaultCommandName is the bare executable name of a provider when none is configured.
func DefaultCommandName(provider string) string {
	if strings.TrimSpace(provider) == ProviderCodex {
		return CodexCommand
	}

	return ClaudeCommand
}

// ResolveCommand turns a configured command into an absolute executable path.
//
// An empty command looks up the provider default. A bare name is searched on PATH
// and in common install locations. An absolute or relative path is verified as an
// executable file. The returned path is always absolute when the lookup succeeds.
func ResolveCommand(provider, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return LocateCommand(provider)
	}

	if filepath.IsAbs(command) || strings.ContainsRune(command, filepath.Separator) ||
		(runtime.GOOS == "windows" && strings.Contains(command, `/`)) {
		return requireExecutable(command)
	}

	return lookUp(command)
}

// LocateCommand finds the default executable of a provider on this machine.
func LocateCommand(provider string) (string, error) {
	return lookUp(DefaultCommandName(provider))
}

func lookUp(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !isSafeCommandName(name) {
		return "", fmt.Errorf("%w: empty or invalid command name", ErrCommandNotFound)
	}

	if path, err := exec.LookPath(name); err == nil {
		return absExecutable(path)
	}

	for _, dir := range candidateDirs() {
		candidate := filepath.Join(dir, name)
		if path, err := requireExecutable(candidate); err == nil {
			return path, nil
		}

		if runtime.GOOS == "windows" {
			if path, err := requireExecutable(candidate + ".exe"); err == nil {
				return path, nil
			}
		}
	}

	if path, err := lookUpViaLoginShell(name); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("%w: %q not found in PATH or common install locations", ErrCommandNotFound, name)
}

func lookUpViaLoginShell(name string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", ErrCommandNotFound
	}

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}

	ctx, cancel := context.WithTimeout(context.Background(), loginShellLocateTimeout)
	defer cancel()

	// name is restricted to a safe command token, so interpolating it into the
	// shell snippet cannot turn into an arbitrary command.
	cmd := exec.CommandContext(ctx, shell, "-lc", "command -v "+name)

	out, err := cmd.Output()
	if err != nil {
		return "", ErrCommandNotFound
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCommandNotFound
	}

	return requireExecutable(path)
}

func requireExecutable(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: empty path", ErrCommandNotFound)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrCommandNotFound, path, err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("%w: %s is a directory", ErrCommandNotFound, path)
	}

	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%w: %s is not executable", ErrCommandNotFound, path)
	}

	return absExecutable(path)
}

func absExecutable(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrCommandNotFound, path, err)
	}

	return abs, nil
}

// isSafeCommandName reports whether name is a bare executable token safe to pass
// to a login shell. Paths and shell metacharacters are refused.
func isSafeCommandName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}

	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}

		return false
	}

	return true
}

func candidateDirs() []string {
	dirs := make([]string, 0, 24)

	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin")
	case "linux":
		dirs = append(dirs, "/usr/local/bin", "/usr/bin", "/snap/bin")
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs,
				filepath.Join(local, "Programs"),
				filepath.Join(local, "Yarn", "bin"),
				filepath.Join(local, "fnm_multishells"),
			)
		}

		if appData := os.Getenv("APPDATA"); appData != "" {
			dirs = append(dirs,
				filepath.Join(appData, "npm"),
				filepath.Join(appData, "nvm"),
			)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return uniqueExistingDirs(dirs)
	}

	dirs = append(dirs,
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
		filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, ".yarn", "bin"),
		filepath.Join(home, ".bun", "bin"),
		filepath.Join(home, ".volta", "bin"),
		filepath.Join(home, ".asdf", "shims"),
		filepath.Join(home, ".local", "share", "fnm", "aliases", "default", "bin"),
	)

	dirs = append(dirs, versionManagerBinDirs(filepath.Join(home, ".nvm", "versions", "node"))...)
	dirs = append(dirs, versionManagerBinDirs(filepath.Join(home, ".fnm", "node-versions"))...)
	dirs = append(dirs, versionManagerBinDirs(filepath.Join(home, ".local", "share", "fnm", "node-versions"))...)

	return uniqueExistingDirs(dirs)
}

// versionManagerBinDirs lists <root>/<version>/bin directories under a node version
// manager install tree. Missing roots are ignored.
func versionManagerBinDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirs = append(dirs, filepath.Join(root, entry.Name(), "bin"))
		// fnm stores binaries under installation/bin rather than bin.
		dirs = append(dirs, filepath.Join(root, entry.Name(), "installation", "bin"))
	}

	return dirs
}

func uniqueExistingDirs(dirs []string) []string {
	seen := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))

	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		if dir == "" || dir == "." {
			continue
		}

		if _, ok := seen[dir]; ok {
			continue
		}

		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		seen[dir] = struct{}{}
		out = append(out, dir)
	}

	return out
}
