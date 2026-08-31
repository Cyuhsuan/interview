import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { useVoiceInput } from '../../../hooks/useVoiceInput'
import { MicButton } from '../VoiceControls/MicButton'
import { VoiceStatusLine } from '../VoiceControls/VoiceStatusLine'
import styles from './ChatInput.module.css'

const MAX_LENGTH = 2000

export function ChatInput({
  disabled,
  onSend,
}: {
  disabled: boolean
  onSend: (text: string) => void
}) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const wasDisabled = useRef(disabled)
  // Voice results skip the review step: the transcript is sent immediately
  // rather than populating the textarea, since sending a chat message isn't
  // an irreversible action (it only proposes candidate values) — booking
  // confirmation remains a separate, explicit step later in the flow.
  const voice = useVoiceInput({
    onResult: (text) => {
      const trimmed = text.trim()
      if (!trimmed || disabled) return
      onSend(trimmed)
    },
  })

  // The textarea is disabled while a message is in flight, which makes the
  // browser blur it. Once it's re-enabled, restore focus so the patient can
  // keep typing without having to click back into the field.
  useEffect(() => {
    if (wasDisabled.current && !disabled) {
      textareaRef.current?.focus()
    }
    wasDisabled.current = disabled
  }, [disabled])

  function submit() {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
    textareaRef.current?.focus()
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    submit()
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  return (
    <form className={styles.form} onSubmit={handleSubmit}>
      <label htmlFor="chat-message" className={styles.label}>
        Type a message
      </label>
      <textarea
        id="chat-message"
        ref={textareaRef}
        className={styles.textarea}
        value={value}
        maxLength={MAX_LENGTH}
        rows={2}
        placeholder="e.g. I'd like a cleaning next Tuesday afternoon"
        disabled={disabled}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
      />
      <button type="submit" className={styles.send} disabled={disabled || !value.trim()}>
        Send
      </button>
      <MicButton status={voice.status} disabled={disabled} onStart={voice.start} onStop={voice.stop} />
      <VoiceStatusLine status={voice.status} onRetry={voice.start} />
    </form>
  )
}
