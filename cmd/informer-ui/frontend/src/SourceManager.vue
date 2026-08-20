<script setup lang="ts">
import {computed, nextTick, onMounted, onUnmounted, reactive, ref, watch} from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NCollapse,
  NCollapseItem,
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
import {Browser, Clipboard, Events} from '@wailsio/runtime'
import {
  CreateCategory,
  CreateSource,
  DeleteSource,
  ExportSourcesToFile,
  ImportSourcesFromFile,
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
  type SourceImportResultDTO,
} from './bindings'
import CategoryPanel from './CategoryPanel.vue'
import SourceCompose from './SourceCompose.vue'
import SourceDiagnose from './SourceDiagnose.vue'
import {errorText} from './errors'
import {compact, requireValue} from './nulls'

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
  // number = existing category id; string = typed new name to create on save;
  // null / 0 = leave empty and let the backend fall back to「未分类」.
  categoryId: number | string | null
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

// the transfer state of this session: one of the two buttons is busy at a time,
// because both open a modal native dialog the user has to answer first.
const exporting = ref(false)
const importing = ref(false)
const showImportResult = ref(false)
const importResult = ref<SourceImportResultDTO | null>(null)

const showDiagnose = ref(false)
const diagnoseRow = ref<SourceDTO | null>(null)

// diagnoseKey remounts the panel on every request, which is what makes it start
// a fresh run: the drawer keeps its content alive between opens, so without this
// a second click would show the previous diagnosis.
const diagnoseKey = ref(0)

const showPreview = ref(false)
const previewLoading = ref(false)
const previewError = ref('')
const previewRow = ref<SourceDTO | null>(null)
const previewArticles = ref<ArticleDTO[]>([])

// PreviewLogEntry mirrors PreviewLogDTO in cmd/informer-ui/binding_preview.go.
// A wails event payload is not part of the generated bindings, so the two shapes
// are kept in step by hand.
interface PreviewLogEntry {
  runId: string
  seq: number
  time: number
  level: 'info' | 'warn' | 'error'
  text: string
}

// PREVIEW_LOG_EVENT is the same constant as PreviewLogEvent on the Go side.
const PREVIEW_LOG_EVENT = 'informer:preview:log'

// MAX_PREVIEW_LOGS bounds what the panel keeps. An agent run narrates every
// search and page it reads, and an unbounded array in a webview is a leak.
const MAX_PREVIEW_LOGS = 500

const previewRunId = ref('')
const previewLogs = ref<PreviewLogEntry[]>([])
const previewLogsDropped = ref(0)
const previewLogExpanded = ref<string[]>([])
const previewLogBox = ref<HTMLElement | null>(null)

// previewLogPinned keeps the panel scrolled to the newest line until the user
// scrolls up to read an earlier one.
const previewLogPinned = ref(true)

const previewLogProblems = computed(() => previewLogs.value.filter(entry => entry.level !== 'info').length)
const previewLastLog = computed(() => previewLogs.value.at(-1)?.text ?? '')

// the log doubles as the progress indicator of a fetch that has nothing else to
// show, so it opens itself while one is running and closes again once it worked.
const previewLogPanel = 'log'

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

// aiMode swaps the create modal's body for the composing chat. The form stays
// mounted-but-hidden and is never reset by the switch: a user who tried the AI
// and went back should find what they had already typed.
const aiMode = ref(false)
const aiBusy = ref(false)
const composePanel = ref<InstanceType<typeof SourceCompose> | null>(null)

const modalTitle = computed(() => {
  if (form.id !== 0) {
    return `编辑订阅 #${form.id}`
  }

  return aiMode.value ? 'AI 定义配置' : '新增订阅'
})

// the chat needs room for a transcript, a configuration card and a log panel;
// 640px is a form's width, not a conversation's.
const modalWidth = computed(() => (aiMode.value ? '820px' : '640px'))
const isAgentType = computed(() => form.parseType === 'agent')
const showRegexFields = computed(() => form.parseType === 'regex' || form.parseType === '')
const showJsonFields = computed(() => form.parseType === 'json' || form.parseType === '')
const categoryOptions = computed(() => categories.value.map(c => ({label: c.name, value: c.id})))

// onCreateCategoryOption turns a typed label into a select option. Matching an
// existing name reuses that id so the unique-name index is never hit by accident.
function onCreateCategoryOption(label: string) {
  const name = label.trim()
  if (!name) {
    return {label: label, value: label}
  }

  const existing = categories.value.find(c => c.name === name)
  if (existing) {
    return {label: existing.name, value: existing.id}
  }

  return {label: name, value: name}
}

// resolveCategoryId turns a select value into a real category id before save:
// pick existing, create a new one from a typed name, or leave 0 for「未分类」.
//
// It takes the value rather than reading the form, because the composing chat
// carries its own category select and must resolve it the same way - creating a
// category by typing its name is one behaviour, not two.
async function resolveCategoryId(raw: number | string | null = form.categoryId): Promise<number> {
  if (raw == null || raw === '' || raw === 0) {
    return 0
  }

  if (typeof raw === 'number') {
    return raw
  }

  const name = String(raw).trim()
  if (!name) {
    return 0
  }

  const existing = categories.value.find(c => c.name === name)
  if (existing) {
    return existing.id
  }

  const created = requireValue(await CreateCategory({id: 0, name, sort: 0}), 'CategoryDTO')
  categories.value = [
    ...categories.value,
    {
      id: created.id,
      name: created.name,
      sort: created.sort,
      sourceCount: 0,
      isDefault: false
    }
  ]
  return created.id
}

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

// unsubs holds the wails event listeners this page owns, released on unmount.
const unsubs: Array<() => void> = []

onMounted(async () => {
  unsubs.push(Events.On(PREVIEW_LOG_EVENT, onPreviewLog))

  await Promise.all([loadCategories(), loadParseTypeOptions(), loadAgentProviders(), loadSources()])
})

onUnmounted(() => {
  for (const off of unsubs) {
    off()
  }
})

function onPreviewLog(e: {data?: PreviewLogEntry}) {
  const entry = e?.data
  // a fetch the user already moved on from keeps reporting until it finishes;
  // its lines belong to a run id that is no longer the current one.
  if (!entry || entry.runId !== previewRunId.value) {
    return
  }

  previewLogs.value.push(entry)

  const overflow = previewLogs.value.length - MAX_PREVIEW_LOGS
  if (overflow > 0) {
    previewLogs.value.splice(0, overflow)
    previewLogsDropped.value += overflow
  }

  if (previewLogPinned.value) {
    void nextTick(scrollPreviewLogToEnd)
  }
}

function scrollPreviewLogToEnd() {
  const box = previewLogBox.value
  if (box) {
    box.scrollTop = box.scrollHeight
  }
}

// onPreviewLogScroll unpins as soon as the user scrolls up, so reading an early
// line is not fought by every line that arrives after it.
function onPreviewLogScroll() {
  const box = previewLogBox.value
  if (!box) {
    return
  }

  previewLogPinned.value = box.scrollHeight - box.scrollTop - box.clientHeight < 24
}

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
  aiMode.value = false
  showModal.value = true
}

// onComposeCreated closes the modal on the subscription the chat just created,
// and reloads: the list behind it still shows the world before it existed.
async function onComposeCreated() {
  aiMode.value = false
  showModal.value = false
  await refreshAll()
}

// the conversation is ended whenever the modal closes, whichever way it closed.
// SourceCompose also does this on unmount; both paths exist because a modal that
// is merely hidden is not always unmounted.
watch(showModal, open => {
  if (!open) {
    composePanel.value?.close()
    aiMode.value = false
    aiBusy.value = false
  }
})

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
  // editing a failing source is what「AI 诊断修复」is for; this entry point is
  // only about a subscription that does not exist yet.
  aiMode.value = false
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
    const categoryId = await resolveCategoryId()
    form.categoryId = categoryId

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
      categoryId,
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

// exportSources writes every stored subscription - not only the filtered ones -
// to a file the user picks. A canceled dialog leaves everything untouched.
async function exportSources() {
  exporting.value = true
  try {
    const result = requireValue(await ExportSourcesToFile(), 'SourceExportResultDTO')
    if (result.canceled) {
      message.info('已取消导出')
      return
    }

    message.success(`已导出 ${result.total} 个订阅到 ${result.path}`)
  } catch (e) {
    message.error(`导出失败：${errorText(e)}`)
  } finally {
    exporting.value = false
  }
}

// importSources merges a picked export file into the stored subscriptions: a
// known subscription is overwritten, an unknown one appended, nothing is deleted.
// The result modal reports the counts and every entry that could not be applied.
async function importSources() {
  importing.value = true
  try {
    const result = requireValue(await ImportSourcesFromFile(), 'SourceImportResultDTO')
    if (result.canceled) {
      message.info('已取消导入')
      return
    }

    importResult.value = result
    showImportResult.value = true
    await refreshAll()
  } catch (e) {
    message.error(`导入失败：${errorText(e)}`)
  } finally {
    importing.value = false
  }
}

// refreshAll reloads the list and the counts shown in the category tree.
async function refreshAll() {
  await Promise.all([loadCategories(), loadSources()])
  await categoryPanel.value?.reload()
}

async function openPreview(row: SourceDTO) {
  // crypto.randomUUID is not guaranteed in a webview served over a custom
  // scheme, and this id only has to be unique among this window's own fetches.
  const runId = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`

  previewRow.value = row
  previewArticles.value = []
  previewError.value = ''
  previewLogs.value = []
  previewLogsDropped.value = 0
  previewLogPinned.value = true
  // open while it runs: until the articles arrive, the log is the only thing
  // that shows the fetch is doing something.
  previewLogExpanded.value = [previewLogPanel]
  // set before the call: the first lines can arrive before it returns.
  previewRunId.value = runId
  previewLoading.value = true
  showPreview.value = true

  try {
    previewArticles.value = compact(await PreviewSource(row.id, runId))
    // it worked, so the articles are what the user came for.
    previewLogExpanded.value = []
  } catch (e) {
    previewError.value = errorText(e)
    // it failed, and the log is exactly where the reason is.
    previewLogExpanded.value = [previewLogPanel]
  } finally {
    // a slow fetch the user already replaced must not clear the new one's state.
    if (previewRunId.value === runId) {
      previewLoading.value = false
    }
  }
}

// a closed drawer drops the lines a still running fetch keeps reporting.
watch(showPreview, open => {
  if (!open) {
    previewRunId.value = ''
  }
})

function formatLogTime(time: number): string {
  return new Date(time).toLocaleTimeString('zh-CN', {hour12: false})
}

async function copyPreviewLogs() {
  const text = previewLogs.value.map(entry => `${formatLogTime(entry.time)} ${entry.text}`).join('\n')
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

// openDiagnose starts an AI diagnosis of one failing subscription. The run is
// kicked off on open rather than behind a second click: the card's button is
// already the deliberate action, and a diagnosis takes minutes.
function openDiagnose(row: SourceDTO) {
  diagnoseRow.value = row
  diagnoseKey.value += 1
  showDiagnose.value = true
}

// onFixApplied refreshes the list so the repaired subscription stops showing the
// configuration and the failure it was just repaired out of.
async function onFixApplied() {
  await refreshAll()

  const stored = sources.value.find(s => s.id === diagnoseRow.value?.id)
  if (stored) {
    diagnoseRow.value = stored
  }
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
          <n-button :loading="exporting" :disabled="importing" size="small" tertiary @click="exportSources">
            导出
          </n-button>
          <n-popconfirm @positive-click="importSources">
            <template #trigger>
              <n-button :loading="importing" :disabled="exporting" size="small" tertiary>导入</n-button>
            </template>
            导入会按「URL 相同视为同一个订阅，没有 URL 时按标题」合并：已存在的订阅覆盖配置，
            没有的追加，不会删除任何订阅。
          </n-popconfirm>
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
                  <!-- offered only where it has something to work with: a
                       diagnosis of a source that never failed has no failure to
                       explain. -->
                  <n-tooltip v-if="row.status === 2">
                    <template #trigger>
                      <n-button size="tiny" type="warning" @click="openDiagnose(row)">AI 诊断修复</n-button>
                    </template>
                    让 AI 读取页面原文，找出解析失败的原因并尝试修好配置
                  </n-tooltip>
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
      :style="{width: modalWidth}"
      :mask-closable="false"
      :closable="!aiBusy"
    >
      <template v-if="form.id === 0" #header-extra>
        <n-button v-if="!aiMode" size="tiny" tertiary @click="aiMode = true">AI 定义配置</n-button>
        <n-button v-else size="tiny" tertiary :disabled="aiBusy" @click="aiMode = false">返回表单</n-button>
      </template>

      <SourceCompose
        v-if="aiMode"
        ref="composePanel"
        :category-options="categoryOptions"
        :default-category-id="form.categoryId"
        :create-category-option="onCreateCategoryOption"
        :resolve-category="resolveCategoryId"
        @created="onComposeCreated"
        @busy="aiBusy = $event"
      />

      <n-form v-else :model="form" label-placement="left" label-width="auto">
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
            :on-create="onCreateCategoryOption"
            filterable
            tag
            clearable
            placeholder="选择已有分类，或输入新分类名后回车；留空则归入「未分类」"
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
          <n-button :disabled="aiBusy" @click="showModal = false">
            {{ aiMode ? '关闭' : '取消' }}
          </n-button>
          <!-- in AI mode the save button belongs to a proposal card, not to the
               modal: there is nothing here to save until one arrives. -->
          <n-button v-if="!aiMode" :loading="saving" type="primary" @click="save">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="showImportResult" preset="card" title="导入结果" style="width: 560px">
      <template v-if="importResult">
        <n-text depth="3" style="font-size: 12px">{{ importResult.path }}</n-text>
        <n-space :size="6" style="margin: 10px 0">
          <n-tag size="small" :bordered="false">共 {{ importResult.total }} 条</n-tag>
          <n-tag size="small" type="success" :bordered="false">新增 {{ importResult.created }}</n-tag>
          <n-tag size="small" type="info" :bordered="false">覆盖 {{ importResult.updated }}</n-tag>
          <n-tag v-if="importResult.failed > 0" size="small" type="error" :bordered="false">
            失败 {{ importResult.failed }}
          </n-tag>
        </n-space>

        <n-alert v-if="importResult.errors && importResult.errors.length > 0" type="warning" title="以下条目未导入">
          <!-- a long file can fail many entries; the list scrolls inside the
               modal instead of pushing the close button off screen. -->
          <n-list :bordered="false" style="max-height: 260px; overflow-y: auto">
            <n-list-item v-for="(err, i) in importResult.errors" :key="i">
              <n-text depth="3" style="font-size: 12px">{{ err }}</n-text>
            </n-list-item>
          </n-list>
        </n-alert>
      </template>

      <template #footer>
        <n-space justify="end">
          <n-button size="small" @click="showImportResult = false">关闭</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-drawer v-model:show="showDiagnose" :width="720">
      <n-drawer-content
        :title="diagnoseRow ? `AI 诊断修复：${diagnoseRow.title || diagnoseRow.url}` : 'AI 诊断修复'"
        closable
      >
        <SourceDiagnose
          v-if="diagnoseRow"
          :key="diagnoseKey"
          :source="diagnoseRow"
          @applied="onFixApplied"
        />
      </n-drawer-content>
    </n-drawer>

    <n-drawer v-model:show="showPreview" :width="640">
      <n-drawer-content
        :title="previewRow ? `测试抓取：${previewRow.title || previewRow.url}` : '测试抓取'"
        closable
      >
        <!-- the spin covers the result area only: the log below it is what makes
             a fetch that takes minutes bearable, and a mask over it defeats the
             whole point. -->
        <n-spin :show="previewLoading">
          <n-alert v-if="previewError" type="error" title="抓取失败">
            {{ previewError }}
            <div style="margin-top: 8px">
              <n-button v-if="previewRow" size="tiny" @click="openPreview(previewRow)">重试</n-button>
            </div>
          </n-alert>

          <div v-else-if="previewLoading" class="preview-progress">
            <n-text depth="3" style="font-size: 12px">
              <n-ellipsis :line-clamp="2">{{ previewLastLog || '正在抓取…' }}</n-ellipsis>
            </n-text>
          </div>

          <template v-else>
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

        <n-collapse v-model:expanded-names="previewLogExpanded" style="margin-top: 14px">
          <n-collapse-item :name="previewLogPanel">
            <template #header>
              <n-space :size="6" align="center">
                <n-text depth="2" style="font-size: 12px">执行日志（{{ previewLogs.length }} 条）</n-text>
                <n-tag v-if="previewLogProblems > 0" size="tiny" type="error" :bordered="false">
                  {{ previewLogProblems }} 异常
                </n-tag>
              </n-space>
            </template>
            <template #header-extra>
              <!-- click.stop so copying never folds the panel shut -->
              <n-button
                size="tiny"
                text
                :disabled="previewLogs.length === 0"
                @click.stop="copyPreviewLogs"
              >复制</n-button>
            </template>
            <div ref="previewLogBox" class="log-box" @scroll="onPreviewLogScroll">
              <div v-if="previewLogsDropped > 0" class="log-line log-warn">
                …已省略较早的 {{ previewLogsDropped }} 条
              </div>
              <div
                v-for="entry in previewLogs"
                :key="entry.seq"
                class="log-line"
                :class="`log-${entry.level}`"
              >
                <span class="log-time">{{ formatLogTime(entry.time) }}</span>{{ entry.text }}
              </div>
              <n-text v-if="previewLogs.length === 0" depth="3" style="font-size: 12px">暂无日志</n-text>
            </div>
          </n-collapse-item>
        </n-collapse>
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

.preview-progress {
  padding: 24px 0;
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
