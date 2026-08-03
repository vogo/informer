<script setup lang="ts">
import {computed, h, onMounted, ref} from 'vue'
import {
  NAlert,
  NButton,
  NDataTable,
  NEllipsis,
  NEmpty,
  NInput,
  NSelect,
  NSpace,
  NTag,
  NText,
  type DataTableColumns
} from 'naive-ui'
import {BrowserOpenURL} from '../wailsjs/runtime/runtime'
import {ListArticles, ListCategories, ListSources} from '../wailsjs/go/main/App'
import type {main} from '../wailsjs/go/models'
import {errorText} from './errors'

type ArticleItemDTO = main.ArticleItemDTO

const pageSize = 30

const items = ref<ArticleItemDTO[]>([])
const loading = ref(false)
const listError = ref('')

const categoryId = ref(0)
const sourceId = ref(0)
const keyword = ref('')

const categoryOptions = ref<{label: string; value: number}[]>([{label: '全部分类', value: 0}])
const sourceOptions = ref<{label: string; value: number}[]>([{label: '全部订阅', value: 0}])

// cursorStack holds the Before value of every page already opened, so paging back
// replays a boundary that really existed instead of guessing one from the current
// page. The last entry is the cursor of the page on screen.
const cursorStack = ref<number[]>([0])
const nextCursor = ref(0)
const hasMore = ref(false)

const pageNumber = computed(() => cursorStack.value.length)
const canGoBack = computed(() => cursorStack.value.length > 1)

onMounted(async () => {
  await Promise.all([loadFilters(), load()])
})

async function loadFilters() {
  try {
    // the article library names every subscription that ever produced an
    // article, so it asks for the unfiltered set on purpose.
    const [categories, sources] = await Promise.all([
      ListCategories(),
      ListSources({categoryId: 0, parseType: '', fetchStatus: '', enabledState: ''})
    ])

    categoryOptions.value = [
      {label: '全部分类', value: 0},
      ...categories.map(c => ({label: c.name, value: c.id}))
    ]
    sourceOptions.value = [
      {label: '全部订阅', value: 0},
      ...sources.map(s => ({label: s.title || s.url, value: s.id}))
    ]
  } catch (e) {
    listError.value = errorText(e)
  }
}

async function load() {
  loading.value = true
  listError.value = ''
  try {
    const page = await ListArticles({
      sourceId: sourceId.value,
      categoryId: categoryId.value,
      keyword: keyword.value.trim(),
      before: cursorStack.value[cursorStack.value.length - 1],
      limit: pageSize
    })
    items.value = page.items
    nextCursor.value = page.nextCursor
    hasMore.value = page.hasMore
  } catch (e) {
    listError.value = errorText(e)
    items.value = []
    hasMore.value = false
  } finally {
    loading.value = false
  }
}

// resetAndLoad restarts at the newest article. Every filter change goes through it,
// because a cursor taken under one filter means nothing under another.
async function resetAndLoad() {
  cursorStack.value = [0]
  await load()
}

async function nextPage() {
  if (!hasMore.value) {
    return
  }

  cursorStack.value = [...cursorStack.value, nextCursor.value]
  await load()
}

async function previousPage() {
  if (!canGoBack.value) {
    return
  }

  cursorStack.value = cursorStack.value.slice(0, -1)
  await load()
}

function formatTime(unixSeconds: number): string {
  if (!unixSeconds) {
    return ''
  }

  const date = new Date(unixSeconds * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')

  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

const columns: DataTableColumns<ArticleItemDTO> = [
  {title: 'ID', key: 'id', width: 72},
  {
    title: '标题',
    key: 'title',
    render: row =>
      h(
        NButton,
        {text: true, type: 'primary', onClick: () => BrowserOpenURL(row.url)},
        {default: () => h(NEllipsis, null, {default: () => row.title || row.url})}
      )
  },
  {
    title: '订阅',
    key: 'sourceTitle',
    width: 180,
    render: row =>
      row.sourceTitle
        ? h(NEllipsis, null, {default: () => row.sourceTitle})
        : h(NText, {depth: 3}, {default: () => '（订阅已删除）'})
  },
  {
    title: '通知时间',
    key: 'informedAt',
    width: 130,
    render: row => {
      if (row.informedAt) {
        return h(NText, null, {default: () => formatTime(row.informedAt)})
      }

      return h(
        NTag,
        {size: 'small', bordered: false, type: row.informed ? 'warning' : 'default'},
        {default: () => (row.informed ? '时间未知' : '未通知')}
      )
    }
  }
]
</script>

<template>
  <div class="page">
    <div class="toolbar">
      <n-space :size="8" align="center" wrap>
        <n-select
          v-model:value="categoryId"
          :options="categoryOptions"
          size="small"
          style="width: 160px"
          @update:value="resetAndLoad"
        />
        <n-select
          v-model:value="sourceId"
          :options="sourceOptions"
          size="small"
          filterable
          style="width: 220px"
          @update:value="resetAndLoad"
        />
        <n-input
          v-model:value="keyword"
          size="small"
          clearable
          placeholder="按标题或链接搜索"
          style="width: 200px"
          @keyup.enter="resetAndLoad"
          @clear="resetAndLoad"
        />
        <n-button size="small" :loading="loading" @click="resetAndLoad">查询</n-button>
      </n-space>
      <n-space :size="8" align="center">
        <n-text depth="3" style="font-size: 12px">第 {{ pageNumber }} 页</n-text>
        <n-button size="small" :disabled="!canGoBack || loading" @click="previousPage">上一页</n-button>
        <n-button size="small" :disabled="!hasMore || loading" @click="nextPage">下一页</n-button>
      </n-space>
    </div>

    <div class="content">
      <n-alert v-if="listError" type="error" title="文章加载失败" style="margin-bottom: 12px">
        {{ listError }}
        <div style="margin-top: 8px">
          <n-button size="tiny" @click="load">重试</n-button>
        </div>
      </n-alert>

      <n-data-table
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="(row: ArticleItemDTO) => row.id"
        size="small"
        striped
      />

      <n-empty
        v-if="!loading && !listError && items.length === 0"
        :description="pageNumber > 1 ? '这一页没有内容，请返回上一页' : '没有符合条件的文章'"
        style="margin-top: 60px"
      />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--n-border-color, #efeff5);
  flex-wrap: wrap;
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}
</style>
