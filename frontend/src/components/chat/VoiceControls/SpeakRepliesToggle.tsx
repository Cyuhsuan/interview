import styles from './VoiceControls.module.css'

export function SpeakRepliesToggle({
  supported,
  enabled,
  onChange,
}: {
  supported: boolean
  enabled: boolean
  onChange: (value: boolean) => void
}) {
  return (
    <label className={styles.speakToggle}>
      <input
        type="checkbox"
        checked={enabled}
        disabled={!supported}
        onChange={(e) => onChange(e.target.checked)}
      />
      Speak replies aloud{!supported && ' (not supported in this browser)'}
    </label>
  )
}
