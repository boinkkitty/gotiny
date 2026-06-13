# Authentication Flow

## Login / Register

```
Client                    API Gateway              User Service              Postgres
  │                           │                         │                       │
  │ POST /login               │                         │                       │
  │ {email, password}         │                         │                       │
  │──────────────────────────▶│                         │                       │
  │                           │ gRPC Login()            │                       │
  │                           │────────────────────────▶│                       │
  │                           │                         │ GetUserByEmail()      │
  │                           │                         │──────────────────────▶│
  │                           │                         │◀──────────────────────│
  │                           │                         │                       │
  │                           │                         │ bcrypt.Compare()      │
  │                           │                         │ (password check)      │
  │                           │                         │                       │
  │                           │                         │ issueTokens()         │
  │                           │                         │  ├─ sign JWT (15min)  │
  │                           │                         │  └─ StoreRefreshToken │
  │                           │                         │──────────────────────▶│
  │                           │                         │◀──────────────────────│
  │                           │◀────────────────────────│                       │
  │ {access_token,            │                         │                       │
  │  refresh_token,           │                         │                       │
  │  expires_in: 900}         │                         │                       │
  │◀──────────────────────────│                         │                       │
```

## Authenticated Request

Access token validation is local (HMAC signature check in the gateway), so every
authenticated request avoids a round trip to user-service.

```
Client                    API Gateway                URL Service
  │                           │                         │
  │ POST /shorten             │                         │
  │ Authorization: Bearer AT  │                         │
  │──────────────────────────▶│                         │
  │                           │ JWTMiddleware:          │
  │                           │  parse token locally    │
  │                           │  extract sub (user_id)  │
  │                           │  (no DB call)           │
  │                           │                         │
  │                           │ gRPC CreateShortURL()   │
  │                           │ x-user-id in metadata   │
  │                           │────────────────────────▶│
  │                           │◀────────────────────────│
  │ {short_url}               │                         │
  │◀──────────────────────────│                         │
```

## Refresh (access token expired)

Each refresh token is single-use. RT1 gets revoked before RT2 is issued. If an
attacker replays RT1 after you've already refreshed, it's dead.

```
Client                    API Gateway              User Service              Postgres
  │                           │                         │                       │
  │ POST /refresh             │                         │                       │
  │ {refresh_token: RT1}      │                         │                       │
  │──────────────────────────▶│                         │                       │
  │                           │ gRPC RefreshToken()     │                       │
  │                           │────────────────────────▶│                       │
  │                           │                         │ GetRefreshToken(RT1)  │
  │                           │                         │──────────────────────▶│
  │                           │                         │◀──────────────────────│
  │                           │                         │                       │
  │                           │                         │ RevokeRefreshToken()  │
  │                           │                         │ (RT1 is now dead)     │
  │                           │                         │──────────────────────▶│
  │                           │                         │◀──────────────────────│
  │                           │                         │                       │
  │                           │                         │ issueTokens()         │
  │                           │                         │  ├─ sign new JWT      │
  │                           │                         │  └─ store RT2         │
  │                           │                         │──────────────────────▶│
  │                           │                         │◀──────────────────────│
  │                           │◀────────────────────────│                       │
  │ {access_token: AT2,       │                         │                       │
  │  refresh_token: RT2}      │                         │                       │
  │◀──────────────────────────│                         │                       │
```

## Logout

```
Client                    API Gateway              User Service              Postgres
  │                           │                         │                       │
  │ POST /logout              │                         │                       │
  │ {refresh_token: RT2}      │                         │                       │
  │──────────────────────────▶│                         │                       │
  │                           │ gRPC Logout()           │                       │
  │                           │────────────────────────▶│                       │
  │                           │                         │ RevokeRefreshToken()  │
  │                           │                         │ (RT2 deleted from DB) │
  │                           │                         │──────────────────────▶│
  │                           │                         │◀──────────────────────│
  │                           │◀────────────────────────│                       │
  │ 204 No Content            │                         │                       │
  │◀──────────────────────────│                         │                       │
```
