import { apiFetch } from './client'
import type { Appointment } from './types'

export async function createAppointment(
  bookingSessionId: string,
  version: number,
  idempotencyKey: string,
): Promise<Appointment> {
  const { data } = await apiFetch<Appointment>('/appointments', {
    method: 'POST',
    headers: {
      'If-Match': String(version),
      'Idempotency-Key': idempotencyKey,
    },
    body: JSON.stringify({ bookingSessionId }),
  })
  return data
}
