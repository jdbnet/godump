<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useBackupsStore } from '@/stores/backups'
import { usePolling } from '@/composables/usePolling'
import { formatBytes } from '@/lib/format'

const backups = useBackupsStore()

const status = computed(() => backups.status)

onMounted(() => backups.refreshStatus())

usePolling(() => backups.refreshStatus(true), 10000, { immediate: false })
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-xl font-semibold text-heading">Overview</h1>
      <p class="mt-1 text-sm text-muted">Backup status across all configured MariaDB instances</p>
    </div>

    <div v-if="backups.statusLoading && !status" class="text-sm text-muted">Loading status...</div>

    <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <div class="card">
        <div class="text-sm text-muted">Managed Instances</div>
        <div class="text-2xl font-semibold">{{ status?.total_instances ?? 0 }}</div>
      </div>
      <div class="card">
        <div class="text-sm text-muted">Discovered Databases</div>
        <div class="text-2xl font-semibold">{{ status?.total_databases ?? 0 }}</div>
      </div>
      <div class="card">
        <div class="text-sm text-muted">Unreachable Instances</div>
        <div
          class="text-2xl font-semibold"
          :class="(status?.unreachable_count ?? 0) > 0 ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'"
        >
          {{ status?.unreachable_count ?? 0 }}
        </div>
      </div>
      <div class="card">
        <div class="text-sm text-muted">Total Backup Size</div>
        <div class="text-2xl font-semibold">{{ formatBytes(status?.total_backup_size ?? 0) }}</div>
      </div>
    </div>

    <div v-if="status?.any_running" class="rounded-lg border border-accent/30 bg-accent/10 px-4 py-3 text-sm text-accent">
      A backup job is currently running.
    </div>
  </div>
</template>
