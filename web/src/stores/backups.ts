import { defineStore } from 'pinia'
import api, {
  type InstanceInventory,
  type LogEntry,
  type StatusResponse,
} from '@/api/client'

export const useBackupsStore = defineStore('backups', {
  state: () => ({
    status: null as StatusResponse | null,
    inventory: [] as InstanceInventory[],
    logs: [] as LogEntry[],
    statusLoading: false,
    inventoryLoading: false,
    logsLoading: false,
    statusInflight: false,
    inventoryInflight: false,
    logsInflight: false,
    actionBusy: false,
  }),
  actions: {
    async refreshStatus(background = false) {
      if (this.statusInflight) return
      this.statusInflight = true
      if (!background) this.statusLoading = true
      try {
        const { data } = await api.get<StatusResponse>('/status')
        this.status = data
      } finally {
        this.statusInflight = false
        if (!background) this.statusLoading = false
      }
    },
    async refreshInventory(background = false) {
      if (this.inventoryInflight) return
      this.inventoryInflight = true
      if (!background) this.inventoryLoading = true
      try {
        const { data } = await api.get<InstanceInventory[]>('/inventory')
        this.inventory = data
      } finally {
        this.inventoryInflight = false
        if (!background) this.inventoryLoading = false
      }
    },
    async refreshLogs(background = false) {
      if (this.logsInflight) return
      this.logsInflight = true
      if (!background) this.logsLoading = true
      try {
        const { data } = await api.get<LogEntry[]>('/logs')
        this.logs = data
      } finally {
        this.logsInflight = false
        if (!background) this.logsLoading = false
      }
    },
    async runAll() {
      if (this.actionBusy) return
      this.actionBusy = true
      try {
        await api.post('/run/all')
        await this.refreshStatus()
      } finally {
        this.actionBusy = false
      }
    },
    async runInstance(name: string) {
      if (this.actionBusy) return
      this.actionBusy = true
      try {
        await api.post(`/run/${encodeURIComponent(name)}`)
        await this.refreshStatus()
      } finally {
        this.actionBusy = false
      }
    },
    async deleteBackup(instance: string, db: string, file: string) {
      await api.post('/delete', null, {
        params: { instance, db, file },
      })
      await this.refreshInventory()
      await this.refreshStatus(true)
    },
    downloadUrl(instance: string, db: string, file: string) {
      const params = new URLSearchParams({ instance, db, file })
      return `/api/download?${params.toString()}`
    },
  },
})
