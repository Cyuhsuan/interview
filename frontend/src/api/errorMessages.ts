// Copy shared by useBookingSession and useChatSession for the ApiError
// codes where both flows show the patient the exact same wording.
// (booking wizard 與 chat 共用的錯誤文案，避免同一句話在兩處各自維護、逐漸走樣。)
export const SLOT_TAKEN_MESSAGE =
  'That time slot was just booked by someone else. Please choose another.'

export const GENERIC_ERROR_MESSAGE = 'Something unexpected happened. Please try again.'
