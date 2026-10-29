import { requestJSON } from './client'
import type { components } from './schema'

export type Meta = components['schemas']['Meta']
export type Readiness = components['schemas']['Readiness']

interface MetaEnvelope {
  data: Meta
}

export async function fetchMeta(signal?: AbortSignal): Promise<Meta> {
  const payload = await requestJSON<MetaEnvelope>('/api/v1/meta', { signal })
  return payload.data
}

/**
 * Readiness deliberately returns HTTP 503 while dependencies are unavailable,
 * so the body is accepted for both 200 and 503.
 */
export async function fetchReadiness(signal?: AbortSignal): Promise<Readiness> {
  return requestJSON<Readiness>('/readyz', { signal, acceptStatuses: [503] })
}
