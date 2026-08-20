import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  withCredentials: true,
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401 && !window.location.pathname.startsWith('/login')) {
      const redirect = encodeURIComponent(window.location.pathname + window.location.search)
      window.location.href = `/login?redirect=${redirect}`
    }
    return Promise.reject(error)
  },
)

export default api

export interface AuthMe {
  authenticated: boolean
  auth_enabled: boolean
}

export interface DBStatus {
  name: string
  last_backup_time: string
  last_backup_size: number
  last_backup_result: string
  last_backup_duration: number
}

export interface InstanceStatus {
  name: string
  host: string
  last_run_time: string
  next_run_time: string
  overall_result: string
  is_running: boolean
  databases: DBStatus[]
}

export interface StatusResponse {
  total_instances: number
  total_databases: number
  unreachable_count: number
  total_backup_size: number
  any_running: boolean
  instances: InstanceStatus[]
}

export interface BackupFile {
  name: string
  size: number
  timestamp: string
}

export interface DBInventory {
  name: string
  files: BackupFile[]
}

export interface InstanceInventory {
  name: string
  databases: DBInventory[]
}

export interface LogEntry {
  timestamp: string
  level: string
  instance?: string
  message: string
}
