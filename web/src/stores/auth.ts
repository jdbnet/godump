import { defineStore } from 'pinia'
import api, { type AuthMe } from '@/api/client'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    authenticated: false,
    authEnabled: false,
    checked: false,
  }),
  actions: {
    async check() {
      if (this.checked && this.authenticated) {
        return
      }
      try {
        const { data } = await api.get<AuthMe>('/auth/me')
        this.authenticated = data.authenticated
        this.authEnabled = data.auth_enabled
      } catch {
        this.authenticated = false
        this.authEnabled = true
      }
      this.checked = true
    },
    async login(username: string, password: string) {
      const { data } = await api.post<AuthMe>('/auth/login', { username, password })
      this.authenticated = data.authenticated
      this.authEnabled = data.auth_enabled
      this.checked = true
    },
    async logout() {
      await api.post('/auth/logout')
      this.authenticated = false
      this.checked = true
    },
  },
})
