# Repository Working Conventions

## Project Goal

Build an English-only customer service chatbot that can schedule appointments for a dental clinic's three professionals. The implementation will use Go for the backend and React for the frontend.

The product documentation baseline is complete. The current focus is filling in the API contract and the production data model before moving into application implementation in delivery order.

## MVP Principle

This project is currently at MVP stage: implementation scope is limited to what the current "recommended delivery phase" requires. Do not pre-build abstractions, configuration options, or infrastructure for requirements that are not yet scheduled into a phase.

- Do not add abstractions, interfaces, or configuration flexibility beyond what the current rules or API contract require; extend only when a concrete need arises.
- Do not pre-design extension points for hypothetical future vendors, scale, or features; the existing swappable interfaces (Calendar, AI provider, etc.) already support future extension and need no additional headroom reserved in advance.
- Prefer the most direct viable implementation; abstract only when duplication is clearly recurring or the contract explicitly requires it.
- The above principles must never override the "Non-negotiable Product Rules" in this document or the security, data-consistency, and fail-closed requirements in each layer's `AGENTS.md`; MVP constrains new complexity, it does not lower existing standards.

## Repository Boundaries

- `frontend/` owns the patient-facing React application, the chat interface, the in-browser voice experience, accessibility, and frontend tests.
- `backend/` owns the Go API, booking rules, data persistence, the AI abstraction layer, calendar integration, security controls, and backend tests.
- The cross-layer API contract must be documented and agreed before implementation begins in either layer.
- Google Calendar, Microsoft Graph, and the AI provider must all sit behind interfaces so other systems can be added later without changing the booking rules.

Before modifying either application, read the nearest `AGENTS.md` in that directory.

## Non-negotiable Product Rules

- Patient-facing language is English only.
- Service durations and which professionals are qualified to perform them are governed by the "Clinic Model" table in the root README; no other document may define different values.
- The bot can schedule appointments but must not diagnose, prescribe, give emergency medical advice, quote prices, or handle insurance. Version 1 must not cancel or reschedule appointments; such requests must be handed off to the clinic.
- Available times may only be determined from service durations, professional qualifications, clinic business hours, internal blocked slots, and confirmed appointments in PostgreSQL; Google Calendar and Outlook must never be used as availability inputs, and availability must be re-checked from PostgreSQL again immediately before the patient's final confirmation.
- If real-time availability cannot be verified from PostgreSQL, the system must fail closed and direct the patient to contact the clinic.
- The AI model may understand language, but whether a booking is valid must be decided by deterministic backend code.

## Delivery Order

See the root README's "Recommended Delivery Phases" for the delivery phases and their completion criteria; this document does not repeat them.

## Documentation Standards

Documentation must be written for a clearly identified reader, must state its assumptions, and must clearly distinguish "implemented" from "planned." Avoid promotional or vague content. Unless limitations are explicitly stated, demo credentials, local files, browser-based voice, and synchronous calendar integration must never be described as production-ready.
