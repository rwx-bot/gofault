package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofault/gofault/core"
)

func TestBaseController_JSON(t *testing.T) {
	ctrl := BaseController{}
	w := httptest.NewRecorder()
	err := ctrl.JSON(w, http.StatusOK, map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("JSON failed: %v", err)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["key"] != "value" {
		t.Fatalf("expected 'value', got %s", resp["key"])
	}
}

func TestOK(t *testing.T) {
	w := httptest.NewRecorder()
	err := OK(w, map[string]string{"msg": "test"})
	if err != nil {
		t.Fatalf("OK failed: %v", err)
	}
	var r Response
	json.Unmarshal(w.Body.Bytes(), &r)
	if r.Code != 0 || r.Message != "success" {
		t.Fatalf("unexpected response: %+v", r)
	}
	// Assert on Result() (the header snapshot taken at WriteHeader) rather than
	// Header(): a recorder keeps accepting Set calls after the status line is
	// written, whereas a real server commits the headers and would serve this
	// body as text/plain.
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

func TestParamExtraction(t *testing.T) {
	ctx := &core.Ctx{Params: map[string]string{"name": "alice"}}
	if Param(ctx, "name") != "alice" {
		t.Fatal("param extraction failed")
	}
}

func TestBaseController_Text(t *testing.T) {
	ctrl := BaseController{}
	w := httptest.NewRecorder()
	if err := ctrl.Text(w, http.StatusAccepted, "plain body"); err != nil {
		t.Fatalf("Text failed: %v", err)
	}
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if w.Body.String() != "plain body" {
		t.Errorf("body = %q, want %q", w.Body.String(), "plain body")
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	if err := Error(w, http.StatusNotFound, "missing"); err != nil {
		t.Fatalf("Error failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var r Response
	json.Unmarshal(w.Body.Bytes(), &r)
	if r.Code != http.StatusNotFound || r.Message != "missing" {
		t.Errorf("response = %+v, want code 404 / message missing", r)
	}
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

func TestQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/search?q=go&page=2", nil)
	ctx := core.NewCtx(httptest.NewRecorder(), req)

	if got := Query(ctx, "q"); got != "go" {
		t.Errorf("Query(q) = %q, want %q", got, "go")
	}
	if got := Query(ctx, "page"); got != "2" {
		t.Errorf("Query(page) = %q, want %q", got, "2")
	}
	if got := Query(ctx, "missing"); got != "" {
		t.Errorf("Query(missing) = %q, want empty", got)
	}
}

type greetController struct {
	BaseController
	called *bool
}

func (g *greetController) Routes() []core.Route { return nil }
func (g *greetController) Prefix() string       { return "" }

func (g *greetController) Greet(ctx *core.Ctx) error {
	*g.called = true
	return OK(ctx.Response, "hello")
}

type wrongSignatureController struct {
	BaseController
}

func (w *wrongSignatureController) Routes() []core.Route { return nil }
func (w *wrongSignatureController) Prefix() string       { return "" }

func (w *wrongSignatureController) Bad(ctx *core.Ctx) string { return "" }

func TestInvokeHandler(t *testing.T) {
	called := false
	ctrl := &greetController{called: &called}
	ctx := core.NewCtx(httptest.NewRecorder(), httptest.NewRequest("GET", "/greet", nil))

	if err := InvokeHandler(ctrl, "Greet", ctx); err != nil {
		t.Fatalf("InvokeHandler: %v", err)
	}
	if !called {
		t.Error("handler was not invoked")
	}
}

func TestInvokeHandler_UnknownMethod(t *testing.T) {
	ctrl := &greetController{}
	ctx := core.NewCtx(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))

	if err := InvokeHandler(ctrl, "Nope", ctx); err == nil {
		t.Error("expected an error for a missing handler method")
	}
}

// A handler with the wrong signature must surface as an error. Before this was
// checked, the type assertion panicked and took down request handling.
func TestInvokeHandler_WrongSignatureDoesNotPanic(t *testing.T) {
	ctrl := &wrongSignatureController{}
	ctx := core.NewCtx(httptest.NewRecorder(), httptest.NewRequest("GET", "/bad", nil))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("InvokeHandler panicked: %v", r)
		}
	}()

	if err := InvokeHandler(ctrl, "Bad", ctx); err == nil {
		t.Error("expected an error for a handler with the wrong signature")
	}
}
