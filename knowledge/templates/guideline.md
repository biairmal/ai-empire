---
type: guideline
id: NEW
title: "{{ Guideline name, e.g. End-to-end testing }}"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
depends_on: []
references: []
---

# {{ Guideline name }}

<!--
Engineering Guideline
Answers: which rule or tool must every task in this scope use?
Project guidelines live in projects/<slug>/guidelines/ and are loaded into EVERY task of that project,
so keep them short and specific: the tool, where things live, and the rules. Role files stay
tool-agnostic and defer to guidelines like this one. For client, stack, or global guidelines,
replace `project:` with `client:` or `stack:` (or remove it for global).
-->

## Applies To

<!-- Which roles and kinds of work this guideline governs. -->

{{ e.g. Tester role; any task that adds or changes end-to-end tests. }}

## Tools

<!-- The tools to use, with versions and where their files live. Never include secrets. -->

| Tool | Purpose | Location |
|------|---------|----------|
| {{ Tool }} | {{ Purpose }} | {{ Path or link }} |

## Rules

<!-- Numbered, checkable rules. -->

1. {{ Rule }}

## How to Run

<!-- Exact commands, or "Not applicable". -->

{{ Command }}
