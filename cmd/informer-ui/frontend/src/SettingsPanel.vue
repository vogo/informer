<script setup lang="ts">
import {onMounted, reactive, ref} from 'vue'
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
  SaveWebhook,
  type HistoryIndexDTO,
} from './bindings'
import {errorText} from './errors'
import {requireValue} from './nulls'

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
                「agent」类型的订阅会调用命令行 Agent 抓取；留空的项使用本机上该 Agent 自身的登录与默认设置。
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
                机器人地址写入配置文件顶层 webhook 字段，命令行显式传入的地址仍然优先；
                代理写入顶层 http_proxy 字段，订阅抓取、机器人推送与 Agent 子进程都会走该代理，保存后立即生效，
                与 Agent 的 base_url（API 网关）无关。两者留空保存表示清除。
              </n-tooltip>
            </span>
          </template>

          <n-form label-placement="left" label-width="72" size="small" :show-feedback="false">
            <n-grid :cols="1" :y-gap="8">
              <n-gi>
                <n-form-item>
                  <template #label>
                    <span class="label-line">
                      机器人
                      <n-tag size="tiny" :type="webhookConfigured ? 'success' : 'default'" :bordered="false">
                        {{ webhookConfigured ? '已配置' : '未配置' }}
                      </n-tag>
                    </span>
                  </template>
                  <n-input-group>
                    <n-input
                      v-model:value="webhookInput"
                      size="small"
                      :placeholder="webhook || '粘贴钉钉 / 飞书机器人 webhook'"
                    />
                    <n-button size="small" :loading="webhookSaving" type="primary" @click="saveWebhook">保存</n-button>
                  </n-input-group>
                </n-form-item>
              </n-gi>
              <n-gi>
                <n-form-item>
                  <template #label>
                    <span class="label-line">
                      代理
                      <n-tag size="tiny" :type="httpProxyConfigured ? 'success' : 'default'" :bordered="false">
                        {{ httpProxyConfigured ? '已配置' : '未配置' }}
                      </n-tag>
                    </span>
                  </template>
                  <n-input-group>
                    <n-input
                      v-model:value="httpProxyInput"
                      size="small"
                      :placeholder="httpProxy || '例如 http://127.0.0.1:7890'"
                    />
                    <n-button size="small" :loading="httpProxySaving" type="primary" @click="saveHTTPProxy">保存</n-button>
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
</style>
