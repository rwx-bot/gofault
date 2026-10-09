// Package controller provides the controller base type and HTTP method helpers.
package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/gofault/gofault/core"
)

// BaseController provides helper methods for writing HTTP handlers.
type BaseController struct{}

// JSON writes a JSON response with the given status code.
func (c *BaseController) JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// Text writes a plain text response.
func (c *BaseController) Text(w http.ResponseWriter, status int, msg string) error {
	w.WriteHeader(status)
	_, err := w.Write([]byte(msg))
	return err
}

// Param extracts a path parameter from the context.
func Param(ctx *core.Ctx, name string) string {
	return ctx.Params[name]
}

// Query extracts a query string parameter.
func Query(ctx *core.Ctx, name string) string {
	return ctx.Request.URL.Query().Get(name)
}

// InvokeHandler dispatches to the named controller method.
//
// The method must have the signature func(*core.Ctx) error. A method that
// exists but has a different signature is a programming error, and is reported
// as such rather than panicking: a bare type assertion on an action with the
// wrong signature would take the process down from inside request handling.
//
// The HTTP method and path are not consulted. Routing has already happened by
// the time a handler runs, so they were dead parameters that made the signature
// suggest a dispatch that never occurred.
func InvokeHandler(ctrl core.Controller, handlerName string, ctx *core.Ctx) error {
	v := reflect.ValueOf(ctrl)
	methodVal := v.MethodByName(handlerName)
	if !methodVal.IsValid() {
		return fmt.Errorf("handler method %q not found on controller", handlerName)
	}

	// Compare structurally: a method's reflected type is the unnamed func
	// type, so it never equals the named ControllerMethod even when the
	// signatures are identical.
	mt := methodVal.Type()
	if mt.Kind() != reflect.Func || mt.NumIn() != 1 || mt.NumOut() != 1 ||
		mt.In(0) != reflect.TypeOf((*core.Ctx)(nil)) ||
		mt.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		return fmt.Errorf("handler method %q has signature %s, want func(*core.Ctx) error", handlerName, mt)
	}

	return methodVal.Interface().(func(ctx *core.Ctx) error)(ctx)
}

// ControllerMethod is the signature for controller action methods.
type ControllerMethod func(ctx *core.Ctx) error

// Response represents a standard API response.
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// OK sends a 200 JSON response.
func OK(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(Response{Code: 0, Message: "success", Data: data})
}

// Error sends an error JSON response.
func Error(w http.ResponseWriter, status int, msg string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(Response{Code: status, Message: msg})
}
