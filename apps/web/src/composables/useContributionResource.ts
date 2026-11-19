import { onScopeDispose, ref, shallowRef, watch } from 'vue'

import { ApiError } from '@/api/client'
import { contributionError } from '@/components/contributions/contributionPresentation'

/** Context must be a stable key or an immutable snapshot, and null when inaccessible. */
export function useContributionResource<T, C>(
  context: () => C | null,
  loader: (context: C, signal: AbortSignal) => Promise<T>,
) {
  const data = shallowRef<T | null>(null)
  const loading = ref(false)
  const pending = ref(false)
  const error = ref<string | null>(null)
  let generation = 0
  let readGeneration = 0
  let currentContext: C | null = null
  let readController: AbortController | null = null
  let writeController: AbortController | null = null

  async function reload(clearError = true): Promise<void> {
    const capturedContext = currentContext
    if (capturedContext === null) return
    const capturedGeneration = generation
    const capturedRead = ++readGeneration
    readController?.abort()
    const controller = new AbortController()
    readController = controller
    loading.value = true
    if (clearError) error.value = null
    try {
      const result = await loader(capturedContext, controller.signal)
      if (generation === capturedGeneration && capturedRead === readGeneration) data.value = result
    } catch (reason) {
      if (
        generation === capturedGeneration &&
        capturedRead === readGeneration &&
        !controller.signal.aborted
      ) {
        // Never retain sensitive data after a failed authoritative read (including revoked access).
        data.value = null
        const message = contributionError(reason)
        error.value = error.value ? `${error.value}；${message}` : message
      }
    } finally {
      if (generation === capturedGeneration && capturedRead === readGeneration)
        loading.value = false
    }
  }

  async function mutate<R>(command: (context: C, signal: AbortSignal) => Promise<R>) {
    if (pending.value || loading.value || currentContext === null) return
    const capturedContext = currentContext
    const capturedGeneration = generation
    const controller = new AbortController()
    writeController = controller
    pending.value = true
    error.value = null
    try {
      const result = await command(capturedContext, controller.signal)
      if (generation !== capturedGeneration || controller.signal.aborted) return
      await reload()
      if (generation === capturedGeneration) return result
    } catch (reason) {
      if (generation !== capturedGeneration || controller.signal.aborted) return
      error.value = contributionError(reason)
      if (reason instanceof ApiError && reason.status === 409) {
        // Read current state; never retry or overwrite a conflicting command.
        await reload(false)
      }
    } finally {
      if (generation === capturedGeneration) {
        pending.value = false
        writeController = null
      }
    }
  }

  const stop = watch(
    context,
    (value) => {
      ++generation
      ++readGeneration
      readController?.abort()
      writeController?.abort()
      currentContext = value
      data.value = null
      error.value = null
      loading.value = false
      pending.value = false
      if (value !== null) void reload()
    },
    { immediate: true, flush: 'sync', deep: true },
  )

  onScopeDispose(() => {
    stop()
    ++generation
    ++readGeneration
    readController?.abort()
    writeController?.abort()
    data.value = null
  })

  return { data, loading, pending, error, reload, mutate }
}
