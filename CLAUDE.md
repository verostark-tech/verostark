# CLAUDE.md — Verostark

## What you are building

Verostark detects deviations between what a royalty
statement from a collecting society paid and what an
independent music publisher is entitled to receive
based on their registered ownership.

The detection is built on the CISAC CRD standard.
The reference for all royalty distribution logic is the
CISAC CRD EDI Format Specifications Version 3.0 Revision 5.
No other standard is assumed. No other standard is referenced.

Every deviation is explained in plain English.
Every explanation tells the administrator exactly
what to do next.

The product serves independent music publishers globally.
The beachhead is Nordic publishers using STIM.
The architecture serves any publisher using any
CISAC-member collecting society.

---

## The Elon Musk Algorithm — apply to every task

Before writing any code, run this sequence:

**Step 1 — Make requirements less dumb**
Every requirement is wrong until proven otherwise.
State the requirement back in one sentence.
Ask: is this actually needed? Who needs it? When?
If the requirement cannot survive one question — delete it.

**Step 2 — Delete the part or process**
If you are not adding something back at least 10% of the time
you are not deleting enough.
Remove every line, function, and abstraction that does not
directly serve the current task.
The best code is no code.

**Step 3 — Simplify**
Only after deleting everything possible — simplify what remains.
Minimum code that solves the problem.
No abstractions for single-use code.
No flexibility that was not explicitly requested.
If 200 lines could be 50 — rewrite it before submitting.

**Step 4 — Accelerate cycle time**
Define success criteria before writing the first line.
Run the test before claiming done.
A task is not done until the test passes.
Cycle: spec → implement → test → verify → ship.

**Step 5 — Automate**
Only automate what has been validated manually first.
Do not automate an unproven process.
Do not build infrastructure for features that do not exist yet.

---

## The YC AI-Native Lifecycle — apply to every task

Every task produces a closed loop:

1. **Spec** — human writes what success looks like
2. **Test** — human writes the acceptance criteria
3. **Implement** — Claude Code generates the implementation
4. **Verify** — tests pass or Claude Code iterates
5. **Ship** — merged only when all criteria pass

Claude Code never ships without passing tests.
Claude Code never assumes a test passed — it runs it.
Claude Code never moves to step N+1 until step N is verified.

If the tests cannot be written — the spec is not clear enough.
Stop. Ask for clarification. Do not implement against an unclear spec.

---

## Principle 1 — Think Before Coding

Do not assume. Do not hide confusion. Surface tradeoffs.

- If a requirement is ambiguous — state the ambiguity
  and ask before writing a single line
- If multiple approaches exist — present them with
  tradeoffs, do not pick silently
- If the requested approach seems wrong — push back
  and explain why
- If something is unclear mid-task — stop, name what
  is unclear, ask for clarification
- State your assumptions explicitly at the start
  of every non-trivial task

Verostark cannot afford wrong assumptions on royalty
calculations. A wrong distribution key applied silently
is a material financial error for a publisher.

---

## Principle 2 — Simplicity First

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked
- No abstractions for single-use code
- No flexibility or configurability that was not requested
- No error handling for impossible scenarios
- If 200 lines could be 50 — rewrite it

The test: would a senior engineer say this is
overcomplicated? If yes, simplify before shipping.

V0.1 scope is CRD ingestion and MEC detection only.
Do not build PERF detection, CWR outbound, Stripe,
email, or multi-PRO rules unless explicitly instructed.
Do not lay groundwork for Phase 2 unless instructed.

---

## Principle 3 — Surgical Changes

Touch only what you must. Clean up only your own mess.

- Do not improve adjacent code, comments, or formatting
- Do not refactor things that are not broken
- Match existing style even if you would do it differently
- If you notice unrelated dead code — mention it, do not delete it
- Remove only imports, variables, and functions that
  YOUR changes made unused

Multi-tenancy is sacred.
Every table has org_id.
Every query filters by org_id.
If a change touches a DB query — verify org_id filter is present.
Never remove it as a side effect.

---

## Principle 4 — Goal-Driven Execution

Define success criteria. Loop until verified.

For every task, state the plan first:

```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Transform every task into a verifiable goal:

Instead of "add the CRD parser" →
"Parse the synthetic CRD file verostark_synthetic_MEC_Q1_2025.crd
and return all StatementLine structs with zero missing fields.
Verify: run test against the fixture.
Assert SOMMARNATT gross = 372000 (3720.00 SEK).
Assert DROMMAR gross = 102600 (1026.00 SEK).
Assert DROMMAR net = 102600 (1026.00 SEK).
Assert right_type = MEC for both works."

Instead of "fix the deviation logic" →
"Given DROMMAR: gross 1026.00 SEK, net 1026.00 SEK,
controlled share 1/1, distribution key 1/3.
Detection must return: CRITICAL flag,
pattern = territorial_override,
overpayment = 684.00 SEK.
Make the test pass."

---

## The Detection Algorithm — source of truth

The detection is derived from first principles.
Never deviate from this without explicit instruction.

### Ground truth
A work is created. Ownership is fixed at creation.
That ownership is declared to a collecting society via CWR (Common Works Registration).
The society then distributes royalties and reports them via CRD (Common Royalty Distribution).
Verostark reads the CRD to detect whether the distribution matches the CWR declaration.
CWR is not in V0.1 scope. The distribution key is Verostark's reference — not the CWR file.
The society distributes using the declared distribution key.
Everything the detection engine produces is a measurement of
how the statement diverges from that ground truth.

### The invariant for Nordic publishers (STIM)
For a STIM-affiliated publisher:

```
MEC (Mechanical): publisher share = 1/3
PERF (Performance): publisher share = 1/3
```

These are written in stone. The detection checks against them.

### The formula

```
net_after_fee  = gross × (1 - fee_rate)
expected_net   = net_after_fee × distribution_key × controlled_share
observed_ratio = net_received / net_after_fee
expected_ratio = distribution_key × controlled_share
deviation      = observed_ratio - expected_ratio

if abs(deviation / expected_ratio) > 1/100_000:
    FLAG
```

### Arithmetic rules — non-negotiable

All monetary and ratio calculations use exact rational arithmetic.
In Go: math/big.Rat throughout.
Never float64. Never float32. Never decimal approximation.

1/3 is stored as big.Rat{1, 3}. Not as 0.3333.
The threshold 1/100_000 is stored as big.Rat{1, 100_000}.
Amounts from the CRD file are parsed to big.Rat from strings.
They are never converted to float at any point in the pipeline.

A deviation of 2/3 (ratio 1/1 instead of 1/3) is a CRITICAL flag.
This is the ICE Cube PRS override pattern.

### Severity classification

```
ratio_excess = observed_ratio / expected_ratio

>= 2.5  → CRITICAL  (full territorial override)
1.5–2.5 → HIGH      (partial override)
1.1–1.5 → MEDIUM    (investigate before disputing)
< 1.1   → POSSIBLE  (may be rounding — flag for review only)
```

---

## The CRD Format — source of truth

The input format is CISAC CRD EDI Version 3.0 Revision 5.
All field positions are fixed-width. Character position is the truth.

### Records used for detection (V0.1)

```
HDR  pos 1-3   File header — publisher context
SDN  pos 1-3   Distribution period, currency, right category
               Key fields:
                 right_category  pos 30 size 3  (MEC/PER/BOT)
                 period_start    pos 33 size 8  (YYYYMMDD)
                 period_end      pos 41 size 8  (YYYYMMDD)
                 amount_decimals pos 113 size 1
                 currency        pos 120 size 3

ESI  pos 1-3   Exploitation source
               Key fields:
                 source_id       pos 20 size 20
                 source_name     pos 40 size 30
                 source_type     pos 70 size 2  (03=DSP)

MWN  pos 1-3   Work identity — carry forward to child records
               Key fields:
                 work_ref        pos 20 size 14
                 work_title      pos 34 size 60
                 iswc            pos 154 size 11

MDR  pos 1-3   Right type — carry forward
               Key fields:
                 right_code      pos 20 size 2  (MD=MEC streaming)
                 right_category  pos 22 size 3  (MEC/PER)

MIP  pos 1-3   Controlled share — carry forward
               Key fields:
                 share_numerator   pos 36 size 5
                 share_denominator pos 41 size 5
                 share_percentage  pos 46 size 8 (use fraction if available)

WER  pos 1-3   Mechanical exploitation — one per income event
               Key fields:
                 dist_category     pos 35 size 2
                 source_id         pos 37 size 20
                 sales_period_start pos 145 size 8
                 sales_period_end  pos 153 size 8
                 currency          pos 246 size 3
                 gross_amount      pos 249 size 18  ← detection input
                 remitted_amount   pos 350 size 18  ← detection input
```

### Parser rules

1. Read file line by line
2. Read positions 1-3 → record type
3. Route to the correct field extractor
4. Carry forward context: SDN → MWN → MDR → MIP → WER
5. Each WER inherits context from its parent MWN, MDR, MIP
6. Parse all amounts as big.Rat from string — never float
7. The decimal precision is set by SDN amount_decimals field
8. An amount of "000000000000372000" with 2 decimal places = 3720.00

### V0.1 scope
Parse WER records only. MEC detection only.
WEP (performance) is Phase 2.

---

## The Synthetic Test File

The canonical test fixture is:
`verostark_synthetic_MEC_Q1_2025.crd`

Built from CISAC CRD 3.0 R5 field positions. Nothing assumed.

Two works:

```
WORK 1 — SOMMARNATT (CLEAN)
  work_ref:    STIM20190442
  iswc:        T1234560013
  right_code:  MD (MEC streaming)
  territory:   Sweden (TIS 0752)
  gross:       3720.00 SEK
  net:         1240.00 SEK
  ratio:       1/3 — CORRECT
  expected:    CLEAN

WORK 2 — DROMMAR (CRITICAL — planted deviation)
  work_ref:    STIM20180331
  iswc:        T1234560042
  right_code:  MD (MEC streaming)
  territory:   Sweden (TIS 0752)
  gross:       1026.00 SEK
  net:         1026.00 SEK
  ratio:       1/1 — WRONG (should be 1/3)
  expected:    CRITICAL FLAG
  overpayment: 684.00 SEK
```

All tests must pass against this fixture.
Zero false positives on SOMMARNATT.
CRITICAL flag on DROMMAR with exact overpayment 684.00 SEK.

---

## Stack — do not deviate

```
Backend:    Encore Go (EU Frankfurt)
Database:   Encore-managed PostgreSQL
Auth:       Clerk — magic link, organisations
AI:         Claude API — claude-sonnet-4-20250514
Frontend:   Lovable (React)
Secrets:    Encore secrets manager
Arithmetic: math/big.Rat — never float64
```

---

## Project structure

```
/crd          CRD fixed-width parser (CISAC 3.0 R5)
/statements   Canonical StatementLine struct
/rules        Distribution key rule engine (STIM SE only in V0.1)
/detection    Ratio check, deviation classifier, orchestrator
/ai           Claude API client, explanation generator
/api          REST endpoints
/auth         Clerk JWT validation
/files        CRD file upload and storage
```

---

## Non-negotiable rules

1. **Accuracy first.** If the royalty calculation is wrong,
   nothing else matters. Stop and surface the problem.

2. **Every DB table has org_id. Every query filters by org_id.**
   No exceptions. Multi-tenancy is sacred.

3. **Every failure state returns a human-readable message.**
   No stack traces. No silence. No technical errors surfaced to the user.

4. **Every explanation written for the user must be readable
   by a copyright administrator without a technical background.**
   If they would need to ask what a word means — rewrite it.

5. **No feature exists without a test.**

6. **All arithmetic is exact rational. Never float.**
   A floating point royalty calculation is a bug by definition.

7. **The CISAC CRD spec is the only reference for file format.**
   Do not infer field positions. Look them up in the spec.
   State the position and size before using them in code.

---

## Definition of Done

- Code written and compiles
- Tests pass against the synthetic CRD fixture
- SOMMARNATT returns CLEAN
- DROMMAR returns CRITICAL with overpayment 684.00 SEK exactly
- Error states return human-readable messages
- No crashes across 5 manual runs
- org_id present on every DB record touched
- Merged to main
- Demo flow unaffected

---

## V0.1 scope boundary

**Building:**
- CRD file upload (single file, CISAC 3.0 R5)
- CRD parser (WER records, MEC only)
- Distribution key engine (STIM SE, 1/3 MEC)
- Deviation detection and classification
- AI explanations via Claude API
- Encore REST API
- Lovable frontend — upload screen + results screen

**Not building:**
- WEP parser (PERF detection — Phase 2)
- CSV ingestion (removed — build on the standard)
- CWR outbound generation
- Stripe payments
- Resend email
- Multi-PRO rules (PRS, GEMA, KODA)
- Writer payment calculation
- Dispute filing
- Mobile responsive design

Do not build what is not on the building list.
Do not reference or scaffold what is on the not-building list
unless explicitly instructed.
