import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { useThemeStore } from './theme'

describe('theme store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.localStorage.clear()
    document.documentElement.classList.remove('dark')
    document.documentElement.style.colorScheme = ''
  })

  it('applies and persists an explicit dark preference', () => {
    const theme = useThemeStore()
    theme.initialize()

    theme.setPreference('dark')

    expect(theme.resolved).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.style.colorScheme).toBe('dark')
    expect(window.localStorage.getItem('nexora.theme')).toBe('dark')
  })

  it('restores a saved preference on initialization', () => {
    window.localStorage.setItem('nexora.theme', 'light')

    const theme = useThemeStore()
    theme.initialize()

    expect(theme.preference).toBe('light')
    expect(theme.resolved).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
