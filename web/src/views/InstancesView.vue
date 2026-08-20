<script setup lang="ts">
import { onMounted } from 'vue'
import { useBackupsStore } from '@/stores/backups'
import { usePolling } from '@/composables/usePolling'
import { dbResultBadgeClass, formatBytes, formatTime, instanceBadgeClass } from '@/lib/format'

const backups = useBackupsStore()

onMounted(() => backups.refreshStatus())

usePolling(() => backups.refreshStatus(true), 10000, { immediate: false })

function instanceLabel(inst: { is_running: boolean; overall_result: string }) {
  if (inst.is_running) return 'Running'
  if (inst.overall_result) return inst.overall_result
  return 'Pending'
}
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-xl font-semibold text-heading">Instances</h1>
      <p class="mt-1 text-sm text-muted">MariaDB servers, schedules, and per-database backup status</p>
    </div>

    <div v-if="backups.statusLoading && !backups.status" class="text-sm text-muted">Loading instances...</div>

    <div v-else-if="!backups.status?.instances.length" class="card text-sm text-muted">
      No instances configured.
    </div>

    <div v-else class="space-y-4">
      <div v-for="inst in backups.status.instances" :key="inst.name" class="card">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-lg font-semibold text-heading">{{ inst.name }}</h2>
              <span class="badge" :class="instanceBadgeClass(inst)">{{ instanceLabel(inst) }}</span>
            </div>
            <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted">
              <span>Host: {{ inst.host }}</span>
              <span>Last run: {{ formatTime(inst.last_run_time) }}</span>
              <span>Next run: {{ formatTime(inst.next_run_time) }}</span>
            </div>
          </div>
          <button
            type="button"
            class="btn-primary"
            :disabled="backups.actionBusy || inst.is_running"
            @click="backups.runInstance(inst.name)"
          >
            Run Now
          </button>
        </div>

        <div v-if="inst.databases.length" class="mt-4">
          <details>
            <summary class="cursor-pointer text-sm font-medium text-muted">
              Show databases ({{ inst.databases.length }})
            </summary>
            <div class="table-scroll mt-3">
              <table class="data-table">
                <thead class="text-muted">
                  <tr>
                    <th>Database</th>
                    <th>Last backup</th>
                    <th>Size</th>
                    <th>Result</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="db in inst.databases" :key="db.name" class="table-row-hover">
                    <td class="text-heading">{{ db.name }}</td>
                    <td>{{ formatTime(db.last_backup_time) }}</td>
                    <td>{{ db.last_backup_size > 0 ? formatBytes(db.last_backup_size) : '-' }}</td>
                    <td>
                      <span class="badge" :class="dbResultBadgeClass(db.last_backup_result)">
                        {{ db.last_backup_result || 'Pending' }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </details>
        </div>
        <p v-else class="mt-3 text-sm text-muted">No databases discovered yet.</p>
      </div>
    </div>
  </div>
</template>
