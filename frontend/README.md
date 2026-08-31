# Frontend Product & Client Guide

## What Patients Will Be Able to Do

The frontend will be an English-only web booking assistant that supports both desktop and mobile. Patients can type or use the microphone; both paths use the same conversation flow and the same backend rules.

The interface has one primary job: help the patient choose a supported service and book a safe, available time slot. It must never present itself as a clinical diagnostic tool.

## Planned Patient Flow

1. Choose or describe Service A–E.
2. See the service duration and which category of professional can perform it.
3. Choose a date.
4. Choose an available professional and start time.
5. Provide the patient's name and the email that will receive the calendar invite.
6. Review service, duration, professional, date, time, timezone, and email.
7. Explicitly confirm the appointment.
8. Receive a booking reference ID and the calendar-delivery status.

Availability shown during the conversation is only a provisional result. The backend re-checks it at the moment of confirmation. If a slot has been taken in the meantime, the interface must state that the status changed and offer new options.

## Voice Behavior

Voice input and voice output use different architectures:

- **Voice input (speech-to-text)**: the browser uses `MediaRecorder` to record the patient's speech and uploads it to this service's own backend at `POST /voice/transcriptions`, which calls the integrated AI transcription provider to get text back (see "Voice Transcription Endpoint" in `backend/README.md`). **This means the patient's recording passes through this service's backend and that AI provider** — it is not purely browser-side processing. This is a deliberate architectural decision (replacing an earlier design where audio never reached the backend); the trade-off is unified control over transcription quality and the ability to switch providers, at the cost of audio data now leaving the browser for part of the flow.
- **Voice output (text-to-speech)**: still uses the browser's native `speechSynthesis`, does not go through the backend, and is not tied to any particular provider — this part is unaffected by the architecture change above.

Expected limitations:

- Browser and OS support for `MediaRecorder`/`speechSynthesis` varies.
- Microphone permission may be denied or unavailable.
- Names, dates, and emails may be mis-transcribed by the AI.
- If the backend transcription service times out or is unavailable, voice input simply fails and prompts the patient to switch to text — there is no fallback template the way there is for chat replies, because transcription has nothing to substitute.

Because of this, voice must always be optional. Recognized text must be shown before any irreversible action, sensitive fields must remain editable, and the appointment must go through explicit confirmation — "irreversible action" here means the final booking confirmation; every message sent via voice input is still just a candidate value that can be corrected in a later turn of the conversation.

### Version 1 Implementation Details (Chat Mode)

- Voice features exist only in Chat conversation mode; the step-by-step Wizard mode offers neither voice input nor read-aloud.
- Voice input only captures the final transcription of a single recording (no live interim transcript is shown), and sends that text as the message immediately rather than waiting in the input box for a manual Send. After recording stops, the UI shows a "Transcribing…" state while waiting for the backend response; once it completes, the text appears in the conversation history as a patient message. If the transcription is wrong, the patient can simply correct it with a following text or voice message. This simplification does not affect the "the appointment must be explicitly confirmed" rule: the final "Confirm booking" step remains separate and requires a manual click.
- "Speak replies" is an opt-in toggle, off by default; the user must turn it on for bot replies to be read aloud. The toggle state is stored only as a boolean in the browser's `localStorage` (key: `voice.speakRepliesEnabled`) and never persists any transcript or conversation content.
- When the microphone is unsupported (no `MediaRecorder`/`getUserMedia`), the voice-input button does not render at all; the "Speak replies" toggle still renders but appears disabled with a "not supported in this browser" note, so the user doesn't mistake the missing feature for a bug.
- Known gap: this feature shipped without automated tests (the repo currently has no test framework at all) — only manual and cross-browser verification has been done. This is a continuation of an existing gap, not something deliberately skipped for this feature, and remains to be addressed once test infrastructure is in place.

### Browser Voice Support Matrix

The table below is still pending manual cross-browser verification (test date, version, and result to be filled in); no browser should be assumed supported before that verification happens:

| Browser | MediaRecorder (voice input) | Speech Synthesis (voice output) | Known Issues |
| --- | --- | --- | --- |
| Chrome (desktop) | pending | pending | — |
| Safari (desktop/iOS) | pending | pending | — |
| Firefox | pending | pending | — |
| Edge | pending | pending | — |

## Scope-of-Service Copy

The interface can help schedule a standard appointment, but must decline:

- Symptom assessment or diagnosis.
- Emergency or acute-care triage decisions.
- Prescriptions or treatment advice.
- Treatment pricing, insurance coverage, or payment.
- Cancellation or rescheduling; version 1 only hands off to the clinic and performs no changes.

Before launch, the clinic must supply the everyday contact phone number, emergency-situation copy, business hours, a privacy notice, and support contact information.

## Client Setup Outside the App

Before publishing the web application, the clinic must:

1. Confirm business hours, holidays, break periods, slot interval, and advance-booking rules.
2. Choose a clinic-owned HTTPS domain, e.g. `appointments.exampleclinic.com`.
3. Publish a privacy notice explaining what booking data is collected, and that voice-input recordings are sent to this service's backend and the integrated AI transcription provider (not purely processed in the browser).
4. Confirm emergency-situation copy and the staff phone hand-off method.
5. Complete clinic-approved Google Cloud and Microsoft Entra sandbox setup.
6. Provide logo, clinic name, contact information, and accessibility-compliant approved colors.
7. Test with synthetic patient data and sandbox calendars before accepting real data.

## Clinic Acceptance Checklist

- A and B can be performed by all three professionals.
- C, D, and E never show Junior as an option.
- A six-hour service is never offered a slot that would run past closing time.
- Available-time and overlap checks rely solely on PostgreSQL data; Google and Outlook only ever receive confirmed appointments.
- A slot taken during checkout is replaced with new options, never resulting in a double booking.
- After confirmation, the correct service, professional, timezone, and duration are displayed.
- Every decline scenario listed under "Scope-of-Service Copy" (diagnosis, emergency, prescription, quote/insurance, cancel/reschedule) leads to the approved hand-off copy.
- Booking can be completed in text-only, keyboard-only, screen-reader, mobile, microphone-denied, and voice-unsupported environments.
- No calendar or AI credential is ever visible in browser requests, browser storage, or the production bundle.

## Planned Operational Guidance

If the backend cannot verify availability from PostgreSQL, the page should ask the patient to retry later or contact the clinic — it must never guess. Once the backend has issued a reference ID, that appointment is confirmed; Google/Outlook delivery status must be shown separately, and if delivery fails, retry, alerting, and manual follow-up take over without ever changing the appointment's status.
