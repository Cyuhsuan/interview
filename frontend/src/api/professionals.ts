import { apiFetch } from './client'
import type { Professional } from './types'

export async function getProfessionals(serviceCode: string): Promise<Professional[]> {
  const { data } = await apiFetch<Professional[]>(
    `/professionals?serviceCode=${encodeURIComponent(serviceCode)}`,
  )
  return data
}
