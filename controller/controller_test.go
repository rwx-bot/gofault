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
