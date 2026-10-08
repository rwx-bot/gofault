# Getting started

This guide takes you from an empty module to a running service, then explains how
to test it.

## Install

```bash
go get github.com/gofault/gofault
```

The framework targets the Go version declared in `go.mod`.

## A minimal application

An application is a set of modules. A module carries controllers, providers,
middleware and lifecycle hooks; a controller carries a route table.

```go
package main

import (
    "log"

    "github.com/gofault/gofault/controller"
    "github.com/gofault/gofault/core"
    "github.com/gofault/gofault/module"
)

// GreetingService is an injectable provider.
type GreetingService struct {
    prefix string
}

func (s *GreetingService) Provide() any { return s }

func (s *GreetingService) Greet(name string) string {
    return s.prefix + ", " + name + "!"
}

type GreetingController struct {
    controller.BaseController
    svc *GreetingService
}

func NewGreetingController(svc *GreetingService) *GreetingController {
    return &GreetingController{svc: svc}
}

func (c *GreetingController) Prefix() string { return "/hello" }

func (c *GreetingController) Routes() []core.Route {
    return []core.Route{
        {Method: "GET", Path: "/", Handler: "Index"},
        {Method: "GET", Path: "/greet/:name", Handler: "Greet"},
    }
}

func (c *GreetingController) Index(ctx *core.Ctx) error {
    return controller.OK(ctx.Response, map[string]string{"message": "gofault is running"})
}

func (c *GreetingController) Greet(ctx *core.Ctx) error {
    return controller.OK(ctx.Response, map[string]string{
        "message": c.svc.Greet(controller.Param(ctx, "name")),
    })
}

func main() {
    svc := &GreetingService{prefix: "Hello"}
    ctrl := NewGreetingController(svc)

    mod := core.NewModule("hello")
    mod.RegisterControllers(ctrl)
    mod.RegisterProviders(svc)

    app := module.New()
    if err := app.RegisterModules(mod); err != nil {
        log.Fatal(err)
    }
    log.Fatal(app.Start(9090))
}
```

Run it and call the routes:

```bash
go run .
curl localhost:9090/hello/
# {"code":0,"message":"success","data":{"message":"gofault is running"}}
curl localhost:9090/hello/greet/World
# {"code":0,"message":"success","data":{"message":"Hello, World!"}}
```

## Adding middleware and error handling

Middleware lives on the module, so it only applies to that module's routes. The
exception filter lives on the router, because it has to catch errors from every
route:

```go
import (
    "github.com/gofault/gofault/exception"
    "github.com/gofault/gofault/middleware"
    "github.com/gofault/gofault/router"
)

func loggingMiddleware(ctx *core.Ctx, next core.Handler) error {
    log.Printf("%s %s", ctx.Request.Method, ctx.Request.URL.Path)
    return next(ctx)
}

mod.RegisterMiddleware(
    middleware.RecoveryMiddleware(middleware.DefaultRecoveryConfig()),
    loggingMiddleware,
)

rtr := router.New()
rtr.ExceptionFilter(exception.NewHTTPExceptionFilter())
app.SetRouter(rtr)
```

Handlers now report failures by returning an error rather than writing a
response themselves:

```go
func (c *GreetingController) Greet(ctx *core.Ctx) error {
    name := controller.Param(ctx, "name")
    if name == "" {
        return exception.BadRequest("name is required")
    }
    return controller.OK(ctx.Response, map[string]string{"message": c.svc.Greet(name)})
}
```

## Declaring module dependencies

`Depends` orders initialisation without constraining the order you register
modules in:

```go
db := core.NewModule("database")
api := core.NewModule("api")
api.Depends = []string{"database"}

// Registration order does not matter; "database" still initialises first.
app.RegisterModules(api, db)
```

Unknown or circular dependencies are rejected at boot rather than at runtime:

```
module "api" depends on unknown module "databse"
circular dependency detected among modules: [a b]
```

## Lifecycle hooks

Implement any of these on a provider to take part in startup and shutdown:

```go
func (s *GreetingService) OnBoot() error {
    log.Println("greeting service ready")
    return nil
}

func (s *GreetingService) OnShutdown() error {
    log.Println("greeting service stopped")
    return nil
}

mod.RegisterOnBoot(svc)
mod.RegisterOnShutdown(svc)
```

Shutdown hooks run in reverse order, and `App.Stop` drains the HTTP server after
they finish, so a hook that flushes a buffer or closes a pool is safe.

## Configuration

`config.Load` reads YAML into typed sections:

```go
cfg, err := config.Load("config.yaml")
if err != nil {
    return err
}

log.Printf("listening on %s", cfg.Server.GetAddress())
log.Printf("dsn %s", cfg.Database.GetDSN())
```

## Testing

```bash
go test ./...
go test ./... -race      # what CI runs
```

Techniques that keep the suite honest:

* **Write against the interfaces.** `core.Handler` and `core.MiddlewareFunc` are
  plain function types, so a test can call a handler with a `httptest`
  recorder and no server:
  ```go
  rec := httptest.NewRecorder()
  ctx := core.NewCtx(rec, httptest.NewRequest("GET", "/hello/", nil))
  if err := ctrl.Index(ctx); err != nil {
      t.Fatal(err)
  }
  ```
* **Assert on `Result()`, not `Header()`.** `httptest.ResponseRecorder` keeps
  accepting header writes after the status line, while a real server commits
  them. A test that reads `rec.Result().Header` reproduces production behaviour
  and catches a `Content-Type` set too late.
* **Always run `-race`.** Concurrency bugs in middleware surface as a race
  report rather than a flaky assertion.

Integration tests that need an external service skip themselves when it is not
reachable, so a bare checkout stays green. Point them at a real one when you
have it:

```bash
GOFAULT_REDIS_ADDR=127.0.0.1:6379 go test ./...
```

## Project layout

```
core/         contracts shared by everything else
ioc/          dependency injection container
provider/     injectable values, including request-scoped ones
router/       method and path dispatch, middleware chain, exception filter
server/       net/http listener and graceful shutdown
module/       composition root
controller/   controller base type and response helpers
middleware/   cross-cutting handlers
exception/    HTTP errors and the filter that maps them to responses
config/       YAML configuration
logger/       structured logging
versioning/   API version negotiation
gorm/ redis/ grpc/   optional integrations
```

## Where to go next

* `docs/architecture.md` for how the pieces fit together and why.
* `examples/hello` for a runnable application with a provider, middleware and
  both lifecycle hooks.
