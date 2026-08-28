import type { AvailabilitySlot, Professional } from '../../../api/types'
import { formatDate, formatTimeRange } from '../../../utils/formatDateTime'
import styles from './SlotOptionButtons.module.css'

function professionalName(professionals: Professional[] | null, id: string): string {
  return professionals?.find((p) => p.id === id)?.displayName ?? 'Available practitioner'
}

export function SlotOptionButtons({
  slots,
  professionals,
  disabled,
  onPick,
}: {
  slots: AvailabilitySlot[]
  professionals: Professional[] | null
  disabled: boolean
  onPick: (slot: AvailabilitySlot) => void
}) {
  if (slots.length === 0) return null

  return (
    <ul className={styles.list}>
      {slots.map((slot) => (
        <li key={`${slot.professionalId}-${slot.start}`}>
          <button
            type="button"
            className={styles.option}
            disabled={disabled}
            onClick={() => onPick(slot)}
          >
            <span className={styles.date}>{formatDate(slot.start, slot.timeZone)}</span>
            <span className={styles.time}>{formatTimeRange(slot.start, slot.end, slot.timeZone)}</span>
            <span className={styles.professional}>{professionalName(professionals, slot.professionalId)}</span>
          </button>
        </li>
      ))}
    </ul>
  )
}
