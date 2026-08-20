<script setup lang="ts">
import { onMounted } from 'vue'
import { useBackupsStore } from '@/stores/backups'
import { confirm } from '@/lib/confirm'
import { formatBytes, formatTime } from '@/lib/format'

const backups = useBackupsStore()

onMounted(() => backups.refreshInventory())

async function deleteFile(instance: string, db: string, file: string) {
  const ok = await confirm({
    title: 'Delete backup?',
    message: `Delete ${file} from ${instance}/${db}? This cannot be undone.`,
    confirmLabel: 'Delete',
  })
  if (!ok) return
  try {
    await backups.deleteBackup(instance, db, file)
  } catch {
    window.alert('Failed to delete backup file')
  }
}
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-xl font-semibold text-heading">Backups</h1>
      <p class="mt-1 text-sm text-muted">Browse backup files on disk by instance and database</p>
    </div>

    <div v-if="backups.inventoryLoading && !backups.inventory.length" class="text-sm text-muted">
      Loading inventory...
    </div>

    <div v-else-if="!backups.inventory.length" class="card text-sm text-muted">
      No backup files found.
    </div>

    <div v-else class="space-y-3">
      <details v-for="inst in backups.inventory" :key="inst.name" class="card">
        <summary class="cursor-pointer text-lg font-medium text-heading">{{ inst.name }}</summary>
        <div class="mt-4 space-y-3">
          <details v-for="db in inst.databases" :key="db.name" class="border-l-2 border-default pl-4">
            <summary class="cursor-pointer text-sm text-muted">
              {{ db.name }}
              <span class="opacity-70">({{ db.files.length }} files)</span>
            </summary>
            <div class="table-scroll mt-2">
              <table class="data-table">
                <thead class="text-muted">
                  <tr>
                    <th>File</th>
                    <th>Created</th>
                    <th>Size</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="file in db.files" :key="file.name" class="table-row-hover">
                    <td class="max-w-xs whitespace-normal break-all text-heading">{{ file.name }}</td>
                    <td>{{ formatTime(file.timestamp) }}</td>
                    <td>{{ formatBytes(file.size) }}</td>
                    <td>
                      <div class="flex gap-2">
                        <a
                          class="btn-row btn-row-edit"
                          :href="backups.downloadUrl(inst.name, db.name, file.name)"
                        >
                          Download
                        </a>
                        <button
                          type="button"
                          class="btn-row text-red-600 hover:bg-red-500/10 dark:text-red-400"
                          @click="deleteFile(inst.name, db.name, file.name)"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </details>
        </div>
      </details>
    </div>
  </div>
</template>
