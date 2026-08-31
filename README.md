# Dental Appointment Support Bot

The product documentation baseline is complete; the project is now moving into the API contract and production data model stage.

## Product Summary

Patients can use text or voice to choose a standard dental service, find a qualified professional with sufficient availability, and confirm an appointment. The system determines availability solely from PostgreSQL; once confirmed, the appointment is written asynchronously to Google Calendar and Outlook. AI is responsible for understanding natural language; service qualifications, required duration, and booking legality are controlled by backend rules.

External patients do not need to register an account or log in; for each booking they simply provide their own name and email, used only as the contact and calendar-invite recipient for that appointment — no long-lived account or cross-appointment identity is created.

## Clinic Model

| Code | English Service Name | Duration | Qualified Professionals |
|---|---|---:|---|
| A | Service A | 60 min | Junior, Senior 1, Senior 2 |
| B | Service B | 60 min | Junior, Senior 1, Senior 2 |
| C | Service C | 150 min | Senior 1, Senior 2 |
| D | Service D | 120 min | Senior 1, Senior 2 |
| E | Service E | 360 min | Senior 1, Senior 2 |

The names, durations, and qualifications above are fixed values. The clinic's timezone, business hours, holidays, break periods, slot interval, and minimum advance-booking window are still pending clinic confirmation and must not use implicit defaults.

## Scope

Planned scope for this phase:

- A responsive web application built with React.
- A Go backend API and a deterministic scheduling domain.
- English-language text conversation.
- Browser-based voice input and voice replies, with text input permanently retained as a fallback.
- Vendor-neutral AI intent and field extraction.
- Asynchronous projection of confirmed PostgreSQL appointments into Google Calendar and Outlook, without reading external busy times.
- Clear service-scope boundaries and safe clinic hand-off.
- Production architecture, security, operations, and user documentation.

Not included in version 1:

- Diagnosis, triage, prescriptions, emergency medical care, insurance, treatment quotes, or payment.
- Any automatic cancellation or rescheduling; such requests are handled manually by the clinic.
- Phone/PSTN channel, multiple languages, or integration with dental practice management systems.
- A staff administration back office, unless separately approved.

## Documentation Index

- [Frontend Product & Client Guide](frontend/README.md): patient experience, voice behavior, UI states, accessibility, install prerequisites, and clinic acceptance criteria.
- [Frontend Implementation Conventions](frontend/AGENTS.md): constraints and quality requirements for the upcoming React phase.
- [Backend Architecture & Internal Guide](backend/README.md): architecture, booking contract, Calendar/AI boundaries, security, and production operations.
- [Backend Implementation Conventions](backend/AGENTS.md): constraints and verification requirements for the upcoming Go phase.

## Recommended Delivery Phases

| Phase | Deliverable | Completion Criteria |
|---|---|---|
| 1 — Contract | Complete API schema, production data model, and Calendar delivery mapping | Cross-layer contract reviewed and ready for implementation and testing. |
| 2 — Backend | Go API, domain tests, data persistence, AI and Calendar adapters | Service qualifications, durations, conflicts, failure scenarios, and scope boundaries all pass tests. |
| 3 — Frontend | React text/voice application | Responsive, keyboard, screen-reader, text-input, and supported-browser voice checks pass. |
| 4 — Integration | Google and Microsoft sandbox connections | Each confirmed appointment creates at most one event per provider; sync failures do not change appointment status. |
| 5 — Production | Managed infrastructure, OAuth, observability, privacy, and recovery controls | Passes security review, restore testing, clinic acceptance, and the release checklist. |

## Decisions Required Before Implementation

1. Confirm the clinic's timezone, business hours, holidays, break periods, slot interval, and minimum advance-booking window.
2. Confirm the Google/Microsoft authorization model, tenant permissions, and credential storage; every active professional must have an independent mapping for both providers.
3. Choose the production AI provider and the applicable health-data and privacy agreements. (The backend currently uses a provider-neutral interface wired to a development/test OpenAI-compatible adapter — see "AI Provider Adapter Contract" in `backend/README.md`; voice-input transcription reuses the same development/test integration — see "Voice Transcription Endpoint" — meaning user recordings pass through this service's backend and that AI provider. This does not constitute approval of this decision item; the formal provider and the applicable health-data/privacy agreements, including for audio data, are still pending clinic confirmation.)
4. Define data retention/deletion, emergency handling, cancellation/rescheduling hand-off, and staff support policies.
