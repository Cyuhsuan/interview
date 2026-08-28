import { apiFetch } from './client'
import type { AvailabilitySlot } from './types'

export async function getAvailability(serviceCode: string, date: string): Promise<AvailabilitySlot[]> {
  const { data } = await apiFetch<AvailabilitySlot[]>(
    `/availability?serviceCode=${encodeURIComponent(serviceCode)}&date=${encodeURIComponent(date)}`,
  )
  return data
}
