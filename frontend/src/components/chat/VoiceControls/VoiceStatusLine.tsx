import type { VoiceInputStatus } from '../../../hooks/useVoiceInput'
import { LiveRegion } from '../../shared/LiveRegion'
import styles from './VoiceControls.module.css'

const STATUS_TEXT: Partial<Record<VoiceInputStatus, string>> = {
  listening: 'Listening…',
  transcribing: 'Transcribing…',
  'permission-denied': 'Microphone permission denied.',
  error: 'Voice input had a problem.',
}

export function VoiceStatusLine({
  status,
  onRetry,
}: {
  status: VoiceInputStatus
  onRetry: () => void
}) {
  const text = STATUS_TEXT[status]
  if (!text) return null

  const canRetry = status === 'permission-denied' || status === 'error'

  return (
    <p className={styles.statusLine} role="status">
      {text}
      {canRetry && (
        <button type="button" className={styles.retryButton} onClick={onRetry}>
          Try again
        </button>
      )}
      <LiveRegion message={text} assertive={canRetry} />
    </p>
  )
}
