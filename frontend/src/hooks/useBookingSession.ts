import { ApiError, NetworkError } from '../api/client'
import { createAppointment } from '../api/appointments'
import {
  createBookingSession,
  patchBookingSession,
  type BookingSessionPatch,
} from '../api/bookingSessions'
import type { Appointment, AvailabilitySlot } from '../api/types'
import { useBookingDispatch, useBookingState } from '../state/BookingContext'
import type { BookingAction } from '../state/types'
import { generateIdempotencyKey } from '../utils/idempotencyKey'
import type { Dispatch } from 'react'

function dispatchApiError(err: unknown, dispatch: Dispatch<BookingAction>): void {
  if (err instanceof ApiError) {
    if (err.code === 'BOOKING_SESSION_EXPIRED') {
      dispatch({ type: 'SESSION_EXPIRED', detail: err.message })
      return
    }
    if (err.code === 'SESSION_VERSION_MISMATCH' || err.code === 'PRECONDITION_REQUIRED') {
      dispatch({
        type: 'SESSION_EXPIRED',
        detail: 'Your session changed elsewhere. Please start the booking again.',
      })
      return
    }
    if (err.code === 'SLOT_NO_LONGER_AVAILABLE') {
      dispatch({
        type: 'SLOT_TAKEN',
        detail: 'That time slot was just booked by someone else. Please choose another.',
      })
      return
    }
    dispatch({
      type: 'ERROR_OCCURRED',
      errorInfo: { code: err.code, detail: err.message, fieldErrors: err.errors },
    })
    return
  }

  if (err instanceof NetworkError) {
    dispatch({ type: 'ERROR_OCCURRED', errorInfo: { code: 'NETWORK', detail: err.message } })
    return
  }

  dispatch({
    type: 'ERROR_OCCURRED',
    errorInfo: { code: 'INTERNAL_ERROR', detail: 'Something unexpected happened. Please try again.' },
  })
}

export function useBookingSession() {
  const state = useBookingState()
  const dispatch = useBookingDispatch()

  async function ensureSession(): Promise<{ id: string; version: number }> {
    if (state.sessionId && state.sessionVersion !== null) {
      return { id: state.sessionId, version: state.sessionVersion }
    }
    dispatch({ type: 'LOADING' })
    try {
      const { data, etag } = await createBookingSession()
      const version = etag ?? 1
      dispatch({ type: 'SESSION_CREATED', sessionId: data.id, version })
      return { id: data.id, version }
    } catch (err) {
      dispatchApiError(err, dispatch)
      throw err
    }
  }

  async function selectService(serviceCode: string): Promise<void> {
    const session = await ensureSession()
    dispatch({ type: 'LOADING' })
    try {
      const { etag } = await patchBookingSession(session.id, session.version, { serviceCode })
      dispatch({ type: 'SERVICE_SELECTED', serviceCode, version: etag ?? session.version })
    } catch (err) {
      dispatchApiError(err, dispatch)
      throw err
    }
  }

  async function selectSlot(slot: AvailabilitySlot): Promise<void> {
    if (!state.sessionId || state.sessionVersion === null) throw new Error('No active session')
    dispatch({ type: 'LOADING' })
    try {
      const { etag } = await patchBookingSession(state.sessionId, state.sessionVersion, {
        selectedSlot: slot,
      })
      dispatch({ type: 'SLOT_SELECTED', slot, version: etag ?? state.sessionVersion })
    } catch (err) {
      dispatchApiError(err, dispatch)
      throw err
    }
  }

  async function setPatientInfo(name: string, email: string): Promise<void> {
    if (!state.sessionId || state.sessionVersion === null) throw new Error('No active session')
    dispatch({ type: 'LOADING' })
    try {
      const { etag } = await patchBookingSession(state.sessionId, state.sessionVersion, {
        patientName: name,
        patientEmail: email,
      })
      dispatch({ type: 'PATIENT_INFO_SET', name, email, version: etag ?? state.sessionVersion })
    } catch (err) {
      dispatchApiError(err, dispatch)
      throw err
    }
  }

  async function confirmBooking(): Promise<Appointment> {
    if (!state.sessionId || state.sessionVersion === null) throw new Error('No active session')
    dispatch({ type: 'LOADING' })
    try {
      const readyPatch: BookingSessionPatch = { status: 'readyToConfirm' }
      const { etag: readyVersion } = await patchBookingSession(
        state.sessionId,
        state.sessionVersion,
        readyPatch,
      )
      const version = readyVersion ?? state.sessionVersion
      dispatch({ type: 'READY_TO_CONFIRM', version })

      const idempotencyKey = generateIdempotencyKey()
      const appointment = await createAppointment(state.sessionId, version, idempotencyKey)
      dispatch({ type: 'APPOINTMENT_CREATED', appointment })
      return appointment
    } catch (err) {
      dispatchApiError(err, dispatch)
      throw err
    }
  }

  return { ensureSession, selectService, selectSlot, setPatientInfo, confirmBooking }
}
