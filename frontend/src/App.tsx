import { useState } from 'react'
import { ChatShell } from './components/chat/ChatShell/ChatShell'
import { WizardShell } from './components/wizard/WizardShell'
import { StepPatientInfo } from './components/wizard/StepPatientInfo/StepPatientInfo'
import { StepReviewConfirm } from './components/wizard/StepReviewConfirm/StepReviewConfirm'
import { StepScheduleSelect } from './components/wizard/StepScheduleSelect/StepScheduleSelect'
import { StepServiceSelect } from './components/wizard/StepServiceSelect/StepServiceSelect'
import { StepSuccess } from './components/wizard/StepSuccess/StepSuccess'
import { BookingProvider } from './state/BookingContext'
import { useBookingState } from './state/BookingContext'
import styles from './App.module.css'

function WizardSteps() {
  const state = useBookingState()

  switch (state.step) {
    case 'service':
      return <StepServiceSelect />
    case 'schedule':
      return <StepScheduleSelect />
    case 'patient':
      return <StepPatientInfo />
    case 'review':
      return <StepReviewConfirm />
    case 'success':
      return <StepSuccess />
  }
}

type Mode = 'wizard' | 'chat'

function App() {
  const [mode, setMode] = useState<Mode>('wizard')

  return (
    <div className={styles.app}>
      <div className={styles.modeSwitcher} role="tablist" aria-label="Booking mode">
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'wizard'}
          className={mode === 'wizard' ? styles.modeButtonActive : styles.modeButton}
          onClick={() => setMode('wizard')}
        >
          Step-by-step form
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'chat'}
          className={mode === 'chat' ? styles.modeButtonActive : styles.modeButton}
          onClick={() => setMode('chat')}
        >
          Chat
        </button>
      </div>

      {mode === 'wizard' ? (
        <BookingProvider>
          <WizardShell>
            <WizardSteps />
          </WizardShell>
        </BookingProvider>
      ) : (
        <ChatShell />
      )}
    </div>
  )
}

export default App
