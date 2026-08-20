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

package parsecfg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/mcp"
)

// Json Schema vocabulary used by the tool schemas.
const (
	SchemaObject      = "object"
	SchemaString      = "string"
	SchemaInteger     = "integer"
	SchemaBoolean     = "boolean"
	SchemaProperties  = "properties"
	SchemaType        = "type"
	SchemaDescription = "description"
)

// Bounds of what one tool call hands back.
const (
	// DefaultFetchLength is how much of the document one fetch call returns when
	// the caller names no length. It is a few screens of html: enough to
	// recognize the shape of a listing, small enough that reading the whole page
	// costs several deliberate calls rather than one context flood.
	DefaultFetchLength = 4000

	// MaxFetchLength bounds one fetch call however much is asked for.
	MaxFetchLength = 20000

	// ContextRunes is how much text surrounds each hit of a contains search.
	ContextRunes = 500

	// MaxMatches bounds how many hits one contains search reports.
	MaxMatches = 5

	// SampleArticles is how many parsed articles a trial quotes back. A handful
	// is enough to tell "the regex matches" from "the regex matches the
	// navigation bar".
	SampleArticles = 8
)

// ErrNoContent marks a fetch that returned nothing to look at.
var ErrNoContent = errors.New("the source returned an empty document")

// ErrNoFetchTarget marks a fetch with no address to go to - the caller named
// none and the source it would have fallen back to carries none either.
var ErrNoFetchTarget = errors.New("no address to fetch")

// Property is one json schema leaf: a type and what it is for.
func Property(kind, description string) map[string]any {
	return map[string]any{SchemaType: kind, SchemaDescription: description}
}

// FetchContentSchema describes the arguments of a fetch tool.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func FetchContentSchema(urlDescription string) map[string]any {
	return map[string]any{
		SchemaType: SchemaObject,
		SchemaProperties: map[string]any{
			"offset": Property(SchemaInteger, "从第几个字符开始返回，默认 0"),
			"length": Property(SchemaInteger, fmt.Sprintf("返回多少个字符，默认 %d，最大 %d",
				DefaultFetchLength, MaxFetchLength)),
			"contains": Property(SchemaString,
				fmt.Sprintf("给定关键字时忽略 offset/length，返回最多 %d 处匹配及其前后各 %d 个字符",
					MaxMatches, ContextRunes)),
			FieldURL:  Property(SchemaString, urlDescription),
			"refresh": Property(SchemaBoolean, "true 时忽略缓存重新抓取，默认复用本次会话已抓到的内容"),
		},
	}
}

// TryParseSchema describes the arguments of a trial tool: the parse affecting
// fields, each optional, plus whatever the caller wants to say about how they
// are meant to be filled in.
func TryParseSchema(description string) map[string]any {
	properties := map[string]any{}

	for _, entry := range fieldTable {
		kind := SchemaString
		if entry.boolP != nil {
			kind = SchemaBoolean
		}

		properties[entry.name] = map[string]any{SchemaType: kind}
	}

	properties[FieldParseType] = map[string]any{
		SchemaType: SchemaString,
		"enum":     feed.LegalParseTypes(),
	}

	return map[string]any{
		SchemaType:        SchemaObject,
		SchemaProperties:  properties,
		SchemaDescription: description,
	}
}

// ContentCache holds the fetched documents for the length of one session, so an
// agent that reads a page in six windows fetches it once. A page that changed
// between two calls would also make every earlier offset meaningless.
type ContentCache struct {
	base *feed.Source

	mu       sync.Mutex
	byURL    map[string]string
	fetchErr map[string]error
}

// NewContentCache builds the cache of one session. The base source is what a
// call with no address of its own falls back to, headers and all; it may carry
// no address at all, in which case every call has to name one.
func NewContentCache(base *feed.Source) *ContentCache {
	return &ContentCache{base: base}
}

// fetchArgs is what one fetch call asks for.
type fetchArgs struct {
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	Contains string `json:"contains"`
	URL      string `json:"url"`
	Refresh  bool   `json:"refresh"`
}

// Handler answers one fetch call.
func (c *ContentCache) Handler(_ context.Context, arguments json.RawMessage) (string, error) {
	var args fetchArgs

	err := DecodeArgs(arguments, &args)
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
func (c *ContentCache) content(link string, refresh bool) (string, error) {
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

	source, err := c.target(link)
	if err != nil {
		return "", err
	}

	data, err := feed.FetchSourceContent(source, nil)
	if err != nil {
		c.fetchErr[link] = fmt.Errorf("抓取失败：%w", err)

		return "", c.fetchErr[link]
	}

	c.byURL[link] = string(data)

	return c.byURL[link], nil
}

// target picks what this call fetches: the named address plainly, or the base
// source with everything it carries.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func (c *ContentCache) target(link string) (*feed.Source, error) {
	if link != "" {
		// an explicit address is fetched plainly: a curl line carries the
		// address inside itself, and running it would fetch the other page.
		return &feed.Source{URL: link}, nil
	}

	if c.base == nil || (c.base.URL == "" && c.base.CURL == "") {
		return nil, fmt.Errorf("%w：这里没有默认地址，请在参数里给出 url", ErrNoFetchTarget)
	}

	return c.base, nil
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
		length = DefaultFetchLength
	}

	if length > MaxFetchLength {
		length = MaxFetchLength
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

	for found < MaxMatches {
		at := strings.Index(content[from:], needle)
		if at < 0 {
			break
		}

		// byte offsets from strings.Index are converted to rune offsets so the
		// windows below never cut a chinese character in half.
		hit := len([]rune(content[:from+at]))
		start := max(hit-ContextRunes, 0)
		end := min(hit+len([]rune(needle))+ContextRunes, total)

		found++

		fmt.Fprintf(&out, "\n--- 第 %d 处，位置 %d ---\n%s\n", found, hit, string(runes[start:end]))

		from += at + len(needle)
	}

	if found == 0 {
		return fmt.Sprintf("文档共 %d 字符，没有找到 %q。用 offset/length 直接翻看原文确认页面结构。",
			total, needle)
	}

	if strings.Contains(content[from:], needle) {
		fmt.Fprintf(&out, "\n（还有更多匹配，只显示了前 %d 处）", MaxMatches)
	}

	return out.String()
}

// TryParseHandler answers one trial call: apply the candidate fields to the base
// configuration, parse for real, and report what came out.
func TryParseHandler(base *feed.Source) mcp.Handler {
	return func(_ context.Context, arguments json.RawMessage) (string, error) {
		var changes Changes

		err := DecodeArgs(arguments, &changes)
		if err != nil {
			return "", err
		}

		var out strings.Builder

		WriteCandidate(&out, &changes, base)

		// a trial parse is bounded by the http client's own deadline, the same
		// one every fetch runs under; it takes no context of its own.
		result := Trial(changes.Apply(base)) //nolint:contextcheck //http client deadline.
		WriteTrial(&out, result)

		return out.String(), nil
	}
}

// WriteCandidate states what the trial changed, so a model reading its own
// answer back can tell two nearly identical attempts apart.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func WriteCandidate(out *strings.Builder, changes *Changes, base *feed.Source) {
	diff := changes.Diff(base)
	if len(diff) == 0 {
		out.WriteString("本次没有改动任何字段，解析的是当前配置。\n")

		return
	}

	out.WriteString("本次改动：\n")

	for _, entry := range diff {
		fmt.Fprintf(out, "- %s: %q -> %q\n", entry.Field, entry.Old, entry.New)
	}
}

// TrialResult is what parsing one candidate produced.
type TrialResult struct {
	// ParseType is the type the candidate resolved to.
	ParseType string

	// Skipped marks a candidate that was not run at all - an agent type, which
	// would start another agent process from inside this one.
	Skipped bool

	// Err is why the parse failed, if it did.
	Err error

	// Articles is what came out, in parse order.
	Articles []*feed.Article
}

// Trial parses one candidate configuration for real, writing nothing anywhere.
func Trial(candidate *feed.Source) TrialResult {
	result := TrialResult{ParseType: candidate.ResolveParseType()}

	// an agent source would start another agent process from inside this one;
	// the nesting is refused rather than run.
	if result.ParseType == feed.ParseTypeAgent {
		result.Skipped = true

		return result
	}

	result.Articles, result.Err = feed.ParseArticles(candidate, nil, nil)

	return result
}

// OK reports whether the trial actually produced articles.
func (r TrialResult) OK() bool {
	return !r.Skipped && r.Err == nil && len(r.Articles) > 0
}

// DistinctURLs counts how many different addresses the trial produced. A rule
// that matched the wrapper around the listing yields many rows and one address,
// which reads as success until this is looked at.
func (r TrialResult) DistinctURLs() int {
	seen := make(map[string]bool, len(r.Articles))
	for _, article := range r.Articles {
		seen[article.URL] = true
	}

	return len(seen)
}

// WriteTrial renders the outcome of a trial.
//
// "Parsed nothing" is reported as a distinct outcome from "failed": a rule that
// matches an empty result is the usual half-fixed state, and calling it success
// is what would end the loop one step early.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func WriteTrial(out *strings.Builder, result TrialResult) {
	fmt.Fprintf(out, "解析类型：%s\n", result.ParseType)

	if result.Skipped {
		out.WriteString("agent 类型的订阅不支持试跑（会在当前 agent 内再启动一个 agent），" +
			"请直接依据配置与页面内容判断提示词该怎么写。")

		return
	}

	if result.Err != nil {
		fmt.Fprintf(out, "解析失败：%v", result.Err)

		return
	}

	fmt.Fprintf(out, "解析成功，共 %d 条", len(result.Articles))

	if len(result.Articles) == 0 {
		out.WriteString("。解析没有报错但一条都没有取到，说明规则匹配到了空结果，还需要继续调整。")

		return
	}

	if len(result.Articles) > 1 && result.DistinctURLs() == 1 {
		out.WriteString("，但它们指向同一个地址，说明规则匹配到的是列表外层而不是每一篇文章，还需要继续调整")
	}

	out.WriteString("，前几条：\n")

	for i, article := range result.Articles {
		if i >= SampleArticles {
			fmt.Fprintf(out, "...（还有 %d 条）", len(result.Articles)-SampleArticles)

			break
		}

		fmt.Fprintf(out, "%d. %s | %s\n", i+1, article.Title, article.URL)
	}
}

// DecodeArgs reads a tool's arguments, treating an absent object as an empty
// one: a model that calls a tool with no arguments at all means the defaults.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
func DecodeArgs(arguments json.RawMessage, into any) error {
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
