<script setup lang="ts">
import {computed, onMounted, reactive, ref} from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDivider,
  NDrawer,
  NDrawerContent,
  NEllipsis,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NList,
  NListItem,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NSpin,
  NSwitch,
  NTag,
  NText,
  NTooltip,
  useMessage
} from 'naive-ui'
import {Browser} from '@wailsio/runtime'
import {
  CreateSource,
  DeleteSource,
  ListCategories,
  ListSources,
  PreviewSource,
  SetSourceEnabled,
  SupportedAgentProviders,
  SupportedParseTypes,
  UpdateSource,
  type ArticleDTO,
  type CategoryDTO,
  type SourceDTO,
} from './bindings'
import CategoryPanel from './CategoryPanel.vue'
import {errorText} from './errors'
import {compact} from './nulls'

// SourceForm mirrors the Go request object one to one, so nothing the service
// returns round trips through an implicit shape.
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
  agentProvider: string
  agentPrompt: string
  categoryId: number
  enabled: boolean
}

const message = useMessage()

const selectedCategoryId = ref(0)
const categoryPanel = ref<InstanceType<typeof CategoryPanel> | null>(null)

// the filter snapshot of this session: the sidebar picks the values, the list is
// always the answer to the whole snapshot rather than to one dimension at a time.
// Nothing is persisted, so reopening the page starts on "everything" again.
const filters = reactive({parseType: '', fetchStatus: '', enabledState: ''})

// the type options come from the backend, so a parse type added there needs no
// change here to be offered and queried.
const parseTypeFilterOptions = ref<string[]>([])

// listSeq keeps the newest query the one on screen: a slower earlier request
// that lands after a faster later one is dropped instead of overwriting it.
let listSeq = 0

const categories = ref<CategoryDTO[]>([])
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
    agentProvider: '',
    agentPrompt: '',
    categoryId: 0,
    enabled: true
  }
}

const modalTitle = computed(() => (form.id === 0 ? '新增订阅' : `编辑订阅 #${form.id}`))
const isAgentType = computed(() => form.parseType === 'agent')
const showRegexFields = computed(() => form.parseType === 'regex' || form.parseType === '')
const showJsonFields = computed(() => form.parseType === 'json' || form.parseType === '')
const categoryOptions = computed(() => categories.value.map(c => ({label: c.name, value: c.id})))
const selectedCategoryName = computed(
  () => categories.value.find(c => c.id === selectedCategoryId.value)?.name ?? '全部订阅'
)
const hasActiveFilter = computed(
  () => filters.parseType !== '' || filters.fetchStatus !== '' || filters.enabledState !== ''
)
const emptyDescription = computed(() => {
  if (hasActiveFilter.value) {
    return '没有符合当前过滤条件的订阅'
  }

  return selectedCategoryId.value === 0
    ? '还没有订阅，点击右上角「新增订阅」开始'
    : '该分类下还没有订阅'
})

const parseTypeOptions = [
  {label: '自动推导（按旧规则）', value: ''},
  {label: 'feed（RSS / Atom）', value: 'feed'},
  {label: 'regex（正则匹配）', value: 'regex'},
  {label: 'json（路径提取）', value: 'json'},
  {label: 'agent（AI 代理抓取）', value: 'agent'}
]

// the provider list comes from the backend, so an agent added there needs no
// change here to be offered.
const agentProviderOptions = ref<{label: string; value: string}[]>([
  {label: '使用全局配置', value: ''}
])

onMounted(async () => {
  await Promise.all([loadCategories(), loadParseTypeOptions(), loadAgentProviders(), loadSources()])
})

async function loadAgentProviders() {
  try {
    const providers = await SupportedAgentProviders()
    agentProviderOptions.value = [
      {label: '使用全局配置', value: ''},
      ...providers.map(value => ({label: value, value}))
    ]
  } catch (e) {
    // the form then only offers「使用全局配置」, which is the working default;
    // everything else on the page keeps functioning.
    message.error(`Agent 类型加载失败：${errorText(e)}`)
  }
}

async function loadParseTypeOptions() {
  try {
    parseTypeFilterOptions.value = await SupportedParseTypes()
  } catch (e) {
    // the sidebar then offers the type dimension as "全部" only; every other
    // filter keeps working, so this never blocks the page.
    message.error(`来源类型加载失败：${errorText(e)}`)
  }
}

async function loadCategories() {
  try {
    categories.value = compact(await ListCategories())
  } catch (e) {
    // the tree panel reports its own failure; the form just loses its options.
    message.error(`分类加载失败：${errorText(e)}`)
  }
}

async function loadSources() {
  const seq = ++listSeq

  loading.value = true
  listError.value = ''
  try {
    const listed = await ListSources({
      categoryId: selectedCategoryId.value,
      parseType: filters.parseType,
      fetchStatus: filters.fetchStatus,
      enabledState: filters.enabledState
    })

    // a newer snapshot is already in flight; this answer describes a filter the
    // sidebar no longer shows.
    if (seq !== listSeq) {
      return
    }

    sources.value = compact(listed)
  } catch (e) {
    if (seq !== listSeq) {
      return
    }

    // the previous result belongs to the previous filter, so it is dropped
    // rather than left on screen next to the new selection.
    sources.value = []
    listError.value = errorText(e)
  } finally {
    if (seq === listSeq) {
      loading.value = false
    }
  }
}

// onCategorySelected reloads the right hand list for the newly picked category.
async function onCategorySelected(id: number) {
  selectedCategoryId.value = id
  await loadSources()
}

// onFilterChanged reloads the list for one changed dimension; the other
// dimensions and the category keep their current value.
async function onFilterChanged(key: 'parseType' | 'fetchStatus' | 'enabledState', value: string) {
  filters[key] = value
  await loadSources()
}

// clearFilters returns the page to its default view: every category, every type,
// every fetch state, enabled and disabled alike.
async function clearFilters() {
  selectedCategoryId.value = 0
  filters.parseType = ''
  filters.fetchStatus = ''
  filters.enabledState = ''
  await loadSources()
}

// onCategoryTreeChanged runs after the tree created, edited or deleted a category.
async function onCategoryTreeChanged() {
  await Promise.all([loadCategories(), loadSources()])
}

function categoryName(id: number): string {
  return categories.value.find(c => c.id === id)?.name ?? '未知分类'
}

function openCreate() {
  Object.assign(form, emptyForm(), {categoryId: selectedCategoryId.value})
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
    agentProvider: row.agentProvider,
    agentPrompt: row.agentPrompt,
    categoryId: row.categoryId,
    enabled: row.enabled
  })
  showModal.value = true
}

async function save() {
  // an agent source is driven by its prompt and has no address at all, so the
  // two types cannot share one required field.
  if (isAgentType.value) {
    if (!form.agentPrompt.trim()) {
      message.warning('请填写 Agent 提示词')
      return
    }
  } else if (!form.url.trim()) {
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
      agentProvider: form.agentProvider,
      agentPrompt: form.agentPrompt,
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
    await refreshAll()
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    saving.value = false
  }
}

async function remove(row: SourceDTO) {
  try {
    await DeleteSource(row.id)
    message.success('订阅已删除')
    await refreshAll()
  } catch (e) {
    message.error(`删除失败：${errorText(e)}`)
  }
}

async function toggleEnabled(row: SourceDTO, enabled: boolean) {
  togglingId.value = row.id
  // the switch reflects the intent immediately; a failure reloads the stored
  // state so the card can never keep showing a toggle that was not persisted.
  const previous = row.enabled
  row.enabled = enabled
  try {
    await SetSourceEnabled(row.id, enabled)
  } catch (e) {
    row.enabled = previous
    message.error(`启停失败：${errorText(e)}`)
  } finally {
    togglingId.value = 0
    await loadSources()
  }
}

// refreshAll reloads the list and the counts shown in the category tree.
async function refreshAll() {
  await Promise.all([loadCategories(), loadSources()])
  await categoryPanel.value?.reload()
}

async function openPreview(row: SourceDTO) {
  previewRow.value = row
  previewArticles.value = []
  previewError.value = ''
  previewLoading.value = true
  showPreview.value = true
  try {
    previewArticles.value = compact(await PreviewSource(row.id))
  } catch (e) {
    previewError.value = errorText(e)
  } finally {
    previewLoading.value = false
  }
}

function openArticle(url: string) {
  void Browser.OpenURL(url)
}
</script>

<template>
  <div class="layout">
    <div class="side">
      <CategoryPanel
        ref="categoryPanel"
        :selected-id="selectedCategoryId"
        :parse-type="filters.parseType"
        :fetch-status="filters.fetchStatus"
        :enabled-state="filters.enabledState"
        :parse-type-options="parseTypeFilterOptions"
        @update:selected-id="onCategorySelected"
        @update:parse-type="v => onFilterChanged('parseType', v)"
        @update:fetch-status="v => onFilterChanged('fetchStatus', v)"
        @update:enabled-state="v => onFilterChanged('enabledState', v)"
        @clear-filters="clearFilters"
        @changed="onCategoryTreeChanged"
      />
    </div>

    <div class="main">
      <div class="toolbar">
        <n-space align="center" :size="8">
          <n-text strong>{{ selectedCategoryName }}</n-text>
          <n-text depth="3" style="font-size: 12px">{{ sources.length }} 个订阅</n-text>
          <!-- the count above is the filtered one; saying so keeps it from
               reading as the total of the selected category. -->
          <n-tag v-if="hasActiveFilter" size="small" type="info" :bordered="false">已过滤</n-tag>
        </n-space>
        <n-space>
          <n-button :loading="loading" size="small" tertiary @click="refreshAll">刷新</n-button>
          <n-button size="small" type="primary" @click="openCreate">新增订阅</n-button>
        </n-space>
      </div>

      <div class="content">
        <n-alert v-if="listError" type="error" title="订阅列表加载失败" style="margin-bottom: 12px">
          {{ listError }}
          <div style="margin-top: 8px">
            <n-button size="tiny" @click="loadSources">重试</n-button>
          </div>
        </n-alert>

        <n-spin :show="loading">
          <div v-if="sources.length > 0" class="cards">
            <n-card v-for="row in sources" :key="row.id" size="small" class="card">
              <template #header>
                <n-ellipsis style="max-width: 100%">
                  {{ row.title || row.url }}
                </n-ellipsis>
              </template>
              <template #header-extra>
                <n-switch
                  :value="row.enabled"
                  :loading="togglingId === row.id"
                  size="small"
                  @update:value="(v: boolean) => toggleEnabled(row, v)"
                />
              </template>

              <n-space :size="6" style="margin-bottom: 6px">
                <n-tag size="small" :bordered="false">{{ row.resolvedParseType }}</n-tag>
                <n-tag size="small" :bordered="false" type="info">{{ categoryName(row.categoryId) }}</n-tag>
                <n-tag v-if="row.status === 1" size="small" type="success">正常</n-tag>
                <n-tag v-else-if="row.status === 2" size="small" type="error">抓取失败</n-tag>
                <n-tag v-else size="small" :bordered="false">未抓取</n-tag>
                <n-tag v-if="!row.enabled" size="small" type="warning" :bordered="false">已停用</n-tag>
              </n-space>

              <n-text depth="3" style="font-size: 12px">
                <n-ellipsis style="max-width: 100%">{{ row.url }}</n-ellipsis>
              </n-text>

              <n-alert
                v-if="row.status === 2 && row.errorInfo"
                type="error"
                :bordered="false"
                style="margin-top: 8px"
              >
                <n-ellipsis :line-clamp="3">{{ row.errorInfo }}</n-ellipsis>
              </n-alert>

              <template #footer>
                <n-space :size="8">
                  <n-button size="tiny" @click="openPreview(row)">测试抓取</n-button>
                  <n-button size="tiny" tertiary @click="openEdit(row)">编辑</n-button>
                  <n-popconfirm @positive-click="remove(row)">
                    <template #trigger>
                      <n-button size="tiny" tertiary type="error">删除</n-button>
                    </template>
                    确认删除订阅「{{ row.title || row.url }}」吗？
                  </n-popconfirm>
                </n-space>
              </template>
            </n-card>
          </div>

          <n-empty
            v-else-if="!loading && !listError"
            :description="emptyDescription"
            style="margin-top: 60px"
          />
          <div v-else style="height: 120px" />
        </n-spin>
      </div>
    </div>

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
        <n-form-item v-if="!isAgentType" label="URL" required>
          <n-input v-model:value="form.url" placeholder="https://example.com/atom.xml" />
        </n-form-item>
        <n-form-item label="分类">
          <n-select
            v-model:value="form.categoryId"
            :options="categoryOptions"
            placeholder="留空则归入「未分类」"
            clearable
          />
        </n-form-item>
        <n-form-item v-if="!isAgentType" label="自定义请求">
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

        <template v-if="isAgentType">
          <n-divider title-placement="left" style="margin: 8px 0">Agent 解析参数</n-divider>
          <n-form-item label="Agent">
            <n-select v-model:value="form.agentProvider" :options="agentProviderOptions" />
          </n-form-item>
          <n-form-item label="提示词" required>
            <n-input
              v-model:value="form.agentPrompt"
              type="textarea"
              placeholder="用自然语言描述要找什么，例如：搜索今天 Go 语言相关的技术文章，给出标题和链接"
              :autosize="{minRows: 4, maxRows: 10}"
            />
          </n-form-item>
          <n-text depth="3" style="display: block; margin: -4px 0 12px 0; font-size: 12px">
            只需要描述任务本身，JSON 输出格式要求由 informer 自动追加；接口地址、密钥与模型在「设置」中配置。
          </n-text>
        </template>

        <template v-if="!isAgentType && showRegexFields">
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

        <template v-if="!isAgentType && showJsonFields">
          <n-divider title-placement="left" style="margin: 8px 0">JSON 解析参数</n-divider>
          <n-form-item label="标题路径">
            <n-input v-model:value="form.jsonTitlePath" placeholder="例如：data/items[]/title" />
          </n-form-item>
          <n-form-item label="链接路径">
            <n-input v-model:value="form.jsonURLPath" placeholder="例如：data/items[]/url" />
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
                  <n-tooltip>
                    <template #trigger>
                      <n-button size="tiny" text type="primary" @click="openArticle(a.url)">打开</n-button>
                    </template>
                    在系统浏览器中打开
                  </n-tooltip>
                </template>
              </n-list-item>
            </n-list>
            <n-empty v-else description="没有解析到文章" style="margin-top: 24px" />
          </template>
        </n-spin>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.side {
  width: 260px;
  flex: none;
  overflow: hidden;
}

.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid var(--n-border-color, #efeff5);
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}

.card {
  overflow: hidden;
}
</style>
