import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { ApiError } from '@/api/client'
import { fetchMeta, fetchReadiness, type Meta, type Readiness } from '@/api/system'

export const useSystemStore = defineStore('system', () => {
  const meta = ref<Meta | null>(null)
  const readiness = ref<Readiness | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const metaError = ref<string | null>(null)
  const readinessError = ref<string | null>(null)
  const lastCheckedAt = ref<Date | null>(null)
  let refreshVersion = 0
  let activeRequest: AbortController | undefined

  const status = computed<'unknown' | 'loading' | 'ready' | 'unavailable' | 'error'>(() => {
    if (loading.value) return 'loading'
    if (error.value) return 'error'
    return readiness.value?.status ?? 'unknown'
  })
  const statusLabel = computed(() => {
    const labels = {
      unknown: '未知',
      loading: '检查中',
      ready: '就绪',
      unavailable: '未就绪',
      error: '检查失败',
    }
    return labels[status.value]
  })
  const statusTone = computed<'neutral' | 'success' | 'warning' | 'danger'>(() => {
    if (status.value === 'ready') return 'success'
    if (status.value === 'unavailable') return 'warning'
    if (status.value === 'error') return 'danger'
    return 'neutral'
  })
  const apiState = computed<'unknown' | 'loading' | 'up' | 'down'>(() => {
    if (loading.value) return 'loading'
    if (metaError.value) return 'down'
    return meta.value ? 'up' : 'unknown'
  })
  const isReady = computed(() => status.value === 'ready')

  function errorMessage(reason: unknown): string {
    return reason instanceof ApiError ? reason.message : '无法连接到 Nexora API'
  }

  async function refresh(): Promise<void> {
    const version = ++refreshVersion
    activeRequest?.abort()
    const request = new AbortController()
    activeRequest = request
    loading.value = true
    error.value = null
    metaError.value = null
    readinessError.value = null
    // Only show results from this attempt. Historical green states must not
    // survive a pending or failed refresh.
    meta.value = null
    readiness.value = null
    lastCheckedAt.value = null

    const [metaResult, readinessResult] = await Promise.allSettled([
      fetchMeta(request.signal),
      fetchReadiness(request.signal),
    ])
    // Cancellation is best effort; an older response must never overwrite a
    // newer attempt, even if the transport ignores AbortSignal.
    if (version !== refreshVersion) return

    if (metaResult.status === 'fulfilled') meta.value = metaResult.value
    else metaError.value = errorMessage(metaResult.reason)
    if (readinessResult.status === 'fulfilled') readiness.value = readinessResult.value
    else readinessError.value = errorMessage(readinessResult.reason)
    error.value = metaError.value ?? readinessError.value

    loading.value = false
    activeRequest = undefined
    // Completion time of the attempt, including a failure; never a success claim.
    lastCheckedAt.value = new Date()
  }

  return {
    meta,
    readiness,
    loading,
    error,
    metaError,
    readinessError,
    lastCheckedAt,
    status,
    statusLabel,
    statusTone,
    apiState,
    isReady,
    refresh,
  }
})
