import { apiFetch } from './client'
import type { Service } from './types'

export async function getServices(): Promise<Service[]> {
  const { data } = await apiFetch<Service[]>('/services')
  return data
}
