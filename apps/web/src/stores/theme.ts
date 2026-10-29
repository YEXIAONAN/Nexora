import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export type ThemePreference = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

const STORAGE_KEY = 'nexora.theme'

function supportsPrefersColorScheme(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function'
}

function prefersDark(): boolean {
  return supportsPrefersColorScheme() && window.matchMedia('(prefers-color-scheme: dark)').matches
}

function isThemePreference(value: unknown): value is ThemePreference {
  return value === 'light' || value === 'dark' || value === 'system'
}

export const useThemeStore = defineStore('theme', () => {
  const preference = ref<ThemePreference>('system')
  const systemPrefersDark = ref(prefersDark())

  const resolved = computed<ResolvedTheme>(() =>
    preference.value === 'system' ? (systemPrefersDark.value ? 'dark' : 'light') : preference.value,
  )

  function apply(): void {
    const isDark = resolved.value === 'dark'
    document.documentElement.classList.toggle('dark', isDark)
    document.documentElement.style.colorScheme = resolved.value
  }

  function setPreference(next: ThemePreference): void {
    preference.value = next
    try {
      window.localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // Private browsing may block storage; the in-memory preference still works.
    }
    apply()
  }

  function initialize(): void {
    try {
      const saved = window.localStorage.getItem(STORAGE_KEY)
      if (isThemePreference(saved)) preference.value = saved
    } catch {
      // Ignore unavailable storage.
    }

    if (supportsPrefersColorScheme()) {
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (event) => {
        systemPrefersDark.value = event.matches
        if (preference.value === 'system') apply()
      })
    }

    apply()
  }

  return { preference, resolved, setPreference, initialize }
})
