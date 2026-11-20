import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import * as authAPI from '@/api/auth'
import type { RegisterInput, User } from '@/api/auth'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const expiresAt = ref<string | null>(null)
  const initialized = ref(false)
  const loading = ref(false)
  let pending: Promise<void> | null = null

  const isAuthenticated = computed(() => user.value !== null)
  const displayName = computed(() => user.value?.display_name || user.value?.username || '')
  const isPlatformAdmin = computed(() =>
    Boolean(user.value?.system_root || user.value?.platform_admin),
  )

  async function initialize(): Promise<void> {
    if (initialized.value) return
    if (pending) return pending
    loading.value = true
    pending = (async () => {
      try {
        user.value = await authAPI.fetchMe()
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
    const result = await authAPI.login(username, password)
    user.value = result.user
    expiresAt.value = result.expires_at
    initialized.value = true
  }

  async function register(input: RegisterInput): Promise<User> {
    return authAPI.register(input)
  }

  async function updateProfile(displayName: string, email: string): Promise<void> {
    user.value = await authAPI.updateProfile(displayName, email)
  }

  async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
    await authAPI.changePassword(currentPassword, newPassword)
    expire()
  }

  function setUser(next: User): void {
    user.value = next
  }

  async function logout(): Promise<void> {
    try {
      await authAPI.logout()
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
    isPlatformAdmin,
    displayName,
    initialize,
    login,
    register,
    updateProfile,
    changePassword,
    setUser,
    logout,
    expire,
  }
})
