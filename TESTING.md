# Testing Documentation

This document describes the comprehensive test suite added to go-odio-api to ensure API reliability and security.

## Test Coverage Overview

All tests are located in the `api/` directory and can be run with:
```bash
go test -v ./api/...
```

## Test Files

### 1. `handlers_systemd_test.go` - Systemd Handler Tests

**Purpose**: Verify secure systemd service control and proper permission enforcement.

**Critical Tests**:
- **`TestHandleSystemdError`** - MOST CRITICAL: Ensures proper HTTP status mapping
  - `PermissionSystemError` → 403 Forbidden (prevents system scope access)
  - `PermissionUserError` → 403 Forbidden (enforces whitelist)
  - Generic errors → 500 Internal Server Error
  - Success → 202 Accepted

- **`TestWithService`** - Middleware validation
  - Extracts scope (system/user) and unit name from URL paths
  - Returns 404 for invalid scope or missing unit

- **Handler Tests** (Start/Stop/Enable/Disable/Restart):
  - **Security**: System scope ALWAYS returns 403 Forbidden
  - **Whitelist**: Non-whitelisted user units return 403 Forbidden
  - **Success**: Whitelisted user units return 202 Accepted

**Security Impact**: These tests protect against unauthorized system service manipulation.

### 2. `handlers_mpris_test.go` - MPRIS Handler Tests

**Purpose**: Verify media player control error handling.

**Key Tests**:
- **`TestHandleMPRISError`** - Error mapping validation
  - `InvalidBusNameError` → 400 Bad Request
  - `ValidationError` → 400 Bad Request
  - `PlayerNotFoundError` → 404 Not Found
  - `CapabilityError` → 403 Forbidden
  - Generic errors → 500 Internal Server Error
  - Success → 202 Accepted

- **`TestWithPlayer`** - Player identification middleware
  - Extracts busName from URL path
  - Passes busName to handler functions

**Coverage**: Validates all MPRIS error types are correctly mapped to HTTP status codes.

### 3. `handlers_audio_test.go` - Audio and Bluetooth Tests

**Purpose**: Test PulseAudio and Bluetooth handler middleware.

**Tests**:
- **`TestWithSink`** - PulseAudio sink extraction
  - Validates sink parameter extraction
  - Returns 404 when sink is missing

- **`TestHandleBluetoothError`** - Bluetooth error handling
  - Success → 202 Accepted
  - Errors → 500 Internal Server Error

- **`TestWithBluetoothAction`** - Bluetooth action wrapper
  - Wraps actions with consistent error handling

**Coverage**: Middleware validation for audio and bluetooth operations.

### 4. `middleware_test.go` - Request Processing Middleware

**Purpose**: Validate JSON parsing, validation, and request body handling.

**Tests**:
- **`TestWithBody`** - JSON body parsing and validation
  - Valid JSON passes through to handler
  - Invalid JSON returns 400 Bad Request
  - Validation errors return 400 Bad Request
  - Empty body returns 400 Bad Request

- **`TestWithBodyVolume`** - Volume-specific validation
  - Valid volumes [0, 1] pass validation
  - Volumes < 0 or > 1 fail with 400 Bad Request

- **`TestValidateVolume`** - Direct validation function testing
  - Validates volume range enforcement
  - Tests boundary conditions (0, 1)

**Coverage**: Input validation and error handling for all JSON endpoints.

## Running Tests

### Run All Tests
```bash
go test -v ./api/...
```

### Run Specific Test Suites
```bash
# Systemd tests
go test -v ./api -run "Systemd|WithService|Service"

# MPRIS tests
go test -v ./api -run "MPRIS|Player"

# Audio/Bluetooth tests
go test -v ./api -run "Sink|Bluetooth"

# Middleware tests
go test -v ./api -run "Body|Volume"
```

### Run with Coverage
```bash
go test -cover ./api/...
```

## Critical Security Tests

The following tests MUST ALWAYS PASS to maintain security:

1. **System Scope Protection** (`TestStartServiceHandler`, `TestStopServiceHandler`, etc.)
   - Prevents unauthorized access to system-level systemd units
   - Ensures 403 Forbidden is returned for all system scope operations

2. **User Whitelist Enforcement** (`TestStartServiceHandler` - non-whitelisted test)
   - Prevents control of arbitrary user services
   - Validates whitelist is properly enforced

3. **Error Mapping** (`TestHandleSystemdError`, `TestHandleMPRISError`)
   - Ensures error information doesn't leak sensitive details
   - Maps internal errors to appropriate HTTP status codes

## Test Patterns

All tests follow these patterns:

### Table-Driven Tests
```go
tests := []struct {
    name           string
    input          SomeType
    wantStatusCode int
    wantBodyMatch  string
}{
    {name: "test case", input: ..., wantStatusCode: 200, ...},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // test implementation
    })
}
```

### HTTP Handler Testing
```go
handler := SomeHandler(backend)
req := httptest.NewRequest("POST", "/path", body)
req.SetPathValue("param", "value")
w := httptest.NewRecorder()
handler(w, req)

if w.Code != wantStatusCode {
    t.Errorf("status = %d, want %d", w.Code, wantStatusCode)
}
```

## Test Maintenance

When adding new handlers or error types:

1. **Add error mapping tests** - Verify new errors map to correct HTTP status
2. **Add middleware tests** - Test any new parameter extraction or validation
3. **Add integration tests** - Verify end-to-end handler behavior
4. **Update this document** - Keep test documentation current

## CI/CD Integration

These tests are designed to run in CI/CD pipelines:
- Fast execution (< 1 second for entire suite)
- No external dependencies required (uses mocks)
- Deterministic results
- Clear failure messages

## Known Limitations

- **No integration testing**: Tests use mocks, not real D-Bus connections
- **No concurrent access tests**: Single-threaded test execution
- **No performance tests**: Focused on correctness, not performance

For integration testing with real backends, see `backend/*/` test files.
