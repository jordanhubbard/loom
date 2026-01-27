# Testing the Self-Healing Workflow

## Overview

This document describes how to test the complete self-healing workflow end-to-end, from error detection through auto-filing, routing, investigation, approval, and fix application.

## Prerequisites

1. **AgentiCorp running**
   ```bash
   make run
   ```

2. **Active agents with roles**
   - QA Engineer
   - Web Designer
   - Backend Engineer

3. **Active AI provider**
   - At least one provider configured and running

## Test Scenario 1: Frontend JavaScript Error

### Step 1: Trigger an Error

Edit `web/static/js/app.js` to introduce a bug:

```javascript
// Add this line somewhere to cause an error
unknownFunction();
```

Reload the browser. The error will be auto-filed.

### Step 2: Verify Auto-Filing

**Expected Result:**
- New bead created: `[auto-filed] [frontend] UI Error: ReferenceError: unknownFunction is not defined`
- Priority: P1 (High)
- Tags: `auto-filed`, `frontend`, `js_error`
- Assigned to: QA Engineer

**Verify:**
```bash
curl http://localhost:8080/api/v1/beads?tags=auto-filed
```

### Step 3: Verify Auto-Routing

**Expected Result:**
- Bead title updated: `[web-designer] [auto-filed] [frontend] UI Error: ...`
- Dispatcher logs show: "Auto-bug detected: ... routing to web-designer"

**Verify:**
```bash
grep "Auto-bug detected" logs/agenticorp.log
```

### Step 4: Verify Agent Investigation

**Expected Result:**
- Web Designer agent is dispatched the bead
- Agent receives specialized bug investigation instructions
- Agent performs actions:
  1. `search_text` for `unknownFunction`
  2. `read_file` for `app.js`
  3. Analyzes root cause
  4. Creates fix proposal with patch
  5. Creates CEO approval bead

**Verify:**
Check for new decision bead:
```bash
curl http://localhost:8080/api/v1/beads?type=decision | jq '.[] | select(.title | contains("Code Fix Approval"))'
```

### Step 5: CEO Review and Approval

**Manual Action:**
1. Open browser: http://localhost:8080
2. Navigate to Beads section
3. Find the `[CEO] Code Fix Approval: ...` bead
4. Review:
   - Root cause analysis
   - Proposed fix (patch)
   - Risk assessment
5. Close bead with comment: "Approved. Apply the fix."

### Step 6: Create Apply-Fix Task

**Manual Action (for now):**
1. Create new task bead:
   - Title: `[apply-fix] Apply approved patch from dc-xxx`
   - Description: Link to approval bead + instructions
   - Assign to: Web Designer
2. Dispatcher will pick it up

**Future Enhancement:**
This will be automated via a motivation trigger.

### Step 7: Agent Applies Fix

**Expected Result:**
- Agent reads approval bead
- Agent extracts patch
- Agent applies using `write_file` or `apply_patch`
- Agent updates cache version
- Agent closes both beads

**Verify:**
```bash
# Check that app.js was fixed
grep "unknownFunction" web/static/js/app.js
# Should not appear (or should be commented out)

# Check beads are closed
curl http://localhost:8080/api/v1/beads/<bug-bead-id>
# Status should be "closed"
```

### Step 8: Verify Fix Works

**Expected Result:**
- Hot-reload detects change
- Browser automatically refreshes
- No more errors in console

## Test Scenario 2: Backend Go Error

### Step 1: Trigger a Go Error

Edit a Go handler to cause a panic:

```go
func (s *Server) handleBeads(w http.ResponseWriter, r *http.Request) {
    panic("test panic") // Add this
    // ... rest of code
}
```

### Step 2: Trigger the Error

```bash
curl http://localhost:8080/api/v1/beads
```

### Step 3: Verify Auto-Filing

**Expected Result:**
- Bead: `[auto-filed] [backend] Runtime Error: panic in handler`
- Tags: `auto-filed`, `backend`, `go_error`
- Priority: P0 (Critical - panic)

### Step 4: Verify Auto-Routing

**Expected Result:**
- Title updated: `[backend-engineer] [auto-filed] [backend] Runtime Error: ...`
- Dispatched to Backend Engineer agent

### Step 5-8: Same as Scenario 1

Follow same investigation → approval → fix → verification workflow.

## Test Scenario 3: Hot-Reload Verification

### Test CSS Changes

1. Edit `web/static/css/style.css`:
   ```css
   body {
       background-color: #f0f0f0; /* Change this */
   }
   ```

2. Save file

**Expected Result:**
- Hot-reload detects change
- CSS reloads without full page refresh
- Background color changes immediately
- Browser console shows: `[HotReload] CSS reloaded`

### Test JavaScript Changes

1. Edit `web/static/js/app.js`:
   ```javascript
   console.log('Hot reload test');
   ```

2. Save file

**Expected Result:**
- Hot-reload detects change
- Full page reload triggered
- Toast notification: "Reloading: JavaScript file changed"
- Console shows new log message after reload

### Test HTML Changes

1. Edit `web/static/index.html`:
   ```html
   <title>AgentiCorp - Test</title>
   ```

2. Save file

**Expected Result:**
- Full page reload triggered
- Page title changes

## Test Scenario 4: Complete Self-Healing Loop

### Setup

1. Enable hot-reload in config.yaml
2. Start AgentiCorp
3. Open browser with DevTools

### Execute

1. **Introduce bug:** Edit `app.js` to add `unknownVar.toString()`
2. **Trigger error:** Reload browser
3. **Wait for auto-file:** Error appears in console, toast notification shown
4. **Wait for routing:** Check dispatcher logs for "Auto-bug detected"
5. **Wait for investigation:** Agent analyzes and creates approval bead (~1-2 minutes)
6. **CEO approves:** Close approval bead with "Approved"
7. **Create apply-fix:** Manual for now
8. **Wait for fix:** Agent applies patch (~30 seconds)
9. **Hot-reload triggers:** Browser refreshes automatically
10. **Verify:** Error gone from console

### Success Criteria

- ✅ Error auto-filed within 5 seconds
- ✅ Bug routed to correct agent (web-designer)
- ✅ Agent investigation complete within 3 minutes
- ✅ Approval bead created with full analysis
- ✅ CEO can review and approve
- ✅ Fix applied correctly
- ✅ Hot-reload triggers automatically
- ✅ Error resolved, no new errors

## Common Issues

### Auto-Filing Not Working

**Symptoms:** Errors in console but no beads created

**Check:**
1. Auto-filing enabled in config
2. Frontend error handler registered
3. API endpoint `/api/v1/auto-file-bug` accessible
4. Check browser console for fetch errors

**Fix:**
```javascript
// In app.js, verify this exists:
window.addEventListener('error', handleGlobalError);
```

### Auto-Routing Not Happening

**Symptoms:** Beads created but not routed to specialists

**Check:**
1. Dispatcher is running
2. AutoBugRouter initialized
3. Check dispatcher logs for errors

**Debug:**
```bash
grep "Auto-bug" logs/agenticorp.log
```

### Agent Not Investigating

**Symptoms:** Bead routed but agent doesn't act

**Check:**
1. Agent is idle and has provider
2. Provider is active
3. Agent role matches persona hint
4. Check for dispatch errors

**Debug:**
```bash
curl http://localhost:8080/api/v1/agents | jq '.[] | select(.status == "idle")'
```

### Hot-Reload Not Working

**Symptoms:** File changes don't trigger reload

**Check:**
1. Hot-reload enabled in config.yaml
2. WebSocket connection established
3. File patterns match changed file
4. Watch directories include changed file

**Debug:**
```javascript
// In browser console:
window.hotReload.status()
// Should show: { connected: true, attempts: 0 }
```

## Performance Metrics

Track these metrics during testing:

| Metric | Target | Actual |
|--------|--------|--------|
| Error to auto-file | < 5s | |
| Auto-file to routing | < 10s | |
| Routing to dispatch | < 30s | |
| Investigation time | < 3min | |
| Approval to apply | < 1min | |
| Total resolution time | < 5min | |
| Hot-reload latency | < 2s | |

## Automation

Future enhancement: Create automated test suite that:
1. Injects known bugs
2. Waits for auto-filed beads
3. Auto-approves low-risk fixes
4. Verifies fixes applied
5. Reports metrics

Example test script:
```bash
#!/bin/bash
# test-self-healing.sh

echo "1. Injecting bug..."
echo 'unknownFunc();' >> web/static/js/app.js

echo "2. Waiting for auto-file..."
sleep 10

echo "3. Checking for bead..."
BEAD=$(curl -s http://localhost:8080/api/v1/beads?tags=auto-filed | jq -r '.[0].id')
echo "Found bead: $BEAD"

echo "4. Waiting for investigation..."
sleep 180

echo "5. Checking for approval bead..."
APPROVAL=$(curl -s http://localhost:8080/api/v1/beads?type=decision | jq -r '.[0].id')
echo "Found approval: $APPROVAL"

echo "Test complete. Manual approval required."
```

## See Also

- [Auto-Bug Dispatch](./auto-bug-dispatch.md)
- [Agent Bug Fix Workflow](./agent-bug-fix-workflow.md)
- [Hot-Reload System](./hot-reload.md)
