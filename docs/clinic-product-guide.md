# Dental Appointment Support Bot — Product Introduction & Onboarding Guide

This document is written for clinic staff. The system is still under development, and this document clearly marks what is "usable now" versus "still needed before launch" — it does not overstate current completeness.

## 1. What This Is

A small assistant that lets patients book their own appointments online by typing (or speaking), reducing some of the phone-booking workload. Through a web-based conversation, the patient picks a service, picks a time slot, and leaves their name and email; the bot confirms a booking directly based on the clinic's actual business hours and staff availability, and automatically sends a calendar invite.

**The bot only speaks English**, and the interface is designed to work on both phone and desktop.

## 2. What the Patient Sees, Roughly

1. Type what service they'd like (or pick from a menu); they can also use the microphone and the bot will convert speech to text.
2. The bot lists which staff are qualified and the service duration.
3. The patient picks a date, then a time slot.
4. They leave their name and email (used for booking reminders and the calendar invite).
5. A final screen lists the full details (service, professional, time, timezone) for the patient to review once.
6. The patient confirms, and the system immediately issues a booking reference number.

If the slot the patient chose gets taken by someone else before confirmation, the system immediately says so and offers other available slots — it never lets two people book the same slot.

## 3. What the Bot Can and Cannot Do

**What it can do**: help the patient choose from the clinic's standard service offerings and complete a booking based on staff qualifications and availability.

**What it cannot do, and will never attempt** (this is a deliberate safety boundary, not an oversight):

- No symptom assessment or diagnosis
- No judgment on whether something is an emergency
- No prescriptions or treatment advice
- No pricing quotes, no insurance discussion
- No cancellation or rescheduling — when a patient asks for this, the bot simply tells them to call the clinic directly

For all of the above, the bot always replies with a fixed hand-off message directing the patient to contact the clinic, and never attempts to answer on its own.
