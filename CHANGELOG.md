## [v0.1.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Core architecture: IoC container and module primitives

## [v0.5.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Middleware chain, lifecycle hooks and exception filtering

## [v1.0.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Complete framework: server, router, controller, provider

## [v1.1.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Configuration layer and HTTP exception mapping

## [v1.2.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Graceful shutdown and example application

## [v1.3.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Route grouping, parameter binding and CI

## [v1.4.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Unified response encoding and error handling

## [v1.5.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Validator and HealthCheck middleware

## [v1.6.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Recovery middleware with stack trace capture

## [v1.7.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Compression middleware

## [v1.8.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- IP filter middleware with CIDR support

## [v1.9.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Request logger middleware

## [v2.0.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Timeout middleware

## [v2.1.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- WebSocket upgrade middleware

## [v2.2.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- OpenAPI spec serving with bundled Swagger UI

## [v2.3.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Prometheus metrics middleware

## [v2.4.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Cache middleware

## [v2.5.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- CORS middleware

## [v2.6.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- JWT authentication middleware

## [v2.7.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Rate limit middleware

## [v2.8.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Request ID middleware

## [v2.9.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Session and file upload middleware

## [v3.0.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Database integration via GORM

## [v3.1.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Redis integration

## [v3.2.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- Static file serving middleware

## [v3.3.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- gRPC server integration

## [v3.4.0]

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- API versioning with header, path and query strategies

## [v3.4.1]

### Fixed
- A disabled middleware no longer panics every request. Every constructor in the
  middleware package returns nil when its config is disabled, and the documented
  usage registers whatever the constructor returns, so a nil landed in the chain
  and `runChain` called it. Nil entries are now dropped in
  Module.RegisterMiddleware, Router.Middleware and Router.Handle. This was
  reachable before v3.4.1 and was made likely by the JWT change below
- JWTAuth returns a middleware that rejects requests with a diagnostic instead
  of nil for an unusable config. MustJWTAuth is available for callers who prefer
  to fail fast at setup

### Compatibility
- Migration paths for the v3.4.1 breaking changes:
  - LegacyJWTConfig(secret) restores the previous token handling (query-string
    tokens allowed, tokens without an expiry accepted). Both weaken security
  - JWTAuth no longer returns nil, so pre-v3.4.1 code that registered the result
    unconditionally works again
  - ValidatorConfig was removed because it had no effect; there is no
    replacement, use ValidateRequest

### Security
- Static file serving: reject paths outside the root. The containment check
  compared `HasPrefix(absPath, absDir)`, which also accepted sibling
  directories sharing the root's name (root `/srv/www` served
  `/srv/www-secret/...`)
- File upload: sanitize the client-supplied filename before joining it onto the
  storage root. A name such as `../../etc/cron.d/x` could previously write
  outside `BaseDir`; `LocalStorage.Delete` had the same gap

### Changed
- JWT: query-string tokens are now opt-in via `JWTConfig.AllowQueryToken`
  (default false). A token in a URL leaks into access logs, `Referer` headers
  and browser history
- JWT: tokens without an `exp` claim are rejected by default. Relax with
  `JWTConfig.RequireExpiry = false`
- JWT: `GenerateToken` now honors its `algorithm` argument and returns an error
  for anything but HS256, instead of silently signing with HS256 under a
  different label
- JWT: `parseToken` verifies the `alg` header after the signature, so a token
  cannot declare an algorithm it was not signed with
- JWT: `JWTAuth` returns `nil` for an empty secret or unsupported algorithm,
  failing closed at wiring time
- `controller.InvokeHandler` reports a handler whose signature is not
  `func(*core.Ctx) error` instead of panicking on the type assertion
- CLI: `gofault new` rejects a project name that is not a single path segment,
  so `gofault new ../evil` no longer scaffolds outside the working directory
- CLI: scaffolded `go.mod` pins `github.com/gofault/gofault v0.0.0` (resolved by
  the local `replace`) instead of the non-existent `v1.0.0`; `gofault version`
  reports the release rather than a hardcoded `v1.0.0`

### Fixed
- Server: `StartWithGracefulShutdown` bound the port inside a goroutine, so a
  bind failure (port in use) was lost and the call blocked forever waiting for a
  signal. The listener is now opened up front and the error is returned
- Server: `Start` returned `http.ErrServerClosed` after a normal graceful
  shutdown, which callers had to special-case. It now returns nil
- Server: `GracefulShutdown` marked the server stopped before attempting the
  shutdown, so a timeout could never be retried and the failure was reported as
  success on the next call. The result is memoised and hooks run exactly once
- Server: no `ReadHeaderTimeout` was set, leaving the server open to Slowloris
  (a client dribbling headers holds connections indefinitely). Defaults to
  10s; override via the new `Server()` accessor
- Static: `FollowSymLinks` was documented but never applied, so a symlink
  inside the root pointing outside it was served while the option was false.
  The resolved target is now validated against the resolved root
- IP filter: `Mode` was documented but never read, so `Mode: "allow"` silently
  behaved like `"block"`. Block and allow are now evaluated per Mode, with an
  unrecognised value falling back to the deny-by-default behaviour
- Exception: `Wrap` (and any `fmt.Errorf("%w")`) hid the HTTPException from
  `IsHTTPException`/`HTTPExceptionOf`, which used a direct type assertion, so a
  wrapped 400 was reported as 500. Both now use `errors.As`
- Exception: `HTTPExceptionFilter.IncludeStackTrace` was documented but never
  applied. It now populates a `stack` field, off by default because the stack
  exposes internal paths and symbols
- Versioning: `HeaderVersionFormat` was stored but never applied; the middleware
  only read `HeaderVersionRE`. `HeaderConfig` now derives a regex from the
  format when none is supplied, as its documentation promised

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- `server.HTTP.Server()` exposes the underlying `*http.Server` for advanced
  configuration
- gRPC: a panic in any interceptor was recovered but never converted to an
  error, so the named results stayed at `(nil, nil)` and gRPC reported the
  call as a **success with a nil response**. `UnaryInterceptor`,
  `StreamInterceptor`, `RecoveryInterceptor` and `StreamRecoveryInterceptor` now
  return `codes.Internal` and log the panic with a stack trace. The old
  `TestRecoveryInterceptor` asserted `err == nil` after a panic, which pinned
  the defect in place
- gRPC interceptors logged with `fmt.Printf`, against the structured-logging
  rule in AGENTS.md. They now use `slog.Default()`
- Redis: `NewClient` returned an error after a failed connectivity probe
  without closing the client, leaking the connection pool and its goroutines.
  Verified: 50 failed constructions now leave the goroutine count unchanged
- GORM: `Config.Silent` was documented as suppressing all GORM logs but was
  never read, so `LogLevel` alone decided verbosity. It now selects
  `logger.Discard`
- Removed dead `dbInstance`/`dbOnce` and `clientInstance`/`clientOnce` globals

### Removed
- `middleware.ValidatorConfig` (with `BindTarget` and `SkipMissing`). It was
  documented as the configuration for a validator middleware, but no validator
  middleware exists and nothing ever read the struct, so setting it silently did
  nothing. `ValidateRequest(ctx, rules)` remains the supported API. Route-scoped
  validation with struct binding is not implemented

### Added
- `middleware.CacheBackend` interface, so a cache no longer has to live in the
  process. `CacheMiddleware` accepts it; `*InMemoryCache` satisfies it, so
  existing callers are unaffected
- `redis.HTTPBackend` adapts `redis.Cache` to `middleware.CacheBackend`, making
  the integration the old comment on `redis.Cache` claimed. Cache misses and
  Redis outages degrade to a miss rather than failing the request
- Concurrent requests could execute each other's middleware chain.
  `Router.ServeHTTP` built the chain with `append(r.middleware, ...)`, which
  writes into the router's backing array whenever it has spare capacity, so
  simultaneous requests clobbered one another's per-route middleware
- `Router.container` was read on every request and written by `SetContainer`
  without synchronization; it is now guarded by a `sync.RWMutex`
- Data race between `App.Start` and `App.Stop`: `server` and `booted` were
  written on the start goroutine while a signal handler calling `Stop` read
  them. `App` fields are now guarded by a mutex
- `testapp3` did not compile: its handlers returned `(int, error)` from a
  method declared `error`. It is a separate module, so `go test ./...` from
  the repo root never covered it
- The `gofault new` scaffold produced a project that could not build: `main.go`
  imported `<name>/controllers` as `github.com/gofault/gofault/<name>/controllers`,
  and the generated controller had the same bad return signature
- `examples/hello` registered `loggingMiddleware` on both the module and the
  router, so it ran twice per request

### Tests
- Statement coverage 83.0% overall. `controller` 31.6% -> 100%, `core` 58.3%
  -> 100%, `module` 51.7% -> 92.5%, `cmd/gofault` 0% -> 69.9%
- Added `middleware/security_test.go` and lifecycle tests covering `App.Start`
  and `App.Stop` concurrently, which is what surfaced the races above
- `TestCreateProject_OutputCompiles` builds a freshly scaffolded project, so a
  broken template fails the suite instead of the user's first `go build`
- `TestRouter_ConcurrentRequestsDoNotShareChain` reproduces the chain-aliasing
  bug under `-race`

### CI
- Added `internal/deadfield`, an AST-based guard that fails when an exported
  struct field is never read, and wired it into CI. Exempt fields require an
  explicit allowlist entry with a reason, and a second test fails if an
  allowlist entry goes stale. This is what the six dead config fields in this
  release had in common: the compiler and go vet are both silent about them
- Added a `submodules` job that builds and vets `testapp3`, and made `release`
  depend on it. Nested modules are invisible to `go test ./...` from the root
