# Dental Appointment Bot — Architecture Notes (Internal)

Assumptions up front: this system is meant to actually go live for patients, not stay a local demo. The scale is exactly what the README describes — one clinic, three professionals, five services.

## What the Architecture Looks Like

- Backend: written in Go, all running in a single process, internally split into modules (Catalog / Scheduling / Booking / Conversation / Calendar), each further split into handler → service → repository layers.
- Database: PostgreSQL — everything about a booking is decided against it.
- Frontend: React + TypeScript, primarily a typed chat, with voice as a bonus.
- External calendars (Google/Microsoft): can only passively receive the data we write to them; they are never used to decide whether a slot is free. Writes happen asynchronously through an outbox mechanism.
- AI: only responsible for understanding what the patient is saying and extracting candidate values — "is this slot actually bookable" is always decided by deterministic backend code; the AI has no say in it.

Current state: Catalog/Scheduling/Booking/Conversation and voice input are all done, and the Calendar outbox mechanism is also done — but it's currently wired to a fake sandbox adapter, not real Google/Microsoft OAuth yet (waiting on the clinic to decide which authorization approach to use). Cancellation/rescheduling isn't in version 1 — the patient is simply asked to call the clinic.

## Why These Choices

**PostgreSQL has final say; Google/Microsoft are write-only**: querying an external calendar has latency and quota limits — if we used it to decide availability, two patients could both see "this slot is free" at the same time and we'd have a conflict immediately. So all the decision logic lives in the database, using the database's own exclusion constraints to block duplicate bookings directly — far more reliable than hand-rolling a distributed lock. Even if the external calendar goes down, patients can still book; the clinic's calendar view just temporarily lags behind.

**Outbox instead of a synchronous call to the Calendar API**: if booking synchronously called the Google API, the patient's wait time would be at the mercy of Google's latency, and if the call failed, it would be messy to decide whether to roll back an already-successful booking (rolling it back would make the patient feel like "I just booked this, why did it get cancelled" — even worse UX). So we split "the booking succeeded" from "it's synced to the calendar": once PostgreSQL has written it, that counts as success and we respond to the patient immediately; the external sync status is reported honestly as it progresses, retries on failure, and even after exhausting retries it never touches the already-confirmed booking.

**A fake sandbox adapter first, real OAuth later**: which authorization model to use for Google/Microsoft is an organizational-level decision the clinic has to make (e.g. whether to enable Workspace delegation), not something engineering can decide unilaterally. Rather than blocking on that, we built and fully tested the entire outbox state machine, retry logic, and worker first; once the clinic decides on an authorization approach, we only need to swap out one adapter — nothing else needs to change.

**AI only understands language, it doesn't decide legality**: AI output is probabilistic and can be wrong, and dental booking involves real scheduling conflicts that can't be left to AI to rubber-stamp. Even if the AI extracts the wrong candidate value, the worst case is the bot gives an off-topic reply — it will never actually book a conflicting or nonexistent slot.

**No cancel/reschedule in version 1**: there's no account system right now, so there's no way to confirm "is the person asking to cancel actually the patient." Building that anyway would carry more risk than it saves in convenience, so it's all routed to manual handling for now.

## What It Will Cost to Actually Launch

**Still not done**:
- Actually connecting real Google/Microsoft OAuth — not a lot of engineering work, but blocked on the clinic deciding the authorization approach and granting access
- JWT session verification, CSRF, rate limiting — the rules are all documented, **but none of it is written yet**; this is the single biggest gap before launch
- How to encrypt and store calendar credentials, and key management — none of this exists yet
- A scheduled job to clean up expired idempotency keys — doesn't exist yet either
- Whether voice actually works across different browsers, and frontend automated tests — both still need to be filled in

**What it will cost**:
- PostgreSQL needs a managed cloud service with backups and point-in-time recovery — can't just be self-hosted casually
- Both AI conversation extraction and voice transcription are billed by usage; the production vendor hasn't been decided yet, so actual cost can't be estimated
- Google/Microsoft APIs have call-quota limits; the worker's retry frequency needs careful tuning or it will get rate-limited
- Secrets and connection strings currently live in `.env`; production needs to move to a managed secrets service

## Security Features: Already Done

- Every ID is generated from true CSPRNG randomness — unguessable
- Version numbers (ETag/If-Match) prevent concurrent edits to the same record from silently overwriting each other
- The database itself blocks double-booking the same person for the same slot, not just application-level logic
- Resubmitting the same request never creates two bookings
- If availability can't be verified, the system returns an error and refuses rather than guessing a result for the patient
- Logs only record who did what and when — never patient name, email, or chat content
- Calendar-sync failure messages are sanitized before storage; the raw response from the external service is never stored
- Error responses use a consistent format and never accidentally leak internal implementation details, SQL, or patient data
- Missing even one required environment variable means the system refuses to start — it never silently falls back to an unsafe default
- Voice uploads are restricted by file type and have a maximum file-size limit
- When a patient asks about diagnosis, prescriptions, pricing, insurance, or cancellation/rescheduling, the bot always declines and hands off to a human, and never silently applies anything from that turn of the conversation to the booking data
