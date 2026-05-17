# identity

Authentication primitives for Mataki Go services. **Identification
only** — proving who the caller is. Authorization (allow-listing,
RBAC, perms) is a separate concern that consumes `Identity` and applies
whatever policy a given service needs.

## Install

    import "github.com/mataki-dev/platform/identity"

## Three halves

### 1. Verifiers

A `Verifier` checks a bearer token and returns an `Identity`. The
contract uses `(nil, nil)` fall-through so chains of issuer-specific
verifiers (Google, Clerk, GitHub OIDC, custom) compose without mutual
coupling. The package ships `NewGoogleVerifier` and a `Chain` type.

`Identity` is concrete: `{ Issuer, Subject, Email, Claims, OBO }`.
Service-specific data (roles, scopes, allow-list membership) lives in
the authorization layer, not here.

### 2. Middleware

`Middleware` plugs into `net/http` or huma. On a successful request it
stashes the resolved `Identity` on the request context; handlers
retrieve it with `FromContext`. Verification failures render as 401 via
the platform `errors` envelope.

Authorization decisions (403, allow-listing, perms checks) belong in a
downstream middleware or handler that reads `Identity` from the
context.

### 3. Outbound transport

`NewTransport` and `NewClient` wrap an `http.Client` so every outbound
request carries a Google-signed ID token plus on-behalf-of headers.
The token source uses the GCP metadata server on Cloud Run and ADC
locally; caching and refresh are handled by `oauth2.TokenSource`.

## Example: server

    func main() {
        ctx := context.Background()

        google, err := identity.NewGoogleVerifier(ctx, "https://api.example.com")
        if err != nil { log.Fatal(err) }

        authn := identity.New(identity.Chain{
            myClerkVerifier,
            google,
        })

        mux := http.NewServeMux()
        mux.Handle("/secure", authn.Handler(authz.RequireRole("admin",
            http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                id, _ := identity.FromContext(r.Context())
                fmt.Fprintf(w, "hello %s", id.Email)
            }),
        )))
        log.Fatal(http.ListenAndServe(":8080", mux))
    }

`authz.RequireRole` above is a service-defined middleware that reads
`Identity` from context, looks up the caller's role, and either allows
or returns 403. It is not part of this package.

## Example: client

    ctx := context.Background()
    client, err := identity.NewClient(
        ctx,
        "https://api.other-service.example.com",
        identity.OnBehalfOf{Actor: identity.ActorService},
    )
    if err != nil { log.Fatal(err) }

    // Per-request OBO override via context.
    ctx = identity.WithOBO(ctx, identity.OnBehalfOf{
        Actor:  identity.ActorUser,
        OrgID:  "org_abc",
        UserID: "user_xyz",
    })
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/widgets", nil)
    resp, err := client.Do(req)
    ...

## On-behalf-of headers

| Header                 | Source                                      |
| ---------------------- | ------------------------------------------- |
| `X-On-Behalf-Of-Actor` | `OnBehalfOf.Actor` (default `"service"`)    |
| `X-On-Behalf-Of-Org`   | `OnBehalfOf.OrgID` (omitted when empty)     |
| `X-On-Behalf-Of-User`  | `OnBehalfOf.UserID` (omitted when empty)    |
| `X-Request-Id`         | `OnBehalfOf.RequestID` (omitted when empty) |

Header names match the upstream Python implementation so half-Python /
half-Go deployments interop during migration.
