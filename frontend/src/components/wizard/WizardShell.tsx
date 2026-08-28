import type { ReactNode } from 'react'
import { useOnlineStatus } from '../../hooks/useOnlineStatus'
import { useBookingState } from '../../state/BookingContext'
import type { WizardStep } from '../../state/types'
import { LiveRegion } from '../shared/LiveRegion'
import styles from './WizardShell.module.css'

const STEPS: { key: WizardStep; label: string }[] = [
  { key: 'service', label: 'Service' },
  { key: 'schedule', label: 'Date & Time' },
  { key: 'patient', label: 'Your Info' },
  { key: 'review', label: 'Review' },
  { key: 'success', label: 'Confirmed' },
]

export function WizardShell({ children }: { children: ReactNode }) {
  const state = useBookingState()
  const isOnline = useOnlineStatus()
  const currentIndex = STEPS.findIndex((s) => s.key === state.step)

  const liveMessage =
    state.status === 'loading'
      ? 'Loading…'
      : state.status === 'error' && state.errorInfo
        ? state.errorInfo.detail
        : ''

  return (
    <div className={styles.shell}>
      <header>
        <h1>Book an Appointment</h1>
        <ol className={styles.steps}>
          {STEPS.map((s, i) => (
            <li
              key={s.key}
              aria-current={s.key === state.step ? 'step' : undefined}
              className={i < currentIndex ? styles.done : undefined}
            >
              {s.label}
            </li>
          ))}
        </ol>
      </header>

      {!isOnline && (
        <div className={styles.offlineBanner} role="status">
          You appear to be offline. Reconnect to continue booking.
        </div>
      )}

      <LiveRegion message={liveMessage} assertive={state.status === 'error'} />

      <main className={styles.section}>{children}</main>
    </div>
  )
}

export { STEPS }
