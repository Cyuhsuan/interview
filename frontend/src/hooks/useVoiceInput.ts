import { useEffect, useRef, useState } from 'react'
import { transcribeAudio } from '../api/voice'

export type VoiceInputStatus =
  | 'unsupported'
  | 'idle'
  | 'listening'
  | 'transcribing'
  | 'permission-denied'
  | 'error'

function isSupported(): boolean {
  return (
    typeof navigator !== 'undefined' &&
    Boolean(navigator.mediaDevices?.getUserMedia) &&
    typeof MediaRecorder !== 'undefined'
  )
}

// Voice input records with MediaRecorder and sends the clip to the backend
// for AI transcription (see backend/README.md's "Voice Transcription
// Endpoint") rather than using the browser's built-in SpeechRecognition —
// this is a deliberate architecture change from an earlier version, see
// frontend/README.md's "語音行為" section.
export function useVoiceInput({ onResult }: { onResult: (text: string) => void }) {
  const supported = isSupported()
  const [status, setStatus] = useState<VoiceInputStatus>(supported ? 'idle' : 'unsupported')
  const recorderRef = useRef<MediaRecorder | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const onResultRef = useRef(onResult)

  useEffect(() => {
    onResultRef.current = onResult
  }, [onResult])

  useEffect(() => {
    return () => {
      recorderRef.current?.stream.getTracks().forEach((track) => track.stop())
      recorderRef.current = null
    }
  }, [])

  async function start() {
    if (!supported) return
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      chunksRef.current = []
      const recorder = new MediaRecorder(stream)
      recorderRef.current = recorder

      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunksRef.current.push(event.data)
      }

      recorder.onstop = () => {
        stream.getTracks().forEach((track) => track.stop())
        const blob = new Blob(chunksRef.current, { type: recorder.mimeType })
        chunksRef.current = []
        setStatus('transcribing')
        transcribeAudio(blob)
          .then((text) => {
            onResultRef.current(text)
            setStatus('idle')
          })
          .catch(() => {
            setStatus('error')
          })
      }

      recorder.start()
      setStatus('listening')
    } catch (err) {
      if (err instanceof DOMException && (err.name === 'NotAllowedError' || err.name === 'SecurityError')) {
        setStatus('permission-denied')
      } else {
        setStatus('error')
      }
    }
  }

  function stop() {
    recorderRef.current?.stop()
  }

  return { status, start, stop }
}
