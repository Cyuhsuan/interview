import { useState, type FormEvent } from 'react'
import { useBookingSession } from '../../../hooks/useBookingSession'
import { useBookingState } from '../../../state/BookingContext'
import { isValidEmail, isValidName } from '../../../utils/validation'
import { ErrorBanner } from '../../shared/ErrorBanner'
import { FormField } from '../../shared/FormField'
import styles from './StepPatientInfo.module.css'

export function StepPatientInfo() {
  const state = useBookingState()
  const { setPatientInfo } = useBookingSession()
  const [name, setName] = useState(state.patientName ?? '')
  const [email, setEmail] = useState(state.patientEmail ?? '')
  const [nameError, setNameError] = useState<string | null>(null)
  const [emailError, setEmailError] = useState<string | null>(null)

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const nameValid = isValidName(name)
    const emailValid = isValidEmail(email)
    setNameError(nameValid ? null : 'Enter your full name.')
    setEmailError(emailValid ? null : 'Enter a valid email address.')
    if (!nameValid || !emailValid) return
    void setPatientInfo(name.trim(), email.trim())
  }

  return (
    <section aria-labelledby="step-heading">
      <h2 id="step-heading">Your contact details</h2>
      <p>We'll send your appointment confirmation to this email address.</p>

      {state.status === 'error' && state.errorInfo && <ErrorBanner message={state.errorInfo.detail} />}

      <form className={styles.form} onSubmit={handleSubmit} noValidate>
        <FormField
          id="patient-name"
          label="Full name"
          type="text"
          autoComplete="name"
          value={name}
          error={nameError ?? undefined}
          onChange={(e) => setName(e.target.value)}
        />
        <FormField
          id="patient-email"
          label="Email address"
          type="email"
          autoComplete="email"
          value={email}
          error={emailError ?? undefined}
          onChange={(e) => setEmail(e.target.value)}
        />
        <button type="submit" className={styles.submit} disabled={state.status === 'loading'}>
          Continue
        </button>
      </form>
    </section>
  )
}
