<script setup lang="ts">
import {computed, onMounted, reactive, ref} from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NEllipsis,
  NForm,
  NFormItem,
  NGi,
  NGrid,
  NInput,
  NInputGroup,
  NInputNumber,
  NModal,
  NSpace,
  NSelect,
  NSpin,
  NSwitch,
  NTag,
  NText,
  NTimePicker,
  NTooltip,
  useMessage
} from 'naive-ui'
import {
  DetectAgentCommand,
  ReadConfig,
  ReadSecrets,
  RebuildHistoryIndex,
  SaveAgentAPIKey,
  SaveAgentConfig,
  SaveConfig,
  SaveHTTPProxy,
  SaveSchedule,
  SaveResultProcessing,
  PreviewResultProcessing,
  type ProcessingPreviewDTO,
  SaveWebhooks,
  type HistoryIndexDTO,
  type WebhookDTO,
} from './bindings'
import {errorText} from './errors'
import {requireValue} from './nulls'
import {renderMarkdown} from './markdown'
import {Browser} from '@wailsio/runtime'

const message = useMessage()

// 卡片内边距压到最小：设置页的目标是一屏内看到尽可能多的配置项，
// 长说明统一收进标题旁的问号 tooltip，不再占用正文行。
const cardHeaderStyle = {padding: '8px 12px'}
const cardContentStyle = {padding: '10px 12px'}
const cardFooterStyle = {padding: '6px 12px'}
const tipStyle = {maxWidth: '340px'}

const loading = ref(false)
const loadError = ref('')
const saving = ref(false)

const configPath = ref('')
const configExists = ref(false)
const preservedKeys = ref<string[]>([])

const form = reactive({
  maxInformFeedSize: 20,
  feedExpireDays: 150,
  sameSiteMaxCount: 3,
  maxFetchNum: 1
})

// the agent section drives every「agent」订阅: the endpoint, the model and the
// tool set. The api key is not part of it - it is saved on its own below, so this
// form never has to hold a credential it was never shown.
const agent = reactive({
  provider: '',
  baseURL: '',
  model: '',
  allowedTools: '',
  timeoutSeconds: 0,
  command: ''
})
const agentProviderOptions = ref<{label: string; value: string}[]>([])
const agentSaving = ref(false)
const agentDetecting = ref(false)
const agentAPIKeyConfigured = ref(false)
const agentAPIKeyInput = ref('')
const agentAPIKeySaving = ref(false)

const secretsPath = ref('')
// every run delivers to the whole list, so the page edits the list instead of one
// address: rows are saved together, and a disabled row stays in the file.
const webhooks = ref<WebhookDTO[]>([])
const webhookSaving = ref(false)
const enabledWebhookCount = computed(
  () => webhooks.value.filter(row => row.enabled && row.url.trim() !== '').length
)

const httpProxyConfigured = ref(false)
const httpProxy = ref('')
const httpProxyInput = ref('')
const httpProxySaving = ref(false)

const schedule = reactive({
  enabled: false,
  time: '10:00'
})
const scheduleSaving = ref(false)
const resultProcessing = reactive({enabled: false, prompt: ''})
const processingSaving = ref(false)
const processingTesting = ref(false)
const processingPreview = ref<ProcessingPreviewDTO | null>(null)
const processingPreviewOpen = ref(false)
const processingError = ref('')
const processingPreviewPrompt = ref('')
const processingExample = '分析今日候选文章，只保留具有明确技术价值、包含具体方法或事实的内容。过滤广告、重复资讯和泛泛观点。按重要性排序，生成简洁的中文导读。宁缺毋滥，没有符合条件的文章时跳过本次推送。'

function openMarkdownLink(event: MouseEvent) {
  const anchor = (event.target as HTMLElement | null)?.closest('a')
  if (!anchor) return
  event.preventDefault()
  const href = anchor.getAttribute('href')
  if (href) void Browser.OpenURL(href)
}

async function saveProcessing() {
  processingSaving.value = true
  try {
    await SaveResultProcessing({...resultProcessing})
    message.success('AI 最终结果处理配置已保存')
  } catch (e) { message.error(errorText(e)) }
  finally { processingSaving.value = false }
}

async function testProcessing() {
  processingTesting.value = true
  processingError.value = ''
  processingPreview.value = null
  processingPreviewPrompt.value = resultProcessing.prompt
  processingPreviewOpen.value = true
  try {
    processingPreview.value = requireValue(await PreviewResultProcessing(processingPreviewPrompt.value), 'ProcessingPreviewDTO')
  } catch (e) { processingError.value = errorText(e) }
  finally { processingTesting.value = false }
}

const rebuilding = ref(false)
const rebuildResult = ref<HistoryIndexDTO | null>(null)
const rebuildError = ref('')

onMounted(load)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [configRaw, secretsRaw] = await Promise.all([ReadConfig(), ReadSecrets()])
    const config = requireValue(configRaw, 'ConfigView')
    const secrets = requireValue(secretsRaw, 'SecretsView')

    configPath.value = config.path
    configExists.value = config.exists
    preservedKeys.value = config.preservedKeys ?? []
    Object.assign(form, config.feed)
    Object.assign(schedule, config.schedule)
    Object.assign(agent, config.agent)
    Object.assign(resultProcessing, config.resultProcessing ?? {enabled: false, prompt: ''})
    agentProviderOptions.value = (config.agentProviders ?? []).map(value => ({label: value, value}))
    webhooks.value = (config.webhooks ?? []).map(row => ({
      name: row.name ?? '',
      url: row.url ?? '',
      enabled: row.enabled ?? true
    }))
    httpProxy.value = config.httpProxy ?? ''
    httpProxyConfigured.value = httpProxy.value.trim() !== ''

    secretsPath.value = secrets.path
    agentAPIKeyConfigured.value = secrets.agentApiKeyConfigured
  } catch (e) {
    loadError.value = errorText(e)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await SaveConfig({
      maxInformFeedSize: form.maxInformFeedSize,
      feedExpireDays: form.feedExpireDays,
      sameSiteMaxCount: form.sameSiteMaxCount,
      maxFetchNum: form.maxFetchNum
    })
    message.success('配置已保存，下一次 informer 运行即生效')
    await load()
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    saving.value = false
  }
}

async function saveSchedule() {
  scheduleSaving.value = true
  try {
    await SaveSchedule({
      enabled: schedule.enabled,
      time: schedule.time
    })
    message.success('定时任务已保存，应用保持打开时按时推送')
    await load()
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    scheduleSaving.value = false
  }
}

async function saveAgent() {
  agentSaving.value = true
  try {
    await SaveAgentConfig({
      provider: agent.provider,
      baseURL: agent.baseURL,
      model: agent.model,
      allowedTools: agent.allowedTools,
      timeoutSeconds: agent.timeoutSeconds,
      command: agent.command
    })
    message.success('Agent 配置已保存，下一次抓取即生效')
    await load()
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    agentSaving.value = false
  }
}

async function detectAgentCommand() {
  agentDetecting.value = true
  try {
    const path = await DetectAgentCommand(agent.provider)
    if (!path) {
      message.warning('未找到可执行文件')
      return
    }
    agent.command = path
    message.success('已找到可执行文件，请保存 Agent 配置')
  } catch (e) {
    message.error(`查找失败：${errorText(e)}`)
  } finally {
    agentDetecting.value = false
  }
}

async function saveAgentAPIKey() {
  agentAPIKeySaving.value = true
  try {
    await SaveAgentAPIKey(agentAPIKeyInput.value)
    message.success(agentAPIKeyInput.value.trim() === '' ? '已清除 API Key' : 'API Key 已保存')
    agentAPIKeyInput.value = ''
    await load()
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    agentAPIKeySaving.value = false
  }
}

function addWebhook() {
  webhooks.value.push({name: '', url: '', enabled: true})
}

function removeWebhook(index: number) {
  webhooks.value.splice(index, 1)
}

// saveWebhooks replaces the whole list, so a removed row is gone from the file and
// a paused row keeps its address for the next enable. An empty address is refused by
// the backend rather than silently dropped here, so the page can point at the row.
async function saveWebhooks() {
  webhookSaving.value = true
  try {
    await SaveWebhooks(
      webhooks.value.map(row => ({
        name: row.name,
        url: row.url,
        enabled: row.enabled
      }))
    )
    message.success(webhooks.value.length === 0 ? '已清除机器人目标' : '机器人目标已保存')
    await load()
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    webhookSaving.value = false
  }
}

async function saveHTTPProxy() {
  httpProxySaving.value = true
  try {
    await SaveHTTPProxy(httpProxyInput.value)
    message.success(httpProxyInput.value.trim() === '' ? '已清除代理' : '代理已保存并立即生效')
    httpProxyInput.value = ''
    await load()
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    httpProxySaving.value = false
  }
}

async function rebuild() {
  rebuilding.value = true
  rebuildError.value = ''
  rebuildResult.value = null
  try {
    rebuildResult.value = await RebuildHistoryIndex()
  } catch (e) {
    rebuildError.value = errorText(e)
  } finally {
    rebuilding.value = false
  }
}
</script>

<template>
  <div class="page">
    <n-alert v-if="loadError" type="error" title="配置加载失败" style="margin-bottom: 12px">
      {{ loadError }}
      <div style="margin-top: 8px">
        <n-button size="tiny" @click="load">重试</n-button>
      </div>
    </n-alert>

    <n-spin :show="loading">
      <!-- 单列纵向排列的紧凑卡片 -->
      <div class="stack">
        <n-card
          size="small"
          :header-style="cardHeaderStyle"
          :content-style="cardContentStyle"
          :footer-style="cardFooterStyle"
        >
          <template #header>
            <span class="card-title">
              抓取与推荐
              <n-tooltip :style="tipStyle">
                <template #trigger><span class="hint">?</span></template>
                配置文件 {{ configPath }}。保存时整份文件被原子替换，命令行与桌面端读取的是同一份配置。
                「每个订阅抓取数」填 0 表示不做全局限制，由各订阅自己的设置决定。
                <template v-if="preservedKeys.length > 0">
                  文件中还有本页不编辑的字段（{{ preservedKeys.join('、') }}），保存时会原样保留。
                </template>
              </n-tooltip>
            </span>
          </template>
          <template #header-extra>
            <n-space :size="6" align="center">
              <n-tag v-if="!configExists" size="tiny" type="warning" :bordered="false">尚未创建</n-tag>
              <n-tag v-if="preservedKeys.length > 0" size="tiny" type="info" :bordered="false">
                保留 {{ preservedKeys.length }} 项
              </n-tag>
              <n-button size="tiny" quaternary :disabled="loading" @click="load">重读</n-button>
              <n-button size="tiny" :loading="saving" type="primary" @click="save">保存</n-button>
            </n-space>
          </template>

          <n-form :model="form" label-placement="left" label-width="104" size="small" :show-feedback="false">
            <n-grid :cols="2" :x-gap="10" :y-gap="8">
              <n-gi>
                <n-form-item label="单次推荐数">
                  <n-input-number v-model:value="form.maxInformFeedSize" :min="1" :max="1000" size="small" style="width: 100%" />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item label="有效天数">
                  <n-input-number v-model:value="form.feedExpireDays" :min="1" :max="36500" size="small" style="width: 100%" />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item label="同站点上限">
                  <n-input-number v-model:value="form.sameSiteMaxCount" :min="1" :max="1000" size="small" style="width: 100%" />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item label="单订阅抓取">
                  <n-input-number v-model:value="form.maxFetchNum" :min="0" :max="1000" size="small" style="width: 100%" />
                </n-form-item>
              </n-gi>
            </n-grid>
          </n-form>

          <template #footer>
            <n-text depth="3" class="tiny">
              配置文件：<n-ellipsis style="max-width: 100%">{{ configPath }}</n-ellipsis>
            </n-text>
          </template>
        </n-card>

        <n-card title="AI 最终结果处理" size="small" :header-style="cardHeaderStyle" :content-style="cardContentStyle">
          <template #header-extra>
            <n-space :size="6">
              <n-button size="tiny" :disabled="!resultProcessing.prompt.trim() || processingTesting" @click="testProcessing">测试处理</n-button>
              <n-button size="tiny" type="primary" :loading="processingSaving" :disabled="resultProcessing.enabled && !resultProcessing.prompt.trim()" @click="saveProcessing">保存</n-button>
            </n-space>
          </template>
          <n-space vertical :size="10">
            <n-space align="center">
              <n-switch v-model:value="resultProcessing.enabled" size="small" />
              <n-text>开启 AI 最终结果处理</n-text>
            </n-space>
            <n-input v-model:value="resultProcessing.prompt" type="textarea" :autosize="{minRows: 4, maxRows: 12}" :maxlength="20000" show-count placeholder="描述文章筛选标准、质量阈值，以及希望生成的分析和导读。" />
            <n-space justify="space-between">
              <n-text depth="3" class="tiny">{{ resultProcessing.enabled ? '开启后，处理结果将作为正式日报和推送内容。' : '当前关闭，推送使用原始日报。' }}</n-text>
              <n-button size="tiny" quaternary :disabled="processingTesting" @click="resultProcessing.prompt = processingExample">填入示例</n-button>
            </n-space>
            <n-text depth="3" class="tiny">
              沿用下方已保存的 Agent 设置。根据标题、来源和时间筛选，不读取正文；全部过滤则跳过推送，处理失败则停止本次推送。
              测试使用文章库中尚未通知的候选，会调用模型，不保存表单、不抓取新文章、不发送推送。
            </n-text>
          </n-space>
        </n-card>

        <n-card
          size="small"
          :header-style="cardHeaderStyle"
          :content-style="cardContentStyle"
          :footer-style="cardFooterStyle"
        >
          <template #header>
            <span class="card-title">
              Agent
              <n-tooltip :style="tipStyle">
                <template #trigger><span class="hint">?</span></template>
                「agent」类型订阅和 AI 最终结果处理共用此配置；留空的项使用本机上该 Agent 自身的登录与默认设置。
                「超时」填 0 表示使用默认窗口（300 秒）。可执行文件留空时，抓取会在 PATH、常见安装目录和登录 Shell
                中查找 claude / codex，找到后写入配置。API Key 存放在独立的 0600 文件 {{ secretsPath }} 中，
                不会写入 informer.json。
              </n-tooltip>
            </span>
          </template>
          <template #header-extra>
            <n-space :size="6" align="center">
              <n-tag size="tiny" :type="agentAPIKeyConfigured ? 'success' : 'default'" :bordered="false">
                {{ agentAPIKeyConfigured ? 'Key 已配置' : 'Key 用本机登录' }}
              </n-tag>
              <n-button size="tiny" :loading="agentSaving" type="primary" @click="saveAgent">保存</n-button>
            </n-space>
          </template>

          <n-form label-placement="left" label-width="72" size="small" :show-feedback="false">
            <n-grid :cols="2" :x-gap="10" :y-gap="8">
              <n-gi>
                <n-form-item label="类型">
                  <n-select v-model:value="agent.provider" :options="agentProviderOptions" size="small" />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item label="超时秒">
                  <n-input-number v-model:value="agent.timeoutSeconds" :min="0" :max="3600" size="small" style="width: 100%" />
                </n-form-item>
              </n-gi>
              <n-gi :span="2">
                <n-form-item label="接口地址">
                  <n-input v-model:value="agent.baseURL" size="small" placeholder="留空用默认，如 https://api.anthropic.com" />
                </n-form-item>
              </n-gi>
              <n-gi :span="2">
                <n-form-item label="模型">
                  <n-input v-model:value="agent.model" size="small" placeholder="留空用 Agent 默认，如 claude-sonnet-5" />
                </n-form-item>
              </n-gi>
              <n-gi :span="2">
                <n-form-item label="可用工具">
                  <n-input v-model:value="agent.allowedTools" size="small" placeholder="逗号分隔，如 WebSearch,WebFetch" />
                </n-form-item>
              </n-gi>
              <n-gi :span="2">
                <n-form-item label="可执行">
                  <n-input-group>
                    <n-input v-model:value="agent.command" size="small" placeholder="留空则运行时自动查找并记住" />
                    <n-button size="small" :loading="agentDetecting" @click="detectAgentCommand">查找</n-button>
                  </n-input-group>
                </n-form-item>
              </n-gi>
              <n-gi :span="2">
                <n-form-item label="API Key">
                  <n-input-group>
                    <n-input
                      v-model:value="agentAPIKeyInput"
                      size="small"
                      type="password"
                      show-password-on="click"
                      placeholder="粘贴 API Key；留空保存表示清除"
                    />
                    <n-button size="small" :loading="agentAPIKeySaving" @click="saveAgentAPIKey">存 Key</n-button>
                  </n-input-group>
                </n-form-item>
              </n-gi>
            </n-grid>
          </n-form>
        </n-card>

        <n-card
          size="small"
          :header-style="cardHeaderStyle"
          :content-style="cardContentStyle"
        >
          <template #header>
            <span class="card-title">
              推送与网络
              <n-tooltip :style="tipStyle">
                <template #trigger><span class="hint">?</span></template>
                机器人目标写入配置文件顶层 webhooks 数组，每次运行全量推送给所有启用的目标，
                命令行显式传入的地址会替换本次运行的目标列表；
                某个目标失败不影响其他目标，只有全部失败才算本次推送失败，日报与文章的通知时间仍然照常记录。
                代理写入顶层 http_proxy 字段，订阅抓取、机器人推送与 Agent 子进程都会走该代理，保存后立即生效，
                与 Agent 的 base_url（API 网关）无关。
              </n-tooltip>
            </span>
          </template>
          <template #header-extra>
            <n-space :size="6" align="center">
              <n-tag size="tiny" :type="enabledWebhookCount > 0 ? 'success' : 'default'" :bordered="false">
                {{ enabledWebhookCount > 0 ? `${enabledWebhookCount} 个启用` : '未配置' }}
              </n-tag>
              <n-button size="tiny" quaternary @click="addWebhook">新增目标</n-button>
              <n-button size="tiny" :loading="webhookSaving" type="primary" @click="saveWebhooks">保存</n-button>
            </n-space>
          </template>

          <n-space vertical :size="10">
            <div class="webhook-list">
              <div v-for="(row, index) in webhooks" :key="index" class="webhook-row">
                <n-switch v-model:value="row.enabled" size="small" />
                <n-input
                  v-model:value="row.url"
                  size="small"
                  placeholder="粘贴钉钉 / 飞书机器人 webhook"
                />
                <n-input v-model:value="row.name" size="small" placeholder="备注（可选）" class="webhook-name" />
                <n-button size="small" quaternary @click="removeWebhook(index)">删除</n-button>
              </div>
              <n-text v-if="webhooks.length === 0" depth="3" class="tiny">
                未配置目标时只生成日报，不发送推送。
              </n-text>
            </div>

            <div>
              <span class="label-line">
                代理
                <n-tag size="tiny" :type="httpProxyConfigured ? 'success' : 'default'" :bordered="false">
                  {{ httpProxyConfigured ? '已配置' : '未配置' }}
                </n-tag>
              </span>
              <n-input-group style="margin-top: 6px">
                <n-input
                  v-model:value="httpProxyInput"
                  size="small"
                  :placeholder="httpProxy || '例如 http://127.0.0.1:7890；留空保存表示清除'"
                />
                <n-button size="small" :loading="httpProxySaving" type="primary" @click="saveHTTPProxy">保存代理</n-button>
              </n-input-group>
            </div>
          </n-space>
        </n-card>

        <n-card
          size="small"
          :header-style="cardHeaderStyle"
          :content-style="cardContentStyle"
        >
          <template #header>
            <span class="card-title">
              定时任务
              <n-tooltip :style="tipStyle">
                <template #trigger><span class="hint">?</span></template>
                桌面端定时仅在应用保持打开时生效，关闭后不会触发；当天已过设定时间才打开应用，会补推一次。
                成功推送后会记下日期，同一天重启不会再补推；失败则在下次轮询重试。
                若服务器同时用系统 crontab 运行命令行版本，两边可能重复推送。
              </n-tooltip>
            </span>
          </template>
          <template #header-extra>
            <n-button size="tiny" :loading="scheduleSaving" type="primary" @click="saveSchedule">保存</n-button>
          </template>

          <n-form label-placement="left" label-width="72" size="small" :show-feedback="false">
            <n-grid :cols="2" :x-gap="10">
              <n-gi>
                <n-form-item label="启用">
                  <n-switch v-model:value="schedule.enabled" size="small" />
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item label="推送时间">
                  <n-time-picker
                    v-model:formatted-value="schedule.time"
                    value-format="HH:mm"
                    format="HH:mm"
                    size="small"
                    :disabled="!schedule.enabled"
                    style="width: 100%"
                  />
                </n-form-item>
              </n-gi>
            </n-grid>
          </n-form>
        </n-card>

        <n-card
          size="small"
          :header-style="cardHeaderStyle"
          :content-style="cardContentStyle"
        >
          <template #header>
            <span class="card-title">
              重建索引
              <n-tooltip :style="tipStyle">
                <template #trigger><span class="hint">?</span></template>
                扫描已生成的日报 Markdown，提取文章链接，为「链接唯一匹配且通知时间为空」的文章补上
                通知时间（精确到日报当天）；已有时间不会被覆盖，匹配不到或匹配到多条的保持为空，
                重复执行结果一致。手动推送已移到「订阅」页面。
              </n-tooltip>
            </span>
          </template>
          <template #header-extra>
            <n-button size="tiny" :loading="rebuilding" @click="rebuild">重建索引</n-button>
          </template>

          <n-space vertical :size="8">
            <n-text v-if="!rebuildError && !rebuildResult" depth="3" class="tiny">
              从历史日报回补文章的通知时间。
            </n-text>

            <n-alert v-if="rebuildError" type="error" title="重建失败" :bordered="false">
              {{ rebuildError }}
            </n-alert>
            <n-alert v-else-if="rebuildResult" type="success" title="重建完成" :bordered="false">
              扫描 {{ rebuildResult.days }} 天日报、{{ rebuildResult.links }} 条链接：
              成功补齐 {{ rebuildResult.filled }} 条，跳过 {{ rebuildResult.skipped }} 条
              （已有时间 {{ rebuildResult.skippedAlreadyIndexed }}、
              库中无此链接 {{ rebuildResult.skippedUnmatched }}、
              匹配到多条 {{ rebuildResult.skippedAmbiguous }}），
              失败 {{ rebuildResult.failed }} 条。
              <ul v-if="(rebuildResult.errors ?? []).length > 0" style="margin: 6px 0 0; padding-left: 18px">
                <li v-for="(err, i) in (rebuildResult.errors ?? [])" :key="i">{{ err }}</li>
              </ul>
            </n-alert>
          </n-space>
        </n-card>
      </div>
    </n-spin>
  </div>
  <n-modal v-model:show="processingPreviewOpen" preset="card" title="AI 最终结果处理 · 测试预览" style="width: min(1000px, 92vw); max-height: 88vh; overflow: auto" :mask-closable="!processingTesting">
    <n-space vertical :size="14">
      <n-text depth="3">本次使用当前提示词与已保存的 Agent 设置；结果仅供预览。</n-text>
      <n-alert v-if="processingTesting" type="info" :bordered="false">正在处理候选文章，可在右上角日志查看进度……</n-alert>
      <n-alert v-if="processingError" type="error" :bordered="false">{{ processingError }}</n-alert>
      <template v-if="processingPreview">
        <n-alert :type="processingPreview.action === 'skip' ? 'warning' : 'success'" :bordered="false">
          {{ processingPreview.action === 'skip' ? `将跳过推送：${processingPreview.summary}` : '处理完成，以下为最终日报预览。' }}
        </n-alert>
        <n-grid :cols="2" :x-gap="16">
          <n-gi><n-card title="原始候选" size="small"><pre class="processing-original">{{ processingPreview.original }}</pre></n-card></n-gi>
          <n-gi><n-card title="处理结果" size="small"><div class="markdown-body" @click="openMarkdownLink" v-html="renderMarkdown(processingPreview.content || processingPreview.summary)" /></n-card></n-gi>
        </n-grid>
        <n-text strong>筛选明细</n-text>
        <div v-for="decision in processingPreview.decisions" :key="decision.id">
          <n-tag size="small" :type="decision.selected ? 'success' : 'default'">{{ decision.selected ? '保留' : '过滤' }}</n-tag>
          <n-text> {{ decision.title }}：{{ decision.reason }}</n-text>
        </div>
      </template>
    </n-space>
  </n-modal>
</template>

<style scoped>
.page {
  height: 100%;
  overflow-y: auto;
  padding: 12px;
}

/* 单列纵向排列，卡片内部保持紧凑 */
.stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.card-title {
  display: inline-flex;
  align-items: center;
  font-size: 14px;
  font-weight: 500;
}

.label-line {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

/* 目标列表逐行排列：开关 / 地址 / 备注 / 删除，地址占满剩余宽度 */
.webhook-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.webhook-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.webhook-name {
  flex: none;
  width: 160px;
}

.hint {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  margin-left: 5px;
  border: 1px solid #d5d5db;
  border-radius: 50%;
  color: #9a9aa5;
  font-size: 10px;
  font-weight: 400;
  line-height: 1;
  cursor: help;
}

.tiny {
  font-size: 12px;
}

.processing-original { white-space: pre-wrap; overflow-wrap: anywhere; margin: 0; font: inherit; }
</style>
