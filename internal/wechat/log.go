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

package wechat

import (
	"errors"
	"time"

	"github.com/vogo/logger"
)

// traceOperation covers validation, lock acquisition and native failures alike.
// This runs in the parent process; the native helper reserves stdout for JSON.
func traceOperation(operation, group string, work func() error) error {
	start := time.Now()
	logger.Infof("[wechat] operation=%s target=%q started", operation, group)
	err := work()
	if err != nil {
		logger.Errorf("[wechat] operation=%s target=%q failed uncertain=%t elapsed=%s error=%v", operation, group, errors.Is(err, ErrUncertain), time.Since(start).Round(time.Millisecond), err)

		return err
	}
	logger.Infof("[wechat] operation=%s target=%q completed elapsed=%s", operation, group, time.Since(start).Round(time.Millisecond))

	return nil
}

func (c *client) setStage(stage string) {
	c.stage = stage
	c.snapshotLogged = false
	logger.Infof("[wechat] stage=%s", stage)
}

//nolint:gocritic //Requests are immutable values passed consistently through the driver interface.
func (c *client) call(req request) (response, error) {
	start := time.Now()
	result, err := c.driver.call(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		logger.Errorf("[wechat] stage=%s op=%s failed elapsed=%s error=%v", c.stage, req.Op, elapsed, err)

		return result, err
	}
	// One metadata summary per stage; polling does not flood the application log.
	// Never record message text, raw UI snapshots or existing draft contents.
	if req.Op == opSnapshot && !c.snapshotLogged {
		editableCount, searchCount, buttonCount := 0, 0, 0
		for _, n := range result.Nodes {
			if editable(n) {
				editableCount++
			}
			if searchField(n) {
				searchCount++
			}
			if n.Role == roleButton {
				buttonCount++
			}
		}
		logger.Infof("[wechat] stage=%s snapshot nodes=%d editable=%d search_fields=%d buttons=%d elapsed=%s", c.stage, len(result.Nodes), editableCount, searchCount, buttonCount, elapsed)
		c.snapshotLogged = true
	} else {
		logger.Debugf("[wechat] stage=%s op=%s completed nodes=%d elapsed=%s", c.stage, req.Op, len(result.Nodes), elapsed)
	}

	return result, nil
}
