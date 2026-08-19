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

package diagnose

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNoReport marks an answer that carries no json report at all - the agent
// talked, but not in the shape the contract asked for.
var ErrNoReport = errors.New("diagnosis carries no json report")

// Report is what one diagnosis concluded.
type Report struct {
	// Fixed says the agent believes the proposed changes repair the source, and
	// that it verified them with try_parse. It is the agent's own claim;
	// informer re-verifies it before a person is offered the change.
	Fixed bool `json:"fixed"`

	// Diagnosis states what went wrong, in the user's language.
	Diagnosis string `json:"diagnosis"`

	// Changes are the proposed edits, absent when nothing can be repaired.
	Changes *Changes `json:"changes"`

	// Advice is what the user should do when informer cannot fix it - a source
	// that moved behind a login, a site that shut down, a feed that no longer
	// exists. It is the honest answer that keeps a failed diagnosis useful.
	Advice string `json:"advice"`
}

// promptTemplate is the whole instruction of a diagnosis run.
//
// It states the job, the tools, the loop and the output shape. The tools are
// named rather than described: their own descriptions travel with them, and
// repeating those here is how the two drift apart.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
const promptTemplate = `你是 informer 的订阅诊断助手。informer 是一个 RSS / 网页订阅聚合工具，
它按订阅配置抓取网页并解析出文章列表。现在订阅 #%d「%s」解析失败了，请你找出原因并尝试修复它的配置。

可用工具（前缀 mcp__%s__）：
- %s：读取订阅当前配置和失败原因，先调用它。
- %s：按 informer 自己的方式抓取页面原文。页面可能很大，用 contains 搜索关键内容比翻页高效。
- %s：用候选配置真实试跑一次解析，可以反复调用。

工作方式：
1. 先读配置和错误，再抓页面原文，看清楚页面现在的真实结构。
2. 提出候选配置，用 %s 试跑；不对就继续调整，直到解析出合理的文章列表。
3. 判断标准不只是「有结果」：标题要是真实的文章标题（不是导航栏、广告、分页按钮），
   链接要是能打开的文章地址。解析出 0 条，或者解析出的是无关内容，都算没修好。

解析类型说明：
- feed：标准 RSS / Atom，只需要 url 正确。
- regex：用 regex 在页面原文上匹配，title_exp / url_exp 用 $1 $2 引用捕获组。
- json：把响应当 JSON，用 json_title_path / json_url_path 取值，路径用 / 分隔，数组段写成 items[]。
- agent：由 AI 自己去找，只有 agent_prompt 有意义。

你可以修改的字段只有：%s。
其它字段（标题、分类、权重、启用状态等）不在你的职责范围内，不要提出改动。

重要约束：
- 你没有保存配置的能力，也不需要有。你只负责给出结论和建议的改动，informer 会自己复核，
  再由用户确认是否应用。
- 不要编造。如果页面需要登录、站点已经关闭、或者你确实修不好，如实说明并给出建议。
- 除了上面三个工具，不要用其它方式访问这个页面：informer 的抓取带着订阅自己的请求头和代理，
  换一种抓法看到的内容不一样，据此做的判断是错的。
`

// outputContract is appended to the instruction and is the only part of the
// prompt informer writes about shape rather than about the job.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
const outputContract = `
---
以上是本次任务。以下是 informer 追加的输出格式要求，必须严格遵守：

1. 最终答复只输出一个 JSON 对象，不要输出任何解释、前言、总结或 Markdown 代码块标记。
2. JSON 对象的结构固定为：
{"fixed":true,"diagnosis":"问题原因，一两句话说清楚","changes":{"regex":"新的正则"},"advice":""}
3. fixed：你是否已经用 %s 验证过这套改动能正确解析出文章。没验证过就填 false。
4. diagnosis：必填，用中文说明失败原因，让不懂正则的人也能看懂。
5. changes：只放需要修改的字段，没有改动的字段不要出现；确实修不好时填 null。
6. advice：修不好时必填，说明用户可以怎么办（例如换一个订阅地址、改用 agent 类型、或者删掉这个订阅）；
   修好了可以留空。`

// BuildPrompt assembles the instruction of one diagnosis run.
func BuildPrompt(session *Session) string {
	instruction := fmt.Sprintf(promptTemplate,
		session.Source.ID,
		session.Source.Title,
		ServerName,
		toolGetSource,
		toolFetchContent,
		toolTryParse,
		toolTryParse,
		strings.Join(RepairableFields(), "、"),
	)

	return instruction + fmt.Sprintf(outputContract, toolTryParse)
}

// ParseReport reads the report out of an agent answer.
//
// The contract asks for a bare object, but a model that wraps it in a code fence
// or prefixes a sentence still answered the question: the shape is recovered
// rather than the whole run thrown away over punctuation.
func ParseReport(raw string) (*Report, error) {
	document := extractJSONObject(stripCodeFence(raw))
	if document == "" {
		return nil, fmt.Errorf("%w: %s", ErrNoReport, truncate(raw, maxQuotedRunes))
	}

	var report Report

	err := json.Unmarshal([]byte(document), &report)
	if err != nil {
		return nil, fmt.Errorf("parse diagnosis json: %w", err)
	}

	report.Diagnosis = strings.TrimSpace(report.Diagnosis)
	report.Advice = strings.TrimSpace(report.Advice)

	if report.Changes.IsEmpty() {
		// an empty object and an absent one both mean "nothing to change"; one
		// shape downstream is one branch fewer everywhere it is read.
		report.Changes = nil
		report.Fixed = false
	}

	return &report, nil
}

// maxQuotedRunes bounds the answer text quoted back into an error.
const maxQuotedRunes = 400

// stripCodeFence removes the ``` wrapper a model may put around its json.
func stripCodeFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	newline := strings.IndexByte(text, '\n')
	if newline < 0 {
		return ""
	}

	text = text[newline+1:]

	if closing := strings.LastIndex(text, "```"); closing >= 0 {
		text = text[:closing]
	}

	return strings.TrimSpace(text)
}

// extractJSONObject returns the outermost json object inside the text, so a
// stray sentence before or after the answer does not cost the whole diagnosis.
func extractJSONObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}

	end := strings.LastIndexByte(text, '}')
	if end <= start {
		return ""
	}

	return text[start : end+1]
}

// truncate shortens text to at most limit runes, cutting on rune boundaries so a
// chinese answer is never sliced into invalid utf-8.
func truncate(text string, limit int) string {
	trimmed := strings.TrimSpace(text)

	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}

	return string(runes[:limit]) + "..."
}
