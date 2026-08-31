import { useEffect, useRef, useState } from 'react'
import { getProfessionals } from '../../../api/professionals'
import type { Professional } from '../../../api/types'
import { useChatSession } from '../../../hooks/useChatSession'
import { useVoiceOutput } from '../../../hooks/useVoiceOutput'
import { formatDate, formatTimeRange } from '../../../utils/formatDateTime'
import { ErrorBanner } from '../../shared/ErrorBanner'
import { LiveRegion } from '../../shared/LiveRegion'
import { LoadingSpinner } from '../../shared/LoadingSpinner'
import { ChatInput } from '../ChatInput/ChatInput'
import { MessageList } from '../MessageList/MessageList'
import { SpeakRepliesToggle } from '../VoiceControls/SpeakRepliesToggle'
import styles from './ChatShell.module.css'

export function ChatShell() {
  const { state, send, pickSlot, confirm, readyToConfirm } = useChatSession()
  const listEndRef = useRef<HTMLDivElement>(null)
  const [professionals, setProfessionals] = useState<Professional[] | null>(null)
  const [professionalFilter, setProfessionalFilter] = useState('any')
  const voiceOutput = useVoiceOutput()
  const lastSpokenIdRef = useRef<string | null>('welcome')

  const serviceCode = state.session?.serviceCode ?? null

  useEffect(() => {
    setProfessionalFilter('any')
    if (!serviceCode) {
      setProfessionals(null)
      return
    }
    let cancelled = false
    getProfessionals(serviceCode)
      .then((list) => {
        if (!cancelled) setProfessionals(list)
      })
      .catch(() => {
        if (!cancelled) setProfessionals([])
      })
    return () => {
      cancelled = true
    }
  }, [serviceCode])

  useEffect(() => {
    listEndRef.current?.scrollIntoView({ block: 'end' })
  }, [state.messages.length])

  const lastBotMessage = [...state.messages].reverse().find((m) => m.role === 'bot')

  useEffect(() => {
    if (lastBotMessage && lastBotMessage.id !== lastSpokenIdRef.current) {
      lastSpokenIdRef.current = lastBotMessage.id
      voiceOutput.speak(lastBotMessage.text)
    }
  }, [lastBotMessage, voiceOutput])
  const appointment = state.appointment

  if (appointment) {
    return (
      <section aria-labelledby="chat-heading" className={styles.shell}>
        <div className={styles.confirmedBanner} role="status">
          <h2 id="chat-heading">Appointment confirmed</h2>
          <p>
            A confirmation has been sent to <strong>{appointment.patientEmail}</strong>.
          </p>
        </div>
        <dl className={styles.summary}>
          <div>
            <dt>Service</dt>
            <dd>{appointment.serviceCode}</dd>
          </div>
          <div>
            <dt>Practitioner</dt>
            <dd>
              {professionals?.find((p) => p.id === appointment.professionalId)?.displayName ??
                appointment.professionalId}
            </dd>
          </div>
          <div>
            <dt>Date</dt>
            <dd>{formatDate(appointment.start, appointment.timeZone)}</dd>
          </div>
          <div>
            <dt>Time</dt>
            <dd>{formatTimeRange(appointment.start, appointment.end, appointment.timeZone)}</dd>
          </div>
        </dl>
        <p>Need to cancel or reschedule? Please contact the clinic directly — this cannot be done through this chat.</p>
      </section>
    )
  }

  return (
    <section aria-labelledby="chat-heading" className={styles.shell}>
      <h2 id="chat-heading">Chat with the clinic assistant</h2>
      <p className={styles.hint}>
        This assistant can help you book an appointment. It can't diagnose, prescribe, quote prices, or
        handle insurance, cancellations, or reschedules — for those, please contact the clinic directly.
      </p>

      <SpeakRepliesToggle
        supported={voiceOutput.supported}
        enabled={voiceOutput.enabled}
        onChange={voiceOutput.setEnabled}
      />

      {professionals && professionals.length > 1 && (
        <fieldset className={styles.practitionerFilter}>
          <legend>Preferred practitioner</legend>
          <label>
            <input
              type="radio"
              name="chat-practitioner-filter"
              value="any"
              checked={professionalFilter === 'any'}
              onChange={() => setProfessionalFilter('any')}
            />
            Any practitioner
          </label>
          {professionals.map((p) => (
            <label key={p.id}>
              <input
                type="radio"
                name="chat-practitioner-filter"
                value={p.id}
                checked={professionalFilter === p.id}
                onChange={() => setProfessionalFilter(p.id)}
              />
              {p.displayName}
            </label>
          ))}
        </fieldset>
      )}

      <div className={styles.conversation}>
        <MessageList
          messages={state.messages}
          professionals={professionals}
          professionalFilter={professionalFilter}
          disabled={state.isSending}
          onPickSlot={(slot) => void pickSlot(slot)}
        />
        <div ref={listEndRef} />
      </div>

      {lastBotMessage && <LiveRegion message={lastBotMessage.text} />}

      {state.isSending && <LoadingSpinner label="Thinking…" />}

      {state.errorMessage && <ErrorBanner message={state.errorMessage} />}

      {readyToConfirm && (
        <button
          type="button"
          className={styles.confirmButton}
          disabled={state.isSending}
          onClick={() => void confirm()}
        >
          Confirm booking
        </button>
      )}

      <ChatInput disabled={state.isSending} onSend={(text) => void send(text)} />
    </section>
  )
}
