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

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/httpx"
	"github.com/vogo/informer/internal/mcp"
	"github.com/vogo/informer/internal/parsecfg"
)

// Tool names, without the client applied mcp__informer__ prefix.
const (
	toolFetchContent  = "fetch_content"
	toolTryParse      = "try_parse"
	toolProposeConfig = "propose_config"
)

// toolNames is the offered set in presentation order; AllowedTools reads from it
// so an added tool is pre-approved by the same edit that adds it.
//
//nolint:gochecknoglobals //ordered constant set.
var toolNames = []string{toolFetchContent, toolTryParse, toolProposeConfig}

// Extra properties of propose_config, beyond the parse configuration itself.
const (
	fieldTitle  = "title"
	fieldReason = "reason"
)

// Tools builds the tool set of one composing conversation.
//
// Three tools: read a page the way informer reads it, try a candidate for real,
// and hand over the one that worked. None of them writes to the database. The
// last one writes a file the parent reads, which is as close to saving as this
// process gets - and the parent verifies it again before anyone sees a button.
func Tools(session *Session) []mcp.Tool {
	// the proxy of the parent applies to the child's own fetches too, so the
	// document the agent reads is the one informer would have read.
	if session.HTTPProxy != "" {
		// a failure only means the fetches go out directly, which is still a
		// conversation; there is no window here to report it to.
		_ = httpx.SetProxy(session.HTTPProxy)
	}

	// there is no source yet, so a fetch has nothing to fall back on and a trial
	// has nothing to inherit. Both start from an empty draft, which is what
	// makes every call here carry everything it needs.
	draft := &feed.Source{}

	return []mcp.Tool{
		{
			Name:        toolFetchContent,
			Description: descFetchContent,
			InputSchema: parsecfg.FetchContentSchema(descFetchURL),
			Handler:     parsecfg.NewContentCache(draft).Handler,
		},
		{
			Name:        toolTryParse,
			Description: descTryParse,
			InputSchema: parsecfg.TryParseSchema(descTryParseFields),
			Handler:     parsecfg.TryParseHandler(draft),
		},
		{
			Name:        toolProposeConfig,
			Description: descProposeConfig,
			InputSchema: proposeSchema(),
			Handler:     proposeHandler(session),
		},
	}
}

// Tool descriptions. They are the only documentation the agent gets, so they say
// what the tool is for rather than what it is called.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
const (
	descFetchContent = "按 informer 自己的抓取方式（含代理）获取一个地址的原文，返回其中一段。" +
		"用 contains 定位关键内容比从头翻页高效得多。" +
		"判断页面结构必须以这里取到的字节为准：WebFetch 等其它抓法拿到的内容不一样。"

	descFetchURL = "要抓取的地址。这里还没有订阅，所以每次调用都必须给出它。"

	descTryParse = "用一份候选配置真实执行一次解析，返回解析出的条数与样例。" +
		"这里还没有订阅，每次调用都必须给出完整配置（至少 url 与 parse_type 及其参数），" +
		"不会沿用上一次调用的内容。这不会写入任何数据，可以反复调用直到解析正确。"

	descTryParseFields = "一份完整的候选配置，不是增量改动"

	descProposeConfig = "把最终配置交给 informer。只有调用它才算提交：" +
		"在回复里用文字描述配置不等于提交，用户那边不会出现保存按钮。" +
		"informer 会自己再跑一次解析核对，解析不出文章的配置会被当场拒绝并告诉你原因。"
)

// Argument descriptions of propose_config.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
const (
	descTitle = "订阅标题，展示给用户看的名字，例如「阮一峰的网络日志」"

	descReason = "一句话说明为什么是这套配置（例如「站点提供 Atom feed」「列表页用正则取标题和链接」），" +
		"会展示给用户"

	descProposeFields = "完整的订阅配置。必须是你已经用 try_parse 验证过的那一份"
)

// proposeSchema describes the arguments of propose_config: the parse
// configuration, plus the two things only this tool asks for.
func proposeSchema() map[string]any {
	schema := parsecfg.TryParseSchema(descProposeFields)

	properties, ok := schema[parsecfg.SchemaProperties].(map[string]any)
	if !ok {
		// TryParseSchema builds this map itself; the branch exists so a future
		// change there fails loudly here rather than dropping two properties.
		properties = map[string]any{}
		schema[parsecfg.SchemaProperties] = properties
	}

	properties[fieldTitle] = parsecfg.Property(parsecfg.SchemaString, descTitle)
	properties[fieldReason] = parsecfg.Property(parsecfg.SchemaString, descReason)

	schema["required"] = []string{fieldTitle, parsecfg.FieldParseType}

	return schema
}

// proposeArgs is what one propose_config call carries: a whole configuration,
// flattened next to the two fields only this tool asks for.
type proposeArgs struct {
	parsecfg.Changes

	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// proposeHandler answers one propose_config call.
//
// It is the one place in this package that judges rather than reports. The
// judgement is deliberately the same one informer will make again in the parent
// process, stated here so the agent finds out now - while it can still fix the
// configuration - rather than after the conversation has ended.
func proposeHandler(session *Session) mcp.Handler {
	return func(_ context.Context, arguments json.RawMessage) (string, error) {
		var args proposeArgs

		err := parsecfg.DecodeArgs(arguments, &args)
		if err != nil {
			return "", err
		}

		// the trial inside runs under the http client's own deadline, the same
		// one every fetch runs under; it takes no context of its own.
		return reviewProposal(session, &args), nil //nolint:contextcheck //http client deadline.
	}
}

// reviewProposal decides whether a proposal is worth recording and says why.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func reviewProposal(session *Session, args *proposeArgs) string {
	title := strings.TrimSpace(args.Title)
	if title == "" {
		return "拒绝：没有给出 title。请给这个订阅起一个展示给用户看的名字。"
	}

	changes := args.Changes
	if changes.IsEmpty() {
		return "拒绝：没有给出任何配置字段。请把完整配置一次性传进来。"
	}

	candidate := changes.Apply(&feed.Source{})

	refusal := trialRefusal(candidate)
	if refusal != "" {
		return refusal
	}

	err := WriteProposal(session.Dir, &Proposal{Title: title, Reason: strings.TrimSpace(args.Reason), Changes: &changes})
	if err != nil {
		return fmt.Sprintf("提案没能记录下来：%v。请稍后再试一次。", err)
	}

	if candidate.ResolveParseType() == feed.ParseTypeAgent {
		return "已记录这份 agent 类型的提案。它无法在这里试跑，" +
			"请把这一点如实告诉用户，并提醒他保存之后用「测试抓取」验证一次。"
	}

	return "已记录提案，informer 会自己再复核一次并把保存按钮给用户。" +
		"请在回复里告诉用户你配的是哪种解析方式、以及你确认过哪几条是真正的文章。"
}

// trialRefusal parses the candidate and returns why it is not acceptable, or an
// empty string when it is.
//
// An agent candidate is accepted without a trial: running one would start
// another agent process from inside this one. What it is not excused from is
// carrying a prompt, since a prompt is the entire configuration of that type.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func trialRefusal(candidate *feed.Source) string {
	if candidate.ResolveParseType() == feed.ParseTypeAgent {
		if strings.TrimSpace(candidate.AgentPrompt) == "" {
			return "拒绝：agent 类型的订阅只有 agent_prompt 有意义，而它是空的。"
		}

		return ""
	}

	result := parsecfg.Trial(candidate)
	if result.Err != nil {
		return fmt.Sprintf("拒绝：这套配置解析失败了：%v。请继续用 try_parse 调整。", result.Err)
	}

	if len(result.Articles) == 0 {
		return "拒绝：这套配置解析出 0 条。规则匹配到了空结果，还需要继续调整。"
	}

	if len(result.Articles) > 1 && result.DistinctURLs() == 1 {
		return fmt.Sprintf("拒绝：解析出的 %d 条指向同一个地址，"+
			"说明规则匹配到的是列表外层而不是每一篇文章。", len(result.Articles))
	}

	return ""
}
