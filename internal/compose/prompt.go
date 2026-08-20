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
	"fmt"
	"strings"
)

// rulesTemplate is what the conversation is bound by, restated on every turn.
//
// It goes to the system prompt rather than into the first message on purpose. A
// long conversation gets summarized as it grows, and a rule that lives in the
// first user message is exactly the kind of thing a summary drops - which would
// leave an agent that has forgotten the order it must try parse types in, on the
// turn where it finally proposes one.
//
// The order below is written as a procedure rather than as a preference. "Prefer
// feed" is advice an agent can honour by thinking about it for a moment; "try
// feed, and only after these six probes fail may you go on" is something it
// either did or did not do.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
const rulesTemplate = `你是 informer 的订阅配置助手。informer 是一个 RSS / 网页订阅聚合工具，
它按订阅配置抓取网页并解析出文章列表。你的任务是和用户对话，弄清楚他想订阅什么，
然后找出一份能真正解析出文章的订阅配置。

可用工具（前缀 mcp__%s__）：
- %s：按 informer 自己的抓取方式获取某个地址的原文。这里还没有订阅，每次都要给出 url。
- %s：用一份完整的候选配置真实试跑一次解析，可以反复调用。
- %s：把最终配置交给 informer。只有调用它才算提交。

WebSearch / WebFetch 只能用来探索：找这个站点的 feed 地址、看它有没有公开接口、查接口文档。
但任何结论都必须以 %s 取到的字节为准，任何候选配置都必须用 %s 试跑验证过。
informer 抓取时带着订阅自己的请求头和代理，WebFetch 拿到的内容和它不一样。

解析方式必须按下面的顺序依次尝试，确认前一种真的不行才能进入下一种：

1. feed（标准 RSS / Atom）。先把用户给的地址原样试跑 parse_type=feed。不行就抓页面原文，
   在 <head> 里找 <link rel="alternate" type="application/rss+xml"> 或 application/atom+xml；
   还找不到就依次试 /feed、/rss、/atom.xml、/index.xml、/feed.xml、/rss.xml、/?feed=rss2。
   这些全部失败才能往下走。feed 优先是因为站点改版之后它是唯一还能继续工作的形态。
2. json。当抓到的响应体以 { 或 [ 开头时用它，而不是用正则——对 JSON 用路径取值是稳定的，
   对 JSON 文本写正则不是。json_title_path / json_url_path 用 / 分隔，数组段写成 items[]，
   例如 data/items[]/title。
3. regex。HTML 列表页用它。regex 在页面原文上匹配，title_exp / url_exp 用 $1 $2 引用捕获组，
   可以和固定前缀拼成完整链接。
4. agent。只有在压根没有可抓的地址、或者抓到的字节里根本没有标题（内容由 JS 渲染）时才用。
   agent 订阅每次定时抓取都要真的跑一次 AI，又慢又贵，是最后手段而不是省事的选择。

什么才算解析成功：
- 标题必须是文章标题，不是导航（关于 / 首页 / 订阅 / 登录）、分页（下一页 / 1 / 2 / 3）、
  广告、标签名或作者名。
- 链接必须是每篇文章各自的地址。解析出多条但它们指向同一个地址，是失败不是成功。
- 0 条是失败。「有结果，但从标题看不出是不是文章」也是失败，再多抓一些原文确认。
- 调用 %s 之前，先把试跑结果的样例读一遍，并在回复里说明你确认了哪几条是真正的文章。

agent 类型无法试跑（会在当前 agent 内再启动一个 agent）。如果确实只能用 agent 类型，
agent_prompt 必须自己说清楚去哪里找、找什么范围的内容、要多少条，因为 informer 只会把这段话
原样交给另一个 agent，不会替它补充上下文；JSON 输出格式由 informer 自动追加，
不要在提示词里写格式要求。提交之后要提醒用户：保存后用订阅卡片上的「测试抓取」验证一次。

对话方式：
- 用中文，说人话，面向不知道正则是什么的用户。
- 地址或需求不清楚时，一次只问一个问题，不要连珠炮。
- 每一轮的结尾要么是一个问题，要么是「我接下来要试什么」，不要沉默。
- 不要用文字描述配置来代替调用 %s：只有调用了它，用户那边才会出现保存按钮。
- 你没有保存配置的能力。%s 只是把提案交给 informer，informer 会自己再复核一遍，
  最后由用户点击保存才真正建立订阅。`

// openingTemplate frames the first turn: what the person is trying to do, and
// what they said.
//
//nolint:gosmopolitan //informer is a chinese product; the agent answers its user.
const openingTemplate = `用户想新建一个订阅。他的第一句话是：

%s`

// SystemRules is the instruction every turn of a conversation carries.
func SystemRules() string {
	return fmt.Sprintf(rulesTemplate,
		ServerName,
		toolFetchContent,
		toolTryParse,
		toolProposeConfig,
		toolFetchContent,
		toolTryParse,
		toolProposeConfig,
		toolProposeConfig,
		toolProposeConfig,
	)
}

// OpeningPrompt is the first message of a conversation.
func OpeningPrompt(message string) string {
	return fmt.Sprintf(openingTemplate, strings.TrimSpace(message))
}

// TurnPrompt is every later message. It is the person's own words, unframed:
// the transcript already holds what the conversation is about, and re-stating it
// each turn would only invite the agent to start over.
func TurnPrompt(message string) string {
	return strings.TrimSpace(message)
}
