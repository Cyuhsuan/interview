import { NetworkError, throwApiErrorFromResponse } from './client'

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
    await throwApiErrorFromResponse(response, '/voice/transcriptions')
  }

  const data = (await response.json()) as { text: string }
  return data.text
}
