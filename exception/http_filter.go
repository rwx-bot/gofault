package exception

import (
	"encoding/json"
	"runtime"

	"github.com/gofault/gofault/core"
)

// HTTPExceptionResponse is the JSON structure returned on HTTP exceptions.
type HTTPExceptionResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// Stack is only populated when HTTPExceptionFilter.IncludeStackTrace is
	// set. It exposes internal file paths and function names, so it must stay
	// off outside development.
	Stack string `json:"stack,omitempty"`
}

// HTTPExceptionFilter is a filter that handles HTTPException errors.
type HTTPExceptionFilter struct {
	// IncludeStackTrace includes error stack in response (for debugging).
	// Leave it off outside development: the stack exposes internal paths and
	// symbols to the client.
	IncludeStackTrace bool
}

// NewHTTPExceptionFilter creates an HTTPExceptionFilter with default settings.
func NewHTTPExceptionFilter() *HTTPExceptionFilter {
	return &HTTPExceptionFilter{}
}

// Capture handles HTTPException errors and writes the appropriate response.
func (f *HTTPExceptionFilter) Capture(ctxAny any, err error) bool {
	ctx, ok := ctxAny.(*core.Ctx)
	if !ok {
		return false
	}

	httpErr, ok := HTTPExceptionOf(err)
	if !ok {
		// Not an HTTPException, let it propagate.
		return false
	}

	resp := HTTPExceptionResponse{
		Code:    httpErr.GetCode(),
		Message: httpErr.GetMessage(),
	}
	if f.IncludeStackTrace {
		resp.Stack = captureStack()
	}

	w := ctx.Response
	// Content-Type must be set before WriteHeader: once the status line is
	// written the header map is committed and further Set calls are ignored,
	// which would leave the JSON body sniffed as text/plain.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpErr.GetStatusCode())
	_ = json.NewEncoder(w).Encode(resp)
	return true
}

// captureStack renders the stack of the goroutine handling the request.
func captureStack() string {
	buf := make([]byte, 8<<10)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// DefaultFilter returns the global default HTTP exception filter.
func DefaultFilter() ExceptionFilter {
	return NewHTTPExceptionFilter()
}
