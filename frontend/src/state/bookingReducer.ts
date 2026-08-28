import type { BookingAction, BookingState } from './types'

export const initialBookingState: BookingState = {
  step: 'service',
  sessionId: null,
  sessionVersion: null,
  serviceCode: null,
  selectedSlot: null,
  patientName: null,
  patientEmail: null,
  status: 'idle',
  errorInfo: null,
  appointment: null,
}

export function bookingReducer(state: BookingState, action: BookingAction): BookingState {
  switch (action.type) {
    case 'LOADING':
      return { ...state, status: 'loading', errorInfo: null }

    case 'SESSION_CREATED':
      return {
        ...state,
        sessionId: action.sessionId,
        sessionVersion: action.version,
        status: 'idle',
        errorInfo: null,
      }

    case 'SESSION_SYNCED':
      return { ...state, sessionVersion: action.version, status: 'idle', errorInfo: null }

    case 'SERVICE_SELECTED':
      return {
        ...state,
        serviceCode: action.serviceCode,
        sessionVersion: action.version,
        step: 'schedule',
        status: 'idle',
        errorInfo: null,
      }

    case 'SLOT_SELECTED':
      return {
        ...state,
        selectedSlot: action.slot,
        sessionVersion: action.version,
        step: 'patient',
        status: 'idle',
        errorInfo: null,
      }

    case 'PATIENT_INFO_SET':
      return {
        ...state,
        patientName: action.name,
        patientEmail: action.email,
        sessionVersion: action.version,
        step: 'review',
        status: 'idle',
        errorInfo: null,
      }

    case 'READY_TO_CONFIRM':
      return { ...state, sessionVersion: action.version, status: 'idle', errorInfo: null }

    case 'APPOINTMENT_CREATED':
      return { ...state, appointment: action.appointment, step: 'success', status: 'idle', errorInfo: null }

    case 'SLOT_TAKEN':
      return {
        ...state,
        selectedSlot: null,
        step: 'schedule',
        status: 'error',
        errorInfo: { code: 'SLOT_NO_LONGER_AVAILABLE', detail: action.detail },
      }

    case 'SESSION_EXPIRED':
      return {
        ...initialBookingState,
        status: 'error',
        errorInfo: { code: 'BOOKING_SESSION_EXPIRED', detail: action.detail },
      }

    case 'ERROR_OCCURRED':
      return { ...state, status: 'error', errorInfo: action.errorInfo }

    case 'GO_TO_STEP':
      return { ...state, step: action.step, status: 'idle', errorInfo: null }

    case 'RESET':
      return initialBookingState

    default:
      return state
  }
}
