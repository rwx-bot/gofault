// Package versioning provides API versioning support for HTTP routers.
package versioning

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofault/gofault/core"
)

// VersionStatus indicates the lifecycle state of an API version.
type VersionStatus int

const (
	// VersionStatusActive is a currently supported version.
	VersionStatusActive VersionStatus = iota
	// VersionStatusDeprecated indicates a version pending retirement.
	VersionStatusDeprecated
	// VersionStatusEOL signals end-of-life and should not be used.
	VersionStatusEOL
)

func (s VersionStatus) String() string {
	switch s {
	case VersionStatusActive:
		return "active"
	case VersionStatusDeprecated:
		return "deprecated"
	case VersionStatusEOL:
		return "eol"
	default:
		return "unknown"
	}
}

// Strategy defines how the version is extracted from a request.
type Strategy int

const (
	// StrategyHeader extracts version from a header (e.g., Accept).
	StrategyHeader Strategy = iota
	// StrategyPathPrefix extracts version from URL path prefix.
	StrategyPathPrefix
	// StrategyQuery extracts version from query parameter.
	StrategyQuery
)

// Config holds versioning middleware configuration.
type Config struct {
	// Strategy for version extraction.
	Strategy Strategy
	// Header is used with StrategyHeader: header name to read.
	Header string
	// HeaderVersionFormat is used with StrategyHeader: printf format (e.g., "v%d" or "version=%s").
	HeaderVersionFormat string
	// HeaderVersionRE is used with StrategyHeader: regex to extract version after matching headerVersionFormat.
	HeaderVersionRE *regexp.Regexp
	// PathPrefix is used with StrategyPathPrefix: URL path prefix (e.g., "/v1").
	PathPrefix string
	// QueryParam is used with StrategyQuery: query parameter name (e.g., "version").
	QueryParam string
	// DefaultVersion returned when no version is detected.
	DefaultVersion int
	// DefaultStatus for versions without explicit status.
	DefaultStatus VersionStatus
}

// HeaderConfig creates a header-based versioning config.
// headerVersionFormat uses %d for numeric version, %s for string version.
// Example: "application/vnd.api+json;version=v%d" extracts "1" from "v1".
//
// headerVersionRE may be nil, in which case one is derived from
// headerVersionFormat. The field used to be stored but never applied, so a
// caller who supplied only the documented format got DefaultVersion for every
// request.
func HeaderConfig(header, headerVersionFormat string, headerVersionRE *regexp.Regexp, defaultVersion int) Config {
	if headerVersionRE == nil && headerVersionFormat != "" {
		headerVersionRE = compileVersionFormat(headerVersionFormat)
	}
	return Config{
		Strategy:            StrategyHeader,
		Header:              header,
		HeaderVersionFormat: headerVersionFormat,
		HeaderVersionRE:     headerVersionRE,
		DefaultVersion:      defaultVersion,
		DefaultStatus:       VersionStatusActive,
	}
}

// compileVersionFormat turns a printf-style version format into a regexp with
// one capturing group around the version. For example
// "application/vnd.api+json;version=v%d" yields a pattern whose first group
// captures the digits after "version=v", so FindStringSubmatch yields the
// version in matches[1] as the middleware expects.
func compileVersionFormat(format string) *regexp.Regexp {
	// Split on the verb so the placeholder can be captured.
	parts := strings.SplitN(format, "%", 2)
	if len(parts) != 2 {
		return nil
	}

	var b strings.Builder
	b.WriteString("^")
	b.WriteString(regexp.QuoteMeta(parts[0]))
	b.WriteString(`(.*)`)
	// The verb and any trailing text stay outside the capture.
	b.WriteString(regexp.QuoteMeta(parts[1][1:]))
	b.WriteString("$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}

// PathPrefixConfig creates a path-prefix based versioning config.
// pathPrefix example: "/v1" matches URLs starting with /v1/.
func PathPrefixConfig(pathPrefix string, defaultVersion int) Config {
	return Config{
		Strategy:       StrategyPathPrefix,
		PathPrefix:     pathPrefix,
		DefaultVersion: defaultVersion,
		DefaultStatus:  VersionStatusActive,
	}
}

// QueryConfig creates a query-parameter based versioning config.
// queryParam example: "version" reads ?version=1 from URL.
func QueryConfig(queryParam string, defaultVersion int) Config {
	return Config{
		Strategy:       StrategyQuery,
		QueryParam:     queryParam,
		DefaultVersion: defaultVersion,
		DefaultStatus:  VersionStatusActive,
	}
}

// Middleware returns a middleware that extracts API version into ctx.Locals["version"].
func Middleware(config Config) core.MiddlewareFunc {
	return func(ctx *core.Ctx, next core.Handler) error {
		version := config.DefaultVersion
		status := config.DefaultStatus

		switch config.Strategy {
		case StrategyHeader:
			if config.Header != "" && config.HeaderVersionRE != nil {
				if h := ctx.Request.Header.Get(config.Header); h != "" {
					matches := config.HeaderVersionRE.FindStringSubmatch(h)
					if len(matches) > 1 {
						if v, err := strconv.Atoi(matches[1]); err == nil {
							version = v
						}
					}
				}
			}

		case StrategyPathPrefix:
			path := ctx.Request.URL.Path
			if config.PathPrefix != "" && (path == config.PathPrefix || strings.HasPrefix(path, config.PathPrefix+"/")) {
				// Extract version number from the PathPrefix itself, e.g., "/v3" -> 3
				if matches := pathVersionRE.FindStringSubmatch(config.PathPrefix); len(matches) > 1 {
					if v, err := strconv.Atoi(matches[1]); err == nil {
						version = v
					}
				}
			}

		case StrategyQuery:
			if config.QueryParam != "" {
				if q := ctx.Request.URL.Query().Get(config.QueryParam); q != "" {
					if v, err := strconv.Atoi(q); err == nil {
						version = v
					}
				}
			}
		}

		ctx.SetLocal("version", version)
		ctx.SetLocal("version_status", status)

		return next(ctx)
	}
}

// VersionInfo holds version metadata for a group of routes.
type VersionInfo struct {
	Version int
	Status  VersionStatus
	Message string
}

// DefaultVersionSet creates a version set with the given versions.
func DefaultVersionSet(versions []VersionInfo) *VersionSet {
	vs := &VersionSet{
		versions: make(map[int]VersionInfo),
		byStatus: make(map[VersionStatus][]int),
	}
	for _, v := range versions {
		vs.versions[v.Version] = v
		vs.byStatus[v.Status] = append(vs.byStatus[v.Status], v.Version)
	}
	return vs
}

// VersionSet manages version lifecycle information.
type VersionSet struct {
	versions map[int]VersionInfo
	byStatus map[VersionStatus][]int
}

// Get returns version info and whether the version exists.
func (vs *VersionSet) Get(version int) (VersionInfo, bool) {
	info, ok := vs.versions[version]
	return info, ok
}

// IsSupported returns true if the version exists and is not EOL.
func (vs *VersionSet) IsSupported(version int) bool {
	info, ok := vs.versions[version]
	return ok && info.Status != VersionStatusEOL
}

// IsDeprecated returns true if the version is deprecated.
func (vs *VersionSet) IsDeprecated(version int) bool {
	info, ok := vs.versions[version]
	return ok && info.Status == VersionStatusDeprecated
}

// Add adds or updates a version entry.
func (vs *VersionSet) Add(info VersionInfo) {
	if vs.versions == nil {
		vs.versions = make(map[int]VersionInfo)
		vs.byStatus = make(map[VersionStatus][]int)
	}
	vs.versions[info.Version] = info
	vs.byStatus[info.Status] = append(vs.byStatus[info.Status], info.Version)
}

// Deprecate marks a version as deprecated with an optional message.
func (vs *VersionSet) Deprecate(version int, msg string) {
	if vs.versions == nil {
		vs.versions = make(map[int]VersionInfo)
		vs.byStatus = make(map[VersionStatus][]int)
	}
	info, ok := vs.versions[version]
	if !ok {
		info = VersionInfo{Version: version}
	}
	info.Status = VersionStatusDeprecated
	info.Message = msg
	vs.versions[version] = info
}

// EOL marks a version as end-of-life with an optional message.
func (vs *VersionSet) EOL(version int, msg string) {
	if vs.versions == nil {
		vs.versions = make(map[int]VersionInfo)
		vs.byStatus = make(map[VersionStatus][]int)
	}
	info, ok := vs.versions[version]
	if !ok {
		info = VersionInfo{Version: version}
	}
	info.Status = VersionStatusEOL
	info.Message = msg
	vs.versions[version] = info
}

// ResponseWriter wraps http.ResponseWriter and adds version headers.
type ResponseWriter struct {
	http.ResponseWriter
	Version int
	Status  int
}

// HeaderVersion returns the version header value.
func (w *ResponseWriter) HeaderVersion() string {
	return fmt.Sprintf("v%d", w.Version)
}

// WriteHeader sends version-aware headers before delegating.
func (w *ResponseWriter) WriteHeader(status int) {
	w.Status = status
	if w.Version > 0 {
		w.Header().Set("API-Version", w.HeaderVersion())
	}
	http.ResponseWriter.WriteHeader(w.ResponseWriter, status)
}

// VersionHandler wraps a handler and extracts version info from context.
func VersionHandler(handler core.Handler, vs *VersionSet) core.Handler {
	return func(ctx *core.Ctx) error {
		version := ctx.GetVersion()
		info, ok := vs.Get(version)
		if !ok {
			// Unknown version - let the handler decide
			return handler(ctx)
		}
		// Inject version info into context
		ctx.SetLocal("version_info", info)
		ctx.SetLocal("version_status", info.Status)

		// Add deprecation header if needed
		if info.Status == VersionStatusDeprecated {
			ctx.RespHeader().Set("X-API-Deprecation", "version="+strconv.Itoa(version))
			if info.Message != "" {
				ctx.RespHeader().Set("X-API-Deprecation-Message", info.Message)
			}
		}

		return handler(ctx)
	}
}

var pathVersionRE = regexp.MustCompile(`^/v(\d+)`)
