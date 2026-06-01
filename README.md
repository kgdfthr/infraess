# infraess

A self-hosted infrastructure stack running behind Traefik with centralized authentication via authentik.

## Services

| Service | URL | Description |
|---|---|---|
| Traefik | `traefik.infraess.com` | Reverse proxy and TLS termination |
| authentik | `sso.infraess.com` | Identity provider and forward auth |
| Demo API | `infraess.com` | Role-based REST API with frontend |
| Grafana | `grafana.infraess.com` | Log visualization |

## Architecture

Traefik is the single entry point for all traffic. Every service sits behind it. Two authentication patterns are in use depending on the service type:

### Forward auth (traefik middleware)

Used for services that have no native authentication. Traefik intercepts every request and asks authentik: *"is this session valid?"*. If not, the user is redirected to the authentik login page. On success, authentik injects headers into the request before it reaches the service.

```
Browser → Traefik → authentik (is this valid?)
                      ↓ yes: inject X-authentik-* headers
                    Traefik → Service
```

Services using forward auth: **Traefik dashboard**, **Demo API**

The service receives these headers on every authenticated request:
- `X-authentik-username`
- `X-authentik-groups` — pipe-separated list of group names
- `X-authentik-email`
- `X-authentik-name`

The service itself is responsible for reading `X-authentik-groups` and enforcing fine-grained access (what a user can do once inside).

### Native OAuth2/OIDC

Used for services that have built-in OAuth2 support. The service handles the login flow itself — it redirects the user to authentik, receives a token, and reads claims from it. Traefik is not involved in the auth flow.

```
Browser → Grafana (not logged in)
  → redirect to sso.infraess.com/authorize
  → user logs in at authentik
  → redirect back to Grafana with auth code
  → Grafana exchanges code for token
  → Grafana reads groups claim → maps to internal roles
```

Services using native OAuth2: **Grafana**

## Access control model

Access is modelled with authentik **Groups**. Groups (not Roles) are used because groups are what authentik passes to external services via the `X-authentik-groups` header and OAuth2 token claims.

### Global groups

Control access to infrastructure services.

| Group | Access |
|---|---|
| `role:reader` | Can read resources in the Demo API |
| `role:writer` | Can read and create resources in the Demo API |
| `role:admin` | Full access — Demo API, Traefik dashboard |

### Grafana-specific groups

Grafana has its own internal role system (Viewer, Editor, Admin) that does not map 1:1 to the global roles. These groups control both access to Grafana and the role assigned within it.

| Group | Grafana role |
|---|---|
| `role:grafana-reader` | Viewer |
| `role:grafana-writer` | Editor |
| `role:grafana-admin` | Admin |

A user with no grafana group cannot log into Grafana at all (`ROLE_ATTRIBUTE_STRICT` is enabled).

### Assigning access

A user gets assigned a global role and any service-specific roles they need. Examples:

```
john  →  role:reader                          (Demo API read-only, no Grafana)
jane  →  role:writer, role:grafana-writer     (Demo API write, Grafana editor)
ops   →  role:admin,  role:grafana-admin      (full access everywhere)
```

---

## Setup

### Prerequisites

- Docker and Docker Compose
- TLS certificates for your domain placed in `configs/traefik/certs/local.crt` and `configs/traefik/certs/local.key`
- DNS or `/etc/hosts` entries pointing your domains to the host

### 1. Configure environment

```bash
cp .env.example .env
```

Edit `.env` and set all values. Generate secrets with:

```bash
openssl rand -base64 60   # AUTHENTIK_SECRET_KEY
```

### 2. Start the stack

```bash
docker compose up -d
```

Wait for authentik to become healthy (about 60 seconds on first boot):

```bash
docker compose logs -f authentik-server
```

### 3. authentik initial setup

Navigate to:
```
https://sso.infraess.com/if/flow/initial-setup/
```

Set the password for the `akadmin` account. Then go to the admin interface at `https://sso.infraess.com/if/admin/`.

### 4. Create groups

Go to **Directory → Groups** and create the following groups one by one:

**Global roles:**
- `role:reader`
- `role:writer`
- `role:admin`

**Grafana roles:**
- `role:grafana-reader`
- `role:grafana-writer`
- `role:grafana-admin`

### 5. Configure the Demo API (forward auth)

#### Create a Proxy Provider

Go to **Applications → Providers → Create**, select **Proxy Provider**:

| Field | Value |
|---|---|
| Name | `demo-proxy` |
| Authentication flow | `default-authentication-flow` |
| Authorization flow | `default-provider-authorization-implicit-consent` |
| Type | Forward auth (single application) |
| External host | `https://infraess.com` |

#### Create an Application

Go to **Applications → Applications → Create**:

| Field | Value |
|---|---|
| Name | `demo` |
| Slug | `demo` |
| Provider | `demo-proxy` |

#### Add to the Embedded Outpost

Go to **Applications → Outposts**, edit the embedded outpost, and add `demo` to the selected applications.

### 6. Enable self-registration

#### Create enrollment stages

Go to **Flows & Stages → Stages → Create** and create these three stages in order:

**Prompt Stage**

| Field | Value |
|---|---|
| Name | `enrollment-prompt` |
| Fields | `username`, `email`, `password`, `password_repeat` |

**User Write Stage**

| Field | Value |
|---|---|
| Name | `enrollment-user-write` |
| Create users as inactive | unchecked |

**User Login Stage**

| Field | Value |
|---|---|
| Name | `enrollment-login` |

#### Create the enrollment flow

Go to **Flows & Stages → Flows → Create**:

| Field | Value |
|---|---|
| Name | `enrollment-flow` |
| Title | `Sign Up` |
| Slug | `enrollment-flow` |
| Designation | `Enrollment` |

Open the flow and bind the stages in order:

| Stage | Order |
|---|---|
| `enrollment-prompt` | 10 |
| `enrollment-user-write` | 20 |
| `enrollment-login` | 30 |

#### Link enrollment to the login page

Go to **Flows & Stages → Stages**, edit `default-authentication-identification`, and set **Enrollment flow** to `enrollment-flow`. A "Sign up" link will now appear on the login page.
