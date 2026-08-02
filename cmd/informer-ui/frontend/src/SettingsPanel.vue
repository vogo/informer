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
  NSpace,
  NSpin,
  NTag,
  NText,
  useMessage
} from 'naive-ui'
import {ReadConfig, ReadSecrets, RebuildHistoryIndex, SaveConfig, SaveWebhook} from '../wailsjs/go/main/App'
import type {main} from '../wailsjs/go/models'
import {errorText} from './errors'

type HistoryIndexDTO = main.HistoryIndexDTO

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

const secretsPath = ref('')
const webhookConfigured = ref(false)
const webhookMasked = ref('')
const webhookInput = ref('')
const webhookSaving = ref(false)

const rebuilding = ref(false)
const rebuildResult = ref<HistoryIndexDTO | null>(null)
const rebuildError = ref('')

onMounted(load)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [config, secrets] = await Promise.all([ReadConfig(), ReadSecrets()])

    configPath.value = config.path
    configExists.value = config.exists
    preservedKeys.value = config.preservedKeys
    Object.assign(form, config.feed)

    secretsPath.value = secrets.path
    webhookConfigured.value = secrets.webhookConfigured
    webhookMasked.value = secrets.webhookMasked
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

      <n-card size="small" title="机器人地址（敏感配置）" style="margin-bottom: 16px">
        <n-text depth="3" style="font-size: 12px">
          存放于独立文件 <n-code :code="secretsPath" inline />，权限固定为 0600，不写入 informer.json，也不入库。
        </n-text>

        <n-descriptions :column="1" size="small" style="margin: 12px 0">
          <n-descriptions-item label="当前状态">
            <n-space :size="8" align="center">
              <n-tag v-if="webhookConfigured" size="small" type="success">已配置</n-tag>
              <n-tag v-else size="small" :bordered="false">未配置</n-tag>
              <n-text v-if="webhookMasked" depth="3" style="font-size: 12px">{{ webhookMasked }}</n-text>
            </n-space>
          </n-descriptions-item>
        </n-descriptions>

        <n-input
          v-model:value="webhookInput"
          type="password"
          show-password-on="click"
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
          <ul v-if="rebuildResult.errors.length > 0" style="margin: 8px 0 0; padding-left: 18px">
            <li v-for="(err, i) in rebuildResult.errors" :key="i">{{ err }}</li>
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
