<script setup lang="ts">
import {onMounted, reactive, ref} from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NCode,
  NDescriptions,
  NDescriptionsItem,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NPopconfirm,
  NSpace,
  NSelect,
  NSpin,
  NSwitch,
  NTag,
  NText,
  NTimePicker,
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
  SaveWebhook,
  TriggerNow,
  type HistoryIndexDTO,
  type InformResultDTO,
} from './bindings'
import {errorText} from './errors'
import {requireValue} from './nulls'

const message = useMessage()

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
const webhookConfigured = ref(false)
const webhook = ref('')
const webhookInput = ref('')
const webhookSaving = ref(false)

const httpProxyConfigured = ref(false)
const httpProxy = ref('')
const httpProxyInput = ref('')
const httpProxySaving = ref(false)

const schedule = reactive({
  enabled: false,
  time: '10:00'
})
const scheduleSaving = ref(false)

const triggering = ref(false)
const triggerResult = ref<InformResultDTO | null>(null)
const triggerError = ref('')

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
    agentProviderOptions.value = (config.agentProviders ?? []).map(value => ({label: value, value}))
    webhook.value = config.webhook ?? ''
    webhookConfigured.value = webhook.value.trim() !== ''
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

async function triggerNow() {
  triggering.value = true
  triggerError.value = ''
  triggerResult.value = null
  try {
    triggerResult.value = await TriggerNow()
  } catch (e) {
    triggerError.value = errorText(e)
  } finally {
    triggering.value = false
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

async function saveWebhook() {
  webhookSaving.value = true
  try {
    await SaveWebhook(webhookInput.value)
    message.success(webhookInput.value.trim() === '' ? '已清除机器人地址' : '机器人地址已保存')
    webhookInput.value = ''
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
    <n-alert v-if="loadError" type="error" title="配置加载失败" style="margin-bottom: 16px">
      {{ loadError }}
      <div style="margin-top: 8px">
        <n-button size="tiny" @click="load">重试</n-button>
      </div>
    </n-alert>

    <n-spin :show="loading">
      <n-card size="small" title="抓取与推荐配置" style="margin-bottom: 16px">
        <template #header-extra>
          <n-tag v-if="!configExists" size="small" type="warning" :bordered="false">尚未创建</n-tag>
        </template>

        <n-text depth="3" style="font-size: 12px">
          配置文件：<n-code :code="configPath" inline />
          。保存时整份文件被原子替换，命令行与桌面端读取的是同一份配置。
        </n-text>

        <n-alert v-if="preservedKeys.length > 0" type="info" :bordered="false" style="margin: 10px 0">
          文件中还有本页不编辑的字段（{{ preservedKeys.join('、') }}），保存时会原样保留。
        </n-alert>

        <n-form :model="form" label-placement="left" label-width="auto" style="margin-top: 12px">
          <n-form-item label="单次推荐文章数">
            <n-input-number v-model:value="form.maxInformFeedSize" :min="1" :max="1000" style="width: 100%" />
          </n-form-item>
          <n-form-item label="文章有效天数">
            <n-input-number v-model:value="form.feedExpireDays" :min="1" :max="36500" style="width: 100%" />
          </n-form-item>
          <n-form-item label="同站点最大条数">
            <n-input-number v-model:value="form.sameSiteMaxCount" :min="1" :max="1000" style="width: 100%" />
          </n-form-item>
          <n-form-item label="每个订阅抓取数">
            <n-input-number v-model:value="form.maxFetchNum" :min="0" :max="1000" style="width: 100%" />
          </n-form-item>
        </n-form>
        <n-text depth="3" style="font-size: 12px">「每个订阅抓取数」填 0 表示不做全局限制，由各订阅自己的设置决定。</n-text>

        <template #footer>
          <n-space justify="end">
            <n-button :disabled="loading" @click="load">重新读取</n-button>
            <n-button :loading="saving" type="primary" @click="save">保存配置</n-button>
          </n-space>
        </template>
      </n-card>

      <n-card size="small" title="Agent 配置" style="margin-bottom: 16px">
        <n-text depth="3" style="font-size: 12px">
          「agent」类型的订阅会调用命令行 Agent 抓取；这里留空的项使用本机上该 Agent 自身的登录与默认设置。
        </n-text>

        <n-form label-placement="left" label-width="auto" style="margin-top: 12px">
          <n-form-item label="Agent 类型">
            <n-select v-model:value="agent.provider" :options="agentProviderOptions" />
          </n-form-item>
          <n-form-item label="接口地址">
            <n-input v-model:value="agent.baseURL" placeholder="留空使用默认；例如 https://api.anthropic.com" />
          </n-form-item>
          <n-form-item label="模型">
            <n-input v-model:value="agent.model" placeholder="留空使用 Agent 默认模型；例如 claude-sonnet-5" />
          </n-form-item>
          <n-form-item label="可用工具">
            <n-input v-model:value="agent.allowedTools" placeholder="逗号分隔，例如 WebSearch,WebFetch" />
          </n-form-item>
          <n-form-item label="超时（秒）">
            <n-input-number v-model:value="agent.timeoutSeconds" :min="0" :max="3600" style="width: 100%" />
          </n-form-item>
          <n-form-item label="可执行文件">
            <n-space :size="8" style="width: 100%">
              <n-input
                v-model:value="agent.command"
                placeholder="留空则运行时自动查找并记住；例如 /opt/homebrew/bin/claude"
                style="flex: 1; min-width: 0"
              />
              <n-button :loading="agentDetecting" @click="detectAgentCommand">自动查找</n-button>
            </n-space>
          </n-form-item>
        </n-form>
        <n-text depth="3" style="font-size: 12px">
          「超时」填 0 表示使用默认窗口（300 秒）。可执行文件留空时，抓取会在 PATH、常见安装目录和登录 Shell 中查找
          claude / codex，找到后写入配置。
        </n-text>

        <n-descriptions :column="1" size="small" style="margin: 12px 0">
          <n-descriptions-item label="API Key">
            <n-space :size="8" align="center">
              <n-tag v-if="agentAPIKeyConfigured" size="small" type="success">已配置</n-tag>
              <n-tag v-else size="small" :bordered="false">未配置（使用本机登录）</n-tag>
            </n-space>
          </n-descriptions-item>
        </n-descriptions>

        <n-space :size="8">
          <n-input
            v-model:value="agentAPIKeyInput"
            type="password"
            show-password-on="click"
            placeholder="粘贴 API Key；留空保存表示清除"
            style="flex: 1"
          />
          <n-button :loading="agentAPIKeySaving" @click="saveAgentAPIKey">保存 Key</n-button>
        </n-space>
        <n-text depth="3" style="display: block; margin-top: 8px; font-size: 12px">
          API Key 存放在独立的 0600 文件 <n-code :code="secretsPath" inline /> 中，不会写入 informer.json。
        </n-text>

        <template #footer>
          <n-space justify="end">
            <n-button :loading="agentSaving" type="primary" @click="saveAgent">保存 Agent 配置</n-button>
          </n-space>
        </template>
      </n-card>

      <n-card size="small" title="机器人地址" style="margin-bottom: 16px">
        <n-text depth="3" style="font-size: 12px">
          写入 <n-code :code="configPath" inline /> 的顶层 <n-code code="webhook" inline /> 字段，不是敏感凭证，也不入库。
        </n-text>

        <n-descriptions :column="1" size="small" style="margin: 12px 0">
          <n-descriptions-item label="当前状态">
            <n-space :size="8" align="center">
              <n-tag v-if="webhookConfigured" size="small" type="success">已配置</n-tag>
              <n-tag v-else size="small" :bordered="false">未配置</n-tag>
              <n-text v-if="webhook" depth="3" style="font-size: 12px">{{ webhook }}</n-text>
            </n-space>
          </n-descriptions-item>
        </n-descriptions>

        <n-input
          v-model:value="webhookInput"
          placeholder="粘贴钉钉 / 飞书机器人 webhook；留空保存表示清除"
        />

        <template #footer>
          <n-space justify="space-between" align="center">
            <n-text depth="3" style="font-size: 12px">
              命令行显式传入的地址仍然优先，不传时使用这里保存的地址。
            </n-text>
            <n-button :loading="webhookSaving" type="primary" @click="saveWebhook">保存地址</n-button>
          </n-space>
        </template>
      </n-card>

      <n-card size="small" title="HTTP 代理" style="margin-bottom: 16px">
        <n-text depth="3" style="font-size: 12px">
          写入 <n-code :code="configPath" inline /> 的顶层 <n-code code="http_proxy" inline /> 字段。
          有配置时，订阅抓取、机器人推送与 Agent 子进程都会走该代理。
        </n-text>

        <n-descriptions :column="1" size="small" style="margin: 12px 0">
          <n-descriptions-item label="当前状态">
            <n-space :size="8" align="center">
              <n-tag v-if="httpProxyConfigured" size="small" type="success">已配置</n-tag>
              <n-tag v-else size="small" :bordered="false">未配置</n-tag>
              <n-text v-if="httpProxy" depth="3" style="font-size: 12px">{{ httpProxy }}</n-text>
            </n-space>
          </n-descriptions-item>
        </n-descriptions>

        <n-input
          v-model:value="httpProxyInput"
          placeholder="例如 http://127.0.0.1:7890；留空保存表示清除"
        />

        <template #footer>
          <n-space justify="space-between" align="center">
            <n-text depth="3" style="font-size: 12px">
              与 Agent 的 base_url（API 网关）无关；保存后立即对后续请求生效。
            </n-text>
            <n-button :loading="httpProxySaving" type="primary" @click="saveHTTPProxy">保存代理</n-button>
          </n-space>
        </template>
      </n-card>

      <n-card size="small" title="定时任务" style="margin-bottom: 16px">
        <n-alert type="info" :bordered="false">
          桌面端定时仅在应用保持打开时生效，关闭后不会触发；当天已过设定时间才打开应用，会补推一次。
          成功推送后会记下日期，同一天重启不会再补推；失败则在下次轮询重试。
          若服务器同时用系统 crontab 运行命令行版本，两边可能重复推送。
        </n-alert>

        <n-form label-placement="left" label-width="auto" style="margin-top: 12px">
          <n-form-item label="启用定时推送">
            <n-switch v-model:value="schedule.enabled" />
          </n-form-item>
          <n-form-item label="每日推送时间">
            <n-time-picker
              v-model:formatted-value="schedule.time"
              value-format="HH:mm"
              format="HH:mm"
              :disabled="!schedule.enabled"
              style="width: 200px"
            />
          </n-form-item>
        </n-form>

        <template #footer>
          <n-space justify="end">
            <n-button :loading="scheduleSaving" type="primary" @click="saveSchedule">保存定时任务</n-button>
          </n-space>
        </template>
      </n-card>

      <n-card size="small" title="手动推送" style="margin-bottom: 16px">
        <n-text depth="3" style="font-size: 12px">
          立即执行一次完整流程：抓取所有启用的订阅、生成今日日报 Markdown，并推送给机器人；
          未配置机器人地址时只生成日报，不发送。
        </n-text>

        <n-alert v-if="triggerError" type="error" title="推送失败" style="margin-top: 12px">
          {{ triggerError }}
        </n-alert>

        <n-alert v-else-if="triggerResult && !triggerResult.success" type="warning" title="推送未完成" style="margin-top: 12px">
          {{ triggerResult.errorInfo }}
          <div v-if="triggerResult.contentFilePath" style="margin-top: 4px">
            日报已写入：<n-code :code="triggerResult.contentFilePath" inline />
          </div>
        </n-alert>

        <n-alert v-else-if="triggerResult" type="success" title="推送完成" style="margin-top: 12px">
          <template v-if="triggerResult.notified">
            已推送 {{ triggerResult.articleCount }} 篇文章到机器人。
          </template>
          <template v-else>
            日报已生成，但未配置机器人地址（或地址无法识别），未发送推送。
          </template>
          <div v-if="triggerResult.contentFilePath" style="margin-top: 4px">
            日报文件：<n-code :code="triggerResult.contentFilePath" inline />
          </div>
        </n-alert>

        <template #footer>
          <n-space justify="end">
            <n-popconfirm @positive-click="() => { triggerNow() }">
              <template #trigger>
                <n-button :loading="triggering" type="primary">立即推送</n-button>
              </template>
              将抓取所有启用的订阅并发送到机器人，确认立即推送？
            </n-popconfirm>
          </n-space>
        </template>
      </n-card>

      <n-card size="small" title="重建历史索引">
        <n-text depth="3" style="font-size: 12px">
          扫描已生成的日报 Markdown，提取其中的文章链接，为「链接唯一匹配且通知时间为空」的文章补上通知时间
          （精确到日报当天）。已有时间不会被覆盖，匹配不到或匹配到多条的文章保持为空，重复执行结果一致。
        </n-text>

        <n-alert v-if="rebuildError" type="error" title="重建失败" style="margin-top: 12px">
          {{ rebuildError }}
        </n-alert>

        <n-alert v-else-if="rebuildResult" type="success" title="重建完成" style="margin-top: 12px">
          扫描 {{ rebuildResult.days }} 天日报、{{ rebuildResult.links }} 条链接：
          成功补齐 {{ rebuildResult.filled }} 条，跳过 {{ rebuildResult.skipped }} 条
          （已有时间 {{ rebuildResult.skippedAlreadyIndexed }}、
          库中无此链接 {{ rebuildResult.skippedUnmatched }}、
          匹配到多条 {{ rebuildResult.skippedAmbiguous }}），
          失败 {{ rebuildResult.failed }} 条。
          <ul v-if="(rebuildResult.errors ?? []).length > 0" style="margin: 8px 0 0; padding-left: 18px">
            <li v-for="(err, i) in (rebuildResult.errors ?? [])" :key="i">{{ err }}</li>
          </ul>
        </n-alert>

        <template #footer>
          <n-space justify="end">
            <n-button :loading="rebuilding" @click="rebuild">开始重建</n-button>
          </n-space>
        </template>
      </n-card>
    </n-spin>
  </div>
</template>

<style scoped>
.page {
  height: 100%;
  overflow-y: auto;
  padding: 16px;
  max-width: 900px;
}
</style>
