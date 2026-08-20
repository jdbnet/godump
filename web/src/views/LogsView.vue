<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { useBackupsStore } from '@/stores/backups'
import { usePolling } from '@/composables/usePolling'

const backups = useBackupsStore()
const logContainer = ref<HTMLElement | null>(null)

function levelClass(level: string) {
  if (level === 'INFO') return 'log-info'
  if (level === 'WARN') return 'log-warn'
  if (level === 'ERROR') return 'log-error'
  return ''
}

async function scrollIfAtBottom() {
  const el = logContainer.value
  if (!el) return
  const atBottom = el.scrollHeight - el.clientHeight <= el.scrollTop + 1
  if (atBottom) {
    await nextTick()
    el.scrollTop = el.scrollHeight
  }
}

onMounted(async () => {
  await backups.refreshLogs()
  await scrollIfAtBottom()
})

usePolling(async () => {
  const el = logContainer.value
  const atBottom = el ? el.scrollHeight - el.clientHeight <= el.scrollTop + 1 : true
  await backups.refreshLogs(true)
  if (atBottom) {
    await nextTick()
    if (el) el.scrollTop = el.scrollHeight
  }
}, 10000, { immediate: false })

watch(() => backups.logs.length, () => scrollIfAtBottom())
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-xl font-semibold text-heading">Logs</h1>
      <p class="mt-1 text-sm text-muted">Recent application and backup activity</p>
    </div>

    <div v-if="backups.logsLoading && !backups.logs.length" class="text-sm text-muted">Loading logs...</div>

    <div ref="logContainer" class="log-console min-h-[24rem]">
      <div v-if="!backups.logs.length" class="text-muted">No log entries yet.</div>
      <div v-for="(entry, index) in backups.logs" :key="index" class="mb-1 whitespace-pre-wrap">
        <span class="text-neutral-600 dark:text-neutral-500">[{{ entry.timestamp }}]</span>
        <span :class="levelClass(entry.level)"> [{{ entry.level }}]</span>
        <span v-if="entry.instance" class="log-instance"> [{{ entry.instance }}]</span>
        {{ ' ' + entry.message }}
      </div>
    </div>
  </div>
</template>
