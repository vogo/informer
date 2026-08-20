<script setup lang="ts">
import {computed, nextTick, onMounted, onUnmounted, ref} from 'vue'
import {
  NAlert,
  NButton,
  NCollapse,
  NCollapseItem,
  NEllipsis,
  NInput,
  NList,
  NListItem,
  NPopconfirm,
  NSelect,
  NSpace,
  NTag,
  NText,
  NTooltip,
  useMessage
} from 'naive-ui'
import type {SelectOption} from 'naive-ui'
import {Browser, Clipboard, Events} from '@wailsio/runtime'
import {CloseCompose, ComposeChat, CreateSourceFromCompose, StartCompose} from './bindings'
import type {ComposeProposalDTO, SourceDTO} from './bindings'
import {errorText} from './errors'
import {compact} from './nulls'
import {renderMarkdown} from './markdown'
import {fieldLabel, fieldValue, parseTypeLabel} from './sourceFields'

const props = defineProps<{
  categoryOptions: SelectOption[]
  defaultCategoryId: number | string | null
  // createCategoryOption and resolveCategory are the parent's, so that typing a
  // new category name here means exactly what it means in the edit form.
  createCategoryOption: (label: string) => SelectOption
  resolveCategory: (raw: number | string | null) => Promise<number>
}>()

const emit = defineEmits<{
  created: [source: SourceDTO]
  busy: [running: boolean]
}>()

const message = useMessage()

// ComposeLogEntry mirrors RunLogDTO in cmd/informer-ui/binding_runlog.go. A
// wails event payload is not part of the generated bindings, so the two shapes
// are kept in step by hand.
interface ComposeLogEntry {
  runId: string
  seq: number
  time: number
  level: 'info' | 'warn' | 'error'
  text: string
}

// COMPOSE_LOG_EVENT is the same constant as ComposeLogEvent on the Go side.
const COMPOSE_LOG_EVENT = 'informer:compose:log'

// MAX_LOGS bounds what the panel keeps, matching the budget the Go sink stops
// at. An unbounded array in a webview is a leak.
const MAX_LOGS = 500

// MAX_TURNS mirrors composeMaxTurns in internal/service/compose.go, so the page
// can warn before the backend refuses rather than after.
const MAX_TURNS = 12

const LOG_PANEL = 'log'

// ChatMessage is one bubble. A proposal belongs to the bubble it arrived with,
// not to the conversation: an earlier configuration stays saveable after a later
// turn proposed a different one.
interface ChatMessage {
  id: number
  role: 'user' | 'agent'
  text: string
  proposal: ComposeProposalDTO | null
}

const sessionId = ref('')
const starting = ref(false)
const sending = ref(false)
const startError = ref('')
const draft = ref('')
const turns = ref(0)
const messages = ref<ChatMessage[]>([])
const categoryId = ref<number | string | null>(props.defaultCategoryId)
const savingId = ref(0)

const runId = ref('')
const logs = ref<ComposeLogEntry[]>([])
const logsDropped = ref(0)
const logExpanded = ref<string[]>([])
const logBox = ref<HTMLElement | null>(null)
const logPinned = ref(true)

const chatBox = ref<HTMLElement | null>(null)

// composing tracks the IME. In a chinese product this is not an edge case: the
// candidate window is open for most of what the user types, and an Enter that
// fired through it would send half a sentence.
const composing = ref(false)

let nextMessageId = 1

const busy = computed(() => starting.value || sending.value)
const canSend = computed(() => !busy.value && sessionId.value !== '' && draft.value.trim() !== '')
const lastLog = computed(() => logs.value.at(-1)?.text ?? '')
const logProblems = computed(() => logs.value.filter(entry => entry.level !== 'info').length)
const turnsLeft = computed(() => MAX_TURNS - turns.value)

const unsub = Events.On(COMPOSE_LOG_EVENT, onLog)

onMounted(() => {
  void start()
})

onUnmounted(() => {
  unsub()
  close()
})

// close ends the conversation on the server so its run directory goes away with
// it. It is fire and forget: the modal is already closing, and a conversation
// that outlives it is swept by the idle timeout anyway.
function close() {
  const id = sessionId.value
  if (id === '') {
    return
  }

  sessionId.value = ''
  void CloseCompose(id).catch(() => undefined)
}

defineExpose({close, busy})

async function start() {
  starting.value = true
  startError.value = ''
  emit('busy', true)

  try {
    sessionId.value = await StartCompose()
  } catch (e) {
    startError.value = errorText(e)
  } finally {
    starting.value = false
    emit('busy', false)
  }
}

async function send() {
  const text = draft.value.trim()
  if (!canSend.value || text === '') {
    return
  }

  // crypto.randomUUID is not guaranteed in a webview served over a custom
  // scheme, and this id only has to be unique among this window's own runs.
  const id = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`

  pushMessage({id: nextMessageId++, role: 'user', text, proposal: null})
  draft.value = ''

  // one run id per turn, so a turn the user gave up on stops writing into the
  // panel. The lines themselves are kept across turns: the whole conversation's
  // log is what makes a wrong proposal debuggable.
  runId.value = id
  logExpanded.value = [LOG_PANEL]
  sending.value = true
  emit('busy', true)

  try {
    const reply = await ComposeChat(sessionId.value, text, id)
    if (!reply) {
      throw new Error('没有收到回复')
    }

    turns.value = reply.turns
    pushMessage({
      id: nextMessageId++,
      role: 'agent',
      text: reply.message,
      proposal: reply.proposal ?? null
    })

    // the answer is what the user came for; the log stays one click away.
    logExpanded.value = []
  } catch (e) {
    pushMessage({
      id: nextMessageId++,
      role: 'agent',
      text: `这一轮失败了：${errorText(e)}`,
      proposal: null
    })
    // it failed, and the log is exactly where the reason is.
    logExpanded.value = [LOG_PANEL]
  } finally {
    if (runId.value === id) {
      sending.value = false
      emit('busy', false)
    }
  }
}

// onEnter sends on a bare Enter and leaves Shift+Enter for a newline, while an
// Enter that closes an IME candidate window does neither.
function onEnter(event: KeyboardEvent) {
  if (composing.value || event.isComposing || event.shiftKey) {
    return
  }

  event.preventDefault()
  void send()
}

function pushMessage(entry: ChatMessage) {
  messages.value.push(entry)
  void nextTick(scrollChatToEnd)
}

function scrollChatToEnd() {
  const box = chatBox.value
  if (box) {
    box.scrollTop = box.scrollHeight
  }
}

async function save(entry: ChatMessage) {
  const proposal = entry.proposal
  if (!proposal || !proposal.savable) {
    return
  }

  savingId.value = entry.id

  try {
    // a typed category name becomes a real category first, exactly as it does
    // when the edit form is saved.
    const resolved = await props.resolveCategory(categoryId.value)
    const created = await CreateSourceFromCompose(proposal.config, proposal.title, resolved)
    if (!created) {
      throw new Error('订阅没有创建成功')
    }

    message.success(`订阅「${created.title}」已创建`)
    emit('created', created)
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    savingId.value = 0
  }
}

function onLog(e: {data?: ComposeLogEntry}) {
  const entry = e?.data
  // a turn the user already moved on from keeps reporting until it finishes; its
  // lines belong to a run id that is no longer the current one.
  if (!entry || entry.runId !== runId.value) {
    return
  }

  logs.value.push(entry)

  const overflow = logs.value.length - MAX_LOGS
  if (overflow > 0) {
    logs.value.splice(0, overflow)
    logsDropped.value += overflow
  }

  if (logPinned.value) {
    void nextTick(scrollLogToEnd)
  }
}

function scrollLogToEnd() {
  const box = logBox.value
  if (box) {
    box.scrollTop = box.scrollHeight
  }
}

// onLogScroll unpins as soon as the user scrolls up, so reading an early line is
// not fought by every line that arrives after it.
function onLogScroll() {
  const box = logBox.value
  if (!box) {
    return
  }

  logPinned.value = box.scrollHeight - box.scrollTop - box.clientHeight < 24
}

function samplesOf(proposal: ComposeProposalDTO) {
  return compact(proposal.verification?.samples ?? [])
}

function fieldsOf(proposal: ComposeProposalDTO) {
  return compact(proposal.fields ?? [])
}

function formatLogTime(time: number): string {
  return new Date(time).toLocaleTimeString('zh-CN', {hour12: false})
}

async function copyLogs() {
  const text = logs.value.map(entry => `${formatLogTime(entry.time)} ${entry.text}`).join('\n')
  try {
    await Clipboard.SetText(text)
    message.success('日志已复制')
  } catch (e) {
    message.error(`复制失败：${errorText(e)}`)
  }
}

function openArticle(url: string) {
  void Browser.OpenURL(url)
}

// onBubbleClick sends a link inside a reply to the system browser. A webview
// that followed one would navigate the application away from itself, and the
// agent quotes addresses constantly.
function onBubbleClick(event: MouseEvent) {
  const anchor = (event.target as HTMLElement | null)?.closest('a')
  if (!anchor) {
    return
  }

  event.preventDefault()

  const href = anchor.getAttribute('href')
  if (href) {
    void Browser.OpenURL(href)
  }
}
</script>

<template>
  <div class="compose">
    <n-alert v-if="startError" type="error" title="无法开始对话" :bordered="false">
      {{ startError }}
      <div style="margin-top: 8px">
        <n-button size="tiny" :loading="starting" @click="start">重试</n-button>
      </div>
    </n-alert>

    <template v-else>
      <div ref="chatBox" class="chat">
        <div class="bubble bubble-agent">
          <div class="bubble-body">
            告诉我你想订阅哪里的什么内容，比如「阮一峰的博客」或者贴一个网址。我会去看这个站点，
            按 feed → JSON → 正则 → AI 的顺序试出一份能真正解析出文章的配置，试通了再给你保存按钮。
          </div>
        </div>

        <div v-for="entry in messages" :key="entry.id" class="bubble" :class="`bubble-${entry.role}`">
          <!-- the html is produced by markdown-it with raw html disabled and then
               sanitised, so nothing the agent answers can execute here. -->
          <div
            v-if="entry.role === 'agent'"
            class="bubble-body markdown"
            @click="onBubbleClick"
            v-html="renderMarkdown(entry.text)"
          />
          <div v-else class="bubble-body">{{ entry.text }}</div>

          <div v-if="entry.proposal" class="proposal">
            <div class="proposal-head">
              <n-space :size="6" align="center">
                <n-text strong>{{ entry.proposal.title }}</n-text>
                <n-tag size="tiny" :bordered="false">{{ parseTypeLabel(entry.proposal.parseType) }}</n-tag>
                <n-tag v-if="entry.proposal.savable" size="tiny" type="success" :bordered="false">
                  复核通过
                </n-tag>
                <n-tag v-else size="tiny" type="warning" :bordered="false">复核未通过</n-tag>
              </n-space>
              <n-text v-if="entry.proposal.reason" depth="3" style="display: block; margin-top: 4px; font-size: 12px">
                {{ entry.proposal.reason }}
              </n-text>
            </div>

            <div class="config">
              <div v-for="field in fieldsOf(entry.proposal)" :key="field.field" class="config-row">
                <div class="config-field">{{ fieldLabel(field.field) }}</div>
                <div class="config-value">{{ fieldValue(field.new) }}</div>
              </div>
            </div>

            <template v-if="entry.proposal.verification">
              <n-alert
                v-if="!entry.proposal.verification.ran"
                type="warning"
                :bordered="false"
                style="margin-top: 8px"
              >
                {{ entry.proposal.verification.note || '这份配置没有经过 informer 复核。' }}
              </n-alert>
              <n-alert
                v-else-if="entry.proposal.verification.error"
                type="error"
                :bordered="false"
                style="margin-top: 8px"
              >
                informer 用这份配置解析失败：{{ entry.proposal.verification.error }}
              </n-alert>
              <n-alert
                v-else-if="entry.proposal.verification.articleCount === 0"
                type="warning"
                :bordered="false"
                style="margin-top: 8px"
              >
                informer 用这份配置解析没有报错，但一条文章都没有取到。
              </n-alert>
              <template v-else>
                <n-text depth="3" style="display: block; margin: 8px 0 6px; font-size: 12px">
                  informer 复核解析出 {{ entry.proposal.verification.articleCount }} 条，
                  请确认下面的标题确实是文章，而不是导航栏或广告：
                </n-text>
                <n-list bordered>
                  <n-list-item v-for="(a, i) in samplesOf(entry.proposal)" :key="i">
                    <n-ellipsis :line-clamp="2">{{ a.title }}</n-ellipsis>
                    <template #suffix>
                      <n-tooltip>
                        <template #trigger>
                          <n-button size="tiny" text type="primary" @click="openArticle(a.url)">打开</n-button>
                        </template>
                        在系统浏览器中打开
                      </n-tooltip>
                    </template>
                  </n-list-item>
                </n-list>
              </template>
            </template>

            <div class="proposal-actions">
              <n-select
                v-model:value="categoryId"
                :options="categoryOptions"
                :on-create="createCategoryOption"
                size="small"
                filterable
                tag
                clearable
                style="width: 240px"
                placeholder="选择分类，或输入新分类名后回车"
              />
              <n-popconfirm :disabled="!entry.proposal.savable" @positive-click="save(entry)">
                <template #trigger>
                  <n-button
                    type="primary"
                    size="small"
                    :loading="savingId === entry.id"
                    :disabled="!entry.proposal.savable || busy"
                  >
                    保存并新建订阅
                  </n-button>
                </template>
                将按上面列出的配置创建一个新订阅并立即启用。
              </n-popconfirm>
              <n-text v-if="!entry.proposal.savable" depth="3" style="font-size: 12px">
                复核没有通过，不提供保存；可以继续告诉 AI 哪里不对。
              </n-text>
            </div>
          </div>
        </div>

        <div v-if="sending" class="bubble bubble-agent">
          <div class="bubble-body">
            <n-text depth="3" style="font-size: 12px">
              <n-ellipsis :line-clamp="2">{{ lastLog || '正在查看这个站点…' }}</n-ellipsis>
            </n-text>
          </div>
        </div>
      </div>

      <div class="input">
        <n-input
          v-model:value="draft"
          type="textarea"
          :autosize="{minRows: 2, maxRows: 5}"
          :disabled="busy || sessionId === ''"
          placeholder="说清楚订阅地址和想要的内容，Enter 发送，Shift+Enter 换行"
          @compositionstart="composing = true"
          @compositionend="composing = false"
          @keydown.enter="onEnter"
        />
        <div class="input-actions">
          <n-text depth="3" style="font-size: 12px">
            {{ starting ? '正在准备…' : `还可以聊 ${turnsLeft} 轮` }}
          </n-text>
          <n-button type="primary" size="small" :loading="sending" :disabled="!canSend" @click="send">
            发送
          </n-button>
        </div>
      </div>

      <n-collapse v-model:expanded-names="logExpanded">
        <n-collapse-item :name="LOG_PANEL">
          <template #header>
            <n-space :size="6" align="center">
              <n-text depth="2" style="font-size: 12px">执行日志（{{ logs.length }} 条）</n-text>
              <n-tag v-if="logProblems > 0" size="tiny" type="error" :bordered="false">
                {{ logProblems }} 异常
              </n-tag>
            </n-space>
          </template>
          <template #header-extra>
            <!-- click.stop so copying never folds the panel shut -->
            <n-button size="tiny" text :disabled="logs.length === 0" @click.stop="copyLogs">复制</n-button>
          </template>
          <div ref="logBox" class="log-box" @scroll="onLogScroll">
            <div v-if="logsDropped > 0" class="log-line log-warn">…已省略较早的 {{ logsDropped }} 条</div>
            <div v-for="entry in logs" :key="entry.seq" class="log-line" :class="`log-${entry.level}`">
              <span class="log-time">{{ formatLogTime(entry.time) }}</span>{{ entry.text }}
            </div>
            <n-text v-if="logs.length === 0" depth="3" style="font-size: 12px">暂无日志</n-text>
          </div>
        </n-collapse-item>
      </n-collapse>
    </template>
  </div>
</template>

<style scoped>
.compose {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.chat {
  /* the modal also holds an input box and a log panel, so the transcript is what
     scrolls: the send button must never leave the viewport. */
  max-height: 46vh;
  overflow-y: auto;
  padding-right: 4px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.bubble {
  display: flex;
  flex-direction: column;
  max-width: 92%;
}

.bubble-user {
  align-self: flex-end;
  align-items: flex-end;
}

.bubble-agent {
  align-self: flex-start;
}

.bubble-body {
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--n-code-color, rgba(0, 0, 0, 0.04));
}

.bubble-user .bubble-body {
  white-space: pre-wrap;
  background: rgba(24, 160, 88, 0.12);
}

.markdown {
  white-space: normal;
}

.markdown :deep(p) {
  margin: 0 0 6px;
}

.markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.markdown :deep(code) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  word-break: break-all;
}

.markdown :deep(pre) {
  /* a proposal explanation quotes whole regexes; the block scrolls rather than
     widening the modal. */
  overflow-x: auto;
  padding: 6px 8px;
  border-radius: 3px;
  background: rgba(0, 0, 0, 0.06);
}

.proposal {
  margin-top: 8px;
  padding: 10px;
  border: 1px solid var(--n-border-color, #efeff5);
  border-radius: 4px;
}

.proposal-head {
  margin-bottom: 8px;
}

.config {
  border: 1px solid var(--n-border-color, #efeff5);
  border-radius: 3px;
  overflow: hidden;
}

.config-row {
  display: grid;
  /* two columns, not three: a subscription that does not exist yet has no
     previous value, and a struck-through empty column reads as a bug. */
  grid-template-columns: 96px 1fr;
  border-bottom: 1px solid var(--n-border-color, #efeff5);
}

.config-row:last-child {
  border-bottom: none;
}

.config-field {
  padding: 6px 8px;
  font-size: 12px;
  opacity: 0.7;
}

.config-value {
  padding: 6px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

.proposal-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 12px;
}

.input {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.input-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.log-box {
  max-height: 200px;
  overflow-y: auto;
  padding: 6px 8px;
  border-radius: 3px;
  background: var(--n-code-color, rgba(0, 0, 0, 0.04));
}

.log-line {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  line-height: 1.6;
  /* a log line carries urls and whole regexes: it wraps rather than pushing the
     modal into a horizontal scroll. */
  white-space: pre-wrap;
  word-break: break-all;
}

.log-time {
  margin-right: 6px;
  opacity: 0.5;
}

.log-warn {
  color: #d97706;
}

.log-error {
  color: #d03050;
}
</style>
