# Security

## UI authentication

The UI (`cmd/ui`) always requires authentication unless you explicitly
disable it. Pick a mode with `UI_AUTH_MODE`:

| Mode | Use when | Identity comes from |
|------|----------|---------------------|
| `login` (default) | Self-hosted, or anywhere without an identity-aware proxy | Login form, then a signed session cookie |
| `iap` | Running on Google Cloud behind Identity-Aware Proxy | IAP's signed JWT on each request |
| `none` | Throwaway local development only | Nobody (every request is `anonymous`) |

The UI refuses to start if the selected mode is misconfigured, for example
`login` without a users file or `iap` without an audience. A misconfigured
mode never quietly falls back to `none`.

`GET /healthz` and `/static/` are always unauthenticated so load balancer
health checks work. Neither serves any telemetry data.

### Login mode

| Variable | Required | Description |
|----------|----------|-------------|
| `UI_USERS_FILE` | yes | One `username:bcrypt-hash` per line. `#` comments are allowed. |
| `UI_SESSION_KEY` | recommended | HMAC key for session cookies, at least 32 bytes. Every replica must share it. If it's unset, a random key is generated at startup, so sessions are lost on restart and don't work across replicas. |
| `UI_SESSION_TTL` | no | Session lifetime, `8h` by default. |
| `UI_COOKIE_INSECURE` | no | `true` lets the cookie travel over plain HTTP. Use it only for local compose. |

Create a users file entry with the UI binary itself. The runtime image has no
shell, so this works without extra tools:

```bash
printf '%s' "$PASSWORD" | go run ./cmd/ui hash-password    # from source
printf '%s' "$PASSWORD" | docker run --rm -i conduit-ui hash-password
echo "alice:<hash>" >> deployments/docker/ui-users          # gitignored
```

`htpasswd -nB alice` output works too.

How login is protected:
- Passwords are checked with bcrypt. When a username doesn't exist, the UI
  still compares against a dummy hash, so response timing doesn't reveal
  which usernames exist.
- Sessions are stateless and signed with HMAC-SHA256. Nothing is stored
  server-side, so a session can't be revoked before it expires. To end
  every session at once, rotate `UI_SESSION_KEY`.
- The session cookie is `__Host-conduit_session`, with `HttpOnly`, `Secure`,
  `SameSite=Lax` and `Path=/` set.
- `POST /login` and `POST /logout` reject cross-site requests, checking
  `Sec-Fetch-Site` and falling back to `Origin`. This blocks login CSRF and
  forced logout.
- Login attempts are rate limited to 5 per minute for each username and
  each client IP.
- The `next` redirect parameter accepts only local paths, which prevents
  open redirects.

### IAP mode (Google Cloud)

| Variable | Required | Description |
|----------|----------|-------------|
| `UI_IAP_AUDIENCE` | yes | `/projects/PROJECT_NUMBER/global/backendServices/SERVICE_ID` for a load balancer backend, or `/projects/PROJECT_NUMBER/apps/PROJECT_ID` for App Engine. |

On every request, the UI checks the `x-goog-iap-jwt-assertion` header:
- **Signature:** must be ES256 and verify against Google's published keys at
  `https://www.gstatic.com/iap/verify/public_key-jwk`. The keys are cached
  for an hour and refetched when a new key ID appears, at most once per
  minute.
- **Claims:** the issuer must be `https://cloud.google.com/iap`, the
  audience must match `UI_IAP_AUDIENCE`, and the token must not be expired.

It never trusts the unsigned `x-goog-authenticated-user-email` header. A
request that reaches the pod without passing through IAP, such as one from
inside the cluster or through a misconfigured load balancer, is rejected.

Clicking **Sign out** redirects to `/_gcp_iap/clear_login_cookie`. That's
IAP's own logout endpoint.

Use `deployments/k8s/gke-iap.yaml` to set this up on GKE: it has the
BackendConfig with IAP enabled, the health check on `/healthz`, a ClusterIP
Service, and an Ingress. Access is granted with
`roles/iap.httpsResourceAccessor`.

### Browser hardening

Every response carries a strict Content-Security-Policy that allows only
same-origin scripts and styles, with no inline script and
`frame-ancestors 'none'`. Responses also set `X-Content-Type-Options: nosniff`,
`X-Frame-Options: DENY` and `Referrer-Policy: no-referrer`.

Templates must not contain inline event handlers, `<script>` blocks or
`style` attributes. `TestTemplatesHaveNoInlineScript` enforces this.
