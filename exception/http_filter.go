package exception

import (
	"encoding/json"

	"github.com/gofault/gofault/core"
)

// HTTPExceptionResponse is the JSON structure returned on HTTP exceptions.
type HTTPExceptionResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// HTTPExceptionFilter is a filter that handles HTTPException errors.
type HTTPExceptionFilter struct {
	// IncludeStackTrace includes error stack in response (for debugging).
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

	w := ctx.Response
	// Content-Type must be set before WriteHeader: once the status line is
	// written the header map is committed and further Set calls are ignored,
	// which would leave the JSON body sniffed as text/plain.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpErr.GetStatusCode())
	json.NewEncoder(w).Encode(map[string]any{
		"code":    resp.Code,
		"message": resp.Message,
	})
	return true
}

// DefaultFilter returns the global default HTTP exception filter.
func DefaultFilter() ExceptionFilter {
	return NewHTTPExceptionFilter()
}
