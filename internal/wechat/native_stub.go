//go:build !darwin || !cgo

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

//nolint:gosmopolitan //Unsupported-platform diagnostics are shown in the Chinese settings UI.
package wechat

import (
	"fmt"
	"os"
)

type unsupportedDriver struct{}

func platformDriver() driver { return unsupportedDriver{} }
func (unsupportedDriver) call(_ request) (response, error) {
	return response{}, fmt.Errorf("%w: 本机微信推送仅支持启用 CGO 的 macOS 版本；Windows 和 Linux 暂不支持", errAutomation)
}
func platformLock(_ string, fn func(*os.File) error) error { return fn(nil) }
