import { createContext, useContext, useReducer, type Dispatch, type ReactNode } from 'react'
import { bookingReducer, initialBookingState } from './bookingReducer'
import type { BookingAction, BookingState } from './types'

const BookingStateContext = createContext<BookingState | null>(null)
const BookingDispatchContext = createContext<Dispatch<BookingAction> | null>(null)

export function BookingProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(bookingReducer, initialBookingState)

  return (
    <BookingStateContext.Provider value={state}>
      <BookingDispatchContext.Provider value={dispatch}>{children}</BookingDispatchContext.Provider>
    </BookingStateContext.Provider>
  )
}

export function useBookingState(): BookingState {
  const context = useContext(BookingStateContext)
  if (!context) throw new Error('useBookingState must be used within a BookingProvider')
  return context
}

export function useBookingDispatch(): Dispatch<BookingAction> {
  const context = useContext(BookingDispatchContext)
  if (!context) throw new Error('useBookingDispatch must be used within a BookingProvider')
  return context
}
