import { useEffect, useState } from 'react'

const STORAGE_KEY = 'voice.speakRepliesEnabled'

export function useVoiceOutput() {
  const supported = typeof window !== 'undefined' && 'speechSynthesis' in window
  const [enabled, setEnabledState] = useState(
    () => supported && window.localStorage.getItem(STORAGE_KEY) === 'true',
  )

  function setEnabled(value: boolean) {
    setEnabledState(value)
    if (supported) window.localStorage.setItem(STORAGE_KEY, String(value))
    if (!value && supported) window.speechSynthesis.cancel()
  }

  useEffect(() => {
    return () => {
      if (supported) window.speechSynthesis.cancel()
    }
  }, [supported])

  function speak(text: string) {
    if (!supported || !enabled) return
    window.speechSynthesis.cancel()
    window.speechSynthesis.speak(new SpeechSynthesisUtterance(text))
  }

  return { supported, enabled, setEnabled, speak }
}
