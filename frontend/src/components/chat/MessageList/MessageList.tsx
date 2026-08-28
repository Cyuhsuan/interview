import type { AvailabilitySlot, Professional } from '../../../api/types'
import type { ChatMessage } from '../../../hooks/useChatSession'
import { MessageBubble } from '../MessageBubble/MessageBubble'
import styles from './MessageList.module.css'

export function MessageList({
  messages,
  professionals,
  professionalFilter,
  disabled,
  onPickSlot,
}: {
  messages: ChatMessage[]
  professionals: Professional[] | null
  professionalFilter: string
  disabled: boolean
  onPickSlot: (slot: AvailabilitySlot) => void
}) {
  return (
    <ul className={styles.list} aria-label="Conversation">
      {messages.map((message) => (
        <MessageBubble
          key={message.id}
          message={message}
          professionals={professionals}
          professionalFilter={professionalFilter}
          disabled={disabled}
          onPickSlot={onPickSlot}
        />
      ))}
    </ul>
  )
}
