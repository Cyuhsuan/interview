import type { VoiceInputStatus } from '../../../hooks/useVoiceInput'
import styles from './VoiceControls.module.css'

export function MicButton({
  status,
  disabled,
  onStart,
  onStop,
}: {
  status: VoiceInputStatus
  disabled: boolean
  onStart: () => void
  onStop: () => void
}) {
  if (status === 'unsupported') return null

  const listening = status === 'listening'
  const transcribing = status === 'transcribing'

  return (
    <button
      type="button"
      className={styles.micButton}
      aria-pressed={listening}
      disabled={disabled || transcribing}
      onClick={listening ? onStop : onStart}
    >
      {listening ? '⏹ Stop & transcribe' : '🎤 Voice input'}
    </button>
  )
}
