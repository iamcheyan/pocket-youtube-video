# Authentication and credential security

## Flows

### Google OAuth authorization code with PKCE

`POST /api/auth/oauth` starts a flow with a fresh state value, a 48-byte random verifier, and an S256 challenge. The authorization URL targets Google with a loopback redirect on an ephemeral `127.0.0.1` port. The local callback verifies the state and source address, exchanges the code at the Google token endpoint with the verifier and redirect URI, and stores the resulting credentials through `EncryptedStore`. The session is single-use. Configure `GOOGLE_OAUTH_CLIENT_ID` and optionally `GOOGLE_OAUTH_CLIENT_SECRET`; without a client ID the authorization URL is not a usable login and the UI reports authentication as unconfigured.

This implementation was not exercised against Google in this environment (no account or client credentials). It is protocol-shaped, not a claim of a successful real login.

### Cookie fallback

`POST /api/auth/cookie` accepts a browser cookie string and account label. It creates an account and stores the cookie only as ciphertext. This is suitable for yt-dlp-compatible cookie export, but importing a real browser cookie jar and authenticated network fetch were not tested.

## Encryption format

`FileCredentialStore` uses AES-256-GCM with a 32-byte random key. Each record concatenates a nonce and GCM ciphertext. The account ID is used as associated data, so copying a ciphertext to another account fails authentication. SQLite `oauth_tokens` stores the packed nonce+ciphertext; no plaintext access/refresh token or cookie is written to the DB or returned by handlers. `Secret` has `json:"-"` fields to prevent accidental serialization.

The key path is `POCKET_YOUTUBE_AUTH_KEY`, defaulting under the data directory. If absent, a random key file is created with mode 0600 (and its parent with 0700). Existing key files with group/other permissions or wrong lengths are rejected rather than silently repaired. **This is local encryption, not an OS keyring.** Anyone who can read the key can decrypt the credentials. Back up/restore the key and DB together; losing the key makes stored credentials unusable.

## Interfaces and future keyring

`CredentialStore` exposes `Put(accountID, Secret) (Ciphertext, error)`, `Get(accountID, Ciphertext) (Secret, error)`, `Delete(accountID) error` and `List() ([]string, error)`. The default implementation is `FileEncryptedStore` (AES-256-GCM, account ID as AAD, 0600 key file); `FileCredentialStore` remains as a type alias.

`SystemKeyringStore` is declared for a future PJOS keyring integration and deliberately fails loudly (`system keyring store is not implemented`) rather than falling back to plaintext. A real implementation should keep ciphertext in SQLite while placing the data-encryption key in a session/OS-protected keyring, with an explicit opt-in or migration.

## Boundaries

- Error responses use generic messages for credential operations; they do not echo cookies, tokens or key contents. Handlers avoid logging credential input.
- `GET /api/accounts` reports account metadata and authentication type, never secret material.
- Logout deactivates accounts; removal deletes the account row, cascading associated encrypted tokens and catalog data.
- The front end displays an explicit "not configured" state when no non-demo account is authenticated.
- Authentication tests cover encryption round-trip, `Put/Get/Delete/List` contract behaviour, key permission enforcement, account binding, JSON omission, and HTTP responses/DB bytes free of a runtime-generated canary cookie. A guard test fails if a canary ever looks placeholder-substituted, so the "no plaintext" assertions cannot silently become vacuous. They do not prove Google/OAuth success.
