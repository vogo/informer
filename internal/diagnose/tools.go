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
	"fmt"
	"strings"

	"github.com/vogo/informer/internal/httpx"
	"github.com/vogo/informer/internal/mcp"
	"github.com/vogo/informer/internal/parsecfg"
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

// ErrNoContent marks a fetch that returned nothing to look at.
var ErrNoContent = parsecfg.ErrNoContent

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

	return []mcp.Tool{
		{
			Name:        toolGetSource,
			Description: descGetSource,
			InputSchema: map[string]any{parsecfg.SchemaType: parsecfg.SchemaObject, parsecfg.SchemaProperties: map[string]any{}},
			Handler: func(context.Context, json.RawMessage) (string, error) {
				return renderSource(session), nil
			},
		},
		{
			Name:        toolFetchContent,
			Description: descFetchContent,
			InputSchema: parsecfg.FetchContentSchema(descFetchURL),
			Handler:     parsecfg.NewContentCache(session.Source).Handler,
		},
		{
			Name:        toolTryParse,
			Description: descTryParse,
			InputSchema: parsecfg.TryParseSchema(descTryParseFields),
			Handler:     parsecfg.TryParseHandler(session.Source),
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
		"判断「页面格式变了」必须以这里取到的字节为准：WebFetch 等其它抓法拿到的内容不一样。"

	descFetchURL = "可选：改抓这个地址而不是订阅当前的地址，用于验证「订阅源换了地址」的猜测"

	descTryParse = "用一份候选配置真实执行一次解析，返回解析出的条数与样例。" +
		"只传要修改的字段，未传的字段沿用当前配置。这不会写入任何数据，可以反复调用直到解析正确。"

	descTryParseFields = "只填写要改动的字段"
)

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

	for _, value := range parsecfg.Describe(source) {
		fmt.Fprintf(&out, "- %s = %s\n", value.Field, value.Text)
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
