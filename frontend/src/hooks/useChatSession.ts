import { useCallback, useRef, useState } from 'react'
import { createAppointment } from '../api/appointments'
import { createBookingSession, patchBookingSession } from '../api/bookingSessions'
import { ApiError, NetworkError } from '../api/client'
import { sendMessage } from '../api/conversation'
import type { Appointment, AvailabilitySlot, BookingSession } from '../api/types'
import { generateIdempotencyKey } from '../utils/idempotencyKey'

export interface ChatMessage {
  id: string
  role: 'patient' | 'bot'
  text: string
  offeredSlots?: AvailabilitySlot[]
  outOfScope?: boolean
}

interface ChatState {
  sessionId: string | null
  version: number | null
  session: BookingSession | null
  messages: ChatMessage[]
  isSending: boolean
  errorMessage: string | null
  appointment: Appointment | null
}

function welcomeMessage(): ChatMessage {
  return {
    id: 'welcome',
    role: 'bot',
    text: "Hi! I can help you book a dental appointment. What service would you like, and when works for you? You can write in your own words.",
  }
}

function describeError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'BOOKING_SESSION_EXPIRED') {
      return 'This chat session has expired. Please refresh the page to start a new conversation.'
    }
    if (err.code === 'SLOT_NO_LONGER_AVAILABLE') {
      return 'That time slot was just booked by someone else. Please choose another.'
    }
    if (err.code === 'AVAILABILITY_UNAVAILABLE') {
      return "We can't confirm available times right now. Please contact the clinic directly to book."
    }
    return err.message || 'Something went wrong. Please try again.'
  }
  if (err instanceof NetworkError) {
    return err.message
  }
  return 'Something unexpected happened. Please try again.'
}

export function useChatSession() {
  const [state, setState] = useState<ChatState>({
    sessionId: null,
    version: null,
    session: null,
    messages: [welcomeMessage()],
    isSending: false,
    errorMessage: null,
    appointment: null,
  })
  const idRef = useRef(0)
  const nextId = () => {
    idRef.current += 1
    return `m${idRef.current}`
  }

  const ensureSession = useCallback(async (): Promise<{ id: string; version: number }> => {
    if (state.sessionId && state.version !== null) {
      return { id: state.sessionId, version: state.version }
    }
    const { data, etag } = await createBookingSession()
    const version = etag ?? 1
    setState((s) => ({ ...s, sessionId: data.id, version, session: data }))
    return { id: data.id, version }
  }, [state.sessionId, state.version])

  const send = useCallback(
    async (text: string) => {
      const trimmed = text.trim()
      if (!trimmed) return
      setState((s) => ({
        ...s,
        isSending: true,
        errorMessage: null,
        messages: [...s.messages, { id: nextId(), role: 'patient', text: trimmed }],
      }))
      try {
        const { id } = await ensureSession()
        const { data, etag } = await sendMessage(id, trimmed)
        setState((s) => ({
          ...s,
          isSending: false,
          version: etag ?? s.version,
          session: data,
          messages: [
            ...s.messages,
            {
              id: nextId(),
              role: 'bot',
              text: data.reply,
              offeredSlots: data.offeredSlots,
              outOfScope: data.outOfScope,
            },
          ],
        }))
      } catch (err) {
        setState((s) => ({ ...s, isSending: false, errorMessage: describeError(err) }))
      }
    },
    [ensureSession],
  )

  const pickSlot = useCallback(
    async (slot: AvailabilitySlot) => {
      if (!state.sessionId || state.version === null) return
      setState((s) => ({ ...s, isSending: true, errorMessage: null }))
      try {
        const { data, etag } = await patchBookingSession(state.sessionId, state.version, {
          selectedSlot: slot,
        })
        setState((s) => ({
          ...s,
          isSending: false,
          version: etag ?? s.version,
          session: data,
          messages: [
            ...s.messages,
            {
              id: nextId(),
              role: 'bot',
              text: 'Got it — that time is selected. What name should I put on the appointment?',
            },
          ],
        }))
      } catch (err) {
        setState((s) => ({ ...s, isSending: false, errorMessage: describeError(err) }))
      }
    },
    [state.sessionId, state.version],
  )

  const confirm = useCallback(async () => {
    if (!state.sessionId || state.version === null) return
    setState((s) => ({ ...s, isSending: true, errorMessage: null }))
    try {
      const { data: readyData, etag: readyEtag } = await patchBookingSession(
        state.sessionId,
        state.version,
        { status: 'readyToConfirm' },
      )
      const version = readyEtag ?? state.version
      const idempotencyKey = generateIdempotencyKey()
      const appointment = await createAppointment(state.sessionId, version, idempotencyKey)
      setState((s) => ({ ...s, isSending: false, session: readyData, appointment }))
    } catch (err) {
      setState((s) => ({ ...s, isSending: false, errorMessage: describeError(err) }))
    }
  }, [state.sessionId, state.version])

  const session = state.session
  const readyToConfirm = Boolean(
    session &&
      session.status !== 'confirmed' &&
      session.serviceCode &&
      session.selectedSlot &&
      session.patientName &&
      session.patientEmail,
  )

  return { state, send, pickSlot, confirm, readyToConfirm }
}
