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

// Package wechat sends plain text through the visible, logged-in macOS client.
// It never uses WeChat's private protocol or reads its database.
//
//nolint:gosmopolitan //Chinese labels are part of the WeChat UI contract and user-facing diagnostics.
package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/vogo/informer/internal/configstore"
)

// MaxMessageRunes bounds a single plain-text UI message; messages are never split.
const MaxMessageRunes = 4000

const (
	StatusSent      = "sent"
	statusPending   = "pending"
	roleSearchField = "AXSearchField"
	roleButton      = "AXButton"
	roleRow         = "AXRow"
	roleStaticText  = "AXStaticText"
)

var errAutomation = errors.New("微信自动化失败")

// Config is an optional local desktop delivery target. It is off by default.
type Config struct {
	Enabled   bool   `json:"enabled"`
	GroupName string `json:"group_name"`
}

// Outcome describes local UI evidence, never a server delivery receipt.
type Outcome struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

var ErrUncertain = errors.New("微信发送结果不确定")

func Validate(config Config) error {
	if config.Enabled || config.GroupName != "" {
		name := strings.TrimSpace(config.GroupName)
		if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
			return fmt.Errorf("%w: 微信群名须为 1–100 字的完整单行名称", errAutomation)
		}
	}

	return nil
}

type node struct {
	Path        []int  `json:"path"`
	Parent      int    `json:"parent"`
	Role        string `json:"role"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Identifier  string `json:"identifier"`
	Value       string `json:"value"`
	Enabled     bool   `json:"enabled"`
	Writable    bool   `json:"writable"`
	Pressable   bool   `json:"pressable"`
}
type request struct {
	Group string `json:"group,omitempty"`
	Body  string `json:"body,omitempty"`
	Op    string `json:"op"`
	Node  *node  `json:"node,omitempty"`
	Value string `json:"value,omitempty"`
}
type response struct {
	Nodes   []node `json:"nodes"`
	Window  string `json:"window"`
	Message string `json:"message"`
	Error   string `json:"error"`
}
type driver interface {
	call(req request) (response, error)
}

// Native AX calls run in a short-lived copy of this executable. A stuck WeChat
// cannot block the scheduler or application shutdown indefinitely.
//
//nolint:gochecknoinits //The same binary must handle isolated AX calls before CLI or Wails startup.
func init() {
	if len(os.Args) != 2 || os.Args[1] != "--informer-wechat-helper" || os.Getenv("INFORMER_WECHAT_HELPER") != "1" {
		return
	}
	var req request
	var result response
	err := json.NewDecoder(os.Stdin).Decode(&req)
	if err != nil {
		result.Error = "无效的微信辅助进程请求"
	} else {
		var err error
		result, err = platformDriver().call(req)
		if err != nil {
			result.Error = err.Error()
		}
	}
	err = json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

type processDriver func(request) (response, error)

func (d processDriver) call(req request) (response, error) { return d(req) }

func callProcess(ctx context.Context, req request, lockFile *os.File) (response, error) {
	exe, err := os.Executable()
	if err != nil {
		return response{}, err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--informer-wechat-helper")
	cmd.Env = append(os.Environ(), "INFORMER_WECHAT_HELPER=1")
	// Keep the flock alive if the parent exits while the AX worker is running.
	if lockFile != nil {
		cmd.ExtraFiles = []*os.File{lockFile}
	}
	cmd.Stdin = strings.NewReader(string(data))
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return response{}, fmt.Errorf("%w: 读取微信界面超时，请检查微信窗口与辅助功能权限", errAutomation)
		}

		return response{}, fmt.Errorf("微信辅助进程失败: %w", err)
	}
	var result response
	err = json.Unmarshal(output, &result)
	if err != nil {
		return result, fmt.Errorf("解析微信辅助进程结果失败: %w", err)
	}
	if result.Error != "" {
		return result, fmt.Errorf("%w: %s", errAutomation, strings.TrimPrefix(result.Error, errAutomation.Error()+": "))
	}

	return result, nil
}

var uiMutex sync.Mutex

type client struct {
	driver   driver
	stateDir string
	pause    func()
}

func withClient(fn func(*client) error) error {
	if !uiMutex.TryLock() {
		return fmt.Errorf("%w: 微信正在由 informer 操作，请稍后再试", errAutomation)
	}
	defer uiMutex.Unlock()
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(configDir, "informer", "wechat")
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	return platformLock(dir, func(lockFile *os.File) error {
		return fn(&client{driver: processDriver(func(req request) (response, error) { return callProcess(ctx, req, lockFile) }), stateDir: dir, pause: func() { time.Sleep(200 * time.Millisecond) }})
	})
}

// Check only checks availability and permissions. It does not change chats.
func Check() (*Outcome, error) {
	var result response
	err := withClient(func(c *client) error {
		var err error
		result, err = c.driver.call(request{Op: "status"})

		return err
	})
	if err != nil {
		return nil, err
	}

	return &Outcome{Status: "ready", Message: result.Message}, nil
}

// Locate opens an exact, unique search result and verifies the group details.
func Locate(group string) (*Outcome, error) {
	group = strings.TrimSpace(group)
	err := Validate(Config{Enabled: true, GroupName: group})
	if err != nil {
		return nil, err
	}
	err = withClient(func(c *client) error { return c.locate(group) })
	if err != nil {
		return nil, err
	}

	return &Outcome{Status: "located", Message: "已定位并校验群聊：" + group + "（未发送消息）"}, nil
}

// Send is serialized across the desktop app and CLI. An unresolved pending send
// blocks ALL subsequent messages to this group until explicitly acknowledged.
func Send(group, markdown string) (*Outcome, error) {
	group = strings.TrimSpace(group)
	err := Validate(Config{Enabled: true, GroupName: group})
	if err != nil {
		return nil, err
	}
	body := PlainText(markdown)
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > MaxMessageRunes {
		return nil, fmt.Errorf("%w: 微信消息须为 1–%d 字；请缩短日报或减少推荐文章数", errAutomation, MaxMessageRunes)
	}
	var outcome *Outcome
	err = withClient(func(c *client) error {
		var err error
		outcome, err = c.send(group, body)

		return err
	})

	return outcome, err
}

// ResolvePending is called only after the user has inspected WeChat. Receipts
// remain, so acknowledging uncertainty does not discard successful deduplication.
func ResolvePending(group string) error {
	group = strings.TrimSpace(group)
	err := Validate(Config{Enabled: true, GroupName: group})
	if err != nil {
		return err
	}

	return withClient(func(c *client) error { return c.resolvePending(group) })
}

func (c *client) resolvePending(group string) error {
	state, err := c.readState(group)
	if err != nil {
		return err
	}
	if state.Status != statusPending {
		return nil
	}

	return os.Remove(c.statePath(group))
}

type receipt struct {
	Status string    `json:"status"`
	Digest string    `json:"digest"`
	At     time.Time `json:"at"`
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:])
}

func (c *client) statePath(group string) string {
	return filepath.Join(c.stateDir, hash(group)+".json")
}

func (c *client) readState(group string) (receipt, error) {
	var state receipt
	data, err := os.ReadFile(c.statePath(group))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	if err != nil {
		return state, fmt.Errorf("微信发送记录损坏，已停止发送: %w", err)
	}
	if state.Status != StatusSent && state.Status != statusPending {
		return state, fmt.Errorf("%w: 微信发送记录状态无效，已停止发送", errAutomation)
	}

	return state, nil
}

func (c *client) writeState(group string, state receipt) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return configstore.WriteAtomic(c.statePath(group), data, 0o600)
}

func uncertain(err error) error {
	return fmt.Errorf("%w，请检查群内消息后在设置中解除暂停；不会自动重发: %w", ErrUncertain, err)
}

func (c *client) snapshot() ([]node, error) {
	result, err := c.driver.call(request{Op: "snapshot"})

	return result.Nodes, err
}

//nolint:gocritic //UI snapshots are immutable values, copied consistently across selector callbacks.
func (c *client) act(op string, n node, value string) error {
	_, err := c.driver.call(request{Op: op, Node: &n, Value: value})

	return err
}

//nolint:gocritic //UI snapshots are immutable values, copied consistently across selector callbacks.
func label(n node, words ...string) bool {
	for _, s := range words {
		if n.Title == s || n.Description == s || n.Value == s {
			return true
		}
	}

	return false
}

//nolint:gocritic //UI snapshots are immutable values, copied consistently across selector callbacks.
func editable(n node) bool {
	return (n.Role == "AXTextArea" || n.Role == "AXTextField" || n.Role == roleSearchField) && n.Writable && n.Enabled
}

//nolint:gocritic //UI snapshots are immutable values, copied consistently across selector callbacks.
func searchField(n node) bool {
	return editable(n) && (n.Role == roleSearchField || label(n, "搜索", "Search"))
}

func choose(nodes []node, match func(node) bool, what string) (node, error) {
	found := []node{}
	for _, n := range nodes {
		if match(n) {
			found = append(found, n)
		}
	}
	if len(found) != 1 {
		return node{}, fmt.Errorf("%w: 无法唯一识别%s（匹配 %d 个）；当前微信界面不受支持或目标有歧义", errAutomation, what, len(found))
	}

	return found[0], nil
}

//nolint:gocritic //UI snapshots are immutable values, copied consistently across selector callbacks.
func pressable(n node) bool { return n.Pressable && n.Enabled }

func (c *client) waitFor(fn func([]node) bool) ([]node, error) {
	for range 12 {
		nodes, err := c.snapshot()
		if err != nil {
			return nil, err
		}
		if fn(nodes) {
			return nodes, nil
		}
		c.pause()
	}

	return nil, fmt.Errorf("%w: 等待微信界面更新超时；请检查是否已登录、是否有弹窗", errAutomation)
}

//nolint:gocritic //UI snapshots are immutable values, copied consistently across selector callbacks.
func titleMatches(n node, group string) bool {
	if n.Role != roleStaticText && n.Role != roleButton {
		return false
	}
	pattern := regexp.MustCompile("^" + regexp.QuoteMeta(group) + `\s*[（(][0-9]+[)）]$`)
	for _, s := range []string{n.Title, n.Value, n.Description} {
		if s == group || pattern.MatchString(s) {
			return true
		}
	}

	return false
}

func inHistory(nodes []node, index int) bool {
	for p := nodes[index].Parent; p >= 0 && p < len(nodes); p = nodes[p].Parent {
		if nodes[p].Role == "AXTable" || nodes[p].Role == "AXList" || nodes[p].Role == roleRow || nodes[p].Role == "AXOutline" {
			return true
		}
	}

	return false
}

func verifiedHeader(nodes []node, group string) bool {
	for i, n := range nodes {
		if titleMatches(n, group) && !inHistory(nodes, i) {
			return true
		}
	}

	return false
}

func composer(nodes []node) (node, error) {
	return choose(nodes, func(n node) bool { return editable(n) && !searchField(n) }, "聊天输入框")
}

func (c *client) locate(group string) error {
	nodes, err := c.openChat(group)
	if err != nil {
		return err
	}

	return c.verifyGroup(group, nodes)
}

func (c *client) openChat(group string) ([]node, error) {
	_, err := c.driver.call(request{Op: "activate"})
	if err != nil {
		return nil, err
	}
	nodes, err := c.waitFor(func(nodes []node) bool {
		_, err := choose(nodes, searchField, "搜索框")

		return err == nil
	})
	if err != nil {
		return nil, err
	}
	search, err := choose(nodes, searchField, "搜索框")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(search.Value) != "" && search.Value != group {
		return nil, fmt.Errorf("%w: 微信搜索框已有内容，请先清空后重试", errAutomation)
	}
	err = c.act("set", search, group)
	if err != nil {
		return nil, err
	}
	// Search matching is deliberately exact. Do not send to the first result.
	var candidate node
	_, err = c.waitFor(func(nodes []node) bool {
		var e error
		candidate, e = searchResult(nodes, group)

		return e == nil
	})
	if err != nil {
		return nil, fmt.Errorf("未找到唯一的同名聊天，请检查完整群名、重名群及搜索结果: %w", err)
	}
	// Re-read after a quiet interval so a partially populated result list cannot
	// turn the first matching row into an accidental target.
	c.pause()
	nodes, err = c.snapshot()
	if err != nil {
		return nil, err
	}
	candidate, err = searchResult(nodes, group)
	if err != nil {
		return nil, err
	}
	err = c.act("press", candidate, "")
	if err != nil {
		return nil, err
	}
	nodes, err = c.waitFor(func(nodes []node) bool {
		s, e := choose(nodes, searchField, "搜索框")

		return e == nil && s.Value == "" && verifiedHeader(nodes, group)
	})
	if err != nil {
		return nil, fmt.Errorf("无法校验已打开聊天的标题: %w", err)
	}

	return nodes, nil
}

func (c *client) verifyGroup(group string, nodes []node) error {
	info, err := choose(nodes, func(n node) bool {
		return n.Role == roleButton && pressable(n) && label(n, "聊天信息", "聊天详情", "Chat Info", "Chat Details")
	}, "聊天信息按钮")
	if err != nil {
		return err
	}
	err = c.act("press", info, "")
	if err != nil {
		return err
	}
	_, err = c.waitFor(func(nodes []node) bool {
		// Personal contacts also have Chat Info. Require the group-only field.
		for _, n := range nodes {
			if label(n, "群聊名称", "群名称", "Group Chat Name", "Group Name") {
				return true
			}
		}

		return false
	})
	if err != nil {
		return fmt.Errorf("无法证明目标是群聊（可能是同名联系人或当前微信版本不受支持）: %w", err)
	}
	nodes, err = c.snapshot()
	if err != nil {
		return err
	}
	if !verifiedHeader(nodes, group) {
		return fmt.Errorf("%w: 群聊校验时聊天标题改变，已停止", errAutomation)
	}
	// Toggle the same information button; no global Escape/Return keystrokes.
	info, err = choose(nodes, func(n node) bool {
		return n.Role == roleButton && pressable(n) && label(n, "聊天信息", "聊天详情", "Chat Info", "Chat Details")
	}, "聊天信息按钮")
	if err != nil {
		return err
	}
	err = c.act("press", info, "")
	if err != nil {
		return err
	}
	_, err = c.waitFor(func(nodes []node) bool {
		for _, n := range nodes {
			if label(n, "群聊名称", "群名称", "Group Chat Name", "Group Name") {
				return false
			}
		}
		_, e := composer(nodes)

		return e == nil && verifiedHeader(nodes, group)
	})

	return err
}

func searchResult(nodes []node, group string) (node, error) {
	candidates := []node{}
	for i, n := range nodes {
		if n.Role != roleRow && n.Role != roleButton {
			continue
		}
		if !pressable(n) {
			continue
		}
		matched := label(n, group)
		if n.Role == roleRow {
			for j, m := range nodes {
				if descendant(nodes, j, i) && label(m, group) && !editable(m) {
					matched = true
				}
			}
		}
		if matched {
			candidates = append(candidates, n)
		}
	}
	// A row may contain a pressable button with the same name. Keep only the row.
	filtered := []node{}
	for _, n := range candidates {
		contained := false
		for _, other := range candidates {
			if len(other.Path) < len(n.Path) && pathPrefix(n.Path, other.Path) {
				contained = true
			}
		}
		if !contained {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) != 1 {
		return node{}, fmt.Errorf("%w: 目标聊天 %q 匹配 %d 个可操作结果", errAutomation, group, len(filtered))
	}

	return filtered[0], nil
}

func descendant(nodes []node, child, parent int) bool {
	for p := nodes[child].Parent; p >= 0 && p < len(nodes); p = nodes[p].Parent {
		if p == parent {
			return true
		}
	}

	return false
}

func pathPrefix(path, prefix []int) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i, v := range prefix {
		if path[i] != v {
			return false
		}
	}

	return true
}

func messageCount(nodes []node, body string) int {
	count := 0
	for i, n := range nodes {
		if n.Role == roleStaticText && !n.Writable && inHistory(nodes, i) && label(n, body) {
			count++
		}
	}

	return count
}

func (c *client) send(group, body string) (*Outcome, error) {
	state, err := c.readState(group)
	if err != nil {
		return nil, err
	}
	if state.Status == statusPending {
		return nil, uncertain(fmt.Errorf("%w: 上次操作尚未确认", errAutomation))
	}
	digest := hash(body)
	if state.Status == StatusSent && state.Digest == digest {
		return &Outcome{Status: StatusSent, Message: "相同消息已在本机微信中确认发送，本次未重复发送"}, nil
	}
	button, before, err := c.prepareDraft(group, body)
	if err != nil {
		return nil, err
	}
	// Persist BEFORE touching the send button, including across a killed process.
	err = c.writeState(group, receipt{Status: statusPending, Digest: digest, At: time.Now()})
	if err != nil {
		return nil, err
	}
	_, err = c.driver.call(request{Op: "send", Node: &button, Group: group, Body: body})
	if err != nil {
		return nil, uncertain(err)
	}
	_, err = c.waitFor(func(nodes []node) bool {
		input, e := composer(nodes)

		return e == nil && input.Value == "" && verifiedHeader(nodes, group) && messageCount(nodes, body) > before
	})
	if err != nil {
		return nil, uncertain(err)
	}
	err = c.writeState(group, receipt{Status: StatusSent, Digest: digest, At: time.Now()})
	if err != nil {
		return nil, uncertain(err)
	}

	return &Outcome{Status: StatusSent, Message: "已在本机微信聊天中确认消息；不代表对方已读或服务端送达回执"}, nil
}

var (
	markdownLink = regexp.MustCompile(`!?\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	heading      = regexp.MustCompile(`(?m)^#{1,6}\s+`)
)

// PlainText retains article titles, links and line breaks without Markdown syntax.
func PlainText(markdown string) string {
	text := strings.ReplaceAll(markdown, "\r\n", "\n")
	text = markdownLink.ReplaceAllString(text, "$1\n$2")
	text = heading.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "**", "")

	return strings.TrimSpace(text)
}

func (c *client) prepareDraft(group, body string) (node, int, error) {
	err := c.locate(group)
	if err != nil {
		return node{}, 0, err
	}
	nodes, err := c.snapshot()
	if err != nil {
		return node{}, 0, err
	}
	input, err := composer(nodes)
	if err != nil {
		return node{}, 0, err
	}
	if input.Value != "" {
		return node{}, 0, fmt.Errorf("%w: 目标群已有未发送草稿，请处理草稿后重试；不会覆盖草稿", errAutomation)
	}
	if !verifiedHeader(nodes, group) {
		return node{}, 0, fmt.Errorf("%w: 聊天标题发生变化，已停止发送", errAutomation)
	}
	before := messageCount(nodes, body)
	err = c.act("set", input, body)
	if err != nil {
		return node{}, 0, err
	}
	nodes, err = c.snapshot()
	if err != nil {
		return node{}, 0, err
	}
	input, err = composer(nodes)
	if err != nil {
		return node{}, 0, err
	}
	if input.Value != body || !verifiedHeader(nodes, group) {
		return node{}, 0, fmt.Errorf("%w: 消息写入或目标群校验失败，已停止；请检查微信草稿", errAutomation)
	}
	button, err := choose(nodes, func(n node) bool { return n.Role == roleButton && pressable(n) && label(n, "发送", "Send") }, "发送按钮")
	if err != nil {
		return node{}, 0, err
	}

	return button, before, nil
}
