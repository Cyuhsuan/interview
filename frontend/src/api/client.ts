import type { ProblemDetails, FieldError } from './types'

const BASE_URL = import.meta.env.VITE_API_BASE_URL

export class ApiError extends Error {
  status: number
  code: string
  errors?: FieldError[]

  constructor(problem: ProblemDetails) {
    super(problem.detail || problem.title)
    this.name = 'ApiError'
    this.status = problem.status
    this.code = problem.code
    this.errors = problem.errors
  }
}

export class NetworkError extends Error {
  constructor(message = 'Unable to reach the clinic server. Check your connection and try again.') {
    super(message)
    this.name = 'NetworkError'
  }
}

interface ApiFetchOptions extends RequestInit {
  headers?: Record<string, string>
}

export interface ApiResult<T> {
  data: T
  etag: number | null
}

function parseETag(response: Response): number | null {
  const raw = response.headers.get('ETag')
  if (!raw) return null
  const stripped = raw.replace(/(^"|"$)/g, '')
  const value = Number.parseInt(stripped, 10)
  return Number.isNaN(value) ? null : value
}

export async function apiFetch<T>(path: string, options: ApiFetchOptions = {}): Promise<ApiResult<T>> {
  let response: Response
  try {
    response = await fetch(`${BASE_URL}${path}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
    })
  } catch {
    throw new NetworkError()
  }

  const etag = parseETag(response)

  if (!response.ok) {
    let problem: ProblemDetails
    try {
      problem = (await response.json()) as ProblemDetails
    } catch {
      throw new ApiError({
        type: 'about:blank',
        title: response.statusText,
        status: response.status,
        code: 'INTERNAL_ERROR',
        detail: 'The server returned an unexpected response.',
        instance: path,
      })
    }
    throw new ApiError(problem)
  }

  if (response.status === 204) {
    return { data: undefined as T, etag }
  }

  const data = (await response.json()) as T
  return { data, etag }
}
