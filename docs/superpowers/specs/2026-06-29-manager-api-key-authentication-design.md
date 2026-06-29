# Manager API Key Authentication Design

## Goal

Allow both the configured `GLOBAL_API_KEY` and active company API keys to log
in through `/manager/login`, including after an explicit logout. Authentication
must remain stable when a key is pasted with surrounding whitespace.

## Current Behavior and Root Cause

The Manager validates a login by calling `GET /instance/all`. Company creation
trims an API key before hashing it, but authentication validates and hashes the
raw header value. The Manager also sends the raw form value. Therefore, a key
copied with surrounding whitespace is stored under one hash and authenticated
under another; the same mismatch affects the direct comparison with
`GLOBAL_API_KEY`.

The global-key branch also calls `EnsureDefaultCompany` during every request.
That operation can write to the database and reports any failure as an
authentication failure. A valid key can consequently receive an indistinguishable
401 response when the actual problem is default-company synchronization or the
database.

## Design

### Canonical API Keys

The Manager will trim the submitted API key before license checks, login, and
persistence. The backend will independently trim the `apikey` header before any
comparison or hash lookup. Company creation and authentication will therefore
use the same canonical representation.

Backend normalization is authoritative because API clients other than the
Manager must receive the same behavior. Frontend normalization prevents storing
or repeatedly displaying a malformed value.

### Global and Company Authentication

`AuthAdmin` will keep two explicit paths:

1. A canonical key equal to `GLOBAL_API_KEY` authenticates as master. When
   `X-Company-Id` is present, that company remains the selected tenant. Without
   the header, the request uses the existing default company.
2. Any other canonical key is looked up as an active company API key and is
   scoped to that company.

The request path will perform a read-only lookup of the default company. Creation,
reactivation, and API-key synchronization of the default company remain startup
migration responsibilities and will not run during login.

No new login endpoint will be introduced. `/instance/all` remains the Manager's
authentication probe to keep the change focused and compatible with the current
UI.

### Error Handling

Missing or unknown keys return HTTP 401 with the existing public error shape.
Repository or default-company lookup failures return HTTP 500 and are logged
without including plaintext API keys. The Manager will use the normalized error
status from the Axios interceptor so it can distinguish invalid credentials from
connection or server failures.

Logout continues to clear local authentication state. It does not revoke or
modify global or company API keys on the server.

## Testing

Regression coverage will verify:

- the global key authenticates before and after client-side logout;
- surrounding whitespace is ignored for global-key authentication;
- an active company key authenticates and scopes `/instance/all` to its company;
- surrounding whitespace is ignored for company-key authentication;
- an invalid key still returns 401;
- a default-company repository failure is not reported as an invalid key;
- the Manager normalizes the submitted key and does not erase credentials in
  response to unrelated 401 responses.

The focused middleware and Manager tests will be run first, followed by the
available Go and JavaScript regression suites.

## Out of Scope

- Changing instance-token authentication for messaging and call endpoints.
- API-key rotation or revocation features.
- Replacing the existing Manager bundle toolchain.
- Changing license activation behavior.
