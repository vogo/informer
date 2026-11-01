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

//nolint:testpackage,gosmopolitan //Exercise the private driver and durable send state without operating the real UI.
package wechat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/logbuf"
)

var errFakeUI = errors.New("fake UI failure")

// fakeUI models search, group details, the draft and visible chat history. Tests
// drive the production workflow, including its durable pre-send marker.
type fakeUI struct {
	requests      []request
	statusErr     error
	group         string
	search        string
	opened        bool
	details       bool
	personal      bool
	duplicate     bool
	draft         string
	sent          []string
	calls         int
	sendErr       error
	hideSent      bool
	changedHeader bool
}

func (f *fakeUI) call(req request) (response, error) {
	f.calls++
	f.requests = append(f.requests, req)
	switch req.Op {
	case "status":
		return response{Message: "ready"}, f.statusErr
	case "activate":
		return response{Message: "ready"}, nil
	case "snapshot":
		return response{Nodes: f.nodes()}, nil
	case "set":
		if searchField(*req.Node) {
			f.search = req.Value
		} else {
			f.draft = req.Value
		}
	case "press":
		switch {
		case req.Node.Role == "AXRow":
			f.opened = true
			f.search = ""
		case label(*req.Node, "聊天信息"):
			f.details = !f.details
		default:
			return response{}, errFakeUI
		}
	case "send":
		if req.Group != f.group || req.Body != f.draft {
			return response{}, errFakeUI
		}
		if f.sendErr != nil {
			return response{}, f.sendErr
		}
		f.sent = append(f.sent, f.draft)
		f.draft = ""
	default:
		return response{}, fmt.Errorf("%w: %s", errFakeUI, req.Op)
	}

	return response{}, nil
}

func (f *fakeUI) nodes() []node {
	ns := []node{{Role: "AXWindow", Parent: -1, Path: []int{}}}
	add := func(n node) { n.Path = []int{len(ns) - 1}; ns = append(ns, n) }
	add(node{Role: "AXTextArea", Title: "搜索", Value: f.search, Parent: 0, Writable: true, Enabled: true})
	if f.search != "" {
		add(node{Role: "AXRow", Title: f.group, Parent: 0, Enabled: true, Pressable: true})
		if f.duplicate {
			add(node{Role: "AXRow", Title: f.group, Parent: 0, Enabled: true, Pressable: true})
		}
	}
	if f.opened {
		header := f.group + "（12）"
		if f.changedHeader && f.draft != "" {
			header = "其他群（12）"
		}
		add(node{Role: "AXStaticText", Value: header, Parent: 0})
		add(node{Role: "AXButton", Title: "聊天信息", Parent: 0, Enabled: true, Pressable: true})
		add(node{Role: "AXTextArea", Value: f.draft, Parent: 0, Writable: true, Enabled: true})
		add(node{Role: "AXButton", Title: "发送", Parent: 0, Enabled: true, Pressable: true})
		if f.details && !f.personal {
			add(node{Role: "AXStaticText", Value: "群聊名称", Parent: 0})
		}
		table := len(ns)
		add(node{Role: "AXTable", Parent: 0})
		if !f.hideSent {
			for _, body := range f.sent {
				add(node{Role: "AXStaticText", Value: body, Parent: table})
			}
		}
	}

	return ns
}

func testClient(t *testing.T, ui *fakeUI) *client {
	t.Helper()

	return &client{driver: ui, stateDir: t.TempDir(), pause: func() {}}
}

func TestSendVerifiedGroupAndDeduplicate(t *testing.T) {
	t.Parallel()
	ui := &fakeUI{group: "技术群"}
	c := testClient(t, ui)
	result, err := c.send(ui.group, "消息\nhttps://example.com")
	require.NoError(t, err)
	require.Equal(t, "sent", result.Status)
	require.Equal(t, []string{"消息\nhttps://example.com"}, ui.sent)
	count := ui.calls
	_, err = c.send(ui.group, "消息\nhttps://example.com")
	require.NoError(t, err)
	require.Equal(t, count, ui.calls, "receipt avoids all further UI operations")
	require.NoError(t, c.locate(ui.group))
	require.Len(t, ui.sent, 1, "locate never sends")
}

func TestSendRefusesAmbiguousAndPersonalChats(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ui   *fakeUI
	}{
		{"duplicate", &fakeUI{group: "同名群", duplicate: true}},
		{"personal", &fakeUI{group: "同名联系人", personal: true}},
		{"draft", &fakeUI{group: "技术群", draft: "用户的草稿"}},
		{"changed header", &fakeUI{group: "技术群", changedHeader: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := testClient(t, tc.ui)
			_, err := c.send(tc.ui.group, "正文")
			require.Error(t, err)
			require.Empty(t, tc.ui.sent)
			_, err = os.Stat(c.statePath(tc.ui.group))
			require.True(t, os.IsNotExist(err), "must not touch send state before validation")
			if tc.name == "draft" {
				require.Equal(t, "用户的草稿", tc.ui.draft)
			}
		})
	}
}

func TestUncertainSendBlocksChangedBodyAfterRestart(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ui   *fakeUI
	}{
		{"native timeout", &fakeUI{group: "技术群", sendErr: errFakeUI}},
		{"unobservable result", &fakeUI{group: "技术群", hideSent: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := testClient(t, tc.ui)
			_, err := c.send(tc.ui.group, "正文")
			require.ErrorIs(t, err, ErrUncertain)
			// A new client/process with a changed report must still refuse retry.
			restarted := &client{driver: tc.ui, stateDir: c.stateDir, pause: func() {}}
			count := tc.ui.calls
			_, err = restarted.send(tc.ui.group, "不同的正文")
			require.ErrorIs(t, err, ErrUncertain)
			require.Equal(t, count, tc.ui.calls)
		})
	}
}

func TestStateWriteFailureNeverSends(t *testing.T) {
	t.Parallel()
	ui := &fakeUI{group: "技术群"}
	c := testClient(t, ui)
	c.stateDir = filepath.Join(c.stateDir, "missing")
	_, err := c.send(ui.group, "正文")
	require.Error(t, err)
	require.Empty(t, ui.sent)
}

func TestCorruptStateStopsBeforeUI(t *testing.T) {
	t.Parallel()
	ui := &fakeUI{group: "技术群"}
	c := testClient(t, ui)
	require.NoError(t, os.WriteFile(c.statePath(ui.group), []byte("broken"), 0o600))
	_, err := c.send(ui.group, "正文")
	require.ErrorContains(t, err, "记录损坏")
	require.Zero(t, ui.calls)
}

func TestMatchingUsesExactNamesAndIgnoresHistoryTitles(t *testing.T) {
	t.Parallel()
	ns := make([]node, 0, 5)
	ns = append(ns, node{Role: "AXWindow", Parent: -1}, node{Role: "AXTable", Parent: 0}, node{Role: "AXStaticText", Value: "技术群（10）", Parent: 1})
	require.False(t, verifiedHeader(ns, "技术群"))
	ns = append(ns, node{Role: "AXStaticText", Value: "其他技术群（10）", Parent: 0})
	require.False(t, verifiedHeader(ns, "技术群"))
	ns = append(ns, node{Role: "AXButton", Title: "技术群（10）", Parent: 0})
	require.True(t, verifiedHeader(ns, "技术群"))
	_, err := searchResult([]node{{Role: "AXRow", Title: "其他技术群", Enabled: true, Pressable: true}}, "技术群")
	require.Error(t, err)
}

func TestConfigAndPlainText(t *testing.T) {
	t.Parallel()
	require.NoError(t, Validate(Config{}))
	for _, name := range []string{"", "  ", "技术\n群", strings.Repeat("群", 101)} {
		require.Error(t, Validate(Config{Enabled: true, GroupName: name}))
	}
	require.Equal(t, "今日\n标题\nhttps://example.com/a?q=1&b=2\n中文内容", PlainText("# 今日\r\n[标题](https://example.com/a?q=1&b=2)\n**中文内容**"))
	_, err := Send("技术群", strings.Repeat("字", MaxMessageRunes+1))
	require.ErrorContains(t, err, "4000")
}

func TestResolvePendingPreservesSuccessfulReceipt(t *testing.T) {
	t.Parallel()
	ui := &fakeUI{group: "技术群"}
	c := testClient(t, ui)
	require.NoError(t, c.writeState(ui.group, receipt{Status: statusPending, Digest: hash("old")}))
	require.NoError(t, c.resolvePending(ui.group))
	_, err := os.Stat(c.statePath(ui.group))
	require.True(t, os.IsNotExist(err))
	_, err = c.send(ui.group, "正文")
	require.NoError(t, err)
	require.NoError(t, c.resolvePending(ui.group))
	_, err = c.send(ui.group, "正文")
	require.NoError(t, err)
	require.Len(t, ui.sent, 1, "clearing pending must not erase confirmed receipt")
}

func TestEnvironmentCheckRequestsPermissionWithoutSending(t *testing.T) {
	t.Parallel()
	for _, denied := range []bool{false, true} {
		t.Run(strconv.FormatBool(denied), func(t *testing.T) {
			t.Parallel()
			ui := &fakeUI{}
			if denied {
				ui.statusErr = errFakeUI
			}
			result, err := testClient(t, ui).check()
			if denied {
				require.ErrorIs(t, err, errFakeUI)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, "ready", result.Status)
			}
			require.Equal(t, []request{{Op: "status", Prompt: true}}, ui.requests)
			require.Empty(t, ui.sent)
		})
	}
}

func TestDeliveryDoesNotRequestPermissionOrOpenSettings(t *testing.T) {
	t.Parallel()
	ui := &fakeUI{group: "技术群"}
	_, err := testClient(t, ui).send(ui.group, "正文")
	require.NoError(t, err)
	for _, req := range ui.requests {
		require.False(t, req.Prompt, "delivery must never open permission settings")
	}
}

//nolint:paralleltest //Capture the process-wide logger before parallel tests resume.
func TestWeChatErrorsReachSystemLogWithStage(t *testing.T) {
	buffer := captureWeChatLog(t)
	var wire response
	decodeErr := json.Unmarshal([]byte(`{"nodes":[{"writable":0}]}`), &wire)
	require.Error(t, decodeErr)
	ui := &fakeUI{group: "技术群"}
	c := testClient(t, ui)
	c.driver = processDriver(func(req request) (response, error) {
		if req.Op == "snapshot" {
			return response{}, decodeErr
		}

		return ui.call(req)
	})
	err := traceOperation("locate", ui.group, func() error { return c.locate(ui.group) })
	require.ErrorIs(t, err, decodeErr)
	entries := buffer.Since(0, 0).Entries
	var text strings.Builder
	errorCount := 0
	for _, entry := range entries {
		text.WriteString(entry.Text)
		if entry.Level == logbuf.LevelError {
			errorCount++
		}
	}
	require.GreaterOrEqual(t, errorCount, 2, "both stage error and operation failure reach the system log")
	require.Contains(t, text.String(), "stage=find_search_field op=snapshot failed")
	require.Contains(t, text.String(), "cannot unmarshal number")
	require.Contains(t, text.String(), "writable")
	require.Contains(t, text.String(), "operation=locate")
}

//nolint:paralleltest //Capture the process-wide logger before parallel tests resume.
func TestWeChatLogsProgressWithoutMessageContents(t *testing.T) {
	buffer := captureWeChatLog(t)
	ui := &fakeUI{group: "技术群"}
	c := testClient(t, ui)
	const body = "private-message-body-must-not-be-logged"
	err := traceOperation("send", ui.group, func() error {
		_, err := c.send(ui.group, body)

		return err
	})
	require.NoError(t, err)
	var text strings.Builder
	for _, entry := range buffer.Since(0, 0).Entries {
		text.WriteString(entry.Text)
	}
	for _, stage := range []string{"activate_wechat", "search_group", "verify_group_type", "write_message", "click_send", "verify_sent_message"} {
		require.Contains(t, text.String(), "stage="+stage)
	}
	require.Contains(t, text.String(), "snapshot nodes=")
	require.Contains(t, text.String(), "completed elapsed=")
	require.NotContains(t, text.String(), body)
	_, err = Send("", body)
	require.Error(t, err)
	last := buffer.Since(0, 1).Entries
	require.Len(t, last, 1)
	require.Equal(t, logbuf.LevelError, last[0].Level)
	require.Contains(t, last[0].Text, "operation=send")
}

func captureWeChatLog(t *testing.T) *logbuf.Buffer {
	t.Helper()
	previous := logger.Writer()
	buffer := logbuf.New(200)
	logger.SetOutput(buffer)
	t.Cleanup(func() { logger.SetOutput(previous) })

	return buffer
}

func TestCollapsedNativeTreeReportsCompatibilityInsteadOfOnlyTimeout(t *testing.T) {
	t.Parallel()
	c := &client{driver: processDriver(func(req request) (response, error) {
		if req.Op == opSnapshot {
			return response{Nodes: []node{{Role: "AXWindow", Parent: -1}, {Role: roleButton, Parent: 0}}}, nil
		}

		return response{}, nil
	}), pause: func() {}}
	_, err := c.openChat("技术群")
	require.ErrorContains(t, err, "未暴露可识别的搜索框")
	require.ErrorContains(t, err, "界面节点=2")
	require.ErrorContains(t, err, "当前微信版本不支持")
}
