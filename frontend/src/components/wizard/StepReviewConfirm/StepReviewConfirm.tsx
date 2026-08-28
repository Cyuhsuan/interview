import { useEffect, useState } from 'react'
import { getProfessionals } from '../../../api/professionals'
import { getServices } from '../../../api/services'
import type { Professional, Service } from '../../../api/types'
import { useOnlineStatus } from '../../../hooks/useOnlineStatus'
import { useBookingSession } from '../../../hooks/useBookingSession'
import { useBookingDispatch, useBookingState } from '../../../state/BookingContext'
import { formatDate, formatTimeRange } from '../../../utils/formatDateTime'
import { ErrorBanner } from '../../shared/ErrorBanner'
import styles from './StepReviewConfirm.module.css'

export function StepReviewConfirm() {
  const state = useBookingState()
  const dispatch = useBookingDispatch()
  const { confirmBooking } = useBookingSession()
  const isOnline = useOnlineStatus()
  const [services, setServices] = useState<Service[] | null>(null)
  const [professionals, setProfessionals] = useState<Professional[] | null>(null)

  useEffect(() => {
    if (!state.serviceCode) return
    getServices()
      .then(setServices)
      .catch(() => setServices([]))
    getProfessionals(state.serviceCode)
      .then(setProfessionals)
      .catch(() => setProfessionals([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.serviceCode])

  const service = services?.find((s) => s.code === state.serviceCode)
  const professional = professionals?.find((p) => p.id === state.selectedSlot?.professionalId)

  async function handleConfirm() {
    try {
      await confirmBooking()
    } catch {
      // error already reflected in booking state by useBookingSession
    }
  }

  if (!state.selectedSlot || !state.serviceCode) {
    return (
      <section aria-labelledby="step-heading">
        <h2 id="step-heading">Review your appointment</h2>
        <ErrorBanner message="Some booking details are missing. Please start again." />
      </section>
    )
  }

  return (
    <section aria-labelledby="step-heading">
      <h2 id="step-heading">Review your appointment</h2>
      <p>Please confirm these details are correct before booking.</p>

      {state.status === 'error' && state.errorInfo && <ErrorBanner message={state.errorInfo.detail} />}

      <dl className={styles.summary}>
        <li>
          <dt>Service</dt>
          <dd>{service?.displayName ?? state.serviceCode}</dd>
        </li>
        <li>
          <dt>Duration</dt>
          <dd>{service ? `${service.durationMinutes} min` : '—'}</dd>
        </li>
        <li>
          <dt>Practitioner</dt>
          <dd>{professional?.displayName ?? 'Assigned practitioner'}</dd>
        </li>
        <li>
          <dt>Date</dt>
          <dd>{formatDate(state.selectedSlot.start, state.selectedSlot.timeZone)}</dd>
        </li>
        <li>
          <dt>Time</dt>
          <dd>{formatTimeRange(state.selectedSlot.start, state.selectedSlot.end, state.selectedSlot.timeZone)}</dd>
        </li>
        <li>
          <dt>Time zone</dt>
          <dd>{state.selectedSlot.timeZone}</dd>
        </li>
        <li>
          <dt>Email</dt>
          <dd>{state.patientEmail}</dd>
        </li>
      </dl>

      <div className={styles.actions}>
        <button
          type="button"
          className={styles.back}
          disabled={state.status === 'loading'}
          onClick={() => dispatch({ type: 'GO_TO_STEP', step: 'patient' })}
        >
          Back
        </button>
        <button
          type="button"
          className={styles.confirm}
          disabled={state.status === 'loading' || !isOnline}
          onClick={() => void handleConfirm()}
        >
          {state.status === 'loading' ? 'Confirming…' : 'Confirm appointment'}
        </button>
      </div>
    </section>
  )
}
