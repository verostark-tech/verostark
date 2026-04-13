# Verostark — Lovable Frontend Spec

## Overview

Verostark detects deviations between what a STIM royalty statement paid and what
a Nordic music publisher's registered catalogue entitles them to receive. The app
explains every deviation in plain English and tells the administrator exactly what
to do next.

**Users:** Copyright administrators at Nordic music publishers.
**Language:** English only for V0.1.
**Auth:** Clerk magic-link, organisation-scoped. Assume Clerk is already integrated
          and the JWT is passed as `Authorization: Bearer <token>` on every request.

---

## Base URL

All API calls use `VITE_API_BASE_URL` from env. Prefix every path with it.

---

## Screen 1 — Dashboard

**Route:** `/`

**Purpose:** Show the health of the publisher's catalogue and latest statement at a glance.

### Layout

Top row — 3 stat cards (side by side):
- **Registered works** — number from `GET /api/works` → `works.length`
- **Statements uploaded** — number from `GET /api/statements` → `statements.length`
- **Open deviations** — number from `GET /api/deviations?status=open` → `flags.length`

Below — two columns:

**Left: Recent statements** (last 5 from `GET /api/statements`)
Table columns: Filename | Period | PRO | Status | Uploaded
Status badge colours:
- `pending` → grey
- `processing` → yellow / amber
- `completed` → green

Each row has a "View deviations" link → `/statements/:id/deviations`

**Right: Recent deviations** (last 5 from `GET /api/deviations?status=open`)
Table columns: Work title | ISWC | Severity | Pattern | Period
Severity badge colours:
- `LOW` → blue
- `MEDIUM` → yellow
- `HIGH` → orange
- `CRITICAL` → red

Each row links to `/deviations/:id`

---

## Screen 2 — Upload

**Route:** `/upload`

**Purpose:** Upload a CWR catalogue file or a STIM royalty statement.

### Layout

Two upload cards side by side.

---

### Card A — CWR Catalogue

**Title:** Register catalogue (CWR)

**Explainer text:**
> Upload your CWR registration file to populate the works catalogue.
> Accepted formats: .cwr, .txt

**Flow:**
1. User picks a file. Show filename and size.
2. On submit:
   a. `POST /files/upload` (multipart, field name `file`) → `{ key, filename, size, mime_type }`
   b. `POST /api/cwr` body `{ "file_key": key }` → `{ works_stored, writers_stored }`
3. On success: show green banner "Catalogue updated — {works_stored} works, {writers_stored} writers registered."
4. On error: show the error message from the API response.

---

### Card B — STIM Statement

**Title:** Upload STIM statement

**Explainer text:**
> Upload your STIM royalty statement. Once uploaded you can run the deviation check.
> Accepted formats: .csv, .txt

**Fields:**
- File picker
- Period (text input, placeholder "e.g. 2024-Q1") — required

**Flow:**
1. User picks file and enters period.
2. On submit:
   a. `POST /files/upload` (multipart, field name `file`) → `{ key, filename, size, mime_type }`
   b. `POST /api/statements` body `{ "filename": filename, "period": period }` → Statement object
3. On success: show green banner "Statement registered. Run the deviation check from the Statements screen."
   Link in banner → `/statements` (Screen 3).
4. On error: show the error message from the API response.

---

## Screen 3 — Statements

**Route:** `/statements`

**Purpose:** List all uploaded statements. Run the deviation check. See per-statement results.

### Layout

Page header: "Statements"

Table: all statements from `GET /api/statements`
Columns: Filename | Period | PRO | Status | Uploaded | Actions

**Actions column:**
- If status is `pending`: show "Run detection" button.
  On click: `POST /api/statements/:id/run` → `{ run_id, flag_count }`
  While loading: disable button, show spinner.
  On success: toast "Detection complete — {flag_count} deviations found." Refresh the row status.
  On error: toast with API error message.
- If status is `processing`: show disabled "Running…" button with spinner.
- If status is `completed`: show "View deviations" link → `/statements/:id/deviations`

---

## Screen 4 — Deviations

**Route:** `/statements/:id/deviations` (filtered view)
**Route:** `/deviations` (all deviations — reachable from Dashboard)
**Route:** `/deviations/:id` (single deviation detail)

---

### 4a — Deviations list

**Data:**
- If on `/statements/:id/deviations`: `GET /api/deviations?statement_id=:id`
- If on `/deviations`: `GET /api/deviations`

**Filters bar (above table):**
- Severity dropdown: All | LOW | MEDIUM | HIGH | CRITICAL
- Status dropdown: All | open | resolved
- When changed, re-fetch with updated query params.

**Table columns:**
Work title | ISWC | Right type | Period | Expected (SEK) | Received (SEK) | Deviation | Severity | Status

Format currency as `{value} SEK` with 2 decimal places.
Deviation column: show signed value (red if negative / underpayment, amber if positive / overpayment).
Severity badge uses the same colours as Dashboard.
Each row is clickable → `/deviations/:id`

**Empty state:**
If no deviations: "No deviations found for these filters."

---

### 4b — Deviation detail

**Route:** `/deviations/:id`

**Data:** `GET /api/deviations/:id`

**Layout:**

Top: breadcrumb "Deviations / {work_title}"

**Summary card:**
- Work title (large heading)
- ISWC | Right type | Period
- Severity badge
- Status badge

**Figures row (3 cards):**
- Expected: `{expected_amount} SEK`
- Received: `{received_amount} SEK`
- Deviation: `{deviation_amount} SEK` with sign, coloured red/amber, show `{deviation_pct * 100}%`

**Explanation card:**
Title: "What happened"
Body: `{explanation}` — plain paragraph text, no formatting

**Recommendation card:**
Title: "What to do next"
Body: `{recommendation}` — plain paragraph text

**Pattern type:**
Show below the figures as a small label: "Pattern: Underpayment" or "Pattern: Overpayment"

---

## Navigation

Sidebar (left, narrow) with links:
- Dashboard (`/`)
- Upload (`/upload`)
- Statements (`/statements`)
- Deviations (`/deviations`)

Show the organisation name from Clerk in the sidebar header.

---

## Error handling

- If any API call returns a non-2xx response, show the `message` field from the JSON body in a red toast notification.
- If the network is unreachable, show "Unable to reach the server. Check your connection."
- Never show stack traces or technical error codes to the user.

---

## Loading states

- Show a skeleton loader (grey shimmer) for tables while data loads.
- Show a spinner on buttons during async actions.
- Disable interactive elements during in-flight requests.

---

## Design tokens (use these exactly)

- Font: Inter
- Background: `#F9FAFB` (grey-50)
- Card background: `#FFFFFF`
- Border: `#E5E7EB` (grey-200)
- Primary action: `#1D4ED8` (blue-700)
- Severity CRITICAL: `#DC2626` (red-600)
- Severity HIGH: `#EA580C` (orange-600)
- Severity MEDIUM: `#D97706` (amber-600)
- Severity LOW: `#2563EB` (blue-600)
- Status completed: `#16A34A` (green-600)
- Status pending: `#6B7280` (grey-500)
- Status processing: `#D97706` (amber-600)
