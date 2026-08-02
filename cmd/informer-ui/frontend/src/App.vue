<script setup lang="ts">
import {NConfigProvider, NDialogProvider, NMessageProvider, zhCN, dateZhCN} from 'naive-ui'
import SourceManager from './SourceManager.vue'
</script>

<template>
  <n-config-provider :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <SourceManager />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
