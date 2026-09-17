---
type: api-spec
id: NEW
title: "{{ API name }} — API Specification"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
api_version: "{{ v1 }}"
openapi: "{{ Path or URL to the OpenAPI 3.1 file, or 'none' }}"
implements: []
depends_on: []
references: []
---

# {{ API name }} — API Specification

<!--
API Specification
Answers: WHAT contract does the system expose?
If an OpenAPI 3.1 file exists it is the machine-readable source of truth; this document
explains conventions, behaviour, and examples for humans and client integrators.
Replace every {{ … }} placeholder. Never put real credentials in examples.
-->

## Overview

<!-- Who the API is for, what it lets them do, base URLs per environment. -->

| Environment | Base URL |
|-------------|----------|
| Production | {{ https://api.example.com/v1 }} |
| Staging | {{ https://staging-api.example.com/v1 }} |

{{ Short description of the API's purpose and consumers. }}

## Conventions

<!-- Formats and rules that apply to every endpoint. -->

| Topic | Convention |
|-------|------------|
| Format | JSON (`application/json`), UTF-8 |
| Naming | {{ snake_case / camelCase }} field names |
| Dates and times | RFC 3339 / ISO 8601, UTC (e.g. `2026-01-31T13:45:00Z`) |
| Identifiers | {{ UUID v4 / ULID / integer }} |
| Pagination | {{ Cursor (`cursor`, `limit`) / page-based }} |
| Filtering and sorting | {{ Query parameter conventions }} |
| Idempotency | {{ `Idempotency-Key` header on unsafe requests, retention period }} |
| Rate limits | {{ Limits and `429` behaviour, headers returned }} |

## Authentication and Authorization

<!-- How clients authenticate (OAuth 2.0, API key, JWT), token lifetimes, scopes/roles, and what each role may call. -->

- **Authentication:** {{ Scheme, header, token lifetime }}
- **Authorization:** {{ Roles or scopes }}

| Role / scope | Allowed operations |
|--------------|--------------------|
| {{ Role }} | {{ Operations }} |

## Endpoints

<!-- One subsection per endpoint. Keep examples realistic but fictional. -->

### {{ METHOD }} {{ /resource }}

{{ What the endpoint does. }}

- **Auth:** {{ Required role / scope }}
- **Idempotent:** {{ Yes / No }}

**Request**

| Parameter | In | Type | Required | Description |
|-----------|----|------|----------|-------------|
| {{ name }} | {{ path / query / body / header }} | {{ type }} | {{ Yes / No }} | {{ Description }} |

```json
{{ Example request body }}
```

**Responses**

| Status | Meaning | Body |
|--------|---------|------|
| 200 | {{ Success meaning }} | {{ Model }} |
| 400 | Validation error | Problem Details |
| 401 | Not authenticated | Problem Details |
| 403 | Not allowed | Problem Details |
| 404 | Not found | Problem Details |

```json
{{ Example success response }}
```

## Data Models

<!-- Shared schemas used by the endpoints. -->

### {{ ModelName }}

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| {{ field }} | {{ type }} | {{ Yes / No }} | {{ Format, length, enum }} | {{ Description }} |

## Errors

<!-- Error responses follow RFC 9457 "Problem Details for HTTP APIs". List every application error code clients may need to handle. -->

```json
{
  "type": "{{ https://docs.example.com/errors/validation }}",
  "title": "{{ Validation failed }}",
  "status": 400,
  "detail": "{{ Human-readable explanation }}",
  "instance": "{{ /requests/abc123 }}"
}
```

| Code / type | HTTP status | When it happens | Client action |
|-------------|-------------|-----------------|---------------|
| {{ code }} | {{ status }} | {{ Cause }} | {{ What the client should do }} |

## Versioning and Compatibility

<!-- Versioning scheme, what counts as a breaking change, deprecation policy and notice period (e.g. `Deprecation` and `Sunset` headers). -->

- **Scheme:** {{ URL path version (`/v1`) / header }}
- **Breaking changes:** {{ Definition and policy }}
- **Deprecation:** {{ Notice period and communication channel }}

## Change Log

| API version | Date | Change | Breaking? |
|-------------|------|--------|-----------|
| {{ v1.0 }} | {{ YYYY-MM-DD }} | {{ Initial release }} | {{ No }} |
