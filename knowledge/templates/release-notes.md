---
type: release-notes
id: NEW
title: "{{ Product name }} {{ version }} — Release Notes"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
release: "{{ v1.2.0 }}"
release_date: "{{ YYYY-MM-DD }}"
documents: []
derived_from: []
references: []
---

# {{ Product name }} {{ version }} — Release Notes

<!--
Release Notes
Answers: WHAT changed in this release, and WHAT do users need to do?
Audience is end users and client stakeholders: plain language, benefits first, no internal
jargon or ticket numbers without context. Versioning follows Semantic Versioning (semver.org).
Replace every {{ … }} placeholder; write "None" for empty sections.
-->

## Release Summary

| Item | Value |
|------|-------|
| Version | {{ v1.2.0 }} |
| Release date | {{ YYYY-MM-DD }} |
| Type | {{ Major / minor / patch }} |

{{ Two or three sentences on what this release brings. }}

## Highlights

<!-- The most valuable new capabilities, described by user benefit. -->

- **{{ Feature name }}** — {{ What users can now do and why it helps }}

## Improvements

- {{ Improvement }}

## Bug Fixes

- {{ Fixed: description of the problem users experienced }}

## Breaking Changes and Migration

<!-- Anything that requires action from users or integrators. Give step-by-step migration instructions. -->

{{ Breaking change and how to migrate, or "None". }}

## Known Issues

| Issue | Workaround | Planned fix |
|-------|------------|-------------|
| {{ Issue }} | {{ Workaround }} | {{ Version / date }} |

## Upgrade Instructions

<!-- For self-hosted or integrator audiences: how to upgrade. For hosted services, what users will notice. -->

{{ Upgrade steps, or "No action required." }}
