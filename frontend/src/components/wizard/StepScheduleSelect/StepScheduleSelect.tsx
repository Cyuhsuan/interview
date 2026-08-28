import { useEffect, useState } from 'react'
import { getAvailability } from '../../../api/availability'
import { ApiError } from '../../../api/client'
import { getProfessionals } from '../../../api/professionals'
import type { AvailabilitySlot, Professional } from '../../../api/types'
import { useBookingSession } from '../../../hooks/useBookingSession'
import { useBookingState } from '../../../state/BookingContext'
import { formatTimeRange } from '../../../utils/formatDateTime'
import { EmptyState } from '../../shared/EmptyState'
import { ErrorBanner } from '../../shared/ErrorBanner'
import { LoadingSpinner } from '../../shared/LoadingSpinner'
import styles from './StepScheduleSelect.module.css'

function todayIsoDate(): string {
  return new Date().toISOString().slice(0, 10)
}

export function StepScheduleSelect() {
  const state = useBookingState()
  const { selectSlot } = useBookingSession()
  const serviceCode = state.serviceCode!

  const [professionals, setProfessionals] = useState<Professional[] | null>(null)
  const [date, setDate] = useState('')
  const [slots, setSlots] = useState<AvailabilitySlot[] | null>(null)
  const [availabilityError, setAvailabilityError] = useState<string | null>(null)
  const [loadingSlots, setLoadingSlots] = useState(false)
  const [professionalFilter, setProfessionalFilter] = useState<string>('any')

  useEffect(() => {
    setProfessionalFilter('any')
    getProfessionals(serviceCode)
      .then(setProfessionals)
      .catch(() => setProfessionals([]))
  }, [serviceCode])

  useEffect(() => {
    if (!date) {
      setSlots(null)
      return
    }
    let cancelled = false
    setLoadingSlots(true)
    setAvailabilityError(null)
    getAvailability(serviceCode, date)
      .then((result) => {
        if (!cancelled) setSlots(result)
      })
      .catch((err) => {
        if (cancelled) return
        if (err instanceof ApiError && err.status === 503) {
          setAvailabilityError(
            'We can\'t confirm available times right now. Please contact the clinic directly to book.',
          )
        } else {
          setAvailabilityError('Unable to load available times. Please try again.')
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingSlots(false)
      })
    return () => {
      cancelled = true
    }
  }, [serviceCode, date])

  function professionalName(id: string): string {
    return professionals?.find((p) => p.id === id)?.displayName ?? 'Available practitioner'
  }

  const visibleSlots =
    professionalFilter === 'any'
      ? slots
      : (slots?.filter((slot) => slot.professionalId === professionalFilter) ?? null)

  return (
    <section aria-labelledby="step-heading">
      <h2 id="step-heading">Choose a date and time</h2>

      {professionals && professionals.length > 0 && (
        <p className={styles.eligibility}>
          This service can be performed by: {professionals.map((p) => p.displayName).join(', ')}.
        </p>
      )}

      {professionals && professionals.length > 1 && (
        <fieldset className={styles.practitionerFilter}>
          <legend>Practitioner</legend>
          <label>
            <input
              type="radio"
              name="practitioner-filter"
              value="any"
              checked={professionalFilter === 'any'}
              onChange={() => setProfessionalFilter('any')}
            />
            Any practitioner
          </label>
          {professionals.map((p) => (
            <label key={p.id}>
              <input
                type="radio"
                name="practitioner-filter"
                value={p.id}
                checked={professionalFilter === p.id}
                onChange={() => setProfessionalFilter(p.id)}
              />
              {p.displayName}
            </label>
          ))}
        </fieldset>
      )}

      <div className={styles.datePicker}>
        <label htmlFor="booking-date">Date</label>
        <input
          id="booking-date"
          type="date"
          min={todayIsoDate()}
          value={date}
          onChange={(e) => setDate(e.target.value)}
        />
      </div>

      {state.status === 'error' && state.errorInfo && <ErrorBanner message={state.errorInfo.detail} />}

      {loadingSlots && <LoadingSpinner label="Checking availability…" />}

      {availabilityError && <ErrorBanner message={availabilityError} onRetry={() => setDate(date)} />}

      {!loadingSlots && !availabilityError && date && slots && slots.length === 0 && (
        <EmptyState message="No available times on this date. Try another date." />
      )}

      {!loadingSlots && date && slots && slots.length > 0 && visibleSlots && visibleSlots.length === 0 && (
        <EmptyState message="This practitioner has no available times on this date. Try another date or practitioner." />
      )}

      {!loadingSlots && visibleSlots && visibleSlots.length > 0 && (
        <ul className={styles.slotList}>
          {visibleSlots.map((slot) => (
            <li key={`${slot.professionalId}-${slot.start}`}>
              <button
                type="button"
                className={styles.slotButton}
                disabled={state.status === 'loading'}
                onClick={() => void selectSlot(slot)}
              >
                <span className={styles.slotTime}>{formatTimeRange(slot.start, slot.end, slot.timeZone)}</span>
                <span className={styles.slotProfessional}>{professionalName(slot.professionalId)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
