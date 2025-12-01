![Logo](./.assets/icon.svg)

# Dynamic Headers Plugin for Traefik

A powerful, regex-based HTTP header manipulation middleware for Traefik that enables dynamic header transformations using named capture groups.

## 🚀 Features

- **Dynamic Header Rewriting**: Transform header values using regex patterns with named capture groups
- **Multiple Sources**: Extract values from URL, path, query, headers, and other request components
- **Named Group Support**: Reference captured groups by name using `${groupName}` syntax
- **Comprehensive Validation**: Pre-flight rule validation with detailed error messages
- **Flexible Targeting**: Modify request headers, response headers, or the host header
- **Default Values**: Graceful fallback when patterns don't match

## 📦 Installation

### Static Configuration

Add the plugin to your Traefik static configuration:

```yaml
# traefik.yml
experimental:
  plugins:
    dynamicheaders:
      moduleName: "github.com/your-org/dynamicheadersplugin"
      version: "v1.0.0"
```

Docker Compose
```yaml
services:
  traefik:
    image: traefik:v3.0
    command:
      - "--experimental.plugins.dynamicheaders.moduleName=github.com/clowzed/dynamic-headers-plugin"
      - "--experimental.plugins.dynamicheaders.version=v1.0.0"
```

## ⚙️ Configuration

## Basic structure
```yaml

# dynamic.yml
http:
  middlewares:
    dynamic-headers:
      plugin:
        dynamicheaders:
          rules:
            - headerName: "X-Processed-By"
              regex: "(?P<service>\\w+)-(?P<env>\\w+)"
              format: "Service: ${service} | Environment: ${env}"
              target: "host"
              default: "unknown-service"
```

### Rule Properties

| Property | Required | Default | Description |
|----------|----------|---------|-------------|
| `headerName` | Yes | - | Name of the header to set/modify |
| `regex` | Yes | - | Go regex pattern with named capture groups |
| `format` | Yes | - | Output format using `${groupName}` placeholders |
| `target` | No | `host` | Source value to match against (see targets below) |
| `default` | No | - | Fallback value if regex doesn't match |

### Available Targets

| Target | Description |
|--------|-------------|
| `host` | Request host (e.g., `example.com:8080`) |
| `path` | URL path (e.g., `/api/v1/users`) |
| `url` | Full URL string |
| `method` | HTTP method (e.g., `GET`, `POST`) |
| `scheme` | URL scheme (e.g., `https`) |
| `query` | Raw query string |
| `userAgent` | User-Agent header value |
| `referer` | Referer header value |
| `header:<name>` | Custom header value (e.g., `header:X-API-Key`) |

## 🔧 Usage Examples

### 1. Extract Path Components

Extract version and resource from URL path:

```yaml
rules:
  - headerName: "X-API-Version"
    regex: "^/api/v(?P<version>\\d+)/.*"
    format: "v${version}"
    target: "path"
    default: "v1"
```
### 2. Transform Query Parameters

Convert query string to custom header:
```yaml
rules:
  - headerName: "X-Request-Id"
    regex: "id=(?P<id>[a-f0-9-]+)"
    format: "req-${id}"
    target: "query"
```

### 3. Parse Host Header

Extract subdomain and domain:

```yaml
rules:
  - headerName: "X-Tenant-Info"
    regex: "(?P<tenant>\\w+)\\.(?P<domain>[\\w\\.]+)"
    format: "tenant=${tenant}, domain=${domain}"
    target: "host"
```

### 4. Chain Multiple Rules
```yaml
rules:
  - headerName: "X-API-Version"
    regex: "^/api/v(?P<version>\\d+)"
    format: "${version}"
    target: "path"

  - headerName: "X-Client-Platform"
    regex: "(?i)(?P<platform>android|ios|windows|linux|mac)"
    format: "${platform}"
    target: "header:User-Agent"
    default: "desktop"
```

## 🛡️ Validation & Error Handling

The plugin validates all rules during initialization. Common validation errors include:

- Missing required fields: headerName, regex, and format are mandatory
- Invalid regex patterns: Must be valid Go regex syntax
- Undefined group references: All ${groupName} in format must match named groups in regex
- Invalid targets: Must be one of the supported target values
Example error message:

```text
Error: rule error: format string references unknown group 'undefinedGroup'
```

## 📝 Regex Reference

### Named Capture Groups


Use `(?P<name>pattern)` syntax to create named groups:

```regex
# Match: /users/123/profile
/users/(?P<user_id>\d+)/(?P<action>\w+)
```
### Group References in Format

Reference captured groups in the format string:

```yaml
format: "User ${user_id} performed ${action}"
```

### Go Regex Syntax

The plugin uses Go's standard regex engine.
Full syntax reference:
[Go regexp package](https://pkg.go.dev/regexp)

## ⚠️ Limitations & Considerations

- Performance: Complex regex patterns may impact performance on high-traffic routes
- Group Naming: Group names must be valid Go identifiers (letters, digits, underscores)
- Case Sensitivity: Regex patterns are case-sensitive by default
- Validation Order: Rules are processed in the order they're defined
