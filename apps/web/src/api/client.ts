const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? ''

const DEFAULT_TIMEOUT_MS = 8000

export class ApiError extends Error {
  readonly status: number | undefined
  readonly code: string | undefined
  readonly requestId: string | undefined

  constructor(message: string, status?: number, code?: string, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

interface ErrorEnvelope {
  error?: {
    code?: string
    message?: string
    request_id?: string
  }
}

interface RequestOptions {
  signal?: AbortSignal
  timeoutMs?: number
  /** HTTP error statuses whose JSON body is still a valid response for this call. */
  acceptStatuses?: number[]
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
  return typeof value === 'object' && value !== null && 'error' in value
}

export async function requestJSON<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { signal, timeoutMs = DEFAULT_TIMEOUT_MS, acceptStatuses = [] } = options
  const controller = new AbortController()
  const abortFromCaller = () => controller.abort()
  if (signal?.aborted) controller.abort()
  else signal?.addEventListener('abort', abortFromCaller)
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)

  try {
    const response = await fetch(`${API_BASE_URL}${path}`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      credentials: 'same-origin',
      signal: controller.signal,
    })

    const payload: unknown = await response.json().catch(() => null)

    if (!response.ok && !acceptStatuses.includes(response.status)) {
      const detail = isErrorEnvelope(payload) ? payload.error : undefined
      throw new ApiError(
        detail?.message ?? `请求失败（HTTP ${response.status}）`,
        response.status,
        detail?.code,
        detail?.request_id,
      )
    }

    if (payload === null) {
      throw new ApiError('服务返回了无法解析的响应', response.status)
    }
    return payload as T
  } catch (error) {
    if (error instanceof ApiError) throw error
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError('请求超时，请稍后重试')
    }
    throw new ApiError('无法连接到 Nexora API')
  } finally {
    window.clearTimeout(timer)
    signal?.removeEventListener('abort', abortFromCaller)
  }
}
