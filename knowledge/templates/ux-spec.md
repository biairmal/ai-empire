---
type: ux-spec
id: NEW
title: "{{ Feature name }} — UX Specification"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
satisfies: ["{{ PRD-00X }}"]
depends_on: []
references: []
---

# {{ Feature name }} — UX Specification

<!--
UX / UI Specification
Answers: HOW should the user experience the feature?
It must satisfy an approved PRD (set `satisfies` above). Link or embed wireframes and designs
(Figma, images in this folder). Replace every {{ … }} placeholder.
-->

## Summary

<!-- What experience are we designing, for which PRD requirements, and what does "good" feel like for the user? -->

{{ Short summary of the experience and the requirements it covers (e.g. FR-01–FR-04). }}

## Users and Scenarios

<!-- The key scenarios, told from the user's perspective, with their context (device, environment, time pressure). -->

| Scenario | Persona | Context | Goal |
|----------|---------|---------|------|
| S1 | {{ Persona }} | {{ Where / when / device }} | {{ What they want to achieve }} |

## User Flows

<!-- Step-by-step flows, including alternative and error paths. A simple numbered list or a diagram is fine. -->

### Flow 1: {{ Flow name }}

1. {{ Step }}
2. {{ Step }}

**Alternative / error paths:** {{ What happens when something goes wrong }}

## Information Architecture

<!-- Navigation, page hierarchy, where the feature lives, and how users reach it. -->

{{ Navigation structure and entry points. }}

## Screens and States

<!-- For every screen: purpose, main elements, and ALL states (default, loading, empty, error, success, disabled, permission denied). Link designs. -->

### Screen: {{ Screen name }}

- **Purpose:** {{ Purpose }}
- **Design:** {{ Link to Figma / image }}
- **Main elements:** {{ Elements }}

| State | What the user sees | Trigger |
|-------|-------------------|---------|
| Default | {{ Description }} | {{ Trigger }} |
| Loading | {{ Description }} | {{ Trigger }} |
| Empty | {{ Description }} | {{ Trigger }} |
| Error | {{ Description }} | {{ Trigger }} |

## Interaction and Validation Rules

<!-- Field rules, formats, limits, confirmation dialogs, undo, keyboard behaviour, timeouts. -->

| Element | Rule | Error message |
|---------|------|---------------|
| {{ Field / control }} | {{ Rule }} | {{ Exact message shown }} |

## Content and Copy

<!-- Final UI text: labels, buttons, empty states, errors, notifications. Note tone of voice and localisation needs. -->

| Key | Text | Notes |
|-----|------|-------|
| {{ ui.key }} | {{ Exact text }} | {{ Tone / translation notes }} |

## Accessibility

<!-- Target WCAG 2.2 level AA unless the contract says otherwise. Cover keyboard navigation, focus order, screen reader labels, contrast, motion, and touch target sizes. -->

- **Target:** WCAG 2.2 AA
- {{ Specific accessibility requirement for this feature }}

## Responsive Behaviour

<!-- Behaviour per breakpoint and device (mobile, tablet, desktop), orientation, and offline or poor-network behaviour if relevant. -->

| Breakpoint | Layout / behaviour |
|------------|--------------------|
| Mobile (< 768 px) | {{ Behaviour }} |
| Desktop (≥ 1024 px) | {{ Behaviour }} |

## Analytics and Tracking

<!-- Events needed to measure the PRD success metrics. Respect privacy: no personal data in event properties unless approved. -->

| Event | Trigger | Properties | Supports metric |
|-------|---------|------------|-----------------|
| {{ event_name }} | {{ When }} | {{ Properties }} | {{ G1 }} |

## Open Questions

| # | Question | Owner | Due | Blocking? |
|---|----------|-------|-----|-----------|
| Q1 | {{ Question }} | {{ Owner }} | {{ Date }} | {{ Yes / No }} |
