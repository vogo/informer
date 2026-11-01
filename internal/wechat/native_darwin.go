//go:build darwin && cgo

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

//nolint:gocritic,gosmopolitan //cgo pseudo-imports confuse dupImport; diagnostics follow the Chinese UI.
package wechat

/*
#cgo CFLAGS: -x objective-c -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework ApplicationServices
#include <stdlib.h>
char *informer_wechat(const char *request);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

type nativeDriver struct{}

func platformDriver() driver { return nativeDriver{} }

//nolint:nlreturn //cgo-generated C.free wrappers have synthetic return positions.
func (nativeDriver) call(req request) (response, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return response{}, err
	}
	input := C.CString(string(data))
	defer C.free(unsafe.Pointer(input))

	output := C.informer_wechat(input)
	if output == nil {
		return response{}, fmt.Errorf("%w: 微信辅助功能接口未返回结果", errAutomation)
	}
	defer C.free(unsafe.Pointer(output))

	var result response
	err = json.Unmarshal([]byte(C.GoString(output)), &result)
	if err != nil {
		return result, fmt.Errorf("解析微信界面失败: %w", err)
	}
	if result.Error != "" {
		return result, fmt.Errorf("%w: %s", errAutomation, result.Error)
	}

	return result, nil
}

// flock is released by the kernel even after a crash. The lock is shared by all
// informer data directories, since they operate the same user's WeChat window.
func platformLock(dir string, fn func(*os.File) error) error {
	file, err := os.OpenFile(filepath.Join(dir, "ui.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return fmt.Errorf("微信正由另一个 informer 操作，请稍后再试: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN) //nolint:errcheck //Closing the file also releases the advisory lock.

	return fn(file)
}
