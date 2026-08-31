import { ApiError, NetworkError } from './client'
import type { ProblemDetails } from './types'

const BASE_URL = import.meta.env.VITE_API_BASE_URL

// Doesn't reuse apiFetch: that helper always forces a
// `Content-Type: application/json` header, which would break the
// multipart/form-data boundary this request needs.
export async function transcribeAudio(audio: Blob): Promise<string> {
  const form = new FormData()
  form.append('audio', audio, 'audio')

  let response: Response
  try {
    response = await fetch(`${BASE_URL}/voice/transcriptions`, {
      method: 'POST',
      body: form,
    })
  } catch {
    throw new NetworkError()
  }

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
        instance: '/voice/transcriptions',
      })
    }
    throw new ApiError(problem)
  }

  const data = (await response.json()) as { text: string }
  return data.text
}
