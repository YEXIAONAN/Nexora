const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? ''

const DEFAULT_TIMEOUT_MS = 8000

export const SESSION_EXPIRED_EVENT = 'nexora:session-expired'

const MUTATING_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

export class ApiError extends Error {
  readonly status: number | undefined
  readonly code: string | undefined
  readonly requestId: string | undefined
  readonly fields: Record<string, string> | undefined

  constructor(
    message: string,
    status?: number,
    code?: string,
    requestId?: string,
    fields?: Record<string, string>,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
    this.fields = fields
  }
}

interface ErrorEnvelope {
  error?: {
    code?: string
    message?: string
    request_id?: string
    fields?: Record<string, string>
  }
}

interface RequestOptions {
  method?: string
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
  timeoutMs?: number
  /** HTTP error statuses whose JSON body is still a valid response for this call. */
  acceptStatuses?: number[]
  /** Login failures are 401 too; they must not be treated as session expiry. */
  skipSessionExpiry?: boolean
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
  return typeof value === 'object' && value !== null && 'error' in value
}

export async function requestJSON<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const {
    method = 'GET',
    body,
    headers = {},
    signal,
    timeoutMs = DEFAULT_TIMEOUT_MS,
    acceptStatuses = [],
    skipSessionExpiry = false,
  } = options
  const controller = new AbortController()
  const abortFromCaller = () => controller.abort()
  if (signal?.aborted) controller.abort()
  else signal?.addEventListener('abort', abortFromCaller)
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)

  try {
    const requestHeaders: Record<string, string> = { Accept: 'application/json', ...headers }
    if (MUTATING_METHODS.has(method)) requestHeaders['X-Nexora-CSRF'] = '1'
    const multipart = body instanceof FormData
    if (body !== undefined && !multipart) requestHeaders['Content-Type'] = 'application/json'

    const response = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers: requestHeaders,
      body: body !== undefined ? (multipart ? body : JSON.stringify(body)) : undefined,
      credentials: 'same-origin',
      signal: controller.signal,
    })

    const payload: unknown = await response.json().catch(() => null)

    // A canceled old context must not expire the current session after body parsing aborts.
    if (controller.signal.aborted) throw new DOMException('Request aborted', 'AbortError')

    if (!response.ok && !acceptStatuses.includes(response.status)) {
      if (response.status === 401 && !skipSessionExpiry) {
        window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT))
      }
      const detail = isErrorEnvelope(payload) ? payload.error : undefined
      throw new ApiError(
        detail?.message ?? `请求失败（HTTP ${response.status}）`,
        response.status,
        detail?.code,
        detail?.request_id,
        detail?.fields,
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
