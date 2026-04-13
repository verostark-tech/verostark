# CLAUDE.md — Verostark

## What you are building

Verostark detects deviations between what a STIM royalty
statement paid and what a Nordic music publisher's registered
catalogue entitles them to receive. It explains every
deviation in plain English and tells the administrator
exactly what to do next.

---

## Principle 1 — Think Before Coding

Don't assume. Don't hide confusion. Surface tradeoffs.

- If a requirement is ambiguous — state the ambiguity
  and ask before writing a single line
- If multiple approaches exist — present them with
  tradeoffs, don't pick silently
- If the requested approach seems wrong — push back
  and explain why
- If something is unclear mid-task — stop, name what
  is unclear, ask for clarification
- State your assumptions explicitly at the start
  of every non-trivial task

The models make wrong assumptions and run with them.
Verostark cannot afford wrong assumptions on royalty
calculations. A wrong distribution key applied silently
is a material error.

---

## Principle 2 — Simplicity First

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked
- No abstractions for single-use code
- No flexibility or configurability that wasn't requested
- No error handling for impossible scenarios
- If 200 lines could be 50 — rewrite it

The test: would a senior engineer say this is
overcomplicated? If yes, simplify before shipping.

Verostark V0.1 scope is STIM Sweden only. Do not build
for PRS, GEMA, or any other PRO. Do not build Stripe,
email, or CWR outbound generation. Do not lay groundwork
for Phase 2 unless explicitly instructed.

---

## Principle 3 — Surgical Changes

Touch only what you must. Clean up only your own mess.

- Don't improve adjacent code, comments, or formatting
- Don't refactor things that aren't broken
- Match existing style even if you'd do it differently
- If you notice unrelated dead code — mention it,
  don't delete it
- Remove only imports, variables, and functions that
  YOUR changes made unused

The test: every changed line must trace directly
to the current task.

Multi-tenancy is sacred. Every table has org_id.
Every query filters by org_id. If a change touches
a DB query — verify org_id filter is present.
Never remove it as a side effect.

---

## Principle 4 — Goal-Driven Execution

Define success criteria. Loop until verified.

For every task, state the plan first:

1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]

Transform every task into a verifiable goal:

Instead of "add the parser" →
"Parse a 150-line STIM CSV and return all
StatementLine structs with zero missing fields.
Verify: run test against synthetic fixture,
assert line count matches, assert no empty
WorkRef or NetAmount fields."

Instead of "fix the deviation logic" →
"Write a test: 1000 SEK received vs 333 SEK
expected → CRITICAL flag, territorial_override
pattern. Make it pass."

---

## Stack — reference only, do not deviate

Backend:    Encore Go
Database:   Encore-managed PostgreSQL, EU Frankfurt
Auth:       Clerk — magic link, organisations
AI:         Claude API — claude-sonnet-4-20250514
Frontend:   Lovable (React)
Secrets:    Encore secrets manager

---

## Project structure

/auth         Clerk JWT validation
/files        File upload, validation, object storage
/validators   ISWC, IPI validators (ported from DMP MIT)
/cwr          CWR fixed-width parser (ported from DMP MIT)
/statements   STIM CSV parser, canonical StatementLine
/rules        Distribution key rule engine
/detection    Work matching, deviation classifier,
              orchestrator
/ai           Claude API client, explanation generator
/api          All REST endpoints

---

## Non-negotiable rules

1. Accuracy first. If the royalty calculation is wrong,
   nothing else matters. Stop and surface the problem.

2. Every DB table has org_id. Every query filters by
   org_id. No exceptions.

3. Every failure state returns a human-readable message.
   No stack traces. No silence. No technical errors
   surfaced to the user.

4. Every explanation written for the user must be
   readable by a copyright administrator without a
   technical background. If they would need to ask
   what a word means — rewrite it.

5. No feature exists without a test.

---

## Definition of Done

- Code written and compiles
- Tests pass
- Error states return human-readable messages
- No crashes across 5 manual runs
- Merged to main
- Demo flow unaffected

---

## V0.1 scope boundary

Building:
  STIM Sweden CSV ingestion
  Distribution key engine — STIM SE only
  Work matching — exact ISWC then fuzzy title
  Deviation detection and classification
  AI explanations via Claude API
  Rule-based recommendations
  Encore REST API
  Lovable frontend — 4 screens

Not building:
  Stripe payments
  Resend email
  CWR outbound generation
  Multi-PRO rules (PRS, GEMA, KODA)
  Writer payment calculation
  Dispute filing
  Mobile responsive design

Do not build what is not on the building list.
Do not reference or scaffold what is on the
not-building list unless explicitly instructed.
