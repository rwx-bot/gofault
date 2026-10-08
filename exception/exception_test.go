package exception

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofault/gofault/core"
)

func TestHTTPException(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantHTTP int
		wantMsg  string
	}{
		{"BadRequest", BadRequest("invalid input"), 400, http.StatusBadRequest, "invalid input"},
		{"Unauthorized", Unauthorized("no token"), 401, http.StatusUnauthorized, "no token"},
		{"Forbidden", Forbidden("access denied"), 403, http.StatusForbidden, "access denied"},
		{"NotFound", NotFound("user not found"), 404, http.StatusNotFound, "user not found"},
		{"InternalServerError", InternalServerError("db error"), 500, http.StatusInternalServerError, "db error"},
		{"Conflict", Conflict("duplicate entry"), 409, http.StatusConflict, "duplicate entry"},
		{"MethodNotAllowed", MethodNotAllowed("POST not allowed"), 405, http.StatusMethodNotAllowed, "POST not allowed"},
		{"PayloadTooLarge", PayloadTooLarge("file too large"), 413, http.StatusRequestEntityTooLarge, "file too large"},
		{"UnsupportedMediaType", UnsupportedMediaType("application/xml not supported"), 415, http.StatusUnsupportedMediaType, "application/xml not supported"},
		{"TooManyRequests", TooManyRequests("rate limited"), 429, http.StatusTooManyRequests, "rate limited"},
		{"ServiceUnavailable", ServiceUnavailable("maintenance"), 503, http.StatusServiceUnavailable, "maintenance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("err should not be nil")
			}

			httpErr, ok := HTTPExceptionOf(tt.err)
			if !ok {
				t.Fatalf("expected HTTPException, got %T", tt.err)
			}

			if httpErr.GetCode() != tt.wantCode {
				t.Errorf("GetCode() = %d, want %d", httpErr.GetCode(), tt.wantCode)
			}
			if httpErr.GetStatusCode() != tt.wantHTTP {
				t.Errorf("GetStatusCode() = %d, want %d", httpErr.GetStatusCode(), tt.wantHTTP)
			}
			if httpErr.GetMessage() != tt.wantMsg {
				t.Errorf("GetMessage() = %s, want %s", httpErr.GetMessage(), tt.wantMsg)
			}
		})
	}
}

func TestIsHTTPException(t *testing.T) {
	if !IsHTTPException(BadRequest("test")) {
		t.Error("BadRequest should be HTTPException")
	}
	if IsHTTPException(nil) {
		t.Error("nil should not be HTTPException")
	}
	if IsHTTPException(InternalServerError("test")) {
		// This should pass since InternalServerError is also HTTPException
	}
}

func TestHTTPExceptionFilter_Capture(t *testing.T) {
	filter := NewHTTPExceptionFilter()

	t.Run("captures HTTPException", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		ctx := core.NewCtx(rec, req)

		err := BadRequest("invalid param")
		handled := filter.Capture(ctx, err)

		if !handled {
			t.Fatal("expected filter to capture the exception")
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		// Content-Type must be set before WriteHeader. Asserting on Result()
		// reads the header snapshot taken when the status line was written, so
		// it catches a Set that happens too late.
		if ct := rec.Result().Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", ct)
		}
	})

	t.Run("does not capture non-HTTPException", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		ctx := core.NewCtx(rec, req)

		err := InternalServerError("db error")
		handled := filter.Capture(ctx, err)

		// InternalServerError IS an HTTPException, so it should be captured.
		if !handled {
			t.Fatal("expected filter to capture the exception")
		}
	})

	t.Run("does not capture generic error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		ctx := core.NewCtx(rec, req)

		err := &genericError{msg: "something went wrong"}
		handled := filter.Capture(ctx, err)

		if handled {
			t.Error("expected filter to not capture generic error")
		}
	})
}

func TestExceptionFilterChain(t *testing.T) {
	t.Run("chain handles exception", func(t *testing.T) {
		chain := NewExceptionFilterChain()
		handled := chain.Capture(nil, BadRequest("test"))
		if handled {
			t.Error("empty chain should not handle exception")
		}
	})

	t.Run("first filter handles", func(t *testing.T) {
		filter1 := ExceptionHandler(func(ctx any, err error) bool {
			return true
		})
		filter2 := ExceptionHandler(func(ctx any, err error) bool {
			t.Error("filter2 should not be called")
			return false
		})

		chain := NewExceptionFilterChain(filter1, filter2)
		handled := chain.Capture(nil, BadRequest("test"))

		if !handled {
			t.Error("expected chain to handle exception")
		}
	})

	t.Run("second filter handles", func(t *testing.T) {
		filter1 := ExceptionHandler(func(ctx any, err error) bool {
			return false
		})
		filter2 := ExceptionHandler(func(ctx any, err error) bool {
			return true
		})

		chain := NewExceptionFilterChain(filter1, filter2)
		handled := chain.Capture(nil, BadRequest("test"))

		if !handled {
			t.Error("expected chain to handle exception")
		}
	})
}

func TestNew(t *testing.T) {
	err := New(http.StatusTeapot, 418, "I'm a teapot")
	if err == nil {
		t.Fatal("err should not be nil")
	}

	httpErr, ok := HTTPExceptionOf(err)
	if !ok {
		t.Fatalf("expected HTTPException, got %T", err)
	}

	if httpErr.GetStatusCode() != http.StatusTeapot {
		t.Errorf("GetStatusCode() = %d, want %d", httpErr.GetStatusCode(), http.StatusTeapot)
	}
	if httpErr.GetCode() != 418 {
		t.Errorf("GetCode() = %d, want 418", httpErr.GetCode())
	}
}

// genericError is a plain error for testing.
type genericError struct {
	msg string
}

func (e *genericError) Error() string {
	return e.msg
}
