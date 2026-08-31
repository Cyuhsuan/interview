# Frontend Working Conventions

This document applies to every file under `frontend/`.

## Implementation Contract

- Use React and TypeScript.
- Patient-facing copy must be in plain English.
- The text-based booking flow is the primary path and must work fully even without microphone permission or voice support.
- Voice controls must clearly show listening, stopped, unsupported, permission-denied, and retry states.
- Never place AI-provider or calendar credentials in the browser.
- Service qualifications, availability, durations, and confirmation results are always taken from the backend response.
- Never show a booking as successful via optimistic UI before the backend has returned a confirmed appointment ID.
- Preserve session context, but never persist non-essential patient data in browser storage long-term.

## Required User States

- Initial service selection.
- Date and available-time selection.
- Collecting patient name and calendar-invite email.
- Preview and explicit confirmation.
- Success state, including date, time, service, professional, and reference ID.
- Loading, no available slots, invalid input, slot just booked by someone else, delayed calendar delivery, rate limit, offline, and unexpected error.
- When a request is out of scope, provide a clear clinic or emergency-service contact path.

## Minimum Quality Bar

- Responsive layout starting from 320 px.
- Keyboard operability, visible focus, semantic headings, labeled controls, live-region announcements, and adequate contrast.
- Respect reduced-motion and the user's audio preferences.
- Never convey state through color, animation, or voice alone.
- Test against the current versions of Chrome, Safari, Firefox, and Edge; document voice-feature differences rather than hiding them.
- Medical and privacy copy must be factual. Avoid overstated claims or promotional language.

## Required Checks Before Delivery

- Typecheck, lint, unit tests, and a production build.
- Component tests for the full text-based booking flow and the important error states.
- Keyboard-only and screen-reader smoke tests.
- Mobile viewport review and the browser voice-support matrix.
- The production bundle must contain no secrets, patient data, debug logs, or external scripts.
