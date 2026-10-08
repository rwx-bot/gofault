# gofault

A modular backend framework for Go: modules, dependency injection and an HTTP
layer that stays out of the way.

- **Modules** compose the application and declare their dependencies
- **Controllers** expose HTTP endpoints through a prefix and a route table
- **Providers** supply dependencies through the IoC container
- **Middleware** wraps the request pipeline and can short-circuit it

## Features

| Area | Package |
|------|---------|
| IoC container (singleton, request and transient scope) | `ioc` |
| Module registry with topological ordering | `module`, `core` |
| Router with `:param` matching | `router` |
| Controller prefix routing and JSON helpers | `controller` |
| Lifecycle hooks (`OnInit`, `OnBoot`, `OnShutdown`) | `core` |
| Exception filter | `exception` |
| Structured logging | `logger` |
| YAML/JSON config loading | `config` |

## Install

```bash
go get github.com/gofault/gofault
```

## Quick start

```go
package main

import (
    "log"

    "github.com/gofault/gofault/controller"
    "github.com/gofault/gofault/core"
    "github.com/gofault/gofault/module"
)

type GreetingController struct {
    controller.BaseController
    prefix string
}

func (c *GreetingController) Prefix() string { return "/hello" }

func (c *GreetingController) Routes() []core.Route {
    return []core.Route{
        {Method: "GET", Path: "/greet/:name", Handler: "Greet"},
    }
}

func (c *GreetingController) Greet(ctx *core.Ctx) error {
    return controller.OK(ctx.Response, map[string]string{
        "message": c.prefix + ", " + controller.Param(ctx, "name") + "!",
    })
}

func main() {
    mod := core.NewModule("hello")
    mod.RegisterControllers(&GreetingController{prefix: "Hello"})

    app := module.New()
    if err := app.RegisterModules(mod); err != nil {
        log.Fatal(err)
    }
    log.Fatal(app.Start(9090))
}
```

```
$ curl localhost:9090/hello/greet/World
{"code":0,"message":"success","data":{"message":"Hello, World!"}}
```

## Architecture

Dependencies point in one direction: every package imports `core`, and `core`
imports nothing.

```
core          contracts only (Ctx, Module, Route, Provider, Controller, hooks)
 ├── ioc      container: singleton / request / transient scopes
 ├── provider injectable values, including request-scoped ones
 ├── router   method + path dispatch, middleware chain, exception filter
 ├── server   net/http listener and graceful shutdown
 └── module   composition root: orders modules, wires routes, starts the server
      ├── middleware   cross-cutting handlers (logging, recovery, timeout, ...)
      └── controller   prefix routing plus response helpers
```

`module.App` is the composition root. It topologically sorts the registered
modules by their `Depends` list, rejects unknown or circular dependencies, then
wires every controller route onto the router with that module's middleware:

```go
app := module.New()
app.SetRouter(rtr)

if err := app.RegisterModules(modA, modB); err != nil {
    log.Fatal(err)
}
log.Fatal(app.Start(9090))
```

## Testing

```bash
go test ./... -race
```

Integration tests that need an external service skip themselves when it is not
reachable, so the suite stays green on a bare checkout.

## What's new in v0.1.0

The first cut of the framework: the `core` contracts, the IoC container and the
module primitives.

### Contracts

```go
type Handler func(ctx *Ctx) error
type MiddlewareFunc func(ctx *Ctx, next Handler) error

type Provider interface{ Provide() any }

type Controller interface {
    Routes() []Route
    Prefix() string
}
```

`Ctx` wraps the request/response pair plus the params extracted from the route:

```go
type Ctx struct {
    Request    *http.Request
    Response   http.ResponseWriter
    Params     map[string]string
    StatusCode int
    Locals     map[string]any
}
```

### Modules

```go
mod := core.NewModule("hello")
mod.RegisterControllers(ctrl).RegisterProviders(svc)
```

### Container

```go
c := ioc.New()
c.Register(NewGreetingService)

svc, err := c.Resolve(&GreetingService{})
```

## What's new in v0.5.0

Middleware chains, lifecycle hooks and a filter for error handling.

### Middleware

A middleware wraps the next handler; returning an error stops the chain and
hands the error to the router's exception filter:

```go
func loggingMiddleware(ctx *core.Ctx, next core.Handler) error {
    log.Printf("%s %s", ctx.Request.Method, ctx.Request.URL.Path)
    return next(ctx)
}
```

### Lifecycle hooks

Three small interfaces let a type participate in startup and shutdown. They are
optional: implement whichever you need.

```go
type OnInit interface{ OnInit() error }
type OnBoot interface{ OnBoot() error }
type OnShutdown interface{ OnShutdown() error }
```

Hooks are registered on the module and run in dependency order, with shutdown
running in reverse:

```go
mod.RegisterOnBoot(hook)
mod.RegisterOnShutdown(hook)
```

### Exceptions

`exception` carries the HTTP status alongside the message, so handlers can
return an error and let the filter turn it into a response:

```go
return exception.BadRequest("invalid payload")
```


## What's new in v1.0.0

### Application bootstrap

`module.App` is the composition root. It sorts modules by their `Depends`
declarations, rejects unknown and circular dependencies, wires every controller
route onto the router, and then starts the server:

```go
app := module.New()
app.SetRouter(rtr)

if err := app.RegisterModules(mod); err != nil {
    log.Fatal(err)
}
log.Fatal(app.Start(9090))
```

### Server, router and controller

`server.HTTP` owns the listener, `router` dispatches on method and path, and
`controller` provides the response helpers:

```go
rtr := router.New()
rtr.Middleware(loggingMiddleware)
rtr.ExceptionFilter(exception.NewHTTPExceptionFilter())

return controller.OK(ctx.Response, map[string]string{"message": "gofault is running"})
```

### Module dependencies

`Depends` orders initialisation without constraining registration order:

```go
mod.Depends = []string{"config"}
```

### Injectable providers

```go
mod.RegisterProviders(provider.ProviderFunc(func() any { return svc }))
```


## What's new in v1.1.0

### Configuration

`config.Load` reads a YAML document into typed sections; `MustLoad` panics
instead of returning an error:

```go
cfg, err := config.Load("config.yaml")
if err != nil {
    return err
}
addr := cfg.Server.GetAddress()
dsn := cfg.Database.GetDSN()
```

### HTTP exception filter

`exception.NewHTTPExceptionFilter` maps an `exception.HTTPException` back to its
status code. Wire it onto the router so handler errors become responses:

```go
rtr.ExceptionFilter(exception.NewHTTPExceptionFilter())
```

Filters compose — build a chain when you need more than one:

```go
chain := exception.NewExceptionFilterChain(
    exception.NewHTTPExceptionFilter(),
)
```


## What's new in v1.2.0

### Graceful shutdown

`App.Stop` runs every `OnShutdown` hook in reverse module order and then drains
the server, so in-flight requests are not cut off:

```go
defer app.Stop()
```

`server.HTTP` owns the listener and exposes the same behaviour directly:

```go
srv := server.New(rtr, 9090)
go srv.Start()

server.RegisterShutdownHook(func() error { return srv.Stop() })
```

### Example application

`examples/hello` is a runnable application that exercises a provider, a
controller with a path parameter, module middleware and both lifecycle hooks.


## What's new in v1.3.0

### Route grouping and parameter binding

A controller owns a prefix and a route table; `:name` segments become request
params:

```go
func (c *AdminController) Prefix() string { return "/admin" }

func (c *AdminController) Routes() []core.Route {
    return []core.Route{
        {Method: "GET", Path: "/users", Handler: "ListUsers"},
        {Method: "POST", Path: "/users/:id", Handler: "UpdateUser"},
    }
}
```

Handlers read them through the `controller` helpers:

```go
id := controller.Param(ctx, "id")
filter := controller.Query(ctx, "filter")
```

### Continuous integration

`.github/workflows/ci.yml` runs the test suite with the race detector, lints
with `golangci-lint`, and publishes release binaries for Linux, macOS and
Windows when a `v*` tag is pushed. The workflow pins the Go version that
`go.mod` declares.


## What's new in v1.4.0

### Unified response encoding

Every JSON response goes through the same envelope, and every helper sets
`Content-Type: application/json` before the status line:

```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

```go
return controller.OK(ctx.Response, user)                      // 200
return controller.Error(ctx.Response, 422, "invalid payload") // 422
```

`BaseController` exposes the same helpers as methods, plus a plain-text variant
for non-JSON endpoints:

```go
c.JSON(ctx.Response, http.StatusOK, user)
c.Text(ctx.Response, http.StatusOK, "pong")
```


## What's new in v1.5.0

### Request validation

`ValidateRequest` checks a set of named rules and returns an error listing every
failure:

```go
err := middleware.ValidateRequest(ctx, middleware.Rules{
    "email": {middleware.Required(), middleware.Email()},
    "age":   {middleware.Min(18), middleware.Max(120)},
})
if err != nil {
    return exception.BadRequest(err.Error())
}
```

Available rules: `Required`, `Min`, `Max`, `MinLength`, `MaxLength`, `Regex`,
`Email`.

### Health checks

`HealthCheckMiddleware` answers a probe path and reports each registered check.
`LatencyHealthCheck` wraps any function and records how long it took:

```go
cfg := middleware.DefaultHealthCheckConfig()
cfg.Path = "/healthz"
cfg.RegisterHealthCheck("db", middleware.LatencyHealthCheck("db", db.Ping))

probe := middleware.HealthCheckMiddleware(cfg)
rtr.Handle("GET", cfg.Path, probe)
```


## What's new in v1.6.0

### Recovery

`RecoveryMiddleware` turns a panic into a `500` response instead of dropping the
connection. `StackTrace` adds the goroutine dump while debugging:

```go
mod.RegisterMiddleware(middleware.RecoveryMiddleware(
    middleware.RecoveryConfig{Enabled: true, StackTrace: false},
))
```

Register it before the other middleware of the same module so it wraps them.


## What's new in v1.7.0

### Compression

`CompressionMiddleware` gzips responses that exceed `MinSize`; smaller bodies pass
through untouched so the header overhead is not paid for nothing:

```go
cfg := middleware.DefaultCompressionConfig()
cfg.Level = gzip.BestSpeed
cfg.MinSize = 1024

mod.RegisterMiddleware(middleware.CompressionMiddleware(cfg))
```


## What's new in v1.8.0

### IP filtering

`IPFilterMiddleware` matches the client address against CIDR ranges. `Mode`
selects how an address that matches neither list is treated:

```go
cfg := middleware.IPFilterConfig{
    Enabled: true,
    Allow:   []string{"10.0.0.0/8", "192.168.1.0/24"},
    Block:   []string{"203.0.113.0/24"},
    Mode:    "allowlist",
}

mod.RegisterMiddleware(middleware.IPFilterMiddleware(cfg))
```


## What's new in v1.9.0

### Request logging

`RequestLoggerMiddleware` records method, path, status and duration for each
request. Bodies and headers are opt-in because reading them is not free:

```go
cfg := middleware.DefaultRequestLoggerConfig()
cfg.LogHeaders = false
cfg.LogBody = false

mod.RegisterMiddleware(middleware.RequestLoggerMiddleware(cfg))
```

The `logger` package provides the underlying logger, plus a lighter middleware
for projects that only want method and path:

```go
log := logger.New("[gofault] ", logger.LevelInfo)
rtr.Middleware(logger.LoggingMiddleware(log))
```


## What's new in v2.0.0

### Timeouts

`TimeoutMiddleware` cancels a request that outlives its deadline and answers
with `504`:

```go
cfg := middleware.DefaultTimeoutConfig()
cfg.Duration = 5 * time.Second
cfg.ErrorMessage = "upstream too slow"

mod.RegisterMiddleware(middleware.TimeoutMiddleware(cfg))
```

The handler runs on its own goroutine, so its output is buffered and published by
a single goroutine. That keeps a handler which finishes late from writing to a
response that was already committed. Such a handler is expected to watch the
request context:

```go
func longHandler(ctx *core.Ctx) error {
    select {
    case <-ctx.Request.Context().Done():
        return ctx.Request.Context().Err()
    case result := <-work:
        return controller.OK(ctx.Response, result)
    }
}
```


## What's new in v2.1.0

### WebSocket

`WebSocketMiddleware` upgrades the request and hands the connection to a
handler. Implement only the callbacks you need:

```go
type chatHandler struct{}

func (chatHandler) HandleConnect(ctx *core.Ctx, conn *websocket.Conn) error {
    return nil
}

func (chatHandler) HandleMessage(ctx *core.Ctx, conn *websocket.Conn, mt int, data []byte) error {
    return conn.WriteMessage(mt, data) // echo
}

func (chatHandler) HandleDisconnect(ctx *core.Ctx, conn *websocket.Conn) {}

cfg := middleware.DefaultWebSocketConfig()
cfg.PingInterval = 30 * time.Second

mod.RegisterMiddleware(middleware.WebSocketMiddleware(cfg, chatHandler{}))
```

`WebSocketHandlerFunc` adapts plain functions when a struct is overkill. The live
connection is also reachable from later middleware and handlers through
`ctx.Locals["ws_conn"]`.


## What's new in v2.2.0

### OpenAPI document

`OpenAPIMiddleware` serves the specification as JSON, and `NewOpenAPIDocument`
builds one programmatically:

```go
doc := middleware.NewOpenAPIDocument("Example API", "1.0.0", "Internal API")
doc.AddPath("/hello", "get", map[string]any{"summary": "Greet someone"})
```

```go
cfg := middleware.DefaultOpenAPIConfig()
cfg.Path = "/openapi.json"
cfg.Title = "Example API"
cfg.Version = "1.0.0"

rtr.Handle("GET", cfg.Path, middleware.OpenAPIMiddleware(cfg))
```

It is a `core.Handler` rather than a `MiddlewareFunc`, so it is mounted on a
single route instead of wrapping the whole chain.


## What's new in v2.3.0

### Prometheus metrics

`MetricsMiddleware` returns the middleware together with the registry it wrote
to, so the scrape endpoint can expose exactly that registry:

```go
cfg := middleware.DefaultMetricsConfig()
cfg.Namespace = "gofault"
cfg.Subsystem = "http"

mw, registry := middleware.MetricsMiddleware(cfg)

mod.RegisterMiddleware(mw)
rtr.Handle("GET", "/metrics",
    promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP)
```

The registry is scoped to the middleware rather than the process-wide default,
which keeps tests and multiple apps from leaking series into each other.


## What's new in v2.4.0

### Response caching

`CacheMiddleware` takes the store explicitly, so its lifetime is visible at the
call site. `SkipFunc` excludes requests that must never be cached:

```go
cfg := middleware.DefaultCacheConfig()
cfg.TTL = 30 * time.Second
cfg.MaxSize = 1000
cfg.SkipFunc = func(ctx *core.Ctx) bool {
    return ctx.Request.Header.Get("Authorization") != ""
}

store := middleware.NewInMemoryCache(cfg)
mod.RegisterMiddleware(middleware.CacheMiddleware(store, cfg))
```


## What's new in v2.5.0

### CORS

`CORS` answers preflight requests and sets the response headers. Origins are
matched explicitly — nothing is wildcarded implicitly:

```go
cfg := middleware.DefaultCORSConfig()
cfg.AllowOrigins = []string{"https://app.example.com"}
cfg.AllowMethods = []string{"GET", "POST"}
cfg.AllowHeaders = []string{"Content-Type", "Authorization"}
cfg.AllowCredentials = true

mod.RegisterMiddleware(middleware.CORS(cfg))
```


## What's new in v2.6.0

### JWT authentication

`JWTAuth` verifies the bearer token (or the configured query parameter) and
rejects the request when it is missing, malformed or expired:

```go
cfg := middleware.DefaultJWTConfig([]byte(os.Getenv("JWT_SECRET")))
cfg.TokenName = "access_token"

mod.RegisterMiddleware(middleware.JWTAuth(cfg))
```

Verified claims are published on the request, so handlers can see who called:

```go
claims := middleware.GetClaims(ctx)
if claims == nil {
    return exception.Unauthorized("not authenticated")
}
return controller.OK(ctx.Response, map[string]string{"user": claims.UserID})
```

Tokens are issued by the same package:

```go
token, err := middleware.GenerateToken(&middleware.Claims{
    Subject: "user-42",
    UserID:  "42",
    Roles:   []string{"admin"},
}, secret, "HS256")
```


## What's new in v2.7.0

### Rate limiting

`NewRateLimiter` builds a token-bucket limiter and `Middleware` exposes it to the
router. `KeyFunc` decides what counts as one caller; `DefaultKeyFunc` uses the
client address:

```go
limiter := middleware.NewRateLimiter(middleware.RateLimiterConfig{
    RequestsPerSecond: 20,
    BurstSize:         40,
    KeyFunc:           middleware.DefaultKeyFunc,
})

mod.RegisterMiddleware(limiter.Middleware())
```

Ask the limiter directly when you need the decision without the HTTP layer:

```go
if !limiter.Allow("tenant-7") {
    return exception.TooManyRequests("slow down")
}
```


## What's new in v2.8.0

### Request IDs

`RequestID` honours an inbound `X-Request-ID` when present and generates one
otherwise, then echoes it on the response so a client can quote it:

```go
mod.RegisterMiddleware(middleware.RequestID())
```

Downstream middleware and handlers read the same value straight off the header,
which makes a request traceable through logs without threading a context value
through every call.


## What's new in v2.9.0

### Sessions

The store is built separately and passed to the middleware, so it can be shared
or replaced in tests:

```go
store := middleware.NewSessionStore(middleware.DefaultSessionConfig())

mod.RegisterMiddleware(middleware.SessionMiddleware(store))
```

```go
sess := middleware.GetSession(ctx)
if sess == nil {
    return exception.Unauthorized("no session")
}
sess.SetValue("user_id", "42")

if id, ok := sess.GetValue("user_id"); ok {
    _ = id
}
```

### File uploads

`UploadMiddleware` parses the configured field, enforces the size and type
limits, and stores each file through a `StorageBackend`. `NewLocalStorage` writes
to disk; handlers read the results back with `GetUploadFiles`:

```go
cfg := middleware.DefaultUploadConfig()
cfg.MaxSize = 10 << 20
cfg.AllowedTypes = []string{"image/png", "image/jpeg"}
cfg.Storage = middleware.NewLocalStorage("./uploads")

mod.RegisterMiddleware(middleware.UploadMiddleware(cfg))
```

```go
for _, f := range middleware.GetUploadFiles(ctx) {
    log.Printf("%s -> %s (%d bytes)", f.OriginalName, f.Path, f.Size)
}
```


## What's new in v3.0.0

### Database access (GORM)

`NewDatabase` pings the server while opening, so a bad DSN fails at boot rather
than on the first query. The dialect is a value instead of a string, and `GetDB`
returns the underlying `*gorm.DB` for anything the wrapper does not cover:

```go
import (
    gormio "gorm.io/gorm"

    "github.com/gofault/gofault/gorm"
)

db, err := gorm.NewDatabase("primary", gorm.Config{
    Dialect:         gorm.DialectPostgres,
    DSN:             os.Getenv("DATABASE_URL"),
    MaxOpenConns:    25,
    MaxIdleConns:    5,
    ConnMaxLifetime: 300, // seconds
})
if err != nil {
    log.Fatal(err)
}
```

The connection is closed on shutdown once the database is part of a module:

```go
mod.RegisterProviders(provider.ValueProvider{Value: db})
mod.RegisterOnShutdown(db)
```

Group writes so a failure rolls the whole unit back:

```go
err := gorm.Transaction(db.GetDB(), func(tx *gormio.DB) error {
    if err := tx.Create(&user).Error; err != nil {
        return err
    }
    return tx.Create(&audit).Error
})
```


## What's new in v3.1.0

### Redis

`NewClient` verifies the connection while dialling and returns a client that also
satisfies `OnShutdown`:

```go
rdb, err := redis.NewClient("primary", redis.Config{
    Addr:        os.Getenv("REDIS_ADDR"),
    PoolSize:    100,
    ReadTimeout: 3, // seconds
})
if err != nil {
    return err
}
```

Helpers cover the common string operations, and `GetClient` exposes the raw
`*redis.Client` for everything else:

```go
ctx := context.Background()

redis.Set(ctx, rdb.GetClient(), "greeting", "hello", time.Hour)
v, _ := redis.Get(ctx, rdb.GetClient(), "greeting")
ok, _ := redis.Exists(ctx, rdb.GetClient(), "greeting")
```

A namespaced cache and a distributed lock build on the same client:

```go
cache := redis.NewCache(rdb.GetClient(), "myapp", 5*time.Minute)
_ = cache.Set(ctx, "user:1", "alice", 0)

lock := redis.NewLock(rdb.GetClient(), "jobs:daily", "worker-1", 30*time.Second)
if acquired, _ := lock.Acquire(ctx); acquired {
    defer lock.Release(ctx)
}
```

The lock releases through a Lua script that checks the owner value first, so a
worker that overran its lease cannot release someone else's lock.


## What's new in v3.2.0

### Static files

`StaticMiddleware` serves a directory, optionally with directory listings for
internal tooling:

```go
cfg := middleware.DefaultStaticConfig()
cfg.Dir = "./public"
cfg.Prefix = "/static"
cfg.Index = "index.html"
cfg.CacheControl = "public, max-age=86400"
cfg.Browse = false

mod.RegisterMiddleware(middleware.StaticMiddleware(cfg))
```

`DirectoryListing` renders the index used when `Browse` is on, so it is easy to
replace with your own template.


## What's new in v3.3.0

### gRPC server

`NewGRPCServerModule` returns a module, so the server takes part in the same
lifecycle as everything else — it listens on `OnBoot` and drains on `OnShutdown`:

```go
cfg := grpc.DefaultConfig()
cfg.Port = 9091
cfg.Network = "tcp"

grpcMod := grpc.NewGRPCServerModule("grpc", cfg)
srv := grpcMod.Server()

srv.RegisterService(&pb.Service_ServiceDesc, impl)

if err := app.RegisterModules(grpcMod); err != nil {
    return err
}
```

TLS material is configured with file paths, and interceptors are values you pass
in rather than methods you call:

```go
import (
    grpcapi "google.golang.org/grpc"

    "github.com/gofault/gofault/grpc"
)

creds, err := grpc.TLSCreds("cert.pem", "key.pem") // credentials.TransportCredentials
if err != nil {
    return err
}

cfg.TLSCert, cfg.TLSKey = "cert.pem", "key.pem"
cfg.UnaryInterceptors = []grpcapi.UnaryServerInterceptor{
    grpc.RecoveryInterceptor(),
    grpc.LoggingInterceptor(),
}
cfg.StreamInterceptors = []grpcapi.StreamServerInterceptor{
    grpc.StreamRecoveryInterceptor(),
}
```

`KeepAliveConfig` builds the keepalive parameters instead of making callers
remember the field names.


## What's new in v3.4.0

### API versioning

Three strategies, each with its own constructor. All of them publish the version
to `ctx.Locals["version"]`, which handlers read with `ctx.GetVersion()`:

```go
// Accept: application/vnd.app.v2+json
acceptRE := regexp.MustCompile(`v(\d+)`)
mod.RegisterMiddleware(versioning.Middleware(
    versioning.HeaderConfig("Accept", "application/vnd.app.v%d+json", acceptRE, 1),
))

// /v3/users
mod.RegisterMiddleware(versioning.Middleware(
    versioning.PathPrefixConfig("/v3", 1),
))

// /users?api-version=2
mod.RegisterMiddleware(versioning.Middleware(
    versioning.QueryConfig("api-version", 1),
))
```

### Version lifecycle

A `VersionSet` records which versions are active, deprecated or end-of-life, and
`VersionHandler` enforces that decision per route:

```go
vs := versioning.DefaultVersionSet([]versioning.VersionInfo{
    {Version: 1, Status: versioning.VersionStatusActive},
    {Version: 2, Status: versioning.VersionStatusActive},
})

vs.Deprecate(1, "use v2")
```

```go
if !vs.IsSupported(ctx.GetVersion()) {
    return exception.BadRequest("unsupported API version")
}
if vs.IsDeprecated(ctx.GetVersion()) {
    ctx.RespHeader().Set("Deprecation", "true")
}
```

`versioning.ResponseWriter` adds the negotiated version to outgoing headers, so
clients can confirm what the server picked.
