<script setup lang="ts">
import {computed, nextTick, onMounted, onUnmounted, ref} from 'vue'
import {
  NAlert,
  NButton,
  NCollapse,
  NCollapseItem,
  NEllipsis,
  NEmpty,
  NList,
  NListItem,
  NPopconfirm,
  NSpace,
  NTag,
  NText,
  NTooltip,
  useMessage
} from 'naive-ui'
import {Browser, Clipboard, Events} from '@wailsio/runtime'
import {ApplySourceFix, DiagnoseSource} from './bindings'
import type {DiagnoseReportDTO, SourceDTO} from './bindings'
import {errorText} from './errors'
import {compact} from './nulls'
import {fieldLabel, fieldValue} from './sourceFields'

const props = defineProps<{source: SourceDTO | null}>()
const emit = defineEmits<{applied: []}>()

const message = useMessage()

// DiagnoseLogEntry mirrors RunLogDTO in cmd/informer-ui/binding_runlog.go. A
// wails event payload is not part of the generated bindings, so the two shapes
// are kept in step by hand.
interface DiagnoseLogEntry {
  runId: string
  seq: number
  time: number
  level: 'info' | 'warn' | 'error'
  text: string
}

// DIAGNOSE_LOG_EVENT is the same constant as DiagnoseLogEvent on the Go side.
const DIAGNOSE_LOG_EVENT = 'informer:diagnose:log'

// MAX_LOGS bounds what the panel keeps, matching the budget the Go sink stops
// at. An unbounded array in a webview is a leak.
const MAX_LOGS = 500

const running = ref(false)
const applying = ref(false)
const runError = ref('')
const report = ref<DiagnoseReportDTO | null>(null)

const runId = ref('')
const logs = ref<DiagnoseLogEntry[]>([])
const logsDropped = ref(0)
const logExpanded = ref<string[]>([])
const logBox = ref<HTMLElement | null>(null)

// logPinned keeps the panel scrolled to the newest line until the user scrolls
// up to read an earlier one.
const logPinned = ref(true)

const LOG_PANEL = 'log'

const logProblems = computed(() => logs.value.filter(entry => entry.level !== 'info').length)
const lastLog = computed(() => logs.value.at(-1)?.text ?? '')

const verification = computed(() => report.value?.verification ?? null)
const samples = computed(() => compact(verification.value?.samples ?? []))
const diff = computed(() => compact(report.value?.diff ?? []))

// applicable is the one condition the apply button follows: informer parsed
// articles with the proposal, in this process, just now. The agent's own claim
// is shown next to it but never decides.
const applicable = computed(() => report.value !== null && report.value.fixed && report.value.fix !== '')

// disputed marks the case worth calling out: the agent says it fixed the source
// and informer's own re-parse disagrees.
const disputed = computed(
  () => report.value !== null && report.value.agentClaimedFixed && !report.value.fixed
)

const unsub = Events.On(DIAGNOSE_LOG_EVENT, onLog)

onUnmounted(unsub)

function onLog(e: {data?: DiagnoseLogEntry}) {
  const entry = e?.data
  // a run the user already moved on from keeps reporting until it finishes; its
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

// the diagnosis starts itself. The parent gives this component a fresh key per
// request, so every「AI 诊断修复」click mounts a new panel that runs at once -
// which is both what the user asked for and one less ref-timing dance than
// reaching into the component after the drawer has animated open.
onMounted(() => {
  if (props.source) {
    void run()
  }
})

function reset() {
  runId.value = ''
  logs.value = []
  logsDropped.value = 0
  logPinned.value = true
  logExpanded.value = []
  report.value = null
  runError.value = ''
}

async function run() {
  const source = props.source
  if (!source) {
    return
  }

  // crypto.randomUUID is not guaranteed in a webview served over a custom
  // scheme, and this id only has to be unique among this window's own runs.
  const id = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`

  reset()
  // open while it runs: a diagnosis takes minutes and the log is the only thing
  // that shows it is doing something.
  logExpanded.value = [LOG_PANEL]
  // set before the call: the first lines can arrive before it returns.
  runId.value = id
  running.value = true

  try {
    report.value = await DiagnoseSource(source.id, id)
    // the conclusion is what the user came for; the log stays one click away.
    logExpanded.value = []
  } catch (e) {
    runError.value = errorText(e)
    // it failed, and the log is exactly where the reason is.
    logExpanded.value = [LOG_PANEL]
  } finally {
    // a slow run the user already replaced must not clear the new one's state.
    if (runId.value === id) {
      running.value = false
    }
  }
}

async function apply() {
  const source = props.source
  const fix = report.value?.fix
  if (!source || !fix) {
    return
  }

  applying.value = true
  logExpanded.value = [LOG_PANEL]

  try {
    await ApplySourceFix(source.id, fix, runId.value)
    message.success('修复已应用')
    // the card outside still shows the old configuration and the old health.
    emit('applied')
    report.value = null
  } catch (e) {
    message.error(`应用失败：${errorText(e)}`)
  } finally {
    applying.value = false
  }
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
</script>

<template>
  <div class="diagnose">
    <n-alert v-if="runError" type="error" title="诊断失败" style="margin-bottom: 12px">
      {{ runError }}
      <div style="margin-top: 8px">
        <n-button size="tiny" :loading="running" @click="run">重试</n-button>
      </div>
    </n-alert>

    <div v-else-if="running" class="progress">
      <n-text depth="3" style="font-size: 12px">
        <n-ellipsis :line-clamp="3">{{ lastLog || '正在诊断…' }}</n-ellipsis>
      </n-text>
      <n-text depth="3" style="display: block; margin-top: 10px; font-size: 12px">
        AI 会读取订阅配置、抓取页面原文、反复试跑候选配置，通常需要一到几分钟。
      </n-text>
    </div>

    <template v-else-if="report">
      <n-space :size="6" style="margin-bottom: 10px">
        <n-tag v-if="report.fixed" size="small" type="success">已找到可用的修复</n-tag>
        <n-tag v-else size="small" type="warning">未能自动修复</n-tag>
        <n-tag v-if="disputed" size="small" type="error" :bordered="false">AI 自评通过但复核未通过</n-tag>
      </n-space>

      <n-alert type="default" title="诊断结论" :bordered="false">
        {{ report.diagnosis || '（AI 没有给出结论）' }}
      </n-alert>

      <n-alert v-if="report.advice" type="warning" title="建议" :bordered="false" style="margin-top: 10px">
        {{ report.advice }}
      </n-alert>

      <template v-if="diff.length > 0">
        <div class="section-title">建议的配置改动</div>
        <div class="diff">
          <div v-for="change in diff" :key="change.field" class="diff-row">
            <div class="diff-field">{{ fieldLabel(change.field) }}</div>
            <div class="diff-value diff-old">{{ fieldValue(change.old) }}</div>
            <div class="diff-value diff-new">{{ fieldValue(change.new) }}</div>
          </div>
        </div>
      </template>

      <template v-if="verification && verification.ran">
        <div class="section-title">informer 复核结果</div>
        <n-alert v-if="verification.error" type="error" :bordered="false">
          用建议的配置解析仍然失败：{{ verification.error }}
        </n-alert>
        <n-alert v-else-if="verification.articleCount === 0" type="warning" :bordered="false">
          用建议的配置解析没有报错，但一条文章都没有取到。
        </n-alert>
        <template v-else>
          <n-text depth="3" style="font-size: 12px">
            用建议的配置解析出 {{ verification.articleCount }} 条，请确认下面的标题确实是文章，
            而不是导航栏或广告：
          </n-text>
          <n-list bordered style="margin-top: 8px">
            <n-list-item v-for="(a, i) in samples" :key="i">
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

      <div class="actions">
        <n-popconfirm :disabled="!applicable" @positive-click="apply">
          <template #trigger>
            <n-button type="primary" size="small" :loading="applying" :disabled="!applicable">
              应用修复
            </n-button>
          </template>
          将把上面列出的字段写入订阅配置，并立即重新抓取一次。原来的配置不会保留备份。
        </n-popconfirm>
        <n-button size="small" tertiary :loading="running" @click="run">重新诊断</n-button>
        <n-text v-if="!applicable" depth="3" style="font-size: 12px">
          复核没有通过，不提供一键应用；可以照着结论手动改，或改用其它解析方式。
        </n-text>
      </div>
    </template>

    <n-empty v-else description="点击「开始诊断」让 AI 分析这个订阅为什么解析失败" style="margin: 40px 0">
      <template #extra>
        <n-button size="small" type="primary" @click="run">开始诊断</n-button>
      </template>
    </n-empty>

    <n-collapse v-model:expanded-names="logExpanded" style="margin-top: 14px">
      <n-collapse-item :name="LOG_PANEL">
        <template #header>
          <n-space :size="6" align="center">
            <n-text depth="2" style="font-size: 12px">诊断日志（{{ logs.length }} 条）</n-text>
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
  </div>
</template>

<style scoped>
.diagnose {
  display: flex;
  flex-direction: column;
}

.progress {
  padding: 24px 0;
}

.section-title {
  margin: 14px 0 6px;
  font-size: 12px;
  opacity: 0.7;
}

.diff {
  border: 1px solid var(--n-border-color, #efeff5);
  border-radius: 3px;
  overflow: hidden;
}

.diff-row {
  display: grid;
  /* the field name is narrow and fixed; the two values share what is left, so a
     long regex wraps instead of pushing the drawer into a horizontal scroll. */
  grid-template-columns: 96px 1fr 1fr;
  border-bottom: 1px solid var(--n-border-color, #efeff5);
}

.diff-row:last-child {
  border-bottom: none;
}

.diff-field {
  padding: 6px 8px;
  font-size: 12px;
  opacity: 0.7;
}

.diff-value {
  padding: 6px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

.diff-old {
  opacity: 0.55;
  text-decoration: line-through;
}

.diff-new {
  color: #18a058;
}

.actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 16px;
}

.log-box {
  max-height: 280px;
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
     drawer into a horizontal scroll. */
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
