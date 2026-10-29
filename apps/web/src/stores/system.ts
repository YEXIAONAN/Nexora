import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { ApiError } from '@/api/client'
import { fetchMeta, fetchReadiness, type Meta, type Readiness } from '@/api/system'

export const useSystemStore = defineStore('system', () => {
  const meta = ref<Meta | null>(null)
  const readiness = ref<Readiness | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const lastCheckedAt = ref<Date | null>(null)

  const isReady = computed(() => readiness.value?.status === 'ready')

  async function refresh(): Promise<void> {
    loading.value = true
    error.value = null

    const [metaResult, readinessResult] = await Promise.allSettled([fetchMeta(), fetchReadiness()])

    if (metaResult.status === 'fulfilled') meta.value = metaResult.value
    if (readinessResult.status === 'fulfilled') readiness.value = readinessResult.value

    if (metaResult.status === 'rejected' || readinessResult.status === 'rejected') {
      const reason =
        metaResult.status === 'rejected'
          ? metaResult.reason
          : readinessResult.status === 'rejected'
            ? readinessResult.reason
            : null
      error.value = reason instanceof ApiError ? reason.message : '无法连接到 Nexora API'
    }

    loading.value = false
    lastCheckedAt.value = new Date()
  }

  return { meta, readiness, loading, error, lastCheckedAt, isReady, refresh }
})
