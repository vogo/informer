<script setup lang="ts">
import {computed, nextTick, onUnmounted, ref, watch} from 'vue'
import {
  NButton,
  NDrawer,
  NDrawerContent,
  NInput,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  NText,
  NTooltip,
  useMessage
} from 'naive-ui'
import {Clipboard} from '@wailsio/runtime'
import {ClearSystemLogs, SystemLogs} from './bindings'
import type {SystemLogEntryDTO} from './bindings'
import {compact, requireValue} from './nulls'
import {errorText} from './errors'

const props = defineProps<{show: boolean}>()
const emit = defineEmits<{'update:show': [value: boolean]}>()

const message = useMessage()

// POLL_INTERVAL_MS is how often an open panel asks for the lines it has not seen.
// The read is a cursor read of an in-memory ring, so it costs nothing to repeat;
// one second is fast enough to watch a running push line by line.
const POLL_INTERVAL_MS = 1000

// LEVEL_RANK orders the levels so the filter can be a floor rather than a set:
// a user looking for a failure wants the warnings around it too.
const LEVEL_RANK: Record<string, number> = {debug: 0, info: 1, warn: 2, error: 3}

const PROBLEM_RANK = LEVEL_RANK.warn

const levelOptions = [
  {label: '全部级别', value: 0},
  {label: '警告与错误', value: PROBLEM_RANK},
  {label: '仅错误', value: LEVEL_RANK.error}
]

const entries = ref<SystemLogEntryDTO[]>([])

// cursor is the highest sequence this panel has seen; every poll asks for what
// comes after it, so a long-lived panel never re-reads the whole buffer.
const cursor = ref(0)

// dropped counts the lines the process buffer overwrote before this panel read
// them. It is shown rather than hidden: a log with a silent hole misleads.
const dropped = ref(0)
const capacity = ref(0)

const loading = ref(false)
const loadError = ref('')
const clearing = ref(false)
const autoRefresh = ref(true)
const minLevel = ref(0)
const keyword = ref('')

const logBox = ref<HTMLElement | null>(null)

// pinned keeps the panel scrolled to the newest line until the user scrolls up
// to read an earlier one.
const pinned = ref(true)

// polling guards against a slow read overlapping the next tick, which would
// consume the same cursor twice and duplicate lines.
let polling = false
let timer: ReturnType<typeof setInterval> | null = null

const visible = computed(() => {
  const needle = keyword.value.trim().toLowerCase()

  return entries.value.filter(entry => {
    if ((LEVEL_RANK[entry.level] ?? LEVEL_RANK.info) < minLevel.value) {
      return false
    }

    return needle === '' || entry.text.toLowerCase().includes(needle)
  })
})

const problems = computed(
  () => entries.value.filter(entry => (LEVEL_RANK[entry.level] ?? LEVEL_RANK.info) >= PROBLEM_RANK).length
)

const filtered = computed(() => visible.value.length !== entries.value.length)

watch(
  () => props.show,
  opened => {
    if (opened) {
      void reload()
    }

    syncTimer()
  }
)

watch(autoRefresh, syncTimer)

onUnmounted(stopTimer)

function syncTimer() {
  if (props.show && autoRefresh.value) {
    startTimer()

    return
  }

  stopTimer()
}

function startTimer() {
  if (timer === null) {
    timer = setInterval(() => void poll(), POLL_INTERVAL_MS)
  }
}

function stopTimer() {
  if (timer !== null) {
    clearInterval(timer)
    timer = null
  }
}

// reload throws away what the panel holds and reads the buffer from its start,
// which is what opening the panel - and the manual refresh - has to do.
async function reload() {
  entries.value = []
  cursor.value = 0
  dropped.value = 0
  pinned.value = true
  loading.value = true

  try {
    await poll()
  } finally {
    loading.value = false
  }
}

async function poll() {
  if (polling) {
    return
  }

  polling = true

  try {
    const page = requireValue(await SystemLogs(cursor.value, 0), '系统日志')

    capacity.value = page.capacity
    dropped.value += page.dropped
    cursor.value = page.latestSeq
    loadError.value = ''

    const fresh = compact(page.entries)
    if (fresh.length > 0) {
      entries.value.push(...fresh)

      // the panel keeps no more than the process does, so a window left open
      // for days cannot grow an unbounded array in the webview.
      const overflow = entries.value.length - page.capacity
      if (overflow > 0) {
        entries.value.splice(0, overflow)
        dropped.value += overflow
      }

      if (pinned.value) {
        void nextTick(scrollToEnd)
      }
    }
  } catch (e) {
    loadError.value = errorText(e)
  } finally {
    polling = false
  }
}

function scrollToEnd() {
  const box = logBox.value
  if (box) {
    box.scrollTop = box.scrollHeight
  }
}

// onScroll unpins as soon as the user scrolls up, so reading an early line is
// not fought by every line that arrives after it.
function onScroll() {
  const box = logBox.value
  if (!box) {
    return
  }

  pinned.value = box.scrollHeight - box.scrollTop - box.clientHeight < 24
}

async function onClear() {
  clearing.value = true

  try {
    // the returned cursor is where the process log continues, so the panel is
    // never told afterwards that it lost the lines it discarded itself.
    cursor.value = await ClearSystemLogs()
    entries.value = []
    dropped.value = 0
    pinned.value = true
  } catch (e) {
    message.error(`清空失败：${errorText(e)}`)
  } finally {
    clearing.value = false
  }
}

async function onCopy() {
  const text = visible.value.map(entry => `${formatTime(entry.time)} [${entry.level}] ${entry.text}`).join('\n')

  try {
    await Clipboard.SetText(text)
    message.success(filtered.value ? '已复制当前筛选出的日志' : '日志已复制')
  } catch (e) {
    message.error(`复制失败：${errorText(e)}`)
  }
}

// formatTime shows the clock down to milliseconds, and prefixes the date only
// for a line from another day: the buffer of a window left open all week would
// otherwise show yesterday's failure as if it had just happened.
function formatTime(time: number): string {
  const at = new Date(time)
  const clock = at.toLocaleTimeString('zh-CN', {hour12: false})
  const millis = String(at.getMilliseconds()).padStart(3, '0')

  if (at.toDateString() === new Date().toDateString()) {
    return `${clock}.${millis}`
  }

  return `${at.toLocaleDateString('zh-CN', {month: '2-digit', day: '2-digit'})} ${clock}.${millis}`
}
</script>

<template>
  <n-drawer
    :show="props.show"
    :width="720"
    placement="right"
    resizable
    @update:show="value => emit('update:show', value)"
  >
    <!-- the drawer body is made a flex column so the log box can own the scroll:
         a nested scroll area would fight the panel's own follow-the-tail. -->
    <n-drawer-content
      title="系统运行日志"
      closable
      :native-scrollbar="false"
      :body-content-style="{display: 'flex', flexDirection: 'column', height: '100%'}"
    >
      <div class="log-panel">
        <n-space align="center" :size="8" class="toolbar">
          <n-select v-model:value="minLevel" :options="levelOptions" size="small" style="width: 130px" />
          <n-input
            v-model:value="keyword"
            size="small"
            clearable
            placeholder="按关键字过滤"
            style="width: 190px"
          />
          <n-tooltip>
            <template #trigger>
              <n-space align="center" :size="4">
                <n-switch v-model:value="autoRefresh" size="small" />
                <n-text depth="3" style="font-size: 12px">自动刷新</n-text>
              </n-space>
            </template>
            每秒读取一次新增日志；关闭后可用「刷新」手动读取
          </n-tooltip>
          <n-button size="small" :loading="loading" @click="reload">刷新</n-button>
          <n-button size="small" :disabled="entries.length === 0" @click="onCopy">复制</n-button>
          <n-button size="small" :loading="clearing" :disabled="entries.length === 0" @click="onClear">
            清空
          </n-button>
        </n-space>

        <n-space align="center" :size="8" class="summary">
          <n-text depth="3" style="font-size: 12px">
            共 {{ entries.length }} 条<template v-if="filtered">，筛选出 {{ visible.length }} 条</template>
          </n-text>
          <n-tag v-if="problems > 0" size="tiny" type="error" :bordered="false">{{ problems }} 异常</n-tag>
          <n-tag v-if="!pinned" size="tiny" :bordered="false">已暂停跟随</n-tag>
          <n-text v-if="loadError" type="error" style="font-size: 12px">读取失败：{{ loadError }}</n-text>
        </n-space>

        <div ref="logBox" class="log-box" @scroll="onScroll">
          <div v-if="dropped > 0" class="log-line log-warn">…已省略较早的 {{ dropped }} 条</div>
          <div
            v-for="entry in visible"
            :key="entry.seq"
            class="log-line"
            :class="`log-${entry.level}`"
          >
            <span class="log-time">{{ formatTime(entry.time) }}</span>{{ entry.text }}
          </div>
          <n-text v-if="visible.length === 0" depth="3" style="font-size: 12px">
            {{ entries.length === 0 ? '本次运行暂无日志' : '没有匹配当前筛选条件的日志' }}
          </n-text>
        </div>

        <n-text depth="3" class="hint">
          日志只保留在本次运行的内存中，最多 {{ capacity || '—' }} 条，重启应用后清空；
          抓取、推送与定时任务的输出都在这里。
        </n-text>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.log-panel {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.toolbar {
  flex: none;
}

.summary {
  flex: none;
  margin: 8px 0 6px;
  min-height: 20px;
}

.log-box {
  flex: 1;
  /* a floor keeps the box readable even where the drawer body refuses to hand
     down a definite height. */
  min-height: 240px;
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

.log-debug {
  opacity: 0.6;
}

.log-warn {
  color: #d97706;
}

.log-error {
  color: #d03050;
}

.hint {
  flex: none;
  margin-top: 8px;
  font-size: 12px;
}
</style>
