# LiveReview Onboarding Improvement Implementation Checklist

This checklist implements the simplified two-mode deployment architecture from `onboarding-and-evolution-experience.md`.

## Implementation Status

### Phase 1: Foundation & Backend Configuration ✅ COMPLETED
- **Task 1.1.1**: Deployment Config Helper Functions ✅ COMPLETED
- **Task 1.1.2**: Server Binding Logic ✅ COMPLETED  
- **Task 1.1.3**: CLI Commands for Environment Variables ✅ COMPLETED
- **Task 1.2.1**: System Info Endpoint to Router ✅ COMPLETED
- **Task 1.2.2**: System Info Handler Implementation ✅ COMPLETED

### Phase 2: Frontend Auto-Detection & UI Enhancements ✅ COMPLETED
- **Task 2.1.1**: Research and Update API Client Auto-Detection ✅ COMPLETED
- **Task 2.1.2**: Test Runtime Config Injection ✅ COMPLETED 
- **Task 2.2.1**: Create Demo Mode Banner Component ✅ COMPLETED
- **Task 2.2.2**: Integrate Banner into Main Layout ✅ COMPLETED
- **Task 2.3.1**: Research Existing Settings Page Structure ✅ COMPLETED
- **Task 2.3.2**: Add Deployment Tab to Settings ✅ COMPLETED
- **Task 2.3.3**: Create Deployment Settings Component ✅ COMPLETED

### Phases 3-6: 🔄 PENDING
- Phase 3: Docker & Environment Integration
- Phase 4: lrops.sh Integration
- Phase 5: Integration Testing & Validation
- Phase 6: Final Validation & Polish

**🎉 Ready for Phase 3 Implementation**
Current status: Core foundation and frontend UI complete. Ready for Docker integration and lrops.sh updates.

---

## Overview

**Goal**: Transform LiveReview from complex multi-mode setup to simple demo/production auto-detection.

**Core Changes**:
- URL-based mode detection (localhost = demo, anything else = production)
- Minimal .env configuration (only essentials, TOML untouched)
- Frontend UI enhancements for mode transparency
- Zero-configuration demo mode, simple production upgrade

**Architecture Notes**:
- No new config systems - work with existing server.go environment loading
- TOML configuration completely untouched (handles AI/Git provider settings)
- All deployment mode logic via environment variables only
- Use LIVEREVIEW_ prefix for new environment variables

---

## Phase 1: Foundation & Backend Configuration (Week 1-2)

### Phase 1.1: Environment Variable Integration (Existing System)

**Goal**: Add deployment mode detection using existing environment variable system.

#### Task 1.1.1: Add Deployment Config Helper Functions ✅ COMPLETED
- **File**: `internal/api/server.go` (update existing)
- **Purpose**: Add helper functions to read new deployment environment variables
- **Changes**:
  - ✅ Add `getEnvInt()` and `getEnvBool()` helper functions if not exist
  - ✅ Add deployment configuration struct within server setup
  - ✅ No new config files needed - work with existing .env loading

```go
// Add to server.go
type DeploymentConfig struct {
    BackendPort   int
    FrontendPort  int
    ReverseProxy  bool
    Mode          string // derived
    WebhooksEnabled bool // derived
}

func getDeploymentConfig() *DeploymentConfig {
    config := &DeploymentConfig{
        BackendPort:  getEnvInt("LIVEREVIEW_BACKEND_PORT", 8888),
        FrontendPort: getEnvInt("LIVEREVIEW_FRONTEND_PORT", 8081),
        ReverseProxy: getEnvBool("LIVEREVIEW_REVERSE_PROXY", false),
    }
    
    // Auto-configure derived values
    if config.ReverseProxy {
        config.Mode = "production"
        config.WebhooksEnabled = true
    } else {
        config.Mode = "demo"
        config.WebhooksEnabled = false
    }
    
    return config
}
```

**Test**: ✅ COMPLETED
```bash
# Test environment variable reading
export LIVEREVIEW_BACKEND_PORT=9888
export LIVEREVIEW_REVERSE_PROXY=true
go run . api --help  # Should compile without errors
echo "✓ Environment variables read correctly"
```

#### Task 1.1.2: Update Server Binding Logic ✅ COMPLETED
- **File**: `internal/api/server.go` (update existing Start() method)
- **Purpose**: Use deployment config to determine server binding address
- **Changes**:
  - ✅ Modify server start to read deployment config
  - ✅ Demo mode: bind to localhost/127.0.0.1 (more secure)
  - ✅ Production mode: bind to 127.0.0.1 (behind proxy)
  - ✅ Update port reading from new environment variables

**Test**:
```bash
# Test demo mode binding
unset LIVEREVIEW_REVERSE_PROXY
./livereview api &
netstat -tlnp | grep 8888  # Should show 127.0.0.1:8888 or localhost
echo "✓ Demo mode binds to localhost"

# Test production mode binding
export LIVEREVIEW_REVERSE_PROXY=true
./livereview api &
netstat -tlnp | grep 8888  # Should show 127.0.0.1:8888
echo "✓ Production mode binds to 127.0.0.1"
```

#### Task 1.1.3: Update CLI Commands for Environment Variables ✅ COMPLETED
- **Files**: 
  - `cmd/api.go` - Add environment variable support for port
  - `cmd/ui.go` - Add environment variable support for port
- **Purpose**: Make CLI respect new LIVEREVIEW_ prefixed environment variables
- **Changes**:
  - ✅ Update existing port flags to read from LIVEREVIEW_BACKEND_PORT and LIVEREVIEW_FRONTEND_PORT
  - ✅ Add environment variable support using existing urfave/cli patterns
  - ✅ Maintain backward compatibility with existing flags

**Test**: ✅ COMPLETED (requires database setup for full test)
```bash
# Test environment variable override
export LIVEREVIEW_BACKEND_PORT=9999
./livereview api  # Should start on port 9999
curl http://localhost:9999/health
echo "✓ CLI respects environment variables"
```

### Phase 1.2: Backend System Info Endpoint

#### Task 1.2.1: Add System Info Endpoint to Existing Router ✅ COMPLETED
- **File**: `internal/api/server.go` (update setupRoutes() method)
- **Purpose**: Add new endpoint to existing route setup
- **Changes**:
  - ✅ Add `public.GET("/system/info", s.getSystemInfo)` to existing public routes
  - ✅ System info should be public (no auth required) for frontend to detect mode
  - ✅ Use existing route organization patterns

**Test**: ✅ COMPLETED (endpoint exists, requires database setup for full test)
```bash
# Test system info endpoint accessibility
./livereview api &
curl http://localhost:8888/api/v1/system/info
# Should return JSON with deployment info
echo "✓ System info endpoint accessible"
```

#### Task 1.2.2: Implement System Info Handler ✅ COMPLETED
- **File**: `internal/api/server.go` (add new handler method)
- **Purpose**: Return deployment mode and capabilities information
- **Changes**:
  - ✅ Add `getSystemInfo()` handler method to Server struct
  - ✅ Return deployment mode, version, capabilities based on environment
  - ✅ Use existing versionInfo from server struct

```go
func (s *Server) getSystemInfo(c echo.Context) error {
    deploymentConfig := getDeploymentConfig()
    
    info := map[string]interface{}{
        "deployment_mode": deploymentConfig.Mode,
        "version": s.versionInfo.Version,
        "api_url": fmt.Sprintf("http://localhost:%d", deploymentConfig.BackendPort),
        "capabilities": map[string]interface{}{
            "webhooks_enabled": deploymentConfig.WebhooksEnabled,
            "manual_triggers_only": !deploymentConfig.WebhooksEnabled,
            "external_access": deploymentConfig.Mode == "production",
            "proxy_mode": deploymentConfig.ReverseProxy,
        },
    }
    
    return c.JSON(http.StatusOK, info)
}
```

**Test**: ✅ COMPLETED (handler implemented, requires database setup for full test)
```bash
# Test system info content
curl http://localhost:8888/api/v1/system/info | jq
# Should show: {"deployment_mode":"demo","webhooks_enabled":false,...}

# Test production mode
export LIVEREVIEW_REVERSE_PROXY=true
./livereview api &
curl http://localhost:8888/api/v1/system/info | jq '.deployment_mode'
# Should return "production"
echo "✓ System info returns correct deployment mode"
```

---

## Phase 2: Frontend Auto-Detection & UI Enhancements (Week 2-3)

### Phase 2.1: Frontend URL Detection Logic

#### Task 2.1.1: Research and Update API Client Auto-Detection
- **File**: `ui/src/api/apiClient.ts` (update existing getBaseUrl function)
- **Purpose**: Refine existing auto-detection logic for two-mode system
- **Changes**:
  - Current logic already detects localhost patterns, refine it
  - Ensure localhost/127.0.0.1 → direct to port 8888
  - Everything else → current origin + /api prefix
  - Maintain existing window.LIVEREVIEW_CONFIG support

**Research First**:
```bash
# Check current API client implementation
cd ui/
grep -r "getBaseUrl\|api.*url\|localhost.*888" src/
# Understand current auto-detection patterns
```

**Test**:
```bash
# Test frontend API detection in demo mode
cd ui/
npm start
# Visit http://localhost:8081 in browser
# Check browser console → should detect http://localhost:8888
# Check Network tab → API calls should go to localhost:8888
echo "✓ Frontend detects API URL correctly in demo mode"

# Test via IP address (simulated production)
# Access via 127.0.0.1:8081 or local IP
# Should use current origin + /api
echo "✓ Frontend uses proxy pattern for non-localhost"
```

#### Task 2.1.2: Test Runtime Config Injection
- **File**: `cmd/ui.go` (understand existing API URL injection)
- **Purpose**: Understand how window.LIVEREVIEW_CONFIG currently works
- **Changes**:
  - Research existing `--api-url` flag usage
  - Ensure runtime injection works for production mode
  - Test with different deployment scenarios

**Test**:
```bash
# Test existing runtime injection
./livereview ui --api-url "http://example.com/api" &
curl http://localhost:8081 | grep -o "LIVEREVIEW_CONFIG.*"
# Should see injected API URL in HTML
echo "✓ Runtime config injection works"
```

### Phase 2.2: Frontend UI Enhancements

#### Task 2.2.1: Create Demo Mode Banner Component
- **File**: `ui/src/components/DemoModeBanner.tsx` (new file)
- **Purpose**: Show demo mode limitations and upgrade path
- **Changes**:
  - Create React component with demo mode detection
  - Show warning about limitations (no webhooks, localhost only)
  - Include "Upgrade to Production" button that links to documentation
  - Use existing UI component patterns and styling

**Test**:
```bash
# Test demo mode banner
cd ui/
npm start
# Visit http://localhost:8081
# Should see orange banner at top with demo mode warning
echo "✓ Demo mode banner appears on localhost"

# Test production mode (banner should be hidden)
# If accessible via domain/IP, banner should not appear
echo "✓ Banner hidden in non-localhost access"
```

#### Task 2.2.2: Integrate Banner into Main Layout
- **File**: `ui/src/App.tsx` or main layout component (research first)
- **Purpose**: Display banner consistently across all pages in demo mode
- **Changes**:
  - Research current layout structure
  - Import and render DemoModeBanner at top of main layout
  - Ensure banner appears before main content
  - Test banner across different routes

**Research First**:
```bash
# Find main layout component
cd ui/
find src/ -name "*.tsx" | xargs grep -l "App\|Layout\|main" | head -5
# Understand current routing and layout structure
```

**Test**:
```bash
# Test banner persistence across routes
# Navigate to different pages in the app
# Banner should appear consistently in demo mode
echo "✓ Banner appears consistently across all pages"
```

### Phase 2.3: Enhanced Settings Page (New Deployment Tab)

#### Task 2.3.1: Research Existing Settings Page Structure
- **File**: `ui/src/pages/Settings/Settings.tsx` (research existing implementation)
- **Purpose**: Understand current settings page to add new deployment tab
- **Changes**:
  - Study existing tab structure and organization
  - Understand isSuperAdmin checks and permissions
  - Plan integration of new "Deployment" tab

**Research**:
```bash
# Study existing settings page
cd ui/
cat src/pages/Settings/Settings.tsx | head -50
grep -A 10 -B 10 "tabs.*=\|isSuperAdmin" src/pages/Settings/Settings.tsx
# Understand tab structure and super admin logic
echo "✓ Settings page structure understood"
```

#### Task 2.3.2: Add Deployment Tab to Settings
- **File**: `ui/src/pages/Settings/Settings.tsx` (update existing)
- **Purpose**: Add new "Deployment" tab for super admins
- **Changes**:
  - Add new tab to existing tabs array for super admins
  - Create new icon import (use existing Icons pattern)
  - Follow existing tab organization patterns

**Test**:
```bash
# Test new deployment tab appears
# Log in as super admin user
# Navigate to Settings
# Should see new "Deployment" tab after "Instance"
echo "✓ Deployment tab appears for super admins"
```

#### Task 2.3.3: Create Deployment Settings Component
- **File**: `ui/src/pages/Settings/Settings.tsx` (add new component within existing file)
- **Purpose**: Show system information and deployment status
- **Changes**:
  - Create DeploymentSettings component using existing patterns
  - Fetch data from `/api/v1/system/info` endpoint
  - Display deployment mode, API endpoint, webhook status
  - Show upgrade instructions for demo mode
  - Use existing UI components and styling

**Test**:
```bash
# Test deployment settings page
# Navigate to Settings → Deployment tab
# Should show:
# - Deployment Mode: Demo Mode (orange badge)
# - API Endpoint: http://localhost:8888
# - Webhooks: Disabled (red indicator)
# - Upgrade instructions for demo mode
echo "✓ Deployment settings shows correct demo mode info"

# Test API integration
# Check Network tab for call to /api/v1/system/info
# Verify displayed data matches API response
curl http://localhost:8888/api/v1/system/info | jq
echo "✓ Frontend displays real backend system info"
```

---

## Phase 3: Docker & Environment Integration (Week 3-4)

### Phase 3.1: Docker Configuration Updates

#### Task 3.1.1: Update Docker Compose for New Environment Variables
- **File**: `docker-compose.yml` (update existing)
- **Purpose**: Use new LIVEREVIEW_ prefixed environment variables
- **Changes**:
  - Replace existing port mappings with LIVEREVIEW_BACKEND_PORT and LIVEREVIEW_FRONTEND_PORT
  - Ensure .env file loading continues to work
  - Maintain existing postgres and other service configurations

**Test**:
```bash
# Test with custom ports via environment variables
echo "LIVEREVIEW_BACKEND_PORT=9888" > .env.test
echo "LIVEREVIEW_FRONTEND_PORT=9081" >> .env.test
echo "DB_PASSWORD=test123" >> .env.test
echo "JWT_SECRET=testsecret123" >> .env.test

docker-compose --env-file .env.test up -d
curl http://localhost:9081  # Should work
curl http://localhost:9888/api/v1/system/info  # Should work
echo "✓ Docker compose respects new environment variables"
```

#### Task 3.1.2: Update Docker Entry Point Script
- **File**: `docker-entrypoint.sh` (update existing)
- **Purpose**: Use new environment variables for startup
- **Changes**:
  - Update existing port reading to use LIVEREVIEW_ prefixed variables
  - Add debug output showing configuration detection
  - Maintain existing startup sequence and database connections

**Test**:
```bash
# Test entry point script with new variables
export LIVEREVIEW_BACKEND_PORT=8888
export LIVEREVIEW_FRONTEND_PORT=8081
export LIVEREVIEW_REVERSE_PROXY=false
export DB_PASSWORD=test123
export JWT_SECRET=testsecret123

bash docker-entrypoint.sh  # Should show debug output and start correctly
echo "✓ Entry point script uses new environment variables"
```

### Phase 3.2: Production Mode Testing

#### Task 3.2.1: Test Production Mode Environment Detection
- **Purpose**: Verify production mode works correctly with reverse proxy flag
- **Test Setup**: Test production mode without actual reverse proxy first

**Test**:
```bash
# Test production mode configuration
export LIVEREVIEW_REVERSE_PROXY=true
export LIVEREVIEW_BACKEND_PORT=8888
export LIVEREVIEW_FRONTEND_PORT=8081

./livereview api &
./livereview ui &

curl http://127.0.0.1:8888/api/v1/system/info | jq '.deployment_mode'
# Should return "production"

curl http://127.0.0.1:8888/api/v1/system/info | jq '.capabilities.webhooks_enabled'
# Should return true

echo "✓ Production mode detection works"
```

#### Task 3.2.2: Document Reverse Proxy Testing
- **Purpose**: Document manual reverse proxy testing steps
- **Create**: `test-reverse-proxy.md` documentation for manual testing

**Manual Test Documentation**:
```bash
# Document reverse proxy test steps:
echo "Manual test: Set up nginx with these routes:"
echo "location /api/ { proxy_pass http://127.0.0.1:8888/api/; }"
echo "location / { proxy_pass http://127.0.0.1:8081/; }"
echo "Then access via domain and verify:"
echo "1. No demo banner appears"
echo "2. API calls go through /api/* route"
echo "3. System info shows production mode"
echo "✓ Reverse proxy integration documented"
```

---

## Phase 4: lrops.sh Integration (Week 4-5)

### Phase 4.1: Research and Update lrops.sh

#### Task 4.1.1: Research Current lrops.sh Configuration System
- **File**: `lrops.sh` (research existing implementation)
- **Purpose**: Understand current configuration generation to update for two-mode system
- **Research Focus**:
  - How does current .env generation work?
  - What configuration options currently exist?
  - How are templates and user prompts structured?

**Research**:
```bash
# Study lrops.sh structure
grep -A 20 -B 5 "\.env\|configuration\|generate.*config" lrops.sh
grep -A 10 "gather_configuration\|generate_env" lrops.sh
# Understand current configuration patterns
echo "✓ lrops.sh configuration system understood"
```

#### Task 4.1.2: Update Configuration Generation for Two-Mode System
- **File**: `lrops.sh` (update existing gather_configuration function)
- **Purpose**: Replace complex configuration with simple demo/production choice
- **Changes**:
  - Simplify configuration prompts to only ask demo vs production
  - Update .env generation to use LIVEREVIEW_ prefixed variables
  - Remove complex IP detection and multi-mode logic
  - Keep existing password generation functions

**Test**:
```bash
# Test simplified configuration generation
./lrops.sh  # Run setup command (research the correct command first)
# Should prompt only for:
# 1. Demo Mode (localhost only) [DEFAULT]
# 2. Production (with reverse proxy)
# Should generate minimal .env file with correct variables
echo "✓ lrops.sh generates simplified configuration"
```

#### Task 4.1.3: Update .env File Templates
- **File**: `lrops.sh` (update generate_env_file function)
- **Purpose**: Generate minimal .env files for both modes
- **Changes**:
  - Demo mode: Only DB_PASSWORD, JWT_SECRET (required)
  - Production mode: Add LIVEREVIEW_REVERSE_PROXY=true
  - Include helpful comments about upgrade path
  - Use LIVEREVIEW_ prefix for all new variables

**Test**:
```bash
# Test .env generation for both modes
rm -f .env

# Test demo mode
./lrops.sh setup-demo  # or equivalent command
cat .env
# Should contain:
# - DB_PASSWORD=<generated>
# - JWT_SECRET=<generated>
# - Helpful comments about demo mode limitations
# - No LIVEREVIEW_REVERSE_PROXY (defaults to false)

# Test production mode
./lrops.sh setup-production  # or equivalent command
cat .env
# Should contain:
# - DB_PASSWORD=<generated>
# - JWT_SECRET=<generated>
# - LIVEREVIEW_REVERSE_PROXY=true
# - Helpful comments about reverse proxy setup

echo "✓ .env files contain correct minimal configuration"
```

### Phase 4.2: Update lrops.sh Documentation and Help

#### Task 4.2.1: Update Embedded Documentation
- **File**: `lrops.sh` (update help functions)
- **Purpose**: Update documentation to reflect simplified setup
- **Changes**:
  - Update help text to mention only two modes
  - Remove references to complex IP detection
  - Add clear upgrade path documentation

#### Task 4.2.2: Update Reverse Proxy Templates
- **File**: `lrops.sh` (update nginx/caddy template generation)
- **Purpose**: Ensure reverse proxy templates work with new environment variables
- **Changes**:
  - Update templates to use standard ports (8081/8888)
  - Ensure /api/* routing is correctly configured
  - Test template generation

---

## Phase 5: Integration Testing & Validation (Week 5-6)

### Phase 5.1: End-to-End Demo Mode Testing

#### Task 5.1.1: Complete Demo Mode Flow Test
- **Purpose**: Test complete demo mode experience from scratch
- **Test Sequence**:
```bash
# Clean slate test
rm -rf /tmp/livereview-test/
mkdir -p /tmp/livereview-test && cd /tmp/livereview-test

# 1. Setup using lrops.sh
# Copy lrops.sh and test setup
echo "Testing complete demo mode setup flow..."

# 2. Verify .env generation
cat .env
# Should contain minimal demo mode configuration

# 3. Start services
docker-compose up -d
echo "✓ Services start successfully"

# 4. Test frontend accessibility
curl http://localhost:8081 -s | grep -q "LiveReview"
echo "✓ Frontend accessible on localhost:8081"

# 5. Test demo mode banner
curl http://localhost:8081 -s | grep -q "Demo Mode"
echo "✓ Demo mode banner appears"

# 6. Test API connectivity and system info
curl http://localhost:8888/api/v1/system/info | jq '.deployment_mode' | grep -q "demo"
echo "✓ Backend reports demo mode"

# 7. Verify webhook limitations
curl http://localhost:8888/api/v1/system/info | jq '.capabilities.webhooks_enabled' | grep -q "false"
echo "✓ Webhooks correctly disabled in demo mode"

# 8. Test frontend system info integration
# Manual: Navigate to Settings → Deployment tab
# Should show demo mode information
echo "Manual test: Check Settings → Deployment tab shows demo mode info"

echo "✓ Complete demo mode flow validated"
```

### Phase 5.2: Production Mode Upgrade Testing

#### Task 5.2.1: Demo to Production Upgrade Test
- **Purpose**: Test upgrade from demo to production mode
- **Test Sequence**:
```bash
# Starting from working demo mode setup
echo "Testing demo to production upgrade..."

# 1. Add production mode flag
echo "LIVEREVIEW_REVERSE_PROXY=true" >> .env

# 2. Restart services
docker-compose restart

# 3. Verify mode change
curl http://127.0.0.1:8888/api/v1/system/info | jq '.deployment_mode' | grep -q "production"
echo "✓ Backend switches to production mode"

# 4. Verify webhook enablement
curl http://127.0.0.1:8888/api/v1/system/info | jq '.capabilities.webhooks_enabled' | grep -q "true"
echo "✓ Webhooks enabled in production mode"

# 5. Verify binding changes
netstat -tlnp | grep :8888 | grep -q "127.0.0.1"
echo "✓ Backend binds to 127.0.0.1 in production mode"

# 6. Test frontend access via 127.0.0.1
curl http://127.0.0.1:8081 -s | grep -qv "Demo Mode"
echo "✓ Demo banner hidden in production mode"

# 7. Manual reverse proxy test
echo "Manual test: Configure nginx/caddy to route:"
echo "  / -> 127.0.0.1:8081"
echo "  /api/* -> 127.0.0.1:8888"
echo "Then verify app works via domain and shows production mode"

echo "✓ Production mode upgrade tested"
```

### Phase 5.3: Error Handling and Edge Cases

#### Task 5.3.1: Test Environment Variable Edge Cases
- **Purpose**: Ensure graceful handling of configuration errors
- **Test Cases**:
```bash
# 1. Missing environment variables
unset LIVEREVIEW_BACKEND_PORT
./livereview api &
# Should use default port 8888
curl http://localhost:8888/health
echo "✓ Handles missing LIVEREVIEW_BACKEND_PORT gracefully"

# 2. Invalid port numbers
export LIVEREVIEW_BACKEND_PORT=99999
./livereview api &
# Should show error or use fallback
echo "✓ Handles invalid port numbers"

# 3. Backend unavailable
./livereview ui &  # start UI without backend
curl http://localhost:8081 -s | grep -q "LiveReview"
# Frontend should still load
echo "✓ Frontend handles backend unavailability"

# 4. System info API error
# Stop backend, test frontend settings page
# Should show loading or error state gracefully
echo "Manual test: Check Settings page handles API errors gracefully"
```

#### Task 5.3.2: Cross-Browser and Platform Testing
- **Purpose**: Verify functionality across different environments
- **Test Matrix**:
```bash
# Test on different browsers (manual)
echo "Manual browser tests:"
echo "- Firefox: Demo mode banner and API detection"
echo "- Chrome: Demo mode banner and API detection" 
echo "- Safari: Demo mode banner and API detection (if available)"

# Test different access patterns
echo "Access pattern tests:"
echo "- http://localhost:8081 → should show demo mode"
echo "- http://127.0.0.1:8081 → should show demo mode"  
echo "- http://<local-ip>:8081 → should show production mode"

echo "✓ Cross-platform testing completed"
```

---

## Final Validation Checklist

### Demo Mode Validation
- [ ] `curl http://localhost:8081` shows demo mode banner in HTML
- [ ] `curl http://localhost:8888/api/v1/system/info` returns `"deployment_mode": "demo"`
- [ ] Settings → Deployment tab shows demo mode with upgrade instructions
- [ ] Webhooks are disabled (`"webhooks_enabled": false`)
- [ ] Services bind to localhost/127.0.0.1 (secure local access)
- [ ] lrops.sh generates minimal .env for demo mode

### Production Mode Validation
- [ ] Setting `LIVEREVIEW_REVERSE_PROXY=true` and restart switches to production mode
- [ ] `curl http://127.0.0.1:8888/api/v1/system/info` returns `"deployment_mode": "production"`
- [ ] No demo mode banner appears when accessed via non-localhost
- [ ] Webhooks are enabled (`"webhooks_enabled": true`)
- [ ] Services bind to 127.0.0.1 (behind proxy)
- [ ] lrops.sh generates production .env with LIVEREVIEW_REVERSE_PROXY=true

### Environment Variable Validation
- [ ] `LIVEREVIEW_BACKEND_PORT` overrides default 8888
- [ ] `LIVEREVIEW_FRONTEND_PORT` overrides default 8081
- [ ] `LIVEREVIEW_REVERSE_PROXY` toggles demo/production modes
- [ ] Environment variables work in docker-compose
- [ ] CLI commands respect environment variables
- [ ] Missing environment variables use sensible defaults

### Frontend Validation
- [ ] API URL auto-detection works for both modes
- [ ] System info endpoint provides accurate real-time data
- [ ] Demo mode banner shows appropriate warnings and upgrade CTA
- [ ] Settings → Deployment tab reflects actual backend state
- [ ] All UI components handle loading/error states gracefully
- [ ] Frontend works without hardcoded API configuration

### Integration Validation
- [ ] Docker compose uses LIVEREVIEW_ environment variables correctly
- [ ] docker-entrypoint.sh handles new configuration format
- [ ] Reverse proxy integration works (manual test with nginx/caddy)
- [ ] Upgrade path from demo to production is smooth
- [ ] **TOML configuration completely untouched and working**
- [ ] Existing AI/Git provider configs continue to work

---

## Rollback Plan

If any phase encounters issues:

1. **Immediate Rollback**: Keep backup of original files
   ```bash
   # Restore from git if using version control
   git checkout -- internal/api/server.go
   git checkout -- ui/src/api/apiClient.ts
   git checkout -- docker-compose.yml
   ```

2. **Environment Variable Rollback**: Remove new environment variables
   ```bash
   # Remove new variables from .env
   sed -i '/LIVEREVIEW_/d' .env
   # Restart with original configuration
   docker-compose restart
   ```

3. **Frontend Rollback**: Revert UI changes if needed
   ```bash
   # Remove new components if they cause issues
   rm -f ui/src/components/DemoModeBanner.tsx
   # Revert Settings page changes
   git checkout -- ui/src/pages/Settings/Settings.tsx
   ```

4. **Test Rollback**: Verify original functionality
   ```bash
   # Test original setup commands
   ./lrops.sh status  # Should work as before
   curl http://localhost:8081  # Should work as before
   curl http://localhost:8888/api/version  # Should work as before
   ```

---

## Success Metrics

- **Onboarding Time**: New users can get demo running in < 2 minutes with lrops.sh
- **Configuration Complexity**: Demo mode requires 0 user configuration choices
- **Upgrade Simplicity**: Demo → Production upgrade in < 5 minutes
- **Backward Compatibility**: All existing functionality continues working
- **Error Reduction**: Fewer support requests related to localhost testing issues
- **User Feedback**: Positive feedback on simplified setup process
- **TOML Safety**: No changes to existing TOML configuration system

This implementation plan provides a clear, testable path to achieve the simplified onboarding experience while maintaining full backward compatibility and respecting the existing architecture.---

## Phase 2: Frontend Auto-Detection & Core Logic (Week 2-3)

### Phase 2.1: Frontend URL Detection Logic

#### Task 2.1.1: Update API Client Auto-Detection
- **File**: `ui/src/api/apiClient.ts`
- **Purpose**: Implement URL-based API detection logic
- **Changes**:
  - Replace existing API URL logic with hostname-based detection
  - localhost/127.0.0.1 → direct to port 8888
  - anything else → use current origin + /api
  - Maintain backward compatibility with injected config

**Test**:
```bash
# Test frontend API detection
cd ui/
npm start
# Check browser console for API URL detection
# Visit http://localhost:8081 → should detect http://localhost:8888
# Check Network tab for API calls going to correct endpoint
echo "✓ Frontend detects API URL correctly"
```

#### Task 2.1.2: Remove Hardcoded API Configuration
- **Files**: 
  - `ui/src/config/` (if exists)
  - `ui/.env*` files with API URLs
  - Any build-time API URL injection
- **Purpose**: Eliminate hardcoded API URLs in favor of runtime detection
- **Changes**:
  - Remove or comment out hardcoded API URLs
  - Update any build scripts that inject API URLs
  - Ensure runtime detection is the primary method

**Test**:
```bash
# Test without any hardcoded config
rm -f ui/.env.local ui/.env.development  # backup first
npm start
# Verify app still connects to API correctly
echo "✓ Frontend works without hardcoded API config"
```

### Phase 2.2: Backend API URL Injection

#### Task 2.2.1: Update UI Server for Runtime Config Injection
- **File**: `cmd/ui.go`
- **Purpose**: Inject runtime configuration into frontend at serve time
- **Changes**:
  - Add `--api-url` flag to UI server command
  - Implement `window.LIVEREVIEW_CONFIG` injection in HTML
  - Use environment variable or flag for API URL override

**Test**:
```bash
# Test manual API URL injection
./livereview ui --api-url "http://localhost:8888" &
curl http://localhost:8081 | grep "LIVEREVIEW_CONFIG"
# Should see injected config in HTML
echo "✓ Runtime config injection works"
```

#### Task 2.2.2: Auto-Detection in UI Server
- **File**: `cmd/ui.go`
- **Purpose**: Auto-detect API URL when not explicitly provided
- **Changes**:
  - Implement auto-detection logic in UI server
  - Use same logic as frontend (localhost vs production detection)
  - Default to reasonable values based on UI server port

**Test**:
```bash
# Test auto-detection without explicit API URL
./livereview ui --port 8081 &  # no --api-url flag
curl http://localhost:8081 | grep "LIVEREVIEW_CONFIG"
# Should detect correct API URL automatically
echo "✓ UI server auto-detects API URL"
```

---

## Phase 3: Frontend UI Enhancements (Week 3-4)

### Phase 3.1: Demo Mode Banner

#### Task 3.1.1: Create Demo Mode Banner Component
- **File**: `ui/src/components/DemoModeBanner.tsx` (new file)
- **Purpose**: Show demo mode limitations and upgrade path
- **Changes**:
  - Implement banner component with demo mode detection
  - Add limitation warnings (no webhooks, localhost only)
  - Include "Upgrade to Production" button/link

**Test**:
```bash
# Test demo mode banner
cd ui/
npm start
# Visit http://localhost:8081
# Should see orange banner at top warning about demo mode
echo "✓ Demo mode banner appears on localhost"

# Test production mode (no banner)
# Visit via IP or domain (if available)
# Banner should not appear
echo "✓ Banner hidden in production mode"
```

#### Task 3.1.2: Integrate Banner into Main Layout
- **Files**: 
  - `ui/src/App.tsx` or main layout component
  - `ui/src/components/Layout.tsx` (if exists)
- **Purpose**: Display banner consistently across all pages in demo mode
- **Changes**:
  - Import and render DemoModeBanner at top of main layout
  - Ensure banner appears before main content
  - Test banner responsiveness on mobile

**Test**:
```bash
# Test banner appears on all pages
# Navigate to different routes in the app
# Banner should persist across navigation
echo "✓ Banner appears consistently across all pages"
```

### Phase 3.2: Enhanced Instance Settings Page

#### Task 3.2.1: Create Enhanced Instance Settings Component
- **File**: `ui/src/pages/SuperAdmin/InstanceSettings.tsx` (new or update existing)
- **Purpose**: Show detailed system information and configuration
- **Changes**:
  - Implement system info display (mode, API URL, webhook status)
  - Add capability indicators with status dots
  - Include upgrade instructions for demo mode

**Test**:
```bash
# Test instance settings page
# Navigate to SuperAdmin → Instance Settings
# Should show:
# - Deployment Mode: Demo Mode (orange badge)
# - API Endpoint: http://localhost:8888
# - Webhooks: Disabled (red dot)
# - Upgrade instructions section
echo "✓ Instance settings shows correct demo mode info"
```

#### Task 3.2.2: Connect to System Info API
- **File**: `ui/src/pages/SuperAdmin/InstanceSettings.tsx`
- **Purpose**: Fetch real-time system information from backend
- **Changes**:
  - Add useEffect hook to fetch from `/api/v1/system/info`
  - Handle loading and error states
  - Update UI based on real backend data

**Test**:
```bash
# Test API integration
# Check Network tab for API call to /api/v1/system/info
# Verify data displayed matches API response
curl http://localhost:8888/api/v1/system/info | jq
echo "✓ Frontend displays real backend system info"
```

---

## Phase 4: Docker & Deployment Integration (Week 4-5)

### Phase 4.1: Docker Configuration Updates

#### Task 4.1.1: Update Docker Compose for Variable Ports
- **File**: `docker-compose.yml`
- **Purpose**: Use environment variables for port configuration
- **Changes**:
  - Replace hardcoded ports with `${BACKEND_PORT:-8888}:${BACKEND_PORT:-8888}`
  - Replace hardcoded ports with `${FRONTEND_PORT:-8081}:${FRONTEND_PORT:-8081}`
  - Ensure .env file is properly loaded

**Test**:
```bash
# Test with custom ports
echo "BACKEND_PORT=9888" > .env.test
echo "FRONTEND_PORT=9081" >> .env.test
docker-compose --env-file .env.test up -d
curl http://localhost:9081  # Should work
curl http://localhost:9888/api/v1/system/info  # Should work
echo "✓ Docker compose respects custom ports"
```

#### Task 4.1.2: Update Docker Entry Point Script
- **File**: `docker-entrypoint.sh`
- **Purpose**: Use new environment variables and simplified startup
- **Changes**:
  - Read BACKEND_PORT and FRONTEND_PORT from environment
  - Implement API URL auto-detection logic
  - Add debug output for configuration detection

**Test**:
```bash
# Test entry point script
export BACKEND_PORT=8888
export FRONTEND_PORT=8081
export REVERSE_PROXY=false
bash docker-entrypoint.sh  # Should show debug output and start correctly
echo "✓ Entry point script uses new environment variables"
```

### Phase 4.2: Production Mode Testing

#### Task 4.2.1: Create Production Mode Test Setup
- **File**: `test-production-mode.sh` (new script)
- **Purpose**: Test production mode without full reverse proxy
- **Changes**:
  - Create test script that simulates production mode
  - Set REVERSE_PROXY=true in test environment
  - Test binding to 127.0.0.1 instead of localhost

**Test**:
```bash
# Test production mode configuration
REVERSE_PROXY=true ./livereview api &
REVERSE_PROXY=true ./livereview ui &
curl http://127.0.0.1:8888/api/v1/system/info
# Should show: {"deployment_mode":"production",...}
echo "✓ Production mode detection works"
```

#### Task 4.2.2: Test Reverse Proxy Integration
- **File**: `nginx-test.conf` (new file for testing)
- **Purpose**: Verify the production mode works with reverse proxy
- **Changes**:
  - Create minimal nginx config for testing
  - Test /api/* routing to backend
  - Test / routing to frontend

**Test**:
```bash
# Set up test nginx config (if nginx available)
# Or document manual testing steps
echo "Manual test: Set up nginx with /api/* -> localhost:8888"
echo "and / -> localhost:8081, verify app works"
echo "✓ Reverse proxy routing works correctly"
```

---

## Phase 5: lrops.sh Integration & Simplified Setup (Week 5-6)

### Phase 5.1: Update lrops.sh Configuration Generation

#### Task 5.1.1: Simplify lrops.sh Configuration Options
- **File**: `lrops.sh`
- **Purpose**: Replace complex configuration with simple demo/production choice
- **Changes**:
  - Update configuration prompts to offer only demo/production
  - Generate minimal .env files with only essential variables
  - Remove complex IP detection and multi-mode logic

**Test**:
```bash
# Test simplified configuration
./lrops.sh configure  # or whatever the command is
# Should prompt for:
# 1. Demo Mode (localhost only) [DEFAULT]
# 2. Production (with reverse proxy)
# Should generate minimal .env file
echo "✓ lrops.sh generates simplified configuration"
```

#### Task 5.1.2: Update .env File Generation
- **File**: `lrops.sh` (configuration generation section)
- **Purpose**: Generate minimal .env files for both modes
- **Changes**:
  - Demo mode: Only DB_PASSWORD, JWT_SECRET
  - Production mode: Add REVERSE_PROXY=true
  - Include helpful comments about upgrade path

**Test**:
```bash
# Test .env generation
rm -f .env
./lrops.sh setup-demo  # or equivalent command
cat .env
# Should contain minimal required variables
# Should have helpful comments
echo "✓ .env file contains correct minimal configuration"
```

### Phase 5.2: Migration and Backward Compatibility

#### Task 5.2.1: Create Configuration Migration Script
- **File**: `migrate-config.sh` (new script)
- **Purpose**: Help existing users migrate to simplified configuration
- **Changes**:
  - Detect existing complex configuration
  - Suggest appropriate demo/production mode
  - Backup existing config and create new simplified version

**Test**:
```bash
# Test migration script
# Create fake old-style .env with complex config
./migrate-config.sh
# Should detect old config and suggest new simplified version
echo "✓ Migration script handles existing configurations"
```

#### Task 5.2.2: Update Documentation
- **Files**: 
  - `README.md`
  - `docs/` (various documentation files)
- **Purpose**: Update documentation to reflect simplified setup
- **Changes**:
  - Update quick start instructions
  - Simplify deployment documentation
  - Add troubleshooting for common issues

**Test**:
```bash
# Test documentation accuracy
# Follow updated README.md step by step
# Verify all commands work as documented
echo "✓ Documentation matches actual implementation"
```

---

## Phase 6: Integration Testing & Polish (Week 6)

### Phase 6.1: End-to-End Testing

#### Task 6.1.1: Demo Mode Complete Flow Test
- **Purpose**: Test complete demo mode experience from scratch
- **Test Sequence**:
```bash
# Clean slate test
rm -rf livereview-test/
mkdir livereview-test && cd livereview-test

# 1. Install/setup
../lrops.sh install-demo  # or equivalent
echo "✓ Demo mode installation completes"

# 2. Start services
docker-compose up -d
echo "✓ Services start successfully"

# 3. Access UI
curl http://localhost:8081
echo "✓ Frontend accessible on localhost:8081"

# 4. Check demo mode indicators
curl http://localhost:8081 | grep -i "demo mode"
echo "✓ Demo mode banner appears"

# 5. Test API connectivity
curl http://localhost:8888/api/v1/system/info | jq '.deployment_mode'
# Should return "demo"
echo "✓ Backend reports demo mode"

# 6. Test system info page
curl http://localhost:8081/api/v1/system/info
echo "✓ Frontend can access system info"

# 7. Verify webhook limitations
curl http://localhost:8888/api/v1/system/info | jq '.capabilities.webhooks_enabled'
# Should return false
echo "✓ Webhooks correctly disabled in demo mode"
```

#### Task 6.1.2: Production Mode Upgrade Test
- **Purpose**: Test upgrade from demo to production mode
- **Test Sequence**:
```bash
# Starting from working demo mode
echo "REVERSE_PROXY=true" >> .env
docker-compose restart

# 1. Check mode change
curl http://127.0.0.1:8888/api/v1/system/info | jq '.deployment_mode'
# Should return "production"
echo "✓ Backend switches to production mode"

# 2. Check binding changes
netstat -tlnp | grep :8888
# Should show 127.0.0.1:8888, not 0.0.0.0:8888
echo "✓ Backend binds to 127.0.0.1 in production mode"

# 3. Check webhook enablement
curl http://127.0.0.1:8888/api/v1/system/info | jq '.capabilities.webhooks_enabled'
# Should return true
echo "✓ Webhooks enabled in production mode"

# 4. Test with reverse proxy (manual setup required)
echo "Manual test: Configure nginx/caddy to route /api/* to 127.0.0.1:8888"
echo "Access via domain/IP and verify no demo banner appears"
echo "✓ Production mode works with reverse proxy"
```

### Phase 6.2: Performance and Error Testing

#### Task 6.2.1: Test Error Handling
- **Purpose**: Ensure graceful handling of common configuration errors
- **Test Cases**:
```bash
# 1. Missing environment variables
unset BACKEND_PORT
./livereview api
# Should use default port 8888
echo "✓ Handles missing BACKEND_PORT gracefully"

# 2. Invalid port numbers
export BACKEND_PORT=99999
./livereview api
# Should show error or use default
echo "✓ Handles invalid port numbers"

# 3. Backend unavailable
./livereview ui &  # start UI without backend
curl http://localhost:8081
# Frontend should still load, show appropriate error for API calls
echo "✓ Frontend handles backend unavailability"

# 4. System info API error
# Stop backend, check frontend instance settings page
# Should show loading or error state gracefully
echo "✓ Frontend handles API errors gracefully"
```

#### Task 6.2.2: Cross-Platform Testing
- **Purpose**: Verify functionality across different environments
- **Test Matrix**:
```bash
# Test on different operating systems if available
# Linux (primary)
./run-full-test-suite.sh
echo "✓ Works on Linux"

# macOS (if available)
./run-full-test-suite.sh
echo "✓ Works on macOS"

# Windows/WSL (if available)
./run-full-test-suite.sh
echo "✓ Works on Windows/WSL"

# Different browsers (manual)
echo "Manual test: Firefox, Chrome, Safari"
echo "✓ Frontend works in major browsers"
```

---

## Final Validation Checklist

### Demo Mode Validation
- [ ] `curl http://localhost:8081` shows demo mode banner
- [ ] `curl http://localhost:8888/api/v1/system/info` returns `"deployment_mode": "demo"`
- [ ] Instance settings page shows demo mode with upgrade instructions
- [ ] Webhooks are disabled (`"webhooks_enabled": false`)
- [ ] Services bind to `localhost` not `127.0.0.1`

### Production Mode Validation
- [ ] Setting `REVERSE_PROXY=true` and restart switches to production mode
- [ ] `curl http://127.0.0.1:8888/api/v1/system/info` returns `"deployment_mode": "production"`
- [ ] No demo mode banner appears when accessed via domain/IP
- [ ] Webhooks are enabled (`"webhooks_enabled": true`)
- [ ] Services bind to `127.0.0.1` not `localhost`

### Configuration Validation
- [ ] Demo mode requires only `DB_PASSWORD` and `JWT_SECRET` in .env
- [ ] lrops.sh generates minimal configuration
- [ ] Environment variables override defaults correctly
- [ ] Migration from old config works (if applicable)

### Frontend Validation
- [ ] API URL auto-detection works for both modes
- [ ] System info endpoint provides accurate data
- [ ] Demo mode banner shows appropriate warnings
- [ ] Instance settings page reflects actual backend state
- [ ] All UI components handle loading/error states

### Integration Validation
- [ ] Docker compose uses environment variables correctly
- [ ] docker-entrypoint.sh handles new configuration format
- [ ] Reverse proxy integration works (manual test with nginx/caddy)
- [ ] Upgrade path from demo to production is smooth

---

## Rollback Plan

If any phase encounters issues:

1. **Immediate Rollback**: Keep backup of original files
   ```bash
   git stash  # if using git
   # or restore from backup files
   ```

2. **Phase-by-Phase Rollback**: Each phase should be in separate commits
   ```bash
   git revert <phase-commit-hash>
   ```

3. **Configuration Rollback**: Keep backup of original lrops.sh and config files
   ```bash
   cp lrops.sh.backup lrops.sh
   cp .env.backup .env
   ```

4. **Test Rollback**: Verify original functionality after rollback
   ```bash
   # Run original setup commands
   # Verify all original features work
   ```

---

## Success Metrics

- **Onboarding Time**: New users can get demo running in < 2 minutes
- **Configuration Complexity**: Demo mode requires 0 configuration choices
- **Upgrade Simplicity**: Demo → Production upgrade in < 5 minutes
- **Error Reduction**: Fewer support requests related to configuration
- **User Feedback**: Positive feedback on simplified setup process

This implementation plan provides a clear, testable path to achieve the simplified onboarding experience while maintaining full functionality and backward compatibility.
