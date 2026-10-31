<script setup lang="ts">
import {onMounted, onUnmounted, ref} from 'vue'
import {
  NButton,
  NConfigProvider,
  NDialogProvider,
  NEllipsis,
  NMessageProvider,
  NResult,
  NSpace,
  NTabPane,
  NTabs,
  NTag,
  NText,
  NTooltip,
  dateZhCN,
  zhCN
} from 'naive-ui'
import {Events, Updater} from '@wailsio/runtime'
import {HomeDir, RestartToUpdate, StartupError, UpdateState, Version} from './bindings'
import ArticleLibrary from './ArticleLibrary.vue'
import DailyReport from './DailyReport.vue'
import SettingsPanel from './SettingsPanel.vue'
import SourceManager from './SourceManager.vue'
import SystemLogViewer from './SystemLogViewer.vue'

const version = ref('')
const homeDir = ref('')
const startupError = ref('')
const ready = ref(false)

// showLogs drives the log panel behind the header button. It reads the process
// log buffer, which is filled whether or not startup succeeded - a failed start
// is exactly when the log is worth opening.
const showLogs = ref(false)

// updateBadge drives the header CTA: downloading → ready → click Restart.
type UpdateBadge = '' | 'downloading' | 'ready' | 'error'
const updateBadge = ref<UpdateBadge>('')
const updateDetail = ref('')
const restarting = ref(false)

const unsubs: Array<() => void> = []
let stateTimer: ReturnType<typeof setInterval> | undefined
let stateRevision = 0
let stateReadPending = false

async function syncUpdateState() {
  if (stateReadPending) return
  stateReadPending = true
  const revision = stateRevision
  try {
    const state = await UpdateState()
    if (revision !== stateRevision) return
    if (state === 'ready') {
      if (updateBadge.value !== 'ready') updateDetail.value = ''
      updateBadge.value = 'ready'
    } else if (['downloading', 'verifying', 'installing'].includes(state)) {
      if (updateBadge.value !== 'downloading') updateDetail.value = ''
      updateBadge.value = 'downloading'
    } else if (state === 'error' && updateBadge.value !== 'error') {
      updateBadge.value = 'error'
      updateDetail.value = '自动更新失败，下次启动或每日检查时重试；详情请查看日志'
    }
  } catch { /* The window may be shutting down. */ }
  finally { stateReadPending = false }
}

onMounted(async () => {
  unsubs.push(Events.On(Updater.Events.DownloadStarted, () => {
    stateRevision++
    updateBadge.value = 'downloading'
    updateDetail.value = ''
  }))
  unsubs.push(Events.On(Updater.Events.DownloadProgress, (e: {data?: {written?: number; total?: number}}) => {
    stateRevision++
    updateBadge.value = 'downloading'
    const p = e?.data
    if (p && p.total && p.total > 0 && typeof p.written === 'number') {
      const pct = Math.min(100, Math.round((p.written / p.total) * 100))
      updateDetail.value = `${pct}%`
    }
  }))
  unsubs.push(Events.On(Updater.Events.UpdateReady, () => {
    stateRevision++
    updateBadge.value = 'ready'
    updateDetail.value = ''
  }))
  unsubs.push(Events.On(Updater.Events.Error, (e: {data?: {message?: string; stage?: string}}) => {
    stateRevision++
    updateBadge.value = 'error'
    updateDetail.value = e?.data?.message || '更新失败'
  }))
  unsubs.push(Events.On(Updater.Events.NoUpdate, () => {
    if (updateBadge.value !== 'ready') {
      stateRevision++
      updateBadge.value = ''
      updateDetail.value = ''
    }
  }))
  await syncUpdateState()
  stateTimer = setInterval(() => { void syncUpdateState() }, 3000)
  version.value = await Version().catch(() => '')
  homeDir.value = await HomeDir().catch(() => '')
  startupError.value = await StartupError().catch(e => String(e))
  ready.value = true
})

onUnmounted(() => {
  stateRevision++
  if (stateTimer) clearInterval(stateTimer)
  for (const off of unsubs) {
    off()
  }
})

async function onRestartUpdate() {
  if (restarting.value) return
  restarting.value = true
  try {
    await RestartToUpdate()
  } catch (e) {
    stateRevision++
    updateBadge.value = 'ready'
    updateDetail.value = e instanceof Error ? e.message : String(e)
    restarting.value = false
  }
}
</script>

<template>
  <n-config-provider :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <div class="shell">
          <header class="header">
            <n-space align="center" :size="12">
              <n-text strong style="font-size: 16px">Informer</n-text>
              <n-tag v-if="version" :bordered="false" size="small" type="info">{{ version }}</n-tag>
              <n-tooltip v-if="updateBadge === 'ready'">
                <template #trigger>
                  <n-button circle size="small" type="success" :loading="restarting" :disabled="restarting"
                    aria-label="新版本已下载，点击重启更新" @click="onRestartUpdate">
                    <template #icon>
                      <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                        <path d="M12 3v12m-5-5 5 5 5-5M5 17v4h14v-4" />
                      </svg>
                    </template>
                  </n-button>
                </template>
                {{ updateDetail || '新版本已下载，点击重启更新' }}
              </n-tooltip>
              <n-text v-else-if="updateBadge === 'downloading'" depth="3" style="font-size: 12px">
                正在准备更新{{ updateDetail ? ` ${updateDetail}` : '…' }}
              </n-text>
              <n-tooltip v-else-if="updateBadge === 'error'">
                <template #trigger><n-tag :bordered="false" size="small" type="error">更新失败</n-tag></template>
                {{ updateDetail || '下次启动或每日检查时重试' }}
              </n-tooltip>
              <n-tooltip v-if="homeDir">
                <template #trigger>
                  <n-text depth="3" style="font-size: 12px">
                    <n-ellipsis style="max-width: 280px">{{ homeDir }}</n-ellipsis>
                  </n-text>
                </template>
                数据目录：{{ homeDir }}
              </n-tooltip>
            </n-space>

            <n-space align="center" :size="8" class="header-right">
              <n-tooltip>
                <template #trigger>
                  <n-button size="small" quaternary @click="showLogs = true">日志</n-button>
                </template>
                查看本次运行的系统日志
              </n-tooltip>

            </n-space>
          </header>

          <n-result
            v-if="startupError"
            status="error"
            title="启动失败"
            :description="startupError"
            style="margin-top: 80px"
          >
            <template #footer>
              <n-text depth="3">
                数据目录初始化或 Service 创建失败，请检查 INFORMER_HOME 与磁盘权限后重启应用。
              </n-text>
            </template>
          </n-result>

          <!-- every tab is mounted lazily and kept alive afterwards, so switching
               back does not re-run a fetch, but the first paint stays cheap. -->
          <n-tabs
            v-else-if="ready"
            class="tabs"
            type="line"
            default-value="sources"
            animated
            pane-class="pane"
            pane-wrapper-class="pane-wrapper"
            tab-style="padding: 0 16px"
            display-directive="show:lazy"
          >
            <n-tab-pane name="sources" tab="订阅">
              <SourceManager />
            </n-tab-pane>
            <n-tab-pane name="daily" tab="日报">
              <DailyReport />
            </n-tab-pane>
            <n-tab-pane name="articles" tab="文章库">
              <ArticleLibrary />
            </n-tab-pane>
            <n-tab-pane name="settings" tab="设置">
              <SettingsPanel />
            </n-tab-pane>
          </n-tabs>

          <footer class="footer">
            <n-text depth="3" style="font-size: 12px">
              informer 桌面版 {{ version }} · 测试抓取执行真实网络请求，但不写库、不修改订阅状态
            </n-text>
          </footer>
          <SystemLogViewer v-model:show="showLogs" />
        </div>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<style scoped>
.shell {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  height: 48px;
  flex: none;
  border-bottom: 1px solid #efeff5;
}

.header-right {
  flex: none;
}

.tabs {
  flex: 1;
  overflow: hidden;
}

.footer {
  flex: none;
  height: 32px;
  line-height: 32px;
  padding: 0 16px;
  border-top: 1px solid #efeff5;
}
</style>

<style>
.tabs .n-tabs-pane-wrapper,
.tabs .pane-wrapper {
  height: 100%;
  overflow: hidden;
}

.tabs .pane {
  height: 100%;
  padding: 0 !important;
  overflow: hidden;
}
</style>
