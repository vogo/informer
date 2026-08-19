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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/httpx"
	"github.com/vogo/informer/internal/mcp"
)

// Tool names, without the client applied mcp__informer__ prefix.
const (
	toolGetSource    = "get_source"
	toolFetchContent = "fetch_content"
	toolTryParse     = "try_parse"
)

// toolNames is the offered set in presentation order; AllowedTools reads from it
// so an added tool is pre-approved by the same edit that adds it.
//
//nolint:gochecknoglobals //ordered constant set.
var toolNames = []string{toolGetSource, toolFetchContent, toolTryParse}

// Json Schema vocabulary used by the tool schemas below.
const (
	schemaObject      = "object"
	schemaString      = "string"
	schemaInteger     = "integer"
	schemaBoolean     = "boolean"
	schemaProperties  = "properties"
	schemaType        = "type"
	schemaDescription = "description"
)

// Bounds of what one tool call hands back.
const (
	// defaultFetchLength is how much of the document one fetch_content call
	// returns when the caller names no length. It is a few screens of html:
	// enough to recognize the shape of a listing, small enough that reading the
	// whole page costs several deliberate calls rather than one context flood.
	defaultFetchLength = 4000

	// maxFetchLength bounds one fetch_content call however much is asked for.
	maxFetchLength = 20000

	// contextRunes is how much text surrounds each hit of a contains search.
	contextRunes = 500

	// maxMatches bounds how many hits one contains search reports.
	maxMatches = 5

	// sampleArticles is how many parsed articles a try_parse call quotes back.
	// A handful is enough to tell "the regex matches" from "the regex matches
	// the navigation bar".
	sampleArticles = 8
)

// ErrNoContent marks a fetch that returned nothing to look at.
var ErrNoContent = errors.New("the source returned an empty document")

// Tools builds the tool set of one diagnosis session.
//
// Every tool is read only or throw away: two of them read, and the third parses
// a candidate configuration in memory. None of them can write a source, and that
// is the guarantee the whole feature rests on - a person applies the fix.
func Tools(session *Session) []mcp.Tool {
	// the proxy of the parent applies to the child's own fetches too, so the
	// document the agent reads is the one informer would have read.
	if session.HTTPProxy != "" {
		// a failure only means the fetches go out directly, which is still a
		// diagnosis; there is no window here to report it to.
		_ = httpx.SetProxy(session.HTTPProxy)
	}

	cache := &contentCache{session: session}

	return []mcp.Tool{
		{
			Name:        toolGetSource,
			Description: descGetSource,
			InputSchema: map[string]any{schemaType: schemaObject, schemaProperties: map[string]any{}},
			Handler: func(context.Context, json.RawMessage) (string, error) {
				return renderSource(session), nil
			},
		},
		{
			Name:        toolFetchContent,
			Description: descFetchContent,
			InputSchema: fetchContentSchema(),
			Handler:     cache.fetchHandler,
		},
		{
			Name:        toolTryParse,
			Description: descTryParse,
			InputSchema: tryParseSchema(),
			Handler:     tryParseHandler(session),
		},
	}
}

// Tool descriptions. They are the only documentation the agent gets, so they say
// what the tool is for rather than what it is called.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
const (
	descGetSource = "读取当前订阅的完整配置，以及它最近一次抓取失败的错误信息。诊断的第一步应该调用它。"

	descFetchContent = "按 informer 自己的抓取方式（含自定义 curl 请求头与代理）获取页面原文，" +
		"返回其中一段。用 contains 定位关键内容（例如某篇已知文章的标题），比从头翻页高效得多。" +
		"这是判断「页面格式变了」的唯一可靠依据：不要用其它方式抓取该页面，抓到的字节会不一样。"

	descTryParse = "用一份候选配置真实执行一次解析，返回解析出的条数与样例。" +
		"只传要修改的字段，未传的字段沿用当前配置。这不会写入任何数据，可以反复调用直到解析正确。"
)

// fetchContentSchema describes the arguments of fetch_content.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func fetchContentSchema() map[string]any {
	return map[string]any{
		schemaType: schemaObject,
		schemaProperties: map[string]any{
			"offset": property(schemaInteger, "从第几个字符开始返回，默认 0"),
			"length": property(schemaInteger, fmt.Sprintf("返回多少个字符，默认 %d，最大 %d",
				defaultFetchLength, maxFetchLength)),
			"contains": property(schemaString,
				fmt.Sprintf("给定关键字时忽略 offset/length，返回最多 %d 处匹配及其前后各 %d 个字符",
					maxMatches, contextRunes)),
			fieldURL: property(schemaString,
				"可选：改抓这个地址而不是订阅当前的地址，用于验证「订阅源换了地址」的猜测"),
			"refresh": property(schemaBoolean, "true 时忽略缓存重新抓取，默认复用本次会话已抓到的内容"),
		},
	}
}

// tryParseSchema describes the arguments of try_parse: the repairable fields,
// each optional.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func tryParseSchema() map[string]any {
	properties := map[string]any{}

	for _, entry := range fieldTable {
		kind := schemaString
		if entry.boolP != nil {
			kind = schemaBoolean
		}

		properties[entry.name] = map[string]any{schemaType: kind}
	}

	properties[fieldParseType] = map[string]any{
		schemaType: schemaString,
		"enum":     feed.LegalParseTypes(),
	}

	return map[string]any{
		schemaType:        schemaObject,
		schemaProperties:  properties,
		schemaDescription: "只填写要改动的字段",
	}
}

// property is one json schema leaf: a type and what it is for.
func property(kind, description string) map[string]any {
	return map[string]any{schemaType: kind, schemaDescription: description}
}

// contentCache holds the fetched document for the length of one session, so an
// agent that reads a page in six windows fetches it once. A page that changed
// between two calls would also make every earlier offset meaningless.
type contentCache struct {
	session *Session

	mu       sync.Mutex
	byURL    map[string]string
	fetchErr map[string]error
}

// fetchArgs is what one fetch_content call asks for.
type fetchArgs struct {
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	Contains string `json:"contains"`
	URL      string `json:"url"`
	Refresh  bool   `json:"refresh"`
}

// fetchHandler answers one fetch_content call.
func (c *contentCache) fetchHandler(_ context.Context, arguments json.RawMessage) (string, error) {
	var args fetchArgs

	err := decodeArgs(arguments, &args)
	if err != nil {
		return "", err
	}

	content, err := c.content(strings.TrimSpace(args.URL), args.Refresh)
	if err != nil {
		return "", err
	}

	runes := []rune(content)
	if len(runes) == 0 {
		return "", ErrNoContent
	}

	if args.Contains != "" {
		return searchContent(runes, args.Contains), nil
	}

	return sliceContent(runes, args.Offset, args.Length), nil
}

// content returns the document, fetching it at most once per address per session
// unless the caller asks for a refresh.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func (c *contentCache) content(link string, refresh bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.byURL == nil {
		c.byURL = map[string]string{}
		c.fetchErr = map[string]error{}
	}

	if !refresh {
		if cached, found := c.byURL[link]; found {
			return cached, nil
		}

		// a source that cannot be reached fails the same way every time; saying
		// so from the cache keeps a retry loop from hammering a dead host.
		if failure, found := c.fetchErr[link]; found {
			return "", failure
		}
	}

	source := c.session.Source
	if link != "" {
		// an explicit address is fetched plainly: a curl line carries the
		// address inside itself, and running it would fetch the old page.
		source = &feed.Source{URL: link}
	}

	data, err := feed.FetchSourceContent(source, nil)
	if err != nil {
		c.fetchErr[link] = fmt.Errorf("抓取失败：%w", err)

		return "", c.fetchErr[link]
	}

	c.byURL[link] = string(data)

	return c.byURL[link], nil
}

// sliceContent returns one window of the document, stating where it sits so the
// agent can ask for the next one.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func sliceContent(runes []rune, offset, length int) string {
	total := len(runes)

	if offset < 0 {
		offset = 0
	}

	if offset >= total {
		return fmt.Sprintf("文档共 %d 字符，偏移 %d 已越过结尾。", total, offset)
	}

	if length <= 0 {
		length = defaultFetchLength
	}

	if length > maxFetchLength {
		length = maxFetchLength
	}

	end := min(offset+length, total)

	var out strings.Builder

	fmt.Fprintf(&out, "文档共 %d 字符，本段 [%d, %d)：\n", total, offset, end)
	out.WriteString(string(runes[offset:end]))

	if end < total {
		fmt.Fprintf(&out, "\n\n（后面还有 %d 字符，用 offset=%d 继续读）", total-end, end)
	}

	return out.String()
}

// searchContent returns the neighborhood of each hit of a keyword. It is what
// turns "find where the article titles live now" from a paging exercise into one
// call.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func searchContent(runes []rune, needle string) string {
	content := string(runes)

	var (
		out   strings.Builder
		found int
		from  int
		total = len(runes)
	)

	fmt.Fprintf(&out, "文档共 %d 字符，搜索 %q：\n", total, needle)

	for found < maxMatches {
		at := strings.Index(content[from:], needle)
		if at < 0 {
			break
		}

		// byte offsets from strings.Index are converted to rune offsets so the
		// windows below never cut a chinese character in half.
		hit := len([]rune(content[:from+at]))
		start := max(hit-contextRunes, 0)
		end := min(hit+len([]rune(needle))+contextRunes, total)

		found++

		fmt.Fprintf(&out, "\n--- 第 %d 处，位置 %d ---\n%s\n", found, hit, string(runes[start:end]))

		from += at + len(needle)
	}

	if found == 0 {
		return fmt.Sprintf("文档共 %d 字符，没有找到 %q。用 offset/length 直接翻看原文确认页面结构。",
			total, needle)
	}

	if strings.Contains(content[from:], needle) {
		fmt.Fprintf(&out, "\n（还有更多匹配，只显示了前 %d 处）", maxMatches)
	}

	return out.String()
}

// tryParseHandler answers one try_parse call: apply the candidate fields to the
// snapshot, parse for real, and report what came out.
func tryParseHandler(session *Session) mcp.Handler {
	return func(_ context.Context, arguments json.RawMessage) (string, error) {
		var changes Changes

		err := decodeArgs(arguments, &changes)
		if err != nil {
			return "", err
		}

		var out strings.Builder

		writeCandidate(&out, &changes, session.Source)

		// a trial parse is bounded by the http client's own deadline, the same
		// one every fetch runs under; it takes no context of its own.
		return tryParseResult(&out, changes.Apply(session.Source)), nil //nolint:contextcheck //http client deadline.
	}
}

// writeCandidate states what the trial changed, so a model reading its own
// answer back can tell two nearly identical attempts apart.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func writeCandidate(out *strings.Builder, changes *Changes, source *feed.Source) {
	diff := changes.Diff(source)
	if len(diff) == 0 {
		out.WriteString("本次没有改动任何字段，解析的是当前配置。\n")

		return
	}

	out.WriteString("本次改动：\n")

	for _, entry := range diff {
		fmt.Fprintf(out, "- %s: %q -> %q\n", entry.Field, entry.Old, entry.New)
	}
}

// tryParseResult parses the candidate for real and renders the outcome.
//
// "Parsed nothing" is reported as a distinct outcome from "failed": a rule that
// matches an empty result is the usual half-fixed state, and calling it success
// is what would end the loop one step early.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func tryParseResult(out *strings.Builder, candidate *feed.Source) string {
	parseType := candidate.ResolveParseType()

	fmt.Fprintf(out, "解析类型：%s\n", parseType)

	// an agent source would start another agent process from inside this one;
	// the nesting is refused rather than run, and repairing such a source means
	// rewriting its prompt, which needs no trial run.
	if parseType == feed.ParseTypeAgent {
		out.WriteString("agent 类型的订阅不支持试跑（会在当前 agent 内再启动一个 agent），" +
			"请直接依据配置与页面内容判断如何修改提示词。")

		return out.String()
	}

	articles, err := feed.ParseArticles(candidate, nil, nil)
	if err != nil {
		fmt.Fprintf(out, "解析失败：%v", err)

		return out.String()
	}

	fmt.Fprintf(out, "解析成功，共 %d 条", len(articles))

	if len(articles) == 0 {
		out.WriteString("。解析没有报错但一条都没有取到，说明规则匹配到了空结果，还需要继续调整。")

		return out.String()
	}

	out.WriteString("，前几条：\n")

	for i, article := range articles {
		if i >= sampleArticles {
			fmt.Fprintf(out, "...（还有 %d 条）", len(articles)-sampleArticles)

			break
		}

		fmt.Fprintf(out, "%d. %s | %s\n", i+1, article.Title, article.URL)
	}

	return out.String()
}

// renderSource states the configuration and the failure in the plain text the
// agent reads best.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func renderSource(session *Session) string {
	source := session.Source

	var out strings.Builder

	fmt.Fprintf(&out, "订阅 #%d「%s」\n", source.ID, source.Title)
	fmt.Fprintf(&out, "当前生效的解析类型：%s\n\n", source.ResolveParseType())
	out.WriteString("配置字段：\n")

	for _, entry := range fieldTable {
		if entry.stringP != nil {
			fmt.Fprintf(&out, "- %s = %q\n", entry.name, entry.getStr(source))

			continue
		}

		fmt.Fprintf(&out, "- %s = %t\n", entry.name, entry.getBool(source))
	}

	if session.StoredError != "" {
		fmt.Fprintf(&out, "\n上次真实抓取记录的错误：\n%s\n", session.StoredError)
	}

	if session.FreshError != "" {
		fmt.Fprintf(&out, "\n本次诊断开始前重试一次，仍然失败：\n%s\n", session.FreshError)
	} else {
		out.WriteString("\n本次诊断开始前重试了一次，居然成功了：可能是间歇性故障，" +
			"请先用 try_parse 复核当前配置，如果稳定成功就不要改动配置。\n")
	}

	return out.String()
}

// decodeArgs reads a tool's arguments, treating an absent object as an empty
// one: a model that calls a tool with no arguments at all means the defaults.
func decodeArgs(arguments json.RawMessage, into any) error {
	trimmed := strings.TrimSpace(string(arguments))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	err := json.Unmarshal(arguments, into)
	if err != nil {
		return fmt.Errorf("参数格式不对：%w", err)
	}

	return nil
}
