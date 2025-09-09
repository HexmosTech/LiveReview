# LiveReview License Enforcement and Management

## Overview

LiveReview requires a valid JWT license key obtained from "hexmos.com/livereview/access-livereview". This document outlines the comprehensive implementation plan for license enforcement, validation, and user experience.

## Core Principles

1. **License required to access/use```toml
[license]
api_base = "https://parse.apps.hexmos.com/parse"
timeout = "30s" 
grace_days = 3
validation_interval = "24h"
include_hardware_id = true
skip_validation = false  # Development only
```** - No functionality without valid license
2. **Daily license checks** - Server-side validation with fw-parse every 24 hours
3. **Tamper-proof daily local validations** - Offline verification with cached public key
4. **3-day grace period for call home** - Allow operation during temporary network issues
5. **Status of the license always visible** - Transparent license state information

## Architecture Components

### 1. License Storage and State Management

#### Backend (LiveReview Go)
- **Location**: `internal/license/` package
- **Database Storage**: SQLite/PostgreSQL table `license_state`
  ```sql
  CREATE TABLE license_state (
    id INTEGER PRIMARY KEY,
    license_token TEXT NOT NULL,
    license_id TEXT NOT NULL,
    email TEXT NOT NULL,
    app_name TEXT DEFAULT 'LiveReview',
    seat_count INTEGER,
    unlimited BOOLEAN DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL,
    license_version INTEGER NOT NULL,
    status TEXT DEFAULT 'active', -- active, warning, grace, expired
    last_validation_attempt TIMESTAMP,
    last_validation_success TIMESTAMP,
    validation_failures INTEGER DEFAULT 0,
    grace_period_start TIMESTAMP,
    cached_public_key TEXT,
    cached_public_key_kid TEXT,
    cached_public_key_updated TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
  );
  ```

#### Frontend (React)
- **Location**: `ui/src/store/license/` Redux slice
- **State Structure**:
  ```typescript
  interface LicenseState {
    token: string | null;
    status: 'valid' | 'warning' | 'grace' | 'expired' | 'invalid' | 'loading';
    email: string | null;
    expiresAt: string | null;
    daysRemaining: number | null;
    seatCount: number | null;
    unlimited: boolean;
    lastValidation: string | null;
    validationFailures: number;
    gracePeriodStart: string | null;
    showLicenseModal: boolean;
    error: string | null;
  }
  ```

### 2. fw-parse Integration Points

#### Required fw-parse Endpoints
Based on the JWT License module analysis, LiveReview will use these public endpoints:

1. **Token Validation** (Primary validation)
   ```
   POST /jwtLicence/validate
   Content-Type: application/json
   
   Request Body:
   {
     "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
     "dryRun": false  // optional, skips heartbeat update if true
   }
   
   Success Response:
   {
     "data": {
       "valid": true,
       "licenceId": "abc123",
       "expiresAt": "2025-12-31T23:59:59.000Z",
       "remainingDays": 113,
       "seatCount": 5,
       "unlimited": false,
       "status": "ACTIVE",
       "stale": false,
       "lastHeartbeatAt": "2025-09-09T10:30:00.000Z",
       "staleSince": null,
       "serverTime": "2025-09-09T10:30:00.000Z",
       "licenceVersion": 1
     }
   }
   
   Error Response:
   {
     "data": {
       "valid": false,
       "reason": "TOKEN_EXPIRED",
       "serverTime": "2025-09-09T10:30:00.000Z"
     }
   }
   ```

2. **Public Key Retrieval** (For offline validation)
   ```
   GET /jwtLicence/publicKey
   
   Response:
   {
     "data": {
       "kid": "v1",
       "publicKey": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBg...",
       "cachedAt": "2025-09-09T10:30:00.000Z"
     }
   }
   ```

#### Configuration
- **fw-parse Base URL**: Environment variable `LIVEREVIEW_LICENSE_API_BASE`
- **Default**: `https://parse.apps.hexmos.com/parse` (production)
- **Local dev**: `http://localhost:1337/parse`

### 3. License Validation Flows

#### 3.1 Initial License Entry Flow
```
User starts LiveReview → Check license_state table → If empty:
  ↓
Show License Entry Modal (blocking) → User enters license token
  ↓
Validate with fw-parse (/jwtLicence/validate) → If valid:
  ↓
Store in license_state table → Fetch and cache public key → Allow app access
  ↓
If invalid: Show error, retry prompt
```

#### 3.2 App Startup Validation Flow
```
App starts → Load license from license_state table
  ↓
Offline validation with cached public key (JWT verify)
  ↓
If offline validation passes:
  ↓
Check last_validation_success timestamp
  ↓
If > 24 hours ago: Schedule background server validation
If < 24 hours ago: Continue with app
  ↓
If offline validation fails: Immediate server validation required
```

#### 3.3 Daily Server Validation Flow
```
Background task (every 24h or on app start if overdue)
  ↓
Call fw-parse /jwtLicence/validate endpoint
  ↓
If success: Update last_validation_success, reset failure counter, status = 'active'
  ↓
If failure: Check error type
  ↓
If NETWORK ERROR (connection timeout, 5xx errors):
  - failure count = 1: status = 'warning', show warning banner
  - failure count = 2: status = 'warning', more prominent warning  
  - failure count = 3: status = 'grace', start grace_period_start timestamp
  - failure count > 3 AND grace period > 3 days: status = 'expired', block app
  ↓
If LICENSE ERROR (TOKEN_EXPIRED, LICENCE_REVOKED, etc.):
  - Immediate status = 'expired', block app, show license renewal modal
```

#### 3.4 Grace Period Logic
```
Grace period activates when server validation fails for 3 consecutive days
  ↓
Allow app usage for up to 3 additional days with prominent warnings
  ↓
During grace period: Continue daily validation attempts
  ↓
If validation succeeds during grace: Reset to 'active' status
If grace period expires: Block app access, show license renewal prompt
```

## 4. User Interface Implementation

### 4.1 License Entry Modal
**Location**: `ui/src/components/LicenseModal.tsx`

**Features**:
- Blocking modal that prevents app access
- License token input field
- Real-time validation feedback
- Link to purchase/renew license
- Error handling and retry mechanism

**Triggers**:
- No license found in database
- License validation consistently fails
- License expired and grace period over
- User manually requests license update

### 4.2 License Status Indicator
**Location**: `ui/src/components/LicenseStatusBar.tsx`

**Placement**: Fixed position at bottom of screen, subtle but always visible

**Status States**:
- **Valid**: Green dot + "Licensed" (minimal)
- **Warning**: Yellow dot + "License validation issues" (clickable for details)
- **Grace**: Orange dot + "Grace period: X days remaining" (prominent)
- **Expired**: Red dot + "License expired - Access blocked" (blocking)

**Visual Design**:
```css
.license-status-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 24px;
  background: rgba(0,0,0,0.05);
  border-top: 1px solid rgba(0,0,0,0.1);
  display: flex;
  align-items: center;
  padding: 0 12px;
  font-size: 12px;
  z-index: 1000;
}
```

### 4.3 Settings License Tab
**Location**: `ui/src/pages/Settings/LicenseTab.tsx`

**Content**:
- Current license status and validity
- License holder email
- Expiration date and days remaining
- Seat count information (if applicable)
- Last validation timestamp
- Manual license update option
- License renewal/purchase links
- Validation history and error details

**Integration**: Add to existing Settings.tsx tabs array:
```typescript
{ id: 'license', name: 'License', icon: <Icons.License /> }
```

### 4.4 License Blocking Screen
**Location**: `ui/src/components/LicenseBlockingScreen.tsx`

**Features**:
- Full-screen overlay when license is expired
- Clear explanation of the issue
- License renewal options
- Emergency contact information
- Offline mode limitations (if any)

## 5. Backend Implementation

### 5.1 License Service Package
**Location**: `internal/license/`

**Files**:
```
internal/license/
├── service.go          // Main license service
├── validator.go        // JWT validation logic  
├── storage.go          // Database operations
├── scheduler.go        // Background validation tasks
├── types.go           // License-related types
└── errors.go          // License-specific errors
```

**Key Functions**:
```go
**Key Functions**:
```go
// service.go
func (s *Service) ValidateLicense(token string) (*LicenseInfo, error)
func (s *Service) StoreLicense(token string) error
func (s *Service) GetCurrentLicense() (*LicenseInfo, error)
func (s *Service) StartValidationScheduler()

// validator.go  
func ValidateOffline(token, publicKey string) (*JWTClaims, error)
func ValidateOnline(token string) (*ValidationResponse, error)
func FetchPublicKey() (*PublicKey, error)
func GetHardwareID() (string, error)

// hardware.go (new file)
func GenerateHardwareFingerprint() (string, error)
func GetMachineID() (string, error)
```

### 5.1.1 Hardware Identification Implementation

**Dependencies to Add**:
```go
// Add to go.mod
require (
    github.com/shirou/gopsutil/v3 v3.23.9 // For system info
    github.com/denisbrodbeck/machineid v1.0.1 // For machine ID
)
```

**Hardware Fingerprinting Logic**:
```go
// hardware.go
package license

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "runtime"
    "strings"
    
    "github.com/denisbrodbeck/machineid"
    "github.com/shirou/gopsutil/v3/host"
    "github.com/shirou/gopsutil/v3/cpu"
)

func GenerateHardwareFingerprint() (string, error) {
    components := []string{}
    
    // Machine ID (most stable)
    if machineID, err := machineid.ProtectedID("LiveReview"); err == nil {
        components = append(components, fmt.Sprintf("mid:%s", machineID))
    }
    
    // OS and architecture
    components = append(components, fmt.Sprintf("os:%s", runtime.GOOS))
    components = append(components, fmt.Sprintf("arch:%s", runtime.GOARCH))
    
    // Host info
    if hostInfo, err := host.Info(); err == nil {
        components = append(components, fmt.Sprintf("host:%s", hostInfo.HostID))
        components = append(components, fmt.Sprintf("platform:%s", hostInfo.Platform))
    }
    
    // CPU info (first CPU model only)
    if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
        model := strings.ReplaceAll(cpuInfo[0].ModelName, " ", "_")
        components = append(components, fmt.Sprintf("cpu:%s", model))
    }
    
    // Create fingerprint hash
    combined := strings.Join(components, "|")
    hash := sha256.Sum256([]byte(combined))
    return hex.EncodeToString(hash[:]), nil
}
```

// storage.go
func StoreLicenseState(state *LicenseState) error
func GetLicenseState() (*LicenseState, error)
func UpdateValidationResult(success bool, details string) error
```

### 5.2 API Endpoints
**Location**: `internal/api/license.go`

**Endpoints**:
```go
// GET /api/v1/license/status - Get current license status
func getLicenseStatus(c echo.Context) error

// POST /api/v1/license/validate - Validate license token  
func validateLicense(c echo.Context) error

// POST /api/v1/license/update - Update license token
func updateLicense(c echo.Context) error

// POST /api/v1/license/refresh - Force refresh validation
func refreshLicenseValidation(c echo.Context) error
```

**Response Formats**:
```go
type LicenseStatusResponse struct {
    Status           string    `json:"status"`
    Valid            bool      `json:"valid"`
    Email            string    `json:"email"`
    ExpiresAt        time.Time `json:"expiresAt"`
    DaysRemaining    int       `json:"daysRemaining"`
    SeatCount        int       `json:"seatCount"`
    Unlimited        bool      `json:"unlimited"`
    LastValidation   time.Time `json:"lastValidation"`
    ValidationErrors []string  `json:"validationErrors,omitempty"`
    GracePeriodDays  int       `json:"gracePeriodDays,omitempty"`
}
```

### 5.2.1 Exact fw-parse API Implementation

**Go Request/Response Structures**:
```go
// types.go
type ValidationRequest struct {
    Token      string `json:"token"`
    DryRun     bool   `json:"dryRun,omitempty"`
    HardwareID string `json:"hardwareId,omitempty"` // For future fw-parse support
}

type ValidationResponse struct {
    Data struct {
        Valid            bool      `json:"valid"`
        LicenceID        string    `json:"licenceId,omitempty"`
        ExpiresAt        time.Time `json:"expiresAt,omitempty"`
        RemainingDays    int       `json:"remainingDays,omitempty"`
        SeatCount        int       `json:"seatCount,omitempty"`
        Unlimited        bool      `json:"unlimited,omitempty"`
        Status           string    `json:"status,omitempty"`
        Stale            bool      `json:"stale,omitempty"`
        LastHeartbeatAt  *time.Time `json:"lastHeartbeatAt,omitempty"`
        StaleSince       *time.Time `json:"staleSince,omitempty"`
        ServerTime       time.Time `json:"serverTime"`
        LicenceVersion   int       `json:"licenceVersion,omitempty"`
        // For failed validation
        Reason           string    `json:"reason,omitempty"`
    } `json:"data"`
}

type PublicKeyResponse struct {
    Data struct {
        Kid       string    `json:"kid"`
        PublicKey string    `json:"publicKey"`
        CachedAt  time.Time `json:"cachedAt"`
    } `json:"data"`
}

// Error response format from fw-parse
type ErrorResponse struct {
    Error struct {
        Code    string `json:"code"`
        Message string `json:"message"`
    } `json:"error"`
}
```

**Network vs License Error Handling**:
```go
// validator.go
func IsNetworkError(err error) bool {
    if err == nil {
        return false
    }
    
    // Check for network-related errors that should trigger grace period
    errorStr := strings.ToLower(err.Error())
    networkErrors := []string{
        "connection timeout",
        "connection refused", 
        "no such host",
        "network unreachable",
        "temporary failure",
        "i/o timeout",
    }
    
    for _, netErr := range networkErrors {
        if strings.Contains(errorStr, netErr) {
            return true
        }
    }
    
    return false
}

func IsLicenseError(errorCode string) bool {
    licenseErrors := []string{
        "TOKEN_EXPIRED",
        "TOKEN_INVALID_SIGNATURE", 
        "TOKEN_MALFORMED",
        "TOKEN_VERSION_MISMATCH",
        "LICENCE_EXPIRED",
        "LICENCE_REVOKED",
        "LICENCE_NOT_FOUND",
    }
    
    for _, licErr := range licenseErrors {
        if errorCode == licErr {
            return true
        }
    }
    return false
}
```

**JWT Offline Validation Implementation**:
```go
// validator.go
import (
    "crypto/rsa"
    "crypto/x509"
    "encoding/pem"
    "fmt"
    "time"
    
    "github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
    LicenceID      string                 `json:"licId"`
    Email          string                 `json:"email"`
    AppName        string                 `json:"appName"`
    SeatCount      int                    `json:"seatCount"`
    Unlimited      bool                   `json:"unlimited"`
    Version        int                    `json:"ver"`
    Metadata       map[string]interface{} `json:"metadata,omitempty"`
    jwt.RegisteredClaims
}

func ValidateOfflineJWT(tokenString, publicKeyPEM, expectedKid string) (*JWTClaims, error) {
    // Parse public key
    block, _ := pem.Decode([]byte(publicKeyPEM))
    if block == nil {
        return nil, fmt.Errorf("failed to parse PEM block")
    }
    
    pub, err := x509.ParsePKIXPublicKey(block.Bytes)
    if err != nil {
        return nil, fmt.Errorf("failed to parse public key: %w", err)
    }
    
    rsaPub, ok := pub.(*rsa.PublicKey)
    if !ok {
        return nil, fmt.Errorf("not an RSA public key")
    }
    
    // Parse and validate token
    token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
        // Verify signing method
        if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        
        // Verify kid header
        if kid, ok := token.Header["kid"].(string); !ok || kid != expectedKid {
            return nil, fmt.Errorf("invalid or missing kid header")
        }
        
        return rsaPub, nil
    })
    
    if err != nil {
        return nil, fmt.Errorf("token validation failed: %w", err)
    }
    
    claims, ok := token.Claims.(*JWTClaims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token claims")
    }
    
    // Additional validations
    if claims.AppName != "LiveReview" {
        return nil, fmt.Errorf("token not issued for LiveReview")
    }
    
    return claims, nil
}
```

### 5.3 Background Validation Scheduler
**Location**: `internal/license/scheduler.go`

**Implementation**:
- Start with application startup
- Run validation check every 24 hours
- Exponential backoff for failed validations
- Graceful handling of network issues
- Comprehensive logging of validation attempts

```go
func (s *Scheduler) Start() error {
    ticker := time.NewTicker(24 * time.Hour)
    go func() {
        for {
            select {
            case <-ticker.C:
                s.performValidation()
            case <-s.stopChan:
                ticker.Stop()
                return
            }
        }
    }()
    return nil
}
```

## 6. Configuration and Environment Variables

### Environment Variables
```bash
# fw-parse connection
LIVEREVIEW_LICENSE_API_BASE=https://parse.apps.hexmos.com/parse
LIVEREVIEW_LICENSE_TIMEOUT=30s
LIVEREVIEW_LICENSE_RETRY_ATTEMPTS=3

# Grace period configuration  
LIVEREVIEW_LICENSE_GRACE_DAYS=3
LIVEREVIEW_LICENSE_VALIDATION_INTERVAL=24h

# Hardware identification
LIVEREVIEW_LICENSE_INCLUDE_HARDWARE_ID=true

# Development/testing
LIVEREVIEW_LICENSE_SKIP_VALIDATION=false  # Only for development
LIVEREVIEW_LICENSE_MOCK_MODE=false        # Use mock responses
```

### Configuration File Integration
Add to `livereview.toml`:
```toml
[license]
api_base = "https://parse.apps.hexmos.com/parse"
timeout = "30s" 
grace_days = 3
validation_interval = "24h"
include_hardware_id = true
skip_validation = false  # Development only
```

## 7. Error Handling and User Communication

### Error Categories
1. **Network Errors**: Temporary connectivity issues (grace period applies)
2. **TOKEN_MALFORMED**: Malformed JWT token (no grace period - immediate block)
3. **TOKEN_EXPIRED**: JWT token expired (no grace period - immediate block)
4. **TOKEN_INVALID_SIGNATURE**: Invalid signature (no grace period - immediate block)
5. **LICENCE_EXPIRED**: License expired (no grace period - immediate block)
6. **LICENCE_REVOKED**: License revoked by admin (no grace period - immediate block)
7. **TOKEN_VERSION_MISMATCH**: License reissued, old token (no grace period - immediate block)
8. **LICENCE_NOT_FOUND**: License not found in system (no grace period - immediate block)

### User-Friendly Messages
```typescript
const LICENSE_MESSAGES = {
  NETWORK_ERROR: "Unable to verify license. Retrying in background...",
  INVALID_LICENSE: "Invalid license key. Please check and try again.",
  EXPIRED_LICENSE: "Your license has expired. Please renew to continue using LiveReview.",
  REVOKED_LICENSE: "This license has been revoked. Please contact support.",
  VERSION_MISMATCH: "License updated. Please enter your new license key.",
  GRACE_PERIOD: "License verification issues. You have {days} days to resolve this."
};
```

## 8. Security Considerations

### Token Storage
- **Backend**: Encrypted storage in database with application-level encryption
- **Frontend**: Store in Redux state (memory only), never in localStorage
- **Transmission**: HTTPS only, never log token values

### Validation Security
- **Offline validation**: Always verify JWT signature with cached public key
- **Online validation**: Use fw-parse endpoints over HTTPS
- **Public key caching**: Validate kid (key ID) matches expected value
- **Replay protection**: Check JWT jti (token ID) if needed

### Tamper Protection
- **Database integrity**: Consider checksums for license state
- **Code integrity**: Obfuscate license validation logic if possible
- **Time validation**: Check system clock manipulation

## 9. Testing Strategy

### Unit Tests
- JWT token validation logic
- Offline validation with public key
- Grace period calculations
- Error handling scenarios

### Integration Tests  
- fw-parse API connectivity
- Database operations
- Background scheduler functionality
- UI state management

### Manual Testing Scenarios
1. Fresh installation - license entry flow
2. Valid license - normal operation
3. Expired license - grace period behavior
4. Network disconnection - offline validation
5. Invalid license tokens - error handling
6. License renewal - token update flow

## 10. Deployment and Migration

### Database Migration
Create migration script for license_state table:
```sql
-- Migration: 001_create_license_state.sql
CREATE TABLE IF NOT EXISTS license_state (
  -- [table schema as defined above]
);

CREATE INDEX idx_license_state_email ON license_state(email);
CREATE INDEX idx_license_state_expires_at ON license_state(expires_at);
```

### Configuration Updates
- Update deployment configs with environment variables
- Ensure fw-parse base URL is correctly set for each environment
- Configure license API timeouts and retry policies

### Rollout Plan
1. **Phase 1**: Deploy backend license service (non-blocking)
2. **Phase 2**: Deploy UI components with license modal
3. **Phase 3**: Enable license enforcement gradually
4. **Phase 4**: Full enforcement with grace period monitoring

## 11. Monitoring and Observability

### Metrics to Track
- License validation success/failure rates
- Grace period activations
- License renewal frequency
- API response times from fw-parse
- User license entry completion rates

### Logging Requirements
- All license validation attempts (success/failure)
- Grace period state changes
- License token updates
- API communication errors
- User license-related actions

### Alerting
- High license validation failure rates
- fw-parse API connectivity issues
- Licenses approaching expiration
- Grace period activations spike

## 12. Future Enhancements

### Planned Features
- Automatic license renewal notifications
- Multi-seat license management
- License usage analytics
- Offline mode with reduced functionality
- License transfer between users
- Integration with billing systems

### fw-parse Dependencies
- Monitor JWT key rotation capabilities
- Support for multiple concurrent keys
- Enhanced audit logging
- Seat count enforcement mechanisms

## 13. Required fw-parse Modifications

Based on the analysis, **NO modifications are required** to fw-parse for basic license enforcement. The existing JWT License module provides all necessary functionality:

### ✅ Available and Ready
- `POST /jwtLicence/validate` - Public endpoint for token validation
- `GET /jwtLicence/publicKey` - Public endpoint for offline validation keys
- Proper error responses with standardized error codes
- No authentication required for public endpoints

### 🔮 Future Enhancements (Optional)
If hardware-based license binding is desired in the future:

1. **Add Hardware ID Support to fw-parse validation**:
   ```javascript
   // In fw-parse validation function, optionally accept:
   {
     "token": "...",
     "hardwareId": "sha256-hash-of-hardware-fingerprint"
   }
   ```

2. **Store Hardware ID in License Record**:
   ```javascript
   // Add field to JL_Licence class
   licence.set('boundHardwareId', hardwareId);
   ```

3. **Validation Logic Enhancement**:
   ```javascript
   // In validateToken function
   if (licence.get('boundHardwareId') && req.body.hardwareId) {
     if (licence.get('boundHardwareId') !== req.body.hardwareId) {
       throw createAppError(ERROR_CODES.HARDWARE_MISMATCH, 'Hardware mismatch', 400);
     }
   }
   ```

### 📋 Implementation Priority
1. **Phase 1**: Implement with current fw-parse (no hw binding) ✅
2. **Phase 2**: Add hardware fingerprinting on LiveReview side (store locally)
3. **Phase 3**: Enhance fw-parse for hardware binding validation (future)

---

This specification provides a comprehensive foundation for implementing robust license enforcement in LiveReview while maintaining a seamless user experience and strong security posture.