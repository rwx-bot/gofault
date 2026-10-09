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

61 defects fixed across 25 commits. Grouped by what they were, not by when they
landed; every entry names the file so it can be reviewed directly.

### Security

- **Static file serving escaped its root.** The containment check compared
  `HasPrefix(absPath, absDir)`, so a sibling directory sharing the root's name
  was served: root `/srv/www` served `/srv/www-secret/...`
  (`middleware/static.go`)
- **Static file serving followed symlinks out of the root.**
  `Config.FollowSymLinks` was documented but never read, so with it false a
  symlink inside the root pointing outside it was still served. The resolved
  target is now validated against the resolved root
- **File upload escaped its storage root.** The client-supplied filename was
  joined onto `BaseDir` unsanitised, so `../../etc/cron.d/x` wrote outside it.
  `LocalStorage.Delete` had the same gap (`middleware/upload.go`)
- **CORS wildcard subdomains matched too much.** `*.example.com` accepted
  `notexample.com` and `evil-example.com`, because the suffix was not required
  to be preceded by a dot (`middleware/cors.go`)
- **Rate limiting could be bypassed.** `DefaultKeyFunc` used the whole
  `X-Forwarded-For` header as the key, so a client rotating the proxy chain got
  a fresh bucket per request (`middleware/ratelimit.go`)
- **A rejected JWT produced a successful response.** `Router.ServeHTTP` ignored
  the return value of `ExceptionFilter.Capture`, whose contract says false lets
  the exception propagate, so a declining filter left the client with 200 and an
  empty body (`router/router.go`)
- **Slowloris.** No `ReadHeaderTimeout` was set, so a client dribbling headers
  held connections open indefinitely (`server/http.go`)
- **A recovered gRPC panic was reported as success.** The four interceptors
  recovered without setting an error, leaving the named results at `(nil, nil)`.
  They now return `codes.Internal` and log a stack trace (`grpc/grpc.go`)
- **`gofault new` wrote outside the working directory.** The project name was
  not validated, so `gofault new ../evil` scaffolded elsewhere
  (`cmd/gofault/main.go`)

### Concurrency and correctness

- **Concurrent requests ran each other's middleware.** `ServeHTTP` built the
  chain with `append(r.middleware, ...)`, which writes into the router's backing
  array whenever it has spare capacity, so simultaneous requests clobbered each
  other's per-route middleware (`router/router.go`)
- **A disabled middleware panicked every request.** Every constructor returns
  nil when its config is disabled and the documented usage registers whatever it
  returns, so a nil landed in the chain and `runChain` called it. Nil entries are
  now dropped in `Module.RegisterMiddleware`, `Router.Middleware` and
  `Router.Handle`. This was reachable before v3.4.1
- **Module start order was non-deterministic.** `sortModules` walked a map, so
  Go's randomised iteration decided the order: the same dependency graph
  produced five different orders across forty runs (`module/module.go`)
- **Three data races.** `Router.container`, and `App.server`/`App.booted` which
  `Start` wrote while a signal handler calling `Stop` read them
- **`ioc` raced on singletons.** `entry.inst` is written under the container
  mutex but read without it, so concurrent first-resolves raced on the field and
  the fast path could observe a partially written interface value
- **Bind failures hung the process.** `StartWithGracefulShutdown` bound the port
  inside a goroutine, so a port already in use was discarded and the call blocked
  forever waiting for a signal (`server/http.go`)
- **Graceful shutdown could not be retried.** It marked the server stopped
  before attempting the shutdown, so a timeout looked like success on the next
  call
- **An exception filter's write could be silently dropped.** Compression
  committed a status line even when the handler failed, so the filter's error
  write became a no-op and the client saw 2xx for a failed request
- **`Wrap` lost the HTTP status.** `IsHTTPException` used a direct type
  assertion, so an HTTPException wrapped with `fmt.Errorf("%w")` was reported as
  500 instead of its own status (`exception/exception.go`)
- **`InvokeHandler` panicked on a wrong signature.** A bare type assertion on
  the controller action took down request handling
- **HEAD and 405 semantics were wrong.** HEAD 404ed against a GET route, and a
  path that matched under a different method reported 404 instead of 405 with an
  `Allow` header (`router/router.go`)

### Resource leaks

- **`NewRateLimiter` leaked a goroutine per instance.** The cleanup loop had no
  exit condition and there was no way to stop it: 200 limiters left 200
  goroutines behind. Added an idempotent `Close`
- **The rate limiter's key map grew without bound.** Timed-out buckets were only
  reaped by a 5-minute ticker, so rotating keys outpaced it. Added `MaxKeys`
  (default 10000) with least-recently-used eviction
- **`redis.NewClient` leaked its pool on failure.** The connectivity probe
  returned an error without closing the client. Verified: 50 failed
  constructions now leave the goroutine count unchanged

### Configuration that did nothing

Each of these was documented as working and read by nothing. Go is silent about
it, so `internal/deadfield` now fails the build on both this and the next class.

- `gorm.Config.Silent` never suppressed logging; it now selects `logger.Discard`
- `exception.IncludeStackTrace` never populated a stack; it now does, off by
  default since the stack exposes internal paths
- `versioning.HeaderVersionFormat` was stored and never applied;
  `HeaderConfig` now derives a regex from it as documented
- `static.FollowSymLinks` and `ipfilter.Mode` are covered above under Security
- `InvokeHandler` accepted an HTTP method and path and read neither, so its
  signature implied a dispatch that never happened. Both parameters removed
- `middleware.ValidatorConfig` documented a validator middleware that does not
  exist; nothing read it. Removed

### Other correctness

- **The scaffold did not compile.** `main.go` imported `<name>/controllers` as
  `github.com/gofault/gofault/<name>/controllers`, and the generated controller
  returned `(int, error)` from a method declared `error`. `testapp3` carried the
  same two defects and, being a separate module, was never compiled by
  `go test ./...` (`cmd/gofault/main.go`)
- **`metrics.normalizePath` was a stub**, so every distinct ID became a new
  Prometheus series
- **The request logger built its attributes and discarded them**, producing no
  output at all, and `readRequestBody` returned an empty string
- **A duplicate DI registration silently replaced the first**, so a later typo
  looked like it had taken effect. Now returns `ErrDuplicateRegistration`
- **Configuration was never validated.** An unknown key such as `sever.port` was
  ignored, leaving the server on its default port. `Load` now rejects unknown
  fields and validates ports, drivers and log settings
- **`GetDSN` always emitted the MySQL form**, which PostgreSQL and SQLite drivers
  cannot parse
- **Compression was O(n²)** for large responses, accumulating into a `[]byte` by
  repeated append
- **The OpenAPI document never listed a route.** `AddPath` existed but was never
  called, so the published spec had no paths
- **A panicking health checker took down the health endpoint**; it is now
  reported as down
- **`gofault version` reported a hardcoded `v1.0.0`**

### Added

- `middleware.CacheBackend`, so a cache need not live in the process.
  `*InMemoryCache` satisfies it, so existing callers are unaffected, and
  `redis.HTTPBackend` makes the integration the old comment promised
- `internal/deadfield`, an AST-based guard that fails the build on an exported
  field nothing reads and on a function parameter never used. Exempt entries
  need a reason and a second test fails when an exemption goes stale
- `server.HTTP.Server()` for advanced configuration
- `OpenAPIHandler`, `RegisterOpenAPI`, `HealthCheckHandler` and
  `RegisterHealthCheckEndpoint`, which separate the handler and middleware roles
  the old names conflated
- A `submodules` CI job that builds and vets `testapp3`, invisible to a
  root-level `go test ./...`

### Changed

- JWT: query-string tokens are opt-in via `AllowQueryToken`, and tokens without
  an `exp` claim are rejected unless `RequireExpiry` is disabled. Both defaults
  are safer; `LegacyJWTConfig` restores the old behaviour for a deliberate
  migration
- `JWTAuth` returns a middleware that rejects requests with a diagnostic for an
  unusable config, instead of nil. `MustJWTAuth` is available for callers who
  prefer to fail fast at setup
- `CacheMiddleware` takes a `CacheBackend` rather than a concrete type
- `server.HTTP.Start` returns nil rather than `http.ErrServerClosed` after a
  graceful shutdown
- `InvokeHandler` takes only the handler name
- CLI: the scaffold pins `gofault v0.0.0`, resolved by the local `replace`,
  instead of the non-existent `v1.0.0`, and targets the current Go version

### Removed

- `middleware.ValidatorConfig`, which had no consumer and no effect.
  `ValidateRequest(ctx, rules)` remains the supported API; route-scoped
  validation with struct binding is not implemented

### Test results

Statement coverage 50.0% -> 86.9%. `controller` 31.6% -> 100%, `core` 58.3%
-> 100%, `exception` 77.1% -> 98.2%, `redis` 78.8% -> 88.5%, `grpc` 81.6%
-> 88.3%, `gorm` 74.1% -> 83.3%, `server` 56.0% -> 90.0%.

Every fix has a test that fails when the fix is reverted. Notable additions:
randomised coverage of the topological sort over 200 generated dependency
graphs, a concurrency test driving 10000 requests through a middleware stack
while watching the goroutine count, and a test that compiles a freshly
scaffolded project.
