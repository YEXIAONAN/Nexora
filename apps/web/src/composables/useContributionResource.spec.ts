import { effectScope, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/api/client'
import { useContributionResource } from './useContributionResource'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => {
  for (const scope of scopes.splice(0)) scope.stop()
})

function resource(
  loader: (key: string, signal: AbortSignal) => Promise<string>,
  initial: string | null = 'team-a/project-a',
) {
  const key = ref(initial)
  const scope = effectScope()
  scopes.push(scope)
  const state = scope.run(() => useContributionResource(() => key.value, loader))!
  return { ...state, key, scope }
}

describe('contribution context isolation', () => {
  it('clears prior Team data synchronously and ignores its late response', async () => {
    const first = deferred<string>()
    const second = deferred<string>()
    const loader = vi.fn((key: string, _signal: AbortSignal) =>
      key === 'team-a/project-a' ? first.promise : second.promise,
    )
    const r = resource(loader)
    const oldSignal = loader.mock.calls[0]![1]
    r.key.value = 'team-b/project-b'
    expect(oldSignal.aborted).toBe(true)
    expect(r.data.value).toBeNull()
    second.resolve('Team B private record')
    await flushPromises()
    expect(r.data.value).toBe('Team B private record')
    first.resolve('Team A private record')
    await flushPromises()
    expect(r.data.value).toBe('Team B private record')
    expect(r.loading.value).toBe(false)
  })

  it('clears loaded data on a Project switch within the same Team', async () => {
    const second = deferred<string>()
    const r = resource(async (key) => (key.endsWith('project-a') ? 'first secret' : second.promise))
    await flushPromises()
    expect(r.data.value).toBe('first secret')
    r.key.value = 'team-a/project-b'
    expect(r.data.value).toBeNull()
    expect(r.loading.value).toBe(true)
    second.resolve('second secret')
    await flushPromises()
    expect(r.data.value).toBe('second secret')
  })

  it('isolates overlapping reloads even when abort is ignored by an underlying API', async () => {
    const old = deferred<string>()
    const fresh = deferred<string>()
    const loader = vi
      .fn<(key: string, signal: AbortSignal) => Promise<string>>()
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(fresh.promise)
    const r = resource(loader)
    const reloading = r.reload()
    fresh.resolve('fresh')
    await reloading
    old.resolve('stale')
    await flushPromises()
    expect(r.data.value).toBe('fresh')
  })

  it('does not let a late write refresh the newly selected Team', async () => {
    const loader = vi.fn(async (key: string) => key)
    const r = resource(loader)
    await flushPromises()
    const write = deferred<string>()
    let writeSignal: AbortSignal | null = null
    const writing = r.mutate((_context, signal) => {
      writeSignal = signal
      return write.promise
    })
    expect(r.pending.value).toBe(true)
    r.key.value = 'team-b/project-b'
    expect((writeSignal as AbortSignal | null)?.aborted).toBe(true)
    await flushPromises()
    write.resolve('old write succeeded on server')
    expect(await writing).toBeUndefined()
    expect(loader).toHaveBeenCalledTimes(2)
    expect(r.data.value).toBe('team-b/project-b')
    expect(r.pending.value).toBe(false)
  })

  it('does not expose a late write error in a different Project', async () => {
    const r = resource(async (key) => key)
    await flushPromises()
    const write = deferred<string>()
    const writing = r.mutate(() => write.promise)
    r.key.value = 'team-a/project-b'
    await flushPromises()
    write.reject(new ApiError('old project conflict', 409))
    await writing
    expect(r.error.value).toBeNull()
    expect(r.data.value).toBe('team-a/project-b')
  })

  it('disables requests and clears sensitive state when context becomes inaccessible', async () => {
    const loader = vi.fn(async () => 'secret')
    const r = resource(loader)
    await flushPromises()
    r.key.value = null
    expect(r.data.value).toBeNull()
    expect(r.error.value).toBeNull()
    const command = vi.fn()
    await r.mutate(command)
    await r.reload()
    expect(command).not.toHaveBeenCalled()
    expect(loader).toHaveBeenCalledTimes(1)
  })

  it('aborts and ignores pending reads on unmount', async () => {
    const read = deferred<string>()
    let signal: AbortSignal | null = null
    const r = resource((_key, s) => {
      signal = s
      return read.promise
    })
    r.scope.stop()
    expect((signal as AbortSignal | null)?.aborted).toBe(true)
    read.resolve('late secret')
    await flushPromises()
    expect(r.data.value).toBeNull()
  })
})

describe('contribution writes', () => {
  it('blocks double submission until confirmation and its authoritative reload complete', async () => {
    const reload = deferred<string>()
    const loader = vi
      .fn<(key: string, signal: AbortSignal) => Promise<string>>()
      .mockResolvedValueOnce('pending')
      .mockReturnValueOnce(reload.promise)
    const r = resource(loader)
    await flushPromises()
    const confirmation = deferred<string>()
    const command = vi.fn(() => confirmation.promise)
    const writing = r.mutate(command)
    await r.mutate(command)
    expect(command).toHaveBeenCalledTimes(1)
    confirmation.resolve('confirmation id')
    await flushPromises()
    expect(r.pending.value).toBe(true)
    await r.mutate(command)
    expect(command).toHaveBeenCalledTimes(1)
    reload.resolve('confirmed')
    expect(await writing).toBe('confirmation id')
    expect(r.data.value).toBe('confirmed')
    expect(r.pending.value).toBe(false)
  })

  it('reloads after 409, retains conflict guidance, and never retries the command', async () => {
    const loader = vi.fn(async () => 'initial')
    const r = resource(loader)
    await flushPromises()
    loader.mockResolvedValueOnce('authoritative new version')
    const command = vi.fn(async () => {
      throw new ApiError('版本不匹配', 409)
    })
    await r.mutate(command)
    expect(command).toHaveBeenCalledTimes(1)
    expect(loader).toHaveBeenCalledTimes(2)
    expect(r.data.value).toBe('authoritative new version')
    expect(r.error.value).toContain('状态可能已改变')
    expect(r.error.value).toContain('不会自动覆盖')
  })

  it('clears stale data if a conflict reload reveals revoked access', async () => {
    const loader = vi
      .fn<(key: string, signal: AbortSignal) => Promise<string>>()
      .mockResolvedValueOnce('secret')
      .mockRejectedValueOnce(new ApiError('not found', 404))
    const r = resource(loader)
    await flushPromises()
    await r.mutate(async () => {
      throw new ApiError('conflict', 409)
    })
    expect(r.data.value).toBeNull()
    expect(r.error.value).toContain('状态可能已改变')
    expect(r.error.value).toContain('当前不可访问')
    expect(r.pending.value).toBe(false)
  })

  it('does not submit a command while authoritative data is loading', async () => {
    const load = deferred<string>()
    const r = resource(() => load.promise)
    const command = vi.fn()
    await r.mutate(command)
    expect(command).not.toHaveBeenCalled()
    load.resolve('fresh')
    await flushPromises()
  })

  it('shows server permission denial without pretending a write succeeded', async () => {
    const loader = vi.fn(async () => 'pending')
    const r = resource(loader)
    await flushPromises()
    const result = await r.mutate(async () => {
      throw new ApiError('beneficiary conflict', 403)
    })
    expect(result).toBeUndefined()
    expect(loader).toHaveBeenCalledTimes(1)
    expect(r.error.value).toContain('独立性要求')
    expect(r.error.value).toContain('beneficiary conflict')
    expect(r.pending.value).toBe(false)
  })
})
