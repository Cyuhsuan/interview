import { WizardShell } from './components/wizard/WizardShell'
import { StepPatientInfo } from './components/wizard/StepPatientInfo/StepPatientInfo'
import { StepReviewConfirm } from './components/wizard/StepReviewConfirm/StepReviewConfirm'
import { StepScheduleSelect } from './components/wizard/StepScheduleSelect/StepScheduleSelect'
import { StepServiceSelect } from './components/wizard/StepServiceSelect/StepServiceSelect'
import { StepSuccess } from './components/wizard/StepSuccess/StepSuccess'
import { BookingProvider } from './state/BookingContext'
import { useBookingState } from './state/BookingContext'

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

function App() {
  return (
    <BookingProvider>
      <WizardShell>
        <WizardSteps />
      </WizardShell>
    </BookingProvider>
  )
}

export default App
