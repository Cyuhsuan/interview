import { apiFetch, type ApiResult } from './client'
import type { AvailabilitySlot, BookingSession } from './types'

export interface ChatTurnResult extends BookingSession {
  reply: string
  offeredSlots: AvailabilitySlot[]
  outOfScope: boolean
}

export async function sendMessage(
  sessionId: string,
  message: string,
): Promise<ApiResult<ChatTurnResult>> {
  return apiFetch<ChatTurnResult>(`/booking-sessions/${encodeURIComponent(sessionId)}/messages`, {
    method: 'POST',
    body: JSON.stringify({ message }),
  })
}
