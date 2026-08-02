<script setup lang="ts">
import {computed, onMounted, reactive, ref} from 'vue'
import {
  NAlert,
  NButton,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NSpin,
  NTag,
  NText,
  useMessage
} from 'naive-ui'
import {CreateCategory, DeleteCategory, ListCategories, UpdateCategory} from '../wailsjs/go/main/App'
import type {main} from '../wailsjs/go/models'
import {errorText} from './errors'

type CategoryDTO = main.CategoryDTO

const props = defineProps<{selectedId: number}>()
const emit = defineEmits<{
  (e: 'update:selectedId', id: number): void
  // changed fires whenever the tree or a subscription assignment moved, so the
  // subscription list next to it reloads instead of showing a stale category.
  (e: 'changed'): void
}>()

const message = useMessage()

const categories = ref<CategoryDTO[]>([])
const loading = ref(false)
const loadError = ref('')

const showEditor = ref(false)
const saving = ref(false)
const form = reactive({id: 0, name: '', sort: 0})

const deleting = ref<CategoryDTO | null>(null)
const deleteBlockedCount = ref(0)
const reassignTo = ref<number | null>(null)
const deleteBusy = ref(false)

const totalSources = computed(() => categories.value.reduce((sum, c) => sum + c.sourceCount, 0))
const editorTitle = computed(() => (form.id === 0 ? '新增分类' : `编辑分类 #${form.id}`))

// the deleted category is never a valid destination for its own subscriptions.
const reassignOptions = computed(() =>
  categories.value
    .filter(c => c.id !== deleting.value?.id)
    .map(c => ({label: `${c.name}（${c.sourceCount} 个订阅）`, value: c.id}))
)

onMounted(load)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    categories.value = await ListCategories()

    // a category selected before it was deleted elsewhere falls back to "全部".
    if (props.selectedId !== 0 && !categories.value.some(c => c.id === props.selectedId)) {
      emit('update:selectedId', 0)
    }
  } catch (e) {
    loadError.value = errorText(e)
  } finally {
    loading.value = false
  }
}

// reload is what the parent calls after it changed a subscription assignment.
defineExpose({reload: load})

function select(id: number) {
  emit('update:selectedId', id)
}

function openCreate() {
  form.id = 0
  form.name = ''
  form.sort = 0
  showEditor.value = true
}

function openEdit(category: CategoryDTO) {
  form.id = category.id
  form.name = category.name
  form.sort = category.sort
  showEditor.value = true
}

async function save() {
  const name = form.name.trim()
  if (!name) {
    message.warning('请填写分类名称')
    return
  }

  saving.value = true
  try {
    const req = {id: form.id, name, sort: Math.trunc(form.sort ?? 0)}
    if (form.id === 0) {
      await CreateCategory(req)
      message.success('分类已创建')
    } else {
      await UpdateCategory(req)
      message.success('分类已更新')
    }
    showEditor.value = false
    await load()
    emit('changed')
  } catch (e) {
    message.error(`保存失败：${errorText(e)}`)
  } finally {
    saving.value = false
  }
}

async function remove(category: CategoryDTO) {
  try {
    const result = await DeleteCategory(category.id, 0)
    if (result.deleted) {
      message.success('分类已删除')
      if (props.selectedId === category.id) {
        emit('update:selectedId', 0)
      }
      await load()
      emit('changed')
      return
    }

    // the service refused because subscriptions still point at the category; ask
    // where they should go instead of deleting anything behind the user's back.
    deleting.value = category
    deleteBlockedCount.value = result.inUseCount
    reassignTo.value = reassignOptions.value[0]?.value ?? null
  } catch (e) {
    message.error(`删除失败：${errorText(e)}`)
  }
}

async function confirmReassignDelete() {
  const category = deleting.value
  const target = reassignTo.value
  if (!category || target === null) {
    return
  }

  deleteBusy.value = true
  try {
    const result = await DeleteCategory(category.id, target)
    message.success(`分类已删除，${result.moved} 个订阅已移动`)
    if (props.selectedId === category.id) {
      emit('update:selectedId', 0)
    }
    deleting.value = null
    await load()
    emit('changed')
  } catch (e) {
    message.error(`删除失败：${errorText(e)}`)
  } finally {
    deleteBusy.value = false
  }
}
</script>

<template>
  <div class="panel">
    <div class="panel-header">
      <n-text strong>分类</n-text>
      <n-button size="tiny" tertiary @click="openCreate">新增</n-button>
    </div>

    <n-alert v-if="loadError" type="error" title="分类加载失败" style="margin: 8px">
      {{ loadError }}
      <div style="margin-top: 8px">
        <n-button size="tiny" @click="load">重试</n-button>
      </div>
    </n-alert>

    <n-spin v-else :show="loading">
      <div class="tree">
        <div class="node" :class="{active: selectedId === 0}" @click="select(0)">
          <span class="node-name">全部订阅</span>
          <n-tag :bordered="false" size="small">{{ totalSources }}</n-tag>
        </div>

        <div
          v-for="category in categories"
          :key="category.id"
          class="node"
          :class="{active: selectedId === category.id}"
          @click="select(category.id)"
        >
          <span class="node-name">{{ category.name }}</span>
          <n-space :size="4" align="center" class="node-actions">
            <n-text depth="3" style="font-size: 11px">#{{ category.sort }}</n-text>
            <n-tag :bordered="false" size="small">{{ category.sourceCount }}</n-tag>
            <n-button size="tiny" text @click.stop="openEdit(category)">编辑</n-button>
            <n-popconfirm v-if="!category.isDefault" @positive-click="remove(category)">
              <template #trigger>
                <n-button size="tiny" text type="error" @click.stop>删除</n-button>
              </template>
              确认删除分类「{{ category.name }}」吗？
            </n-popconfirm>
          </n-space>
        </div>

        <n-empty
          v-if="!loading && categories.length === 0"
          description="还没有分类"
          size="small"
          style="margin: 24px 0"
        />
      </div>
    </n-spin>

    <n-modal v-model:show="showEditor" preset="card" :title="editorTitle" style="width: 420px" :mask-closable="false">
      <n-form :model="form" label-placement="left" label-width="auto">
        <n-form-item label="名称" required>
          <n-input v-model:value="form.name" placeholder="例如：技术博客" />
        </n-form-item>
        <n-form-item label="排序值">
          <n-input-number v-model:value="form.sort" :precision="0" :step="1" style="width: 100%" />
        </n-form-item>
      </n-form>
      <n-text depth="3" style="font-size: 12px">排序值越小越靠前，相同排序值按分类 ID 先后排列。</n-text>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showEditor = false">取消</n-button>
          <n-button :loading="saving" type="primary" @click="save">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal
      :show="deleting !== null"
      preset="card"
      title="分类下还有订阅"
      style="width: 460px"
      :mask-closable="false"
      @update:show="v => { if (!v) deleting = null }"
    >
      <n-text>
        分类「{{ deleting?.name }}」下还有 {{ deleteBlockedCount }} 个订阅。
        删除分类不会删除订阅，请先选择这些订阅移动到的分类。
      </n-text>
      <n-select
        v-model:value="reassignTo"
        :options="reassignOptions"
        placeholder="选择目标分类"
        style="margin-top: 12px"
      />
      <template #footer>
        <n-space justify="end">
          <n-button @click="deleting = null">取消</n-button>
          <n-button
            :disabled="reassignTo === null"
            :loading="deleteBusy"
            type="error"
            @click="confirmReassignDelete"
          >
            移动订阅并删除分类
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  border-right: 1px solid var(--n-border-color, #efeff5);
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid var(--n-border-color, #efeff5);
}

.tree {
  padding: 6px;
  overflow-y: auto;
}

.node {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
}

.node:hover {
  background: rgba(0, 0, 0, 0.04);
}

.node.active {
  background: rgba(24, 160, 88, 0.12);
}

.node-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-actions {
  flex: none;
}
</style>
