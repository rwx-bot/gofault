package middleware

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/exception"
)

// ValidationError represents a single validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	var msgs []string
	for _, e := range ve {
		msgs = append(msgs, e.Field+": "+e.Message)
	}
	return strings.Join(msgs, "; ")
}

// ValidationRule defines a single validation rule.
type ValidationRule interface {
	Validate(field string, value interface{}) *ValidationError
}

// Required validates that a value is not empty.
type Required struct{}

func (r *Required) Validate(field string, value interface{}) *ValidationError {
	if value == nil || (reflect.TypeOf(value).Kind() == reflect.String && strings.TrimSpace(value.(string)) == "") {
		return &ValidationError{Field: field, Message: "is required"}
	}
	return nil
}

// MinLength validates minimum string length.
type MinLength struct {
	Length int
}

func (r *MinLength) Validate(field string, value interface{}) *ValidationError {
	if str, ok := value.(string); ok {
		if len(str) < r.Length {
			return &ValidationError{Field: field, Message: "must be at least " + itoa(r.Length) + " characters"}
		}
	}
	return nil
}

// MaxLength validates maximum string length.
type MaxLength struct {
	Length int
}

func (r *MaxLength) Validate(field string, value interface{}) *ValidationError {
	if str, ok := value.(string); ok {
		if len(str) > r.Length {
			return &ValidationError{Field: field, Message: "must be at most " + itoa(r.Length) + " characters"}
		}
	}
	return nil
}

// Regex validates that a string matches a regex pattern.
type Regex struct {
	Pattern string
	regex   *regexp.Regexp
}

func (r *Regex) Validate(field string, value interface{}) *ValidationError {
	if str, ok := value.(string); ok {
		if r.regex == nil {
			r.regex = regexp.MustCompile(r.Pattern)
		}
		if !r.regex.MatchString(str) {
			return &ValidationError{Field: field, Message: "has invalid format"}
		}
	}
	return nil
}

// Email validates email format.
type Email struct{}

func (r *Email) Validate(field string, value interface{}) *ValidationError {
	if str, ok := value.(string); ok {
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
		if !emailRegex.MatchString(str) {
			return &ValidationError{Field: field, Message: "must be a valid email address"}
		}
	}
	return nil
}

// Min validates minimum numeric value.
type Min struct {
	Value float64
}

func (r *Min) Validate(field string, value interface{}) *ValidationError {
	switch v := value.(type) {
	case int:
		if float64(v) < r.Value {
			return &ValidationError{Field: field, Message: "must be at least " + itoa(int(r.Value))}
		}
	case float64:
		if v < r.Value {
			return &ValidationError{Field: field, Message: "must be at least " + ftos(r.Value)}
		}
	case string:
		// Skip for string types
	}
	return nil
}

// Max validates maximum numeric value.
type Max struct {
	Value float64
}

func (r *Max) Validate(field string, value interface{}) *ValidationError {
	switch v := value.(type) {
	case int:
		if float64(v) > r.Value {
			return &ValidationError{Field: field, Message: "must be at most " + itoa(int(r.Value))}
		}
	case float64:
		if v > r.Value {
			return &ValidationError{Field: field, Message: "must be at most " + ftos(r.Value)}
		}
	}
	return nil
}

// Rules defines validation rules for a struct field.
type Rules map[string][]ValidationRule

// ValidateRequest validates the request against defined rules.
//
// Rules are supplied per call rather than per route; there is no validator
// middleware. A previous ValidatorConfig (with BindTarget and SkipMissing)
// promised one but was never read by any code, so setting it did nothing.
func ValidateRequest(ctx *core.Ctx, rules Rules) error {
	var errs ValidationErrors

	for field, rules := range rules {
		value := getFieldValue(ctx, field)
		for _, rule := range rules {
			if err := rule.Validate(field, value); err != nil {
				errs = append(errs, *err)
			}
		}
	}

	if len(errs) > 0 {
		return exception.BadRequest(errs.Error())
	}
	return nil
}

// getFieldValue gets a field value from the request.
func getFieldValue(ctx *core.Ctx, field string) interface{} {
	// Try path params first
	if ctx.Params != nil {
		if v, ok := ctx.Params[field]; ok {
			return v
		}
	}

	// Try query params
	if ctx.Request.URL != nil {
		if v := ctx.Request.URL.Query().Get(field); v != "" {
			return v
		}
	}

	return nil
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

// ftos converts float64 to string without importing strconv.
func ftos(f float64) string {
	if f == float64(int64(f)) {
		return itoa(int(f))
	}
	// For simplicity, return formatted string
	i := int(f * 100)
	return itoa(i/100) + "." + itoa((i%100)/10) + itoa((i%100)%10)
}
