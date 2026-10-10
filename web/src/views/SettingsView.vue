<script setup lang="ts">
import { onMounted, ref } from 'vue'
import api from '@/api/client'
import { confirm } from '@/lib/confirm'
import { formatTime } from '@/lib/format'

interface APIKey {
  id: string
  name: string
  prefix: string
  created_at: string | null
  source: string
}

interface CreatedAPIKey extends APIKey {
  key: string
}

const keys = ref<APIKey[]>([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const name = ref('')
const created = ref<CreatedAPIKey | null>(null)
const copied = ref(false)

function errorMessage(err: unknown, fallback: string) {
  if (typeof err === 'object' && err && 'response' in err) {
    const message = (err as { response?: { data?: { error?: string } } }).response?.data?.error
    if (message) return message
  }
  return fallback
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await api.get<{ keys: APIKey[] }>('/keys')
    keys.value = data.keys ?? []
  } catch (err) {
    error.value = errorMessage(err, 'Could not load API keys')
  } finally {
    loading.value = false
  }
}

async function createKey() {
  saving.value = true
  error.value = ''
  copied.value = false
  try {
    const { data } = await api.post<CreatedAPIKey>('/keys', { name: name.value })
    created.value = data
    name.value = ''
    await load()
  } catch (err) {
    error.value = errorMessage(err, 'Could not create the API key')
  } finally {
    saving.value = false
  }
}

async function copyKey() {
  if (!created.value) return
  await navigator.clipboard.writeText(created.value.key)
  copied.value = true
}

async function revoke(key: APIKey) {
  const ok = await confirm({
    title: 'Revoke API key?',
    message: `Revoke "${key.name}"? Applications using it will no longer be able to read backup status.`,
    confirmLabel: 'Revoke',
  })
  if (!ok) return
  error.value = ''
  try {
    await api.delete(`/keys/${encodeURIComponent(key.id)}`)
    if (created.value?.id === key.id) created.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err, 'Could not revoke the API key')
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-xl font-semibold text-heading">API keys</h1>
      <p class="mt-1 text-sm text-muted">
        Read-only keys for the status API. A key can read backup health and cannot start, change, or delete backups.
        The full key is shown only once, when you create it.
      </p>
    </div>

    <div v-if="error" class="rounded-lg border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-300">
      {{ error }}
    </div>

    <form class="card space-y-3" @submit.prevent="createKey">
      <div>
        <label for="key-name" class="mb-1 block text-sm text-muted">Key name</label>
        <input id="key-name" v-model="name" type="text" required maxlength="80" class="input-field" placeholder="Dashboard" />
      </div>
      <button type="submit" class="btn-primary" :disabled="saving || !name.trim()">
        {{ saving ? 'Creating...' : 'Create key' }}
      </button>
    </form>

    <div v-if="created" class="card space-y-3 border-amber-500/40">
      <p class="text-sm text-heading">Copy this key now. It will not be shown again.</p>
      <input :value="created.key" readonly class="input-field font-mono" />
      <button type="button" class="btn-secondary" @click="copyKey">
        {{ copied ? 'Copied' : 'Copy key' }}
      </button>
    </div>

    <div v-if="loading" class="text-sm text-muted">Loading API keys...</div>

    <div v-else-if="!keys.length" class="card text-sm text-muted">No API keys yet.</div>

    <div v-else class="card">
      <div class="table-scroll">
        <table class="data-table">
          <thead class="text-muted">
            <tr>
              <th>Name</th>
              <th>Prefix</th>
              <th>Created</th>
              <th>Source</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="key in keys" :key="key.id" class="table-row-hover">
              <td class="text-heading">{{ key.name }}</td>
              <td class="font-mono text-xs">{{ key.prefix ? key.prefix + '...' : 'Hidden' }}</td>
              <td>{{ key.created_at ? formatTime(key.created_at) : 'Not recorded' }}</td>
              <td>{{ key.source === 'config' ? 'Configuration' : 'This app' }}</td>
              <td>
                <button v-if="key.source !== 'config'" type="button" class="btn-danger" @click="revoke(key)">
                  Revoke
                </button>
                <span v-else class="text-xs text-muted">Remove it from the configuration file, then restart GoDump.</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
