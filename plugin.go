// Package dynamicheadersplugin provides a Traefik middleware plugin for dynamic header modification.
package dynamicheadersplugin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// Compile regex for finding {placeholder} patterns in the format string
// This safely handles nested or malformed braces in the replacement process.
var placeholderRegex = regexp.MustCompile(`\${([^}]+)}`)

// Config holds the plugin configuration including all header modification rules.
type Config struct {
	Rules []HeaderSettingRule `json:"rules,omitempty"`
}

// CreateConfig creates and returns a new Config instance with initialized rules.
func CreateConfig() *Config {
	return &Config{
		Rules: make([]HeaderSettingRule, 0),
	}
}

// Plugin represents the Traefik middleware plugin for dynamic header modification.
type Plugin struct {
	config *Config
	next   http.Handler
	name   string
}

// ServeHTTP implements the http.Handler interface for the plugin.
func (plugin Plugin) ServeHTTP(requestWriter http.ResponseWriter, request *http.Request) {
	for _, rule := range plugin.config.Rules {
		rule.Apply(request)
	}

	plugin.next.ServeHTTP(requestWriter, request)
}

// New creates and initializes a new Plugin instance with the provided configuration.
// It performs comprehensive validation of all rules before plugin instantiation to ensure
// the plugin starts in a valid state.
//
// Parameters:
//   - config: Pointer to a PluginConfiguration containing the plugin's rules and settings.
//     Must not be nil and must contain valid rules.
//
// Returns:
//   - *Plugin: A fully initialized plugin instance ready for use.
//   - error: Descriptive error if any rule validation fails or if config is nil.
//     Returns nil on successful initialization.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if config == nil {
		return nil, errors.New("plugin configuration is missing")
	}

	for i := range config.Rules {
		err := config.Rules[i].Validate()
		if err != nil {
			return nil, fmt.Errorf("rule error: %w", err)
		}
	}

	return Plugin{
		name:   name,
		next:   next,
		config: config,
	}, nil
}

// HeaderSettingRule defines a rule for modifying HTTP headers through regex pattern matching and substitution.
// It supports operations on request/response headers with configurable targets and default values.
type HeaderSettingRule struct {
	// HeaderName specifies the HTTP header to be modified (e.g., "Content-Type", "Authorization").
	// This field is required for all operations.
	HeaderName string `json:"headerName,omitempty"`

	// Regex contains the regular expression pattern to match against the header value.
	// Must use Go's regex syntax (https://golang.org/pkg/regexp/).
	// This field is required for all rewrite operations.
	Regex string `json:"regex,omitempty"`

	CompiledRegex *regexp.Regexp `json:"-"`

	RegexGroupNames []string `json:"-"`

	// Format defines the replacement pattern for matched regex groups.
	// Uses re2 syntax for group references (e.g., $1, $2 for capture groups, $0 for entire match).
	// Defaults to "$0" (maintains original value) if not specified.
	Format string `json:"format,omitempty"`

	// Target specifies where the header modification should be applied.
	// Valid values: "request", "response", or "host" (default).
	Target string `json:"target,omitempty"`

	// Default provides a fallback value when the regex doesn't match the header value.
	// If empty and no match occurs, the header remains unchanged.
	Default string `json:"default,omitempty"`
}

// Validate performs structural and semantic validation of the HeaderSettingRule configuration.
// Returns an error if any required field is missing or contains invalid values.
//
// Validation checks:
//   - HeaderName and Regex are required fields
//   - Compiles the regex to ensure syntactic validity
//   - Sets default values for Format and Target if not provided
//
// Returns:
//   - error: descriptive validation error if rule configuration is invalid, nil otherwise
func (rule *HeaderSettingRule) Validate() error {
	// Validate required fields
	if rule.HeaderName == "" {
		return errors.New("headerName is required")
	}

	if rule.Regex == "" {
		return errors.New("regex is required")
	}

	if rule.Format == "" {
		return errors.New("format is required")
	}

	if rule.Target == "" {
		rule.Target = "host" // Default to Host header modification
	}

	exp, err := regexp.Compile(rule.Regex)
	if err != nil {
		return fmt.Errorf("invalid regex pattern '%s': %w", rule.Regex, err)
	}

	rule.CompiledRegex = exp

	namedGroups := make(map[string]bool)

	rule.RegexGroupNames = exp.SubexpNames()

	for _, name := range exp.SubexpNames() {
		if name != "" {
			namedGroups[name] = true
		}
	}

	// Find all references in format string (format: ${name})
	matches := placeholderRegex.FindAllStringSubmatch(rule.Format, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		groupName := match[1]

		if !namedGroups[groupName] {
			return fmt.Errorf("format string references unknown group '%s'", groupName)
		}
	}

	return nil
}

// Apply applies the header setting rule to the incoming HTTP request by:
// 1. Extracting the target value from the request based on rule configuration
// 2. Compiling the regex pattern (pre-validated during rule creation)
// 3. Formatting the target value using the regex groups and format template
// 4. Setting the resulting value to the specified request header
//
// If formatting fails, the error is logged and the header remains unmodified.
func (rule *HeaderSettingRule) Apply(request *http.Request) {
	// Extract target value from request based on rule configuration (URL, body, etc.)
	target := rule.GetTarget(request)

	// Compile regex pattern - guaranteed to succeed due to prior validation
	// MustCompile panics only if the regex is invalid, which is prevented by validation
	regex := rule.CompiledRegex

	// Apply regex formatting with capture groups
	formatted, err := FormatWithGroups(regex, target, rule.Format, rule.RegexGroupNames)
	if err != nil {
		// Log formatting failure but don't block request processing
		log.Printf("failed to format header value: header=%s target=%q regex=%q format=%q error=%v",
			rule.HeaderName, target, rule.Regex, rule.Format, err)

		if rule.Default != "" {
			request.Header.Set(rule.HeaderName, rule.Default)
		}

		return
	}

	// Set the formatted value to the specified header
	request.Header.Set(rule.HeaderName, formatted)
}

// GetTarget extracts the target value from the HTTP request based on the rule's target configuration.
// It supports various request components including host, path, URL, method, scheme, query parameters,
// user agent, referer, and custom headers. Returns an empty string if the target doesn't exist.
//
// Supported targets:
//   - "host": Request host (e.g., "example.com:8080")
//   - "path": URL path (e.g., "/api/v1/users")
//   - "url": Full URL string
//   - "method": HTTP method (e.g., "GET", "POST")
//   - "scheme": URL scheme (e.g., "https", "http")
//   - "query": Raw query string (e.g., "page=1&limit=10")
//   - "userAgent": User-Agent header value
//   - "referer": Referer header value
//   - "header:<name>": Custom header value (e.g., "header:X-API-Key")
func (rule *HeaderSettingRule) GetTarget(request *http.Request) string {
	switch rule.Target {
	case "host":
		return request.Host
	case "path":
		return request.URL.Path
	case "url":
		return request.URL.String()
	case "method":
		return request.Method
	case "scheme":
		return request.URL.Scheme
	case "query":
		return request.URL.RawQuery
	case "userAgent":
		return request.UserAgent()
	case "referer":
		return request.Referer()
	default:
		// Handle dynamic header targets with "header:" prefix
		if after, ok := strings.CutPrefix(rule.Target, "header:"); ok {
			headerName := after
			return request.Header.Get(headerName)
		}

		// Fallback to host for unknown targets - provides predictable default behavior
		// This ensures the function always returns a meaningful value
		return request.Host
	}
}

// FormatWithGroups Applies a regex pattern to an input string and formats the result
// using named capture groups from the regex pattern. It enables dynamic string
// construction by substituting named group matches into a template format.
//
// Parameters:
//   - pattern: Compiled regex pattern containing named capture groups
//   - input: String to match against the regex pattern
//   - format: Template string with {named} placeholders for group substitution
//
// Returns:
//   - Formatted string with group values substituted, or empty string on error
//   - Error if input doesn't match pattern or other formatting issues occur
//
// Example:
//
//	pattern: regexp.MustCompile(`(?P<name>\w+)\s+(?P<age>\d+)`)
//	input: "John 25"
//	format: "User ${name} is ${age} years old"
//	returns: "User John is 25 years old", nil
func FormatWithGroups(pattern *regexp.Regexp, input, format string, subexpNames []string) (string, error) {
	// Find all submatches including named capture groups
	// Returns nil if no match found, allowing early exit
	//
	match := pattern.FindStringSubmatch(input)
	if match == nil {
		return "", fmt.Errorf("input %q does not match pattern %q", input, pattern.String())
	}

	// Create mapping from group names to their captured values
	// Skip the first group (index 0) which is the full match
	groupValues := make(map[string]string)

	for i, name := range subexpNames {
		// Only process named groups (non-empty names) and ensure valid index
		if name != "" && i > 0 && i < len(match) {
			groupValues[name] = match[i]
		}
	}

	// Replace all {group} placeholders with their corresponding values
	// Unmatched placeholders will be replaced with empty strings
	result := placeholderRegex.ReplaceAllStringFunc(format, func(placeholder string) string {
		// Extract group name by removing surrounding braces
		groupName := placeholder[2 : len(placeholder)-1]

		// Return captured value or empty string if group not found
		// This provides graceful degradation for missing groups
		return groupValues[groupName]
	})

	return result, nil
}
