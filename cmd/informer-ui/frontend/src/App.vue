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
import {HomeDir, RestartToUpdate, StartupError, Version} from './bindings'
import ArticleLibrary from './ArticleLibrary.vue'
import DailyReport from './DailyReport.vue'
import SettingsPanel from './SettingsPanel.vue'
import SourceManager from './SourceManager.vue'

const version = ref('')
const homeDir = ref('')
const startupError = ref('')
const ready = ref(false)

// updateBadge drives the header CTA: downloading → ready → click Restart.
type UpdateBadge = '' | 'downloading' | 'ready' | 'error'
const updateBadge = ref<UpdateBadge>('')
const updateDetail = ref('')
const restarting = ref(false)

const unsubs: Array<() => void> = []

onMounted(async () => {
  version.value = await Version().catch(() => '')
  homeDir.value = await HomeDir().catch(() => '')
  startupError.value = await StartupError().catch(e => String(e))
  ready.value = true

  unsubs.push(Events.On(Updater.Events.DownloadStarted, () => {
    updateBadge.value = 'downloading'
    updateDetail.value = ''
  }))
  unsubs.push(Events.On(Updater.Events.DownloadProgress, (e: {data?: {written?: number; total?: number}}) => {
    updateBadge.value = 'downloading'
    const p = e?.data
    if (p && p.total && p.total > 0 && typeof p.written === 'number') {
      const pct = Math.min(100, Math.round((p.written / p.total) * 100))
      updateDetail.value = `${pct}%`
    }
  }))
  unsubs.push(Events.On(Updater.Events.UpdateReady, () => {
    updateBadge.value = 'ready'
    updateDetail.value = ''
  }))
  unsubs.push(Events.On(Updater.Events.Error, (e: {data?: {message?: string; stage?: string}}) => {
    updateBadge.value = 'error'
    updateDetail.value = e?.data?.message || '更新失败'
  }))
  unsubs.push(Events.On(Updater.Events.NoUpdate, () => {
    if (updateBadge.value !== 'ready') {
      updateBadge.value = ''
      updateDetail.value = ''
    }
  }))
})

onUnmounted(() => {
  for (const off of unsubs) {
    off()
  }
})

async function onRestartUpdate() {
  restarting.value = true
  try {
    await RestartToUpdate()
  } catch (e) {
    updateBadge.value = 'error'
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
              <n-tooltip v-if="homeDir">
                <template #trigger>
                  <n-text depth="3" style="font-size: 12px">
                    <n-ellipsis style="max-width: 420px">{{ homeDir }}</n-ellipsis>
                  </n-text>
                </template>
                数据目录：{{ homeDir }}
              </n-tooltip>
            </n-space>

            <n-space align="center" :size="8" class="header-right">
              <n-text v-if="updateBadge === 'downloading'" depth="3" style="font-size: 12px">
                正在下载更新{{ updateDetail ? ` ${updateDetail}` : '…' }}
              </n-text>
              <n-button
                v-else-if="updateBadge === 'ready'"
                type="warning"
                size="small"
                :loading="restarting"
                @click="onRestartUpdate"
              >
                重启生效新版本
              </n-button>
              <n-tooltip v-else-if="updateBadge === 'error'">
                <template #trigger>
                  <n-tag :bordered="false" size="small" type="error">更新失败</n-tag>
                </template>
                {{ updateDetail || '请稍后重试' }}
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
