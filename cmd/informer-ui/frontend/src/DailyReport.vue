<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {NAlert, NButton, NCollapse, NCollapseItem, NEmpty, NSpin, NTag, NText} from 'naive-ui'
import {Browser} from '@wailsio/runtime'
import {DailyContent, DailyIndex} from './bindings'
import {errorText} from './errors'
import {renderMarkdown} from './markdown'
import {compact} from './nulls'

type DailyDay = {date: string; size: number}
type DailyMonth = {month: string; days: DailyDay[]}
type DailyYear = {year: string; months: DailyMonth[]}

const years = ref<DailyYear[]>([])
const indexLoading = ref(false)
const indexError = ref('')

const selectedDate = ref('')
const contentLoading = ref(false)
const contentError = ref('')
const contentHtml = ref('')
const contentEmpty = ref(false)

// the newest year and month open by default, which is the day a user wants first.
const expandedYears = computed(() => (years.value.length > 0 ? [years.value[0].year] : []))

onMounted(loadIndex)

async function loadIndex() {
  indexLoading.value = true
  indexError.value = ''
  try {
    years.value = compact(await DailyIndex()).map((year): DailyYear => ({
      year: year.year,
      months: compact(year.months).map((month): DailyMonth => ({
        month: month.month,
        days: compact(month.days).map(day => ({date: day.date, size: day.size})),
      })),
    }))

    const first = years.value[0]?.months[0]?.days[0]?.date
    if (first) {
      await openDate(first)
    }
  } catch (e) {
    indexError.value = errorText(e)
  } finally {
    indexLoading.value = false
  }
}

async function openDate(date: string) {
  selectedDate.value = date
  contentLoading.value = true
  contentError.value = ''
  contentHtml.value = ''
  contentEmpty.value = false
  try {
    const markdown = await DailyContent(date)
    contentEmpty.value = markdown.trim() === ''
    contentHtml.value = renderMarkdown(markdown)
  } catch (e) {
    contentError.value = errorText(e)
  } finally {
    contentLoading.value = false
  }
}

// openLink keeps navigation out of the webview: a report link opens in the system
// browser instead of replacing the app window.
function onContentClick(event: MouseEvent) {
  const anchor = (event.target as HTMLElement | null)?.closest('a')
  if (!anchor) {
    return
  }

  event.preventDefault()

  const href = anchor.getAttribute('href')
  if (href) {
    void Browser.OpenURL(href)
  }
}
</script>

<template>
  <div class="layout">
    <div class="side">
      <div class="side-header">
        <n-text strong>日报</n-text>
        <n-button size="tiny" tertiary :loading="indexLoading" @click="loadIndex">刷新</n-button>
      </div>

      <div class="side-body">
        <n-alert v-if="indexError" type="error" title="日期列表加载失败" style="margin: 8px">
          {{ indexError }}
          <div style="margin-top: 8px">
            <n-button size="tiny" @click="loadIndex">重试</n-button>
          </div>
        </n-alert>

        <n-spin v-else :show="indexLoading">
          <n-collapse v-if="years.length > 0" :default-expanded-names="expandedYears" arrow-placement="right">
            <n-collapse-item v-for="year in years" :key="year.year" :title="`${year.year} 年`" :name="year.year">
              <n-collapse :default-expanded-names="year.months[0] ? [year.months[0].month] : []" arrow-placement="right">
                <n-collapse-item
                  v-for="month in year.months"
                  :key="month.month"
                  :name="month.month"
                  :title="`${month.month.slice(5)} 月（${month.days.length}）`"
                >
                  <div
                    v-for="day in month.days"
                    :key="day.date"
                    class="day"
                    :class="{active: selectedDate === day.date}"
                    @click="openDate(day.date)"
                  >
                    <span>{{ day.date }}</span>
                    <n-tag v-if="day.size === 0" size="tiny" :bordered="false">空</n-tag>
                  </div>
                </n-collapse-item>
              </n-collapse>
            </n-collapse-item>
          </n-collapse>

          <n-empty
            v-else-if="!indexLoading"
            description="还没有生成过日报"
            size="small"
            style="margin: 32px 0"
          />
        </n-spin>
      </div>
    </div>

    <div class="main">
      <div class="main-header">
        <n-text strong>{{ selectedDate || '未选择日期' }}</n-text>
      </div>

      <div class="main-body">
        <n-alert v-if="contentError" type="error" title="日报读取失败">
          {{ contentError }}
          <div style="margin-top: 8px">
            <n-button size="tiny" @click="openDate(selectedDate)">重试</n-button>
          </div>
        </n-alert>

        <n-spin v-else :show="contentLoading">
          <n-empty v-if="!selectedDate" description="在左侧选择一个日期查看当天日报" style="margin-top: 80px" />
          <n-empty v-else-if="contentEmpty" description="当天的日报文件是空的" style="margin-top: 80px" />
          <!-- the html is produced by markdown-it with raw html disabled and then
               sanitised, so nothing a report file contains can execute here. -->
          <div v-else class="markdown" @click="onContentClick" v-html="contentHtml" />
        </n-spin>
      </div>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.side {
  width: 240px;
  flex: none;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--n-border-color, #efeff5);
  overflow: hidden;
}

.side-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid var(--n-border-color, #efeff5);
}

.side-body {
  flex: 1;
  overflow-y: auto;
  padding: 4px 8px;
}

.day {
  padding: 4px 8px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.day:hover {
  background: rgba(0, 0, 0, 0.04);
}

.day.active {
  background: rgba(24, 160, 88, 0.12);
}

.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.main-header {
  padding: 10px 16px;
  border-bottom: 1px solid var(--n-border-color, #efeff5);
}

.main-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 24px;
}

.markdown {
  width: 100%;
  font-size: 14px;
  line-height: 1.8;
  word-break: break-word;
}

.markdown :deep(h1),
.markdown :deep(h2),
.markdown :deep(h3) {
  margin: 16px 0 8px;
}

.markdown :deep(ul),
.markdown :deep(ol) {
  padding-left: 22px;
}

.markdown :deep(a) {
  color: #18a058;
  text-decoration: none;
}

.markdown :deep(a:hover) {
  text-decoration: underline;
}

.markdown :deep(pre) {
  background: rgba(0, 0, 0, 0.04);
  padding: 12px;
  border-radius: 4px;
  overflow-x: auto;
}
</style>
