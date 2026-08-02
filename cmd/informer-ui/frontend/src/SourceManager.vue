<script setup lang="ts">
import {computed, h, onMounted, reactive, ref} from 'vue'
import {
  NAlert,
  NButton,
  NDataTable,
  NDivider,
  NDrawer,
  NDrawerContent,
  NEllipsis,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NLayout,
  NLayoutContent,
  NLayoutFooter,
  NLayoutHeader,
  NList,
  NListItem,
  NModal,
  NPopconfirm,
  NResult,
  NSelect,
  NSpace,
  NSpin,
  NSwitch,
  NTag,
  NText,
  NTooltip,
  useMessage,
  type DataTableColumns
} from 'naive-ui'
import {BrowserOpenURL} from '../wailsjs/runtime/runtime'
import {
  CreateSource,
  DeleteSource,
  HomeDir,
  ListSources,
  PreviewSource,
  SetSourceEnabled,
  StartupError,
  UpdateSource,
  Version
} from '../wailsjs/go/main/App'
import type {main} from '../wailsjs/go/models'

type SourceDTO = main.SourceDTO
type ArticleDTO = main.ArticleDTO

// SaveSourceRequest mirrors the Go request object one to one, so nothing the
// service returns round trips through an implicit shape.
interface SourceForm {
  id: number
  title: string
  url: string
  curl: string
  weight: number
  maxFetchNum: number
  regex: string
  titleExp: string
  urlExp: string
  redirect: boolean
  sort: boolean
  isJSON: boolean
  jsonTitlePath: string
  jsonURLPath: string
  parseType: string
  categoryId: number
  enabled: boolean
}

const message = useMessage()

const version = ref('')
const homeDir = ref('')
const startupError = ref('')

const sources = ref<SourceDTO[]>([])
const loading = ref(false)
const listError = ref('')

const showModal = ref(false)
const saving = ref(false)
const togglingId = ref(0)

const showPreview = ref(false)
const previewLoading = ref(false)
const previewError = ref('')
const previewRow = ref<SourceDTO | null>(null)
const previewArticles = ref<ArticleDTO[]>([])

const form = reactive<SourceForm>(emptyForm())

function emptyForm(): SourceForm {
  return {
    id: 0,
    title: '',
    url: '',
    curl: '',
    weight: 0,
    maxFetchNum: 0,
    regex: '',
    titleExp: '',
    urlExp: '',
    redirect: false,
    sort: false,
    isJSON: false,
    jsonTitlePath: '',
    jsonURLPath: '',
    parseType: '',
    categoryId: 0,
    enabled: true
  }
}

const modalTitle = computed(() => (form.id === 0 ? '新增订阅' : `编辑订阅 #${form.id}`))
const showRegexFields = computed(() => form.parseType === 'regex' || form.parseType === '')
const showJsonFields = computed(() => form.parseType === 'json' || form.parseType === '')

const parseTypeOptions = [
  {label: '自动推导（按旧规则）', value: ''},
  {label: 'feed（RSS / Atom）', value: 'feed'},
  {label: 'regex（正则匹配）', value: 'regex'},
  {label: 'json（路径提取）', value: 'json'}
]

onMounted(async () => {
  version.value = await Version().catch(() => '')
  homeDir.value = await HomeDir().catch(() => '')

  const err = await StartupError().catch(() => '')
  if (err) {
    startupError.value = err
    return
  }

  await loadSources()
})

async function loadSources() {
  loading.value = true
  listError.value = ''
  try {
    sources.value = await ListSources()
  } catch (e) {
    listError.value = String(e)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, emptyForm())
  showModal.value = true
}

function openEdit(row: SourceDTO) {
  Object.assign(form, emptyForm(), {
    id: row.id,
    title: row.title,
    url: row.url,
    curl: row.curl,
    weight: row.weight,
    maxFetchNum: row.maxFetchNum,
    regex: row.regex,
    titleExp: row.titleExp,
    urlExp: row.urlExp,
    redirect: row.redirect,
    sort: row.sort,
    isJSON: row.isJSON,
    jsonTitlePath: row.jsonTitlePath,
    jsonURLPath: row.jsonURLPath,
    parseType: row.parseType,
    categoryId: row.categoryId,
    enabled: row.enabled
  })
  showModal.value = true
}

async function save() {
  if (!form.url.trim()) {
    message.warning('请填写订阅 URL')
    return
  }

  saving.value = true
  try {
    const req = {
      id: form.id,
      title: form.title,
      url: form.url,
      curl: form.curl,
      weight: form.weight,
      maxFetchNum: form.maxFetchNum,
      regex: form.regex,
      titleExp: form.titleExp,
      urlExp: form.urlExp,
      redirect: form.redirect,
      sort: form.sort,
      // an explicit json parse type keeps the legacy flag in sync; in auto
      // mode the user keeps full control of the historical field.
      isJSON: form.parseType === 'json' ? true : form.isJSON,
      jsonTitlePath: form.jsonTitlePath,
      jsonURLPath: form.jsonURLPath,
      parseType: form.parseType,
      categoryId: form.categoryId,
      enabled: form.enabled
    }

    if (form.id === 0) {
      await CreateSource(req)
      message.success('订阅已创建')
    } else {
      await UpdateSource(req)
      message.success('订阅已更新')
    }

    showModal.value = false
    await loadSources()
  } catch (e) {
    message.error(`保存失败：${String(e)}`)
  } finally {
    saving.value = false
  }
}

async function remove(row: SourceDTO) {
  try {
    await DeleteSource(row.id)
    message.success('订阅已删除')
    await loadSources()
  } catch (e) {
    message.error(`删除失败：${String(e)}`)
  }
}

async function toggleEnabled(row: SourceDTO, enabled: boolean) {
  togglingId.value = row.id
  try {
    await SetSourceEnabled(row.id, enabled)
    await loadSources()
  } catch (e) {
    message.error(`启停失败：${String(e)}`)
    await loadSources()
  } finally {
    togglingId.value = 0
  }
}

async function openPreview(row: SourceDTO) {
  previewRow.value = row
  previewArticles.value = []
  previewError.value = ''
  previewLoading.value = true
  showPreview.value = true
  try {
    previewArticles.value = await PreviewSource(row.id)
  } catch (e) {
    previewError.value = String(e)
  } finally {
    previewLoading.value = false
  }
}

function openArticle(url: string) {
  BrowserOpenURL(url)
}

const columns: DataTableColumns<SourceDTO> = [
  {title: 'ID', key: 'id', width: 64},
  {
    title: '标题',
    key: 'title',
    width: 180,
    render: row =>
      h(NEllipsis, null, {
        default: () => row.title || h(NText, {depth: 3}, {default: () => '（无标题）'})
      })
  },
  {
    title: 'URL',
    key: 'url',
    ellipsis: {tooltip: true}
  },
  {
    title: '解析类型',
    key: 'resolvedParseType',
    width: 100,
    render: row => h(NTag, {size: 'small', bordered: false}, {default: () => row.resolvedParseType})
  },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: row => {
      if (row.status === 2) {
        return h(
          NTooltip,
          null,
          {
            trigger: () => h(NTag, {size: 'small', type: 'error'}, {default: () => '抓取失败'}),
            default: () => row.errorInfo || '未知错误'
          }
        )
      }
      if (row.status === 1) {
        return h(NTag, {size: 'small', type: 'success'}, {default: () => '正常'})
      }
      return h(NTag, {size: 'small', bordered: false}, {default: () => '未抓取'})
    }
  },
  {
    title: '启用',
    key: 'enabled',
    width: 70,
    render: row =>
      h(NSwitch, {
        value: row.enabled,
        loading: togglingId.value === row.id,
        onUpdateValue: (v: boolean) => void toggleEnabled(row, v)
      })
  },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render: row =>
      h(NSpace, {size: 'small'}, {
        default: () => [
          h(NButton, {size: 'small', onClick: () => openPreview(row)}, {default: () => '测试抓取'}),
          h(NButton, {size: 'small', tertiary: true, onClick: () => openEdit(row)}, {default: () => '编辑'}),
          h(
            NPopconfirm,
            {onPositiveClick: () => void remove(row)},
            {
              trigger: () =>
                h(NButton, {size: 'small', tertiary: true, type: 'error'}, {default: () => '删除'}),
              default: () => `确认删除订阅「${row.title || row.url}」吗？`
            }
          )
        ]
      })
  }
]
</script>

<template>
  <n-layout class="page" position="absolute">
    <n-layout-header bordered class="header">
      <n-space align="center" :size="12">
        <n-text strong style="font-size: 16px">Informer 订阅管理</n-text>
        <n-tag v-if="version" :bordered="false" size="small" type="info">{{ version }}</n-tag>
        <n-tooltip v-if="homeDir">
          <template #trigger>
            <n-text depth="3" style="font-size: 12px">
              <n-ellipsis style="max-width: 320px">{{ homeDir }}</n-ellipsis>
            </n-text>
          </template>
          数据目录：{{ homeDir }}
        </n-tooltip>
      </n-space>
      <n-space>
        <n-button :loading="loading" size="small" tertiary @click="loadSources">刷新</n-button>
        <n-button size="small" type="primary" @click="openCreate">新增订阅</n-button>
      </n-space>
    </n-layout-header>

    <n-layout-content content-style="padding: 16px;" :native-scrollbar="false">
      <n-result
        v-if="startupError"
        status="error"
        title="启动失败"
        :description="startupError"
        style="margin-top: 80px"
      >
        <template #footer>
          <n-text depth="3">数据目录初始化或 Service 创建失败，请检查 INFORMER_HOME 与磁盘权限后重启应用。</n-text>
        </template>
      </n-result>

      <template v-else>
        <n-alert v-if="listError" type="error" :title="'订阅列表加载失败'" closable>
          {{ listError }}
          <n-button size="tiny" style="margin-left: 8px" @click="loadSources">重试</n-button>
        </n-alert>

        <n-data-table
          :columns="columns"
          :data="sources"
          :loading="loading"
          :row-key="(row: SourceDTO) => row.id"
          :pagination="{pageSize: 15}"
          size="small"
          striped
        />

        <n-empty
          v-if="!loading && !listError && sources.length === 0"
          description="还没有订阅，点击右上角「新增订阅」开始"
          style="margin-top: 60px"
        />
      </template>
    </n-layout-content>

    <n-layout-footer bordered position="absolute" style="height: 36px; line-height: 36px; padding: 0 16px">
      <n-text depth="3" style="font-size: 12px">
        informer 桌面版 {{ version }} · 测试抓取执行真实网络请求，但不写库、不修改订阅状态
      </n-text>
    </n-layout-footer>

    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="modalTitle"
      style="width: 640px"
      :mask-closable="false"
    >
      <n-form :model="form" label-placement="left" label-width="auto">
        <n-form-item label="标题">
          <n-input v-model:value="form.title" placeholder="订阅名称，例如：阮一峰 blog" />
        </n-form-item>
        <n-form-item label="URL" required>
          <n-input v-model:value="form.url" placeholder="https://example.com/atom.xml" />
        </n-form-item>
        <n-form-item label="自定义请求">
          <n-input
            v-model:value="form.curl"
            type="textarea"
            placeholder="可选：以 curl 命令描述带请求头 / 请求体的抓取方式"
            :autosize="{minRows: 2, maxRows: 4}"
          />
        </n-form-item>
        <n-form-item label="解析类型">
          <n-select v-model:value="form.parseType" :options="parseTypeOptions" />
        </n-form-item>

        <template v-if="showRegexFields">
          <n-divider title-placement="left" style="margin: 8px 0">正则解析参数</n-divider>
          <n-form-item label="正则表达式">
            <n-input v-model:value="form.regex" placeholder='例如：,"url":"([^"]+)","title":"([^"]+)",' />
          </n-form-item>
          <n-form-item label="标题表达式">
            <n-input v-model:value="form.titleExp" placeholder="例如：$2" />
          </n-form-item>
          <n-form-item label="链接表达式">
            <n-input v-model:value="form.urlExp" placeholder="例如：$1" />
          </n-form-item>
        </template>

        <template v-if="showJsonFields">
          <n-divider title-placement="left" style="margin: 8px 0">JSON 解析参数</n-divider>
          <n-form-item label="标题路径">
            <n-input v-model:value="form.jsonTitlePath" placeholder="例如：data[].title" />
          </n-form-item>
          <n-form-item label="链接路径">
            <n-input v-model:value="form.jsonURLPath" placeholder="例如：data[].url" />
          </n-form-item>
          <n-form-item v-if="form.parseType === ''" label="is_json 旧标志">
            <n-switch v-model:value="form.isJSON" />
          </n-form-item>
        </template>

        <n-divider title-placement="left" style="margin: 8px 0">抓取选项</n-divider>
        <n-form-item label="最大抓取数">
          <n-input-number v-model:value="form.maxFetchNum" :min="0" placeholder="0 使用全局默认" style="width: 100%" />
        </n-form-item>
        <n-form-item label="排序权重">
          <n-input-number v-model:value="form.weight" :min="0" style="width: 100%" />
        </n-form-item>
        <n-form-item label="链接重定向">
          <n-switch v-model:value="form.redirect" />
        </n-form-item>
        <n-form-item label="结果排序">
          <n-switch v-model:value="form.sort" />
        </n-form-item>
        <n-form-item v-if="form.id !== 0" label="启用">
          <n-switch v-model:value="form.enabled" />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">取消</n-button>
          <n-button :loading="saving" type="primary" @click="save">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-drawer v-model:show="showPreview" :width="520">
      <n-drawer-content
        :title="previewRow ? `测试抓取：${previewRow.title || previewRow.url}` : '测试抓取'"
        closable
      >
        <n-spin :show="previewLoading">
          <n-alert v-if="previewError" type="error" title="抓取失败">
            {{ previewError }}
            <div style="margin-top: 8px">
              <n-button v-if="previewRow" size="tiny" @click="openPreview(previewRow)">重试</n-button>
            </div>
          </n-alert>

          <template v-else-if="!previewLoading">
            <n-text depth="3" style="font-size: 12px">
              共解析出 {{ previewArticles.length }} 条候选文章（停用订阅同样可以预览；预览不写库）
            </n-text>
            <n-list v-if="previewArticles.length > 0" bordered style="margin-top: 8px">
              <n-list-item v-for="(a, i) in previewArticles" :key="i">
                <n-ellipsis :line-clamp="2">{{ a.title }}</n-ellipsis>
                <template #suffix>
                  <n-button size="tiny" text type="primary" @click="openArticle(a.url)">打开</n-button>
                </template>
              </n-list-item>
            </n-list>
            <n-empty v-else description="没有解析到文章" style="margin-top: 24px" />
          </template>
        </n-spin>
      </n-drawer-content>
    </n-drawer>
  </n-layout>
</template>

<style scoped>
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  height: 52px;
}
</style>
