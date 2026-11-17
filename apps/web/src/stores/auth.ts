import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { fetchMe, login as apiLogin, logout as apiLogout, type User } from '@/api/auth'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const expiresAt = ref<string | null>(null)
  const initialized = ref(false)
  const loading = ref(false)
  let pending: Promise<void> | null = null

  const isAuthenticated = computed(() => user.value !== null)
  const displayName = computed(() => user.value?.display_name || user.value?.username || '')

  async function initialize(): Promise<void> {
    if (initialized.value) return
    if (pending) return pending
    loading.value = true
    pending = (async () => {
      try {
        user.value = await fetchMe()
      } catch {
        // Fail closed: any failure (401, network, API down) means anonymous.
        user.value = null
      } finally {
        expiresAt.value = null
        initialized.value = true
        loading.value = false
        pending = null
      }
    })()
    return pending
  }

  async function login(username: string, password: string): Promise<void> {
    const result = await apiLogin(username, password)
    user.value = result.user
    expiresAt.value = result.expires_at
    initialized.value = true
  }

  async function logout(): Promise<void> {
    try {
      await apiLogout()
    } catch {
      // Server-side revocation is best effort; the local identity is always
      // cleared so the UI never keeps a session the user asked to end.
    }
    expire()
  }

  function expire(): void {
    user.value = null
    expiresAt.value = null
  }

  return {
    user,
    expiresAt,
    initialized,
    loading,
    isAuthenticated,
    displayName,
    initialize,
    login,
    logout,
    expire,
  }
})
