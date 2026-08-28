import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
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
    </form>
  )
}
