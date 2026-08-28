import type { AvailabilitySlot, Professional } from '../../../api/types'
import type { ChatMessage } from '../../../hooks/useChatSession'
import { SlotOptionButtons } from '../SlotOptionButtons/SlotOptionButtons'
import styles from './MessageBubble.module.css'

export function MessageBubble({
  message,
  professionals,
  professionalFilter,
  disabled,
  onPickSlot,
}: {
  message: ChatMessage
  professionals: Professional[] | null
  professionalFilter: string
  disabled: boolean
  onPickSlot: (slot: AvailabilitySlot) => void
}) {
  const isBot = message.role === 'bot'
  const bubbleClass = [
    isBot ? styles.botBubble : styles.patientBubble,
    isBot && message.outOfScope ? styles.outOfScope : '',
  ]
    .filter(Boolean)
    .join(' ')

  const offeredSlots =
    message.offeredSlots && professionalFilter !== 'any'
      ? message.offeredSlots.filter((slot) => slot.professionalId === professionalFilter)
      : message.offeredSlots

  return (
    <li className={isBot ? styles.botRow : styles.patientRow}>
      <div className={bubbleClass}>
        <span className={styles.senderLabel}>{isBot ? 'Clinic assistant' : 'You'}</span>
        <p>{message.text}</p>
        {offeredSlots && offeredSlots.length > 0 && (
          <SlotOptionButtons
            slots={offeredSlots}
            professionals={professionals}
            disabled={disabled}
            onPick={onPickSlot}
          />
        )}
        {message.offeredSlots && message.offeredSlots.length > 0 && offeredSlots?.length === 0 && (
          <p className={styles.noMatch}>No times for the selected practitioner. Try "Any practitioner".</p>
        )}
      </div>
    </li>
  )
}
