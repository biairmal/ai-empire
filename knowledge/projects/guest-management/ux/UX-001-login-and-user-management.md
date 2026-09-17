---
type: ux-spec
id: UX-001
title: "Login and User Management — UX Specification"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Product Owner"
created: "2026-09-17"
updated: "2026-09-17"
satisfies: [PRD-001]
depends_on: [ADR-009]
references: [API-001]
---

# Login and User Management — UX Specification

<!--
Extracted on 2026-09-17 from guest-management-be/docs/DEVELOPMENT_PLAN.md B11 "UX Design" and "Frontend Tasks",
guest-management-fe/docs/DESIGN_SYSTEM.md, and the implemented
screens in guest-management-fe/app. The visual mock-ups live on a Claude Design canvas; its link is kept in
guest-management-fe/docs/design-canvas.local.md (git-ignored, personal).
-->

## Summary

This covers the staff login, the prompt to change a password, and tenant user administration, which are PRD-001 FR-02, FR-14, FR-19 and story US-06. It also sets the design foundations every later screen reuses: the light and dark theme, the app shell, the breadcrumb header, and the filter and sort toolbar. A good result: a user can't change or delete another account by accident, admins understand the effect of every account action before confirming it, and the interface is comfortable to use all day in either theme.

**Status:** all screens are built in `guest-management-fe` with mock data (`app/_lib/mock-data.ts`). API integration has not started.

## Users and Scenarios

| Scenario | Persona | Context | Goal |
|----------|---------|---------|------|
| S1 | Any staff user | Desktop browser, first login with an admin-assigned password | Log in and set their own password, or skip for now |
| S2 | Tenant Admin (`manage_users`) | Desktop, routine admin work | Find a user, edit their email or role |
| S3 | Tenant Admin | A new colleague is joining | Create a user with an initial password and a role |
| S4 | Tenant Admin | A user forgot their password | Reset it, knowing the user will be asked to change it again |
| S5 | Tenant Master | Handing over ownership | Transfer master status safely |
| S6 | Tenant Admin | Someone left the organisation | Delete the user deliberately |
| S7 | Any staff user | Any time | Change their own password from the header menu |

## User Flows

### Flow 1: Login

1. The user enters their email and password and selects "Log in" (`POST /auth/login`).
2. If `must_change_password` is true, the app opens the forced password change screen. Otherwise it opens the app (today `/users`, later the dashboard).

**Alternative / error paths:** Invalid credentials show one generic error, never saying which field is wrong. There is no "forgot password" link because no such endpoint exists.

### Flow 2: Forced password change

1. A full-screen page appears before the app shell, with New password and Confirm password fields.
2. "Set new password and continue" calls `POST /users/me/password` and then opens the app.
3. "Skip for now" opens the app without changing anything. The flag only prompts the user; it is not enforced.

**Alternative / error paths:** Mismatched or too-short passwords show inline field errors.

### Flow 3: Administer a user

1. From Users (list), the user selects a row, which opens User detail (`/users/[id]`).
2. On the detail page they can edit Email and Role and save (`PUT /users/{id}`), or run an account action: Reset password, Transfer ownership (shown only if the signed-in user is the master), or Delete user (in the Danger zone).
3. Every account action opens its own confirmation dialog before the request is sent.

**Alternative / error paths:** 404 shows "user not found". 403 hides the action. 409 on save shows the conflict message.

### Flow 4: Create a user

1. From Users, "New user" opens `/users/new`.
2. The user fills in Email, Password, and Role and selects "Create" (`POST /users`). The app then returns to the list or opens the new user's detail page.

**Alternative / error paths:** A 409 for a duplicate email shows an inline error on the Email field.

## Information Architecture

- Unauthenticated: `/login`, `/change-password`, and a public shell at `/marketing` (placeholder hero only).
- Authenticated app shell (`AppShell`): collapsible sider (220–240 px, or an 80 px icon rail) with Dashboard; Events › Transactions / Configuration › Category / Ticket Template; Guests; Staff; Incidents; Templates; **Users**; Workflow steps. The header has an avatar menu with the signed-in email, "Change password", and "Log out".
- Users: `/users` (list), `/users/new` (create), `/users/[id]` (detail). The Users item and pages are hidden for callers without `manage_users`.
- Inner pages use the shared page header: a back button, a breadcrumb (ancestors muted, current page bold), then the title with page actions on the right.

## Screens and States

### Screen: Login (`app/login/page.tsx`)

- **Purpose:** Authenticate.
- **Design:** Canvas pages "auth" and "auth-dark".
- **Main elements:** Centred card, Email input, Password input, error alert, full-width primary "Log in" button.

| State | What the user sees | Trigger |
|-------|-------------------|---------|
| Default | Empty form | Page load |
| Loading | Button shows a spinner and is disabled | Submit |
| Error | Red alert "Invalid email or password." | 401 |
| Success | Redirect | 200 |

### Screen: Forced password change (`app/change-password/page.tsx`)

- **Purpose:** Prompt the user to replace an admin-assigned password.
- **Main elements:** New and Confirm password fields, primary "Set new password and continue", text button "Skip for now".

| State | What the user sees | Trigger |
|-------|-------------------|---------|
| Default | Empty form | `must_change_password = true` |
| Loading | Primary button spinner | Submit |
| Error | Field errors or an alert | Validation error or 400 |
| Success | App shell | 200 |

### Screen: User list (`app/users/page.tsx`)

- **Purpose:** Find users; view only.
- **Main elements:** Search (name or email); "Filter" and "Sort" buttons with count badges that open popover builders; primary "New user"; a table with **Name** (gold dot for tenant master, small dot for must-change-password), **Email**, **Role** tag, **Created date**, **Updated date**. There are no row-action icons; the whole row is a link.

| State | What the user sees | Trigger |
|-------|-------------------|---------|
| Default | Paginated table (server-side pagination, sort, filter) | Load |
| Loading | Table skeleton or spinner | Fetch |
| Empty | Empty state with "New user" | No users or no matches |
| Error | Error alert with retry | 4xx or 5xx |
| Permission denied | Page and nav item hidden | No `manage_users` |

### Screen: Create user (`app/users/new/page.tsx`)

- **Purpose:** Create an account.
- **Main elements:** Email, Password (the only screen with a password field for someone else), Role select (Super Admin / Tenant Admin / Tenant Staff, hardcoded), Cancel and Create. There is no tenant-master field.

| State | What the user sees | Trigger |
|-------|-------------------|---------|
| Default | Empty form | Load |
| Loading | Create button spinner | Submit |
| Error | Field errors; duplicate email error on Email | 400 or 409 |
| Success | User detail page or list with a success message | 201 |

### Screen: User detail (`app/users/[id]/page.tsx`)

- **Purpose:** Edit a user and run account actions.
- **Main elements:** Header (avatar, email, Role / Master / Must-change tags); an Email and Role form with Save and Cancel (no password field; tenant master shown as a read-only tag); "Account actions": Reset password, Transfer ownership (only for the current master); "Danger zone" card: Delete user.

| State | What the user sees | Trigger |
|-------|-------------------|---------|
| Default | Loaded user | Load |
| Loading | Skeleton | Fetch |
| Not found | "User not found" with a back link | 404 |
| Saving | Save button spinner | Submit |
| Error | Alert or field error | 400 or 409 |

### Screen: Dialogs (`_components/ResetPasswordModal`, `TransferMasterModal`, `DeleteUserModal`, `app/_components/ChangeMyPasswordModal`)

- **Purpose:** Confirm account actions one at a time.
- **Main elements:**
  - Reset password: warning alert that the user will be asked to change it again at next login; new-password field with a "Generate" button (random string on the client, shown in IBM Plex Mono). Calls `POST /users/{id}/password`.
  - Transfer ownership: error alert warning that the change is immediate; target select (the tenant's other active users); confirmation checkbox that enables the red submit button. Calls `POST /users/{id}/transfer-master`.
  - Delete user: error alert and an explicit "are you sure" line. Calls `DELETE /users/{id}`.
  - Change my password: new and confirm fields. Calls `POST /users/me/password`. This is a separate component from Reset password (ADR-009).

| State | What the user sees | Trigger |
|-------|-------------------|---------|
| Default | Dialog open, submit enabled when valid | Button click |
| Loading | Submit spinner, dialog stays open | Submit |
| Error | Inline alert in the dialog | 4xx or 5xx |
| Success | Dialog closes; message shown; data refreshed | 200 or 204 |

## Interaction and Validation Rules

| Element | Rule | Error message |
|---------|------|---------------|
| Email (login, create, edit) | Required, valid email | Not specified in the design (Q4) |
| Password (create, reset, own) | Required, at least 8 characters (backend rule) | Not specified (Q4) |
| Confirm password | Must match | Not specified (Q4) |
| Role select | Required on create; only system-scoped roles | — |
| Transfer submit | Disabled until a target is selected and the checkbox is ticked | — |
| Every mutating account action | Needs its own confirmation dialog; reachable only from the detail page | — |
| Tenant master | Never editable in forms; changes only through Transfer | — |
| List rows | Navigate only; never change data | — |
| Session | Attach the Bearer token; on 401, call `/auth/refresh` once, then send the user to login | — |

## Content and Copy

| Key | Text | Notes |
|-----|------|-------|
| login.submit | Log in | — |
| login.error | Invalid email or password. | Same text for every credential error |
| forcedChange.submit | Set new password and continue | — |
| forcedChange.skip | Skip for now | Muted text button |
| users.new | New user | — |
| users.actions.reset | Reset password | — |
| users.actions.transfer | Transfer ownership | Only for the current master |
| users.actions.delete | Delete user | Danger zone |
| transfer.title | Transfer tenant master ownership | — |
| transfer.warning | You are the current tenant master. Confirming hands is_tenant_master to the user below and clears your own — effective immediately, only reversible by the new master transferring it back. | As implemented; the wording exposes the field name `is_tenant_master` (Q4) |
| transfer.confirm | I understand I will immediately lose tenant-master privileges. | Checkbox label |
| header.menu | Change password / Log out | Avatar dropdown |

Only English copy exists. The localisation need is unknown (Q5).

## Accessibility

- **Target:** Not formally set. WCAG 2.2 AA is proposed (PRD-001 Q8).
- Theme token pairs were chosen for readability in both themes. Contrast has not been measured against WCAG.
- Danger actions don't rely on colour alone: they have explicit labels, warning text, and a confirmation step.
- The tenant-master and must-change indicators are dots with a `title` only. They need an accessible text alternative (Q6).
- The collapsed icon-only sider needs accessible labels and tooltips for each item.

## Responsive Behaviour

| Breakpoint | Layout / behaviour |
|------------|--------------------|
| Mobile (< 768 px) | Not designed (Q7) |
| Tablet | Not designed; the sider can collapse to an 80 px rail |
| Desktop (≥ 1024 px) | Designed: sider of 220–240 px, content on the canvas colour |

Theme tokens (DESIGN_SYSTEM.md §1, Direction D "Full Dark Elevated"): canvas `#111116` / `#EEEFF5`, chrome `#1A1A22` / `#F8F8FC`, pop `#212129` / `#FFFFFF`, border `#2A2A34` / `#E2E2EC`, text `#F1F1F5` / `#17171F`, primary `#4F46E5`, accent teal `#2DD4BF` / `#0F766E`. Danger is outlined in dark mode and filled in light mode. Fonts are IBM Plex Sans and IBM Plex Mono.

## Analytics and Tracking

Not applicable for now: PRD-001 defines no product analytics, so no events are specified. Revisit when PRD-001 Q2 (success measurement) is answered.

## Open Questions

| # | Question | Owner | Due | Blocking? |
|---|----------|-------|-----|-----------|
| Q1 | The backend has no `name` on users, but the list, search, filter, and sort depend on it. Add it (migration, DTO, API)? | Bandana Irmal A | Before API integration | Yes |
| Q2 | `user_id` on `TokenPair` is designed but not implemented. Add it, or have the frontend decode `sub` from the token? | Bandana Irmal A | Before API integration | No |
| Q3 | Role options are hardcoded because there is no `GET /roles`. Add a lookup endpoint? The frontend mock uses fake role ids, so real seeded UUIDs are needed. | Bandana Irmal A | Before API integration | Yes |
| Q4 | What is the exact validation error copy for each field? | Bandana Irmal A | Not set | No |
| Q5 | Is localisation needed (for example Indonesian)? | Bandana Irmal A | Not set | No |
| Q6 | What accessible text should the Name-cell dot indicators have? | Bandana Irmal A | Not set | No |
| Q7 | Is mobile or tablet support needed for admin screens? | Bandana Irmal A | Not set | No |
| Q8 | The filter operators `contains`, `>`, `<`, and `between` on dates are designed, but the backend supports only `eq` and `like` filters, with no date ranges. Extend the backend or reduce the UI? | Bandana Irmal A | Before API integration | Yes |
| Q9 | Where are tokens stored (memory, cookie, localStorage), and what happens on logout, given that the backend has no revocation? | Bandana Irmal A | Before API integration | Yes |
| Q10 | Where should the user land after login once the dashboard exists? | Bandana Irmal A | Not set | No |

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Satisfies:** [[PRD-001-guest-management-platform|PRD-001 · Guest Management Platform]]
- **Depends on:** [[ADR-009-separate-self-service-me-routes-from-admin-routes|ADR-009 · Separate self-service /me routes from admin routes]]
- **References:** [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]]
<!-- relations:end -->
