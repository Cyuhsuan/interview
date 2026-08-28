import { useBookingDispatch, useBookingState } from '../../../state/BookingContext'
import { formatDate, formatTimeRange } from '../../../utils/formatDateTime'
import styles from './StepSuccess.module.css'

export function StepSuccess() {
  const state = useBookingState()
  const dispatch = useBookingDispatch()
  const appointment = state.appointment

  if (!appointment) return null

  return (
    <section aria-labelledby="step-heading">
      <div className={styles.banner} role="status">
        <h2 id="step-heading">Appointment confirmed</h2>
        <p>
          A confirmation has been sent to <strong>{appointment.patientEmail}</strong>. Please arrive a
          few minutes before your appointment time.
        </p>
      </div>

      <dl className={styles.summary}>
        <li>
          <dt>Reference ID</dt>
          <dd>{appointment.id}</dd>
        </li>
        <li>
          <dt>Service</dt>
          <dd>{appointment.serviceCode}</dd>
        </li>
        <li>
          <dt>Date</dt>
          <dd>{formatDate(appointment.start, appointment.timeZone)}</dd>
        </li>
        <li>
          <dt>Time</dt>
          <dd>{formatTimeRange(appointment.start, appointment.end, appointment.timeZone)}</dd>
        </li>
        <li>
          <dt>Time zone</dt>
          <dd>{appointment.timeZone}</dd>
        </li>
        <li>
          <dt>Name</dt>
          <dd>{appointment.patientName}</dd>
        </li>
      </dl>

      <p>
        Need to cancel or reschedule? Please contact the clinic directly — this cannot be done through
        this chat.
      </p>

      <button type="button" className={styles.newBooking} onClick={() => dispatch({ type: 'RESET' })}>
        Book another appointment
      </button>
    </section>
  )
}
