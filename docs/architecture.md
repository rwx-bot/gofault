# Architecture

gofault is a small framework with one rule: **dependencies point at `core`**.
`core` is the only package with no framework imports, so nothing can create a
cycle and every other package stays swappable.

## Layers

```
core                       contracts: Ctx, Module, Route, Provider,
                           Controller, Handler, MiddlewareFunc, hooks
├── ioc                    container: singleton / request / transient
├── provider               injectable values and request-scoped accessors
├── router                 method + path dispatch, middleware chain,
│                          exception filters
├── server                 net/http listener, graceful shutdown
├── logger, config         infrastructure helpers
├── exception              HTTP errors and the filter that renders them
├── versioning             version negotiation
├── middleware             cross-cutting handlers built on core only
├── controller             prefix routing and response helpers
├── module                 composition root
├── gorm, redis, grpc      optional integrations
└── <your app>             registers modules into module.App
```

The measured import graph (each entry lists the framework packages it imports)
is deliberately shallow:

| package | imports |
|---|---|
| `core`, `ioc`, `server` | — |
| `config`, `provider` | `ioc` (`provider` only) |
| `controller`, `exception`, `logger` | `core` |
| `versioning` | `core` |
| `middleware` | `core`, `exception` |
| `router` | `core`, `exception`, `ioc` |
| `gorm`, `grpc`, `redis` | `core` |
| `module` | `core`, `controller`, `ioc`, `provider`, `router`, `server` |

Nothing imports `middleware`, and nothing imports the integrations. That is why
you can delete `gorm`, `redis` and `grpc` from a build without touching the core.

## Request lifecycle

1. `server.HTTP` accepts the connection and builds a `*core.Ctx` with
   `core.NewCtx`, which initialises `Params` and `Locals`.
2. `router.Router.ServeHTTP` matches method and path, extracts `:param`
   segments into `ctx.Params`, and begins a request scope in the container.
3. The router's middleware chain runs outermost first, then the module
   middleware attached to the matched route.
4. `controller.InvokeHandler` reflects the handler name from the route onto the
   controller and calls it.
5. If a handler returns an error, the router hands it to the exception filter
   chain; `exception.NewHTTPExceptionFilter` recognises `HTTPException` values
   and writes the matching status code and JSON body.
6. The request scope ends, releasing request-scoped instances.

## Modules and ordering

A module is a struct, not an interface:

```go
mod := core.NewModule("api")
mod.Depends = []string{"database"}
mod.RegisterControllers(ctrl).RegisterProviders(svc).RegisterMiddleware(mw)
mod.RegisterOnInit(h).RegisterOnBoot(h).RegisterOnShutdown(h)
```

`module.App.RegisterModules` rejects duplicate names and empty names up front.
`App.Init` then topologically sorts modules by `Depends` (Kahn's algorithm) and
fails on unknown dependencies or cycles — before the server starts, so a wiring
mistake is a boot failure rather than a runtime one.

Hook order follows that sort: `OnInit` and `OnBoot` run in dependency order,
`OnShutdown` runs in reverse.

## Dependency injection

`ioc` resolves by type and supports three scopes:

| registration | lifetime |
|---|---|
| `Register` | singleton, created once |
| `RegisterScoped` | per request, via `ResolveFromCtx` |
| `RegisterTransient` | a new instance per resolve |

Constructors are plain functions; their parameters are resolved recursively:

```go
c := ioc.New()
c.Register(NewGreetingService)

svc, err := c.Resolve(&GreetingService{})   // pointer to the zero value
```

`Resolve` takes a pointer to a zero value of the target type so the container can
key on the type without the caller restating it. Request-scoped resolution goes
through the context (`ResolveFromCtx`), which the router sets up per request.

## Middleware semantics

```go
type Handler func(ctx *Ctx) error
type MiddlewareFunc func(ctx *Ctx, next Handler) error
```

Plain function types, which is what makes middleware composable without an
interface and testable without a server. Returning before calling `next`
short-circuits the chain; returning an error propagates it to the exception
filter.

Middleware is registered in two places, and the distinction matters:

* `rtr.Middleware(...)` — runs for every route.
* `mod.RegisterMiddleware(...)` — runs only for routes of that module.

## Concurrency

Two places run work off the request goroutine, and both are written so that only
one goroutine ever writes a response:

* **Timeout middleware** runs the handler on its own goroutine and lets it write
  into a private buffer. If the deadline fires first, the middleware writes
  `504` and the buffered output is discarded. A handler that finishes late can
  therefore never write to a response that was already committed, and the
  request context is the signal it should stop.
* **WebSocket** upgrades the connection, stores it in `ctx.Locals["ws_conn"]`,
  and runs a ping ticker with its own wait group so the connection is closed
  exactly once.

Everything else is synchronous. Run `go test ./... -race`; the middleware package
is where a race will show up first.

## Error handling

`exception` separates the error from its rendering:

```go
return exception.NotFound("user not found")     // any depth, any goroutine
```

```go
rtr.ExceptionFilter(exception.NewHTTPExceptionFilter())
```

Filters implement `ExceptionFilter` and can be chained with
`NewExceptionFilterChain`. The filter runs on the router, so it also covers
errors returned by module middleware.

## Testing strategy

* Unit tests never start a server: they build a `*core.Ctx` with
  `core.NewCtx(httptest.NewRecorder(), httptest.NewRequest(...))` and call a
  handler directly.
* HTTP-level assertions read `rec.Result().Header`, the header snapshot taken at
  `WriteHeader`. Reading `rec.Header()` instead is a false pass, because a
  recorder keeps accepting writes after the status line while a real server does
  not.
* Integration tests that need Redis skip themselves when it is unreachable, and
  CI provides a real instance so they actually run. See `docs/getting-started.md`
  for the environment variable.
* CI runs the suite with `-race`, which is how the timeout middleware's
  double-write was found.
