import { apiFetch, type ApiResult } from './client'
import type { AvailabilitySlot, BookingSession, BookingSessionStatus } from './types'

export async function createBookingSession(): Promise<ApiResult<BookingSession>> {
  return apiFetch<BookingSession>('/booking-sessions', {
    method: 'POST',
    body: JSON.stringify({}),
  })
}

export async function getBookingSession(id: string): Promise<ApiResult<BookingSession>> {
  return apiFetch<BookingSession>(`/booking-sessions/${encodeURIComponent(id)}`)
}

export interface BookingSessionPatch {
  serviceCode?: string
  selectedSlot?: AvailabilitySlot
  patientName?: string
  patientEmail?: string
  status?: BookingSessionStatus
}

export async function patchBookingSession(
  id: string,
  version: number,
  patch: BookingSessionPatch,
): Promise<ApiResult<BookingSession>> {
  return apiFetch<BookingSession>(`/booking-sessions/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'If-Match': String(version) },
    body: JSON.stringify(patch),
  })
}
