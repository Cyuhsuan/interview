export interface Service {
  id: string
  code: string
  displayName: string
  durationMinutes: number
}

export interface Professional {
  id: string
  code: string
  displayName: string
}

export interface AvailabilitySlot {
  professionalId: string
  start: string
  end: string
  timeZone: string
}

export type BookingSessionStatus = 'collecting' | 'readyToConfirm' | 'confirmed' | 'expired'

export interface BookingSession {
  id: string
  status: BookingSessionStatus
  serviceCode: string | null
  selectedSlot: AvailabilitySlot | null
  patientName: string | null
  patientEmail: string | null
  expiresAt: string
}

export interface Appointment {
  id: string
  serviceCode: string
  professionalId: string
  patientName: string
  patientEmail: string
  start: string
  end: string
  timeZone: string
}

export interface FieldError {
  field: string
  code: string
}

export interface ProblemDetails {
  type: string
  title: string
  status: number
  code: string
  detail: string
  instance: string
  errors?: FieldError[]
}
