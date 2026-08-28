import type { Appointment, AvailabilitySlot, FieldError } from '../api/types'

export type WizardStep = 'service' | 'schedule' | 'patient' | 'review' | 'success'

export interface ErrorInfo {
  code: string
  detail: string
  fieldErrors?: FieldError[]
}

export interface BookingState {
  step: WizardStep
  sessionId: string | null
  sessionVersion: number | null
  serviceCode: string | null
  selectedSlot: AvailabilitySlot | null
  patientName: string | null
  patientEmail: string | null
  status: 'idle' | 'loading' | 'error'
  errorInfo: ErrorInfo | null
  appointment: Appointment | null
}

export type BookingAction =
  | { type: 'LOADING' }
  | { type: 'SESSION_CREATED'; sessionId: string; version: number }
  | { type: 'SESSION_SYNCED'; version: number }
  | { type: 'SERVICE_SELECTED'; serviceCode: string; version: number }
  | { type: 'SLOT_SELECTED'; slot: AvailabilitySlot; version: number }
  | { type: 'PATIENT_INFO_SET'; name: string; email: string; version: number }
  | { type: 'READY_TO_CONFIRM'; version: number }
  | { type: 'APPOINTMENT_CREATED'; appointment: Appointment }
  | { type: 'SLOT_TAKEN'; detail: string }
  | { type: 'SESSION_EXPIRED'; detail: string }
  | { type: 'ERROR_OCCURRED'; errorInfo: ErrorInfo }
  | { type: 'GO_TO_STEP'; step: WizardStep }
  | { type: 'RESET' }
