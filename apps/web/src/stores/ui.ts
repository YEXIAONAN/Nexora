import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastTone = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  tone: ToastTone
  text: string
}

interface ConfirmState {
  title: string
  message: string
  confirmLabel: string
  tone: 'default' | 'danger'
  resolve: (value: boolean) => void
}

export const useUiStore = defineStore('ui', () => {
  const toasts = ref<Toast[]>([])
  const confirmState = ref<ConfirmState | null>(null)
  let nextId = 1

  function notify(text: string, tone: ToastTone = 'success'): void {
    const id = nextId++
    toasts.value = [...toasts.value, { id, tone, text }]
    window.setTimeout(() => dismiss(id), 4200)
  }

  function dismiss(id: number): void {
    toasts.value = toasts.value.filter((toast) => toast.id !== id)
  }

  function confirm(options: {
    title: string
    message: string
    confirmLabel?: string
    tone?: 'default' | 'danger'
  }): Promise<boolean> {
    confirmState.value?.resolve(false)
    return new Promise((resolve) => {
      confirmState.value = {
        title: options.title,
        message: options.message,
        confirmLabel: options.confirmLabel ?? '确认',
        tone: options.tone ?? 'default',
        resolve,
      }
    })
  }

  function resolveConfirm(value: boolean): void {
    confirmState.value?.resolve(value)
    confirmState.value = null
  }

  return { toasts, confirmState, notify, dismiss, confirm, resolveConfirm }
})
