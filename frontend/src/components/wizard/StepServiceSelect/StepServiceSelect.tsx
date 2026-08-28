import { useEffect, useState } from 'react'
import { getServices } from '../../../api/services'
import type { Service } from '../../../api/types'
import { useBookingSession } from '../../../hooks/useBookingSession'
import { useBookingState } from '../../../state/BookingContext'
import { ErrorBanner } from '../../shared/ErrorBanner'
import { LoadingSpinner } from '../../shared/LoadingSpinner'
import styles from './StepServiceSelect.module.css'

export function StepServiceSelect() {
  const state = useBookingState()
  const { ensureSession, selectService } = useBookingSession()
  const [services, setServices] = useState<Service[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoadError(null)
      try {
        await ensureSession()
        const list = await getServices()
        if (!cancelled) setServices(list)
      } catch {
        if (!cancelled) setLoadError('Unable to load services right now. Please try again.')
      }
    }

    void load()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (loadError) {
    return (
      <section aria-labelledby="step-heading">
        <h2 id="step-heading">Choose a service</h2>
        <ErrorBanner message={loadError} onRetry={() => window.location.reload()} />
      </section>
    )
  }

  if (!services) {
    return (
      <section aria-labelledby="step-heading">
        <h2 id="step-heading">Choose a service</h2>
        <LoadingSpinner label="Loading services…" />
      </section>
    )
  }

  return (
    <section aria-labelledby="step-heading">
      <h2 id="step-heading">Choose a service</h2>
      <p>Select the service you'd like to book. We'll show you which practitioners can perform it.</p>
      {state.status === 'error' && state.errorInfo && <ErrorBanner message={state.errorInfo.detail} />}
      <ul className={styles.list}>
        {services.map((service) => (
          <li key={service.id}>
            <button
              type="button"
              className={styles.card}
              disabled={state.status === 'loading'}
              onClick={() => void selectService(service.code)}
            >
              <span className={styles.cardTitle}>{service.displayName}</span>
              <span className={styles.cardMeta}>{service.durationMinutes} min</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}
