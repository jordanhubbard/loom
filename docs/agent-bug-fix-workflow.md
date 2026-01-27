# Agent Bug Fix Workflow

## Overview

This document specifies the workflow for agents to investigate auto-filed bugs, propose fixes, obtain CEO approval, and apply the fixes. This completes the self-healing loop where errors are automatically detected, routed, investigated, and fixed.

## Architecture

### Phase 1: Auto-Filing (✅ Complete)
- Frontend/backend errors detected
- Bug reports auto-filed via `/api/v1/auto-file-bug`
- Beads created with structured error information

### Phase 2: Auto-Routing (✅ Complete)
- AutoBugRouter analyzes bug type
- Persona hints added to bead titles
- Beads dispatched to specialist agents

### Phase 3: Investigation & Fix (🚧 Current)
- Agent analyzes bug context
- Agent searches and reads relevant code
- Agent identifies root cause
- Agent proposes specific fix
- Agent creates CEO approval bead
- CEO reviews and approves/rejects
- Agent applies approved fix

## Bug Investigation Workflow

### Step 1: Receive Auto-Filed Bug

When an agent is dispatched an auto-filed bug bead, it receives:

**Bead Title:**
```
[backend-engineer] [auto-filed] [frontend] API Error: 500 Internal Server Error
```

**Bead Description:**
```markdown
## Auto-Filed Bug Report

**Source:** frontend
**Error Type:** js_error
**Severity:** high
**Occurred At:** 2026-01-27T00:30:12Z

### Error Message
ReferenceError: apiCall is not defined

### Stack Trace
at app.js:3769:45
at loadMotivations (app.js:3750:10)

### Context
```json
{
  "url": "http://localhost:8080/",
  "line": 3769,
  "column": 45,
  "source_file": "app.js",
  "user_agent": "Chrome/144.0.0.0",
  "viewport": "1803x1045"
}
```
```

### Step 2: Extract Investigation Parameters

Agent should extract from the bead description:
- **Error message**: What went wrong
- **Stack trace**: Where it happened (file, line, function)
- **Source file**: Which file to investigate
- **Error type**: What kind of error (JS, Go, API, etc.)
- **Context**: Additional debugging info

### Step 3: Search for Relevant Code

**Action: `search_text`**

Search for:
1. Exact error location from stack trace
2. Function/variable names mentioned in error
3. Related API endpoints or handlers

**Example:**
```json
{
  "action": "search_text",
  "parameters": {
    "query": "loadMotivations",
    "path": "web/static/js/",
    "file_pattern": "*.js"
  }
}
```

### Step 4: Read Relevant Files

**Action: `read_file`**

Read files identified in search to understand:
- Current implementation
- Dependencies
- Related code

**Example:**
```json
{
  "action": "read_file",
  "parameters": {
    "path": "web/static/js/app.js",
    "start_line": 3750,
    "end_line": 3780
  }
}
```

### Step 5: Analyze Root Cause

Agent should:
1. Identify the specific bug (e.g., undefined variable, nil pointer, API mismatch)
2. Understand why it happened (e.g., duplicate declaration, missing import, wrong response format)
3. Determine correct fix approach (e.g., remove duplicate, add import, fix parsing)
4. Consider side effects and related code

### Step 6: Propose Fix

**Action: `apply_patch` (dry-run mode)**

Create a unified diff patch showing the proposed changes:

**Example Patch:**
```diff
--- a/web/static/js/app.js
+++ b/web/static/js/app.js
@@ -3750,7 +3750,8 @@
 async function loadMotivations() {
     const historyRes = await fetch(`${API_BASE}/motivations/history`);
     if (historyRes.ok) {
-        motivationsState.history = await historyRes.json();
+        const historyData = await historyRes.json();
+        motivationsState.history = historyData.history || [];
         renderMotivations();
     }
 }
```

### Step 7: Create CEO Approval Bead

**Action: `create_bead`**

Create a new bead requesting CEO approval with:

**Title Format:**
```
[CEO] Code Fix Approval: <Brief Description>
```

**Description Format:**
```markdown
## Code Fix Proposal

**Original Bug:** <Link to auto-filed bug bead>
**Bug Type:** <Error type>
**Severity:** <Severity level>

### Root Cause Analysis

<Detailed explanation of what went wrong and why>

### Proposed Fix

<High-level description of the solution>

### Changes Required

<Unified diff patch showing all code changes>

### Testing Strategy

<How to verify the fix works>

### Risk Assessment

**Risk Level:** Low/Medium/High

**Potential Side Effects:**
- <List any potential issues>

**Rollback Plan:**
- <How to revert if needed>

### Recommendation

I recommend <approval/rejection> because <reasoning>.

---
*Proposed by: <Agent Name>*
*Original Bug: <Bead ID>*
```

**Example:**
```json
{
  "action": "create_bead",
  "parameters": {
    "title": "[CEO] Code Fix Approval: Fix motivations API response parsing",
    "description": "<Full proposal as above>",
    "type": "decision",
    "priority": 0,
    "tags": ["code-fix", "approval-required", "auto-bug-fix"]
  }
}
```

### Step 8: Update Original Bug Bead

Add a comment to the original auto-filed bug bead with:
- Link to CEO approval bead
- Summary of findings
- Status: "Fix proposed, awaiting approval"

### Step 9: Wait for CEO Approval

Agent monitors the CEO approval bead:
- If approved: Proceed to Step 10
- If rejected: Return to investigation or close as "won't fix"
- If feedback provided: Revise proposal and resubmit

### Step 10: Apply Approved Fix

**Action: `apply_patch`**

Apply the patch that was approved:

```json
{
  "action": "apply_patch",
  "parameters": {
    "patch": "<The unified diff>",
    "verify": true
  }
}
```

### Step 11: Verify Fix

Depending on the type of fix:

**Frontend Changes:**
1. Increment cache version in HTML (`v=X` → `v=X+1`)
2. Test in browser (if applicable)
3. Check for new errors

**Backend Changes:**
1. Run `go build` to verify compilation
2. Run relevant tests
3. Restart service if in development

**Action: `execute_command`** (if available)

```json
{
  "action": "execute_command",
  "parameters": {
    "command": "go test ./internal/api/...",
    "timeout": 60
  }
}
```

### Step 12: Close Beads

**Original Bug Bead:**
- Status: Closed
- Resolution: Fixed
- Comment: "Fixed by applying patch from <approval bead>"

**CEO Approval Bead:**
- Status: Closed
- Resolution: Approved and applied
- Comment: "Fix successfully applied and verified"

## Workflow State Machine

```
┌─────────────────────┐
│  Bug Dispatched     │
│  to Agent           │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Extract Context    │
│  Parse Error Info   │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Search Code        │
│  Find Relevant Files│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Read Files         │
│  Analyze Code       │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Identify Root      │
│  Cause              │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Create Patch       │
│  Propose Fix        │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Create CEO         │
│  Approval Bead      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Wait for CEO       │
│  Decision           │
└──────┬──────┬───────┘
       │      │
   Approved Rejected
       │      │
       ▼      ▼
┌──────────┐ ┌──────────┐
│Apply Fix │ │Close Bug │
│         │ │Won't Fix │
└────┬─────┘ └──────────┘
     │
     ▼
┌──────────┐
│Verify Fix│
└────┬─────┘
     │
     ▼
┌──────────┐
│Close Both│
│Beads     │
└──────────┘
```

## Agent Prompt Template

When an agent receives an auto-filed bug bead, it should follow this investigation template:

```
I've been assigned auto-filed bug bead <BEAD_ID>: "<TITLE>"

Let me investigate:

1. EXTRACT CONTEXT
   Error: <error_message>
   Location: <file>:<line>
   Type: <error_type>

2. SEARCH CODE
   [search_text action to find relevant code]

3. READ FILES
   [read_file actions for identified files]

4. ROOT CAUSE ANALYSIS
   The bug occurred because: <explanation>

   Specific issue: <detailed cause>

5. PROPOSED FIX
   Solution approach: <description>

   Changes needed:
   <unified diff patch>

6. RISK ASSESSMENT
   Risk level: <Low/Medium/High>
   Side effects: <list>
   Testing: <strategy>

7. CREATE APPROVAL REQUEST
   [create_bead action with full proposal]

I recommend <approval/rejection> because <reasoning>.
```

## CEO Approval Process

### CEO Reviews Proposal

CEO examines:
1. Root cause analysis - is it correct?
2. Proposed fix - is it the right approach?
3. Risk assessment - acceptable?
4. Testing strategy - sufficient?

### CEO Actions

**Approve:**
```
Approved. Proceed with applying the fix.
```
Agent proceeds to apply patch.

**Reject:**
```
Rejected. <Reason>
```
Agent closes proposal and returns to investigation.

**Request Changes:**
```
Needs revision: <Specific feedback>
```
Agent revises proposal and resubmits.

## Hot-Reload Integration (Future)

Once hot-reload is implemented:

1. Agent applies fix to development environment
2. Hot-reload automatically reloads changed files
3. Agent monitors for new errors
4. If no errors after 30s, mark as successful
5. If new errors appear, rollback and report

## Error Handling

### Agent Cannot Identify Cause

If agent cannot determine root cause:
1. Create investigation bead with findings
2. Tag as `needs-human-review`
3. Escalate to CEO with context
4. Close original bug as `needs-investigation`

### Fix Causes New Errors

If applied fix introduces new errors:
1. Auto-file new bug reports
2. Link to original fix proposal
3. Rollback if possible
4. Create escalation bead for CEO

### Multiple Related Bugs

If multiple auto-filed bugs have same root cause:
1. Create single fix proposal
2. Reference all related bug beads
3. Close all related beads when fix applied

## Metrics and Monitoring

Track these metrics:
- **Time to investigation**: Bug filed → Agent starts investigation
- **Time to proposal**: Investigation → Fix proposal created
- **Time to approval**: Proposal → CEO approval
- **Time to resolution**: Approval → Fix applied and verified
- **Success rate**: % of fixes that don't cause regressions
- **Auto-fix rate**: % of bugs fixed without human intervention

## Example: Complete Workflow

### 1. Bug Auto-Filed
```
Bead: ac-js-error-001
Title: [auto-filed] [frontend] ReferenceError: apiCall is not defined
Tags: auto-filed, frontend, js_error
Priority: P1
Assigned: qa-engineer
```

### 2. Auto-Routed
```
Title updated: [web-designer] [auto-filed] [frontend] ReferenceError: apiCall is not defined
Dispatched to: agent-web-designer-001
```

### 3. Investigation
```
Agent searches for: "apiCall" in web/static/js/
Agent reads: app.js, diagrams.js
Agent finds: Duplicate const API_BASE declaration
```

### 4. Fix Proposal
```
Bead: dc-fix-001
Title: [CEO] Code Fix Approval: Remove duplicate API_BASE in diagrams.js
Type: decision
Priority: P0
Contains: Root cause, patch, risk assessment
```

### 5. CEO Approval
```
CEO reviews dc-fix-001
CEO adds comment: "Approved. Good analysis."
CEO closes with resolution: "approved"
```

### 6. Fix Applied
```
Agent applies patch to diagrams.js
Agent increments cache version v=1 → v=2
Agent adds comment to ac-js-error-001: "Fixed"
Agent closes ac-js-error-001 with resolution: "fixed"
```

### 7. Verification
```
No new errors filed in next 5 minutes
Mark as successful fix
Update metrics
```

## Next Steps

1. ✅ Design workflow (this document)
2. Create bug investigation action or update agent prompts
3. Implement CEO approval workflow in REPL
4. Add hot-reload for automatic testing
5. Build metrics dashboard

## See Also

- [Auto-Filing System](./auto-filing.md)
- [Auto-Bug Dispatch](./auto-bug-dispatch.md)
- [Agent Actions](./agent-actions.md)
- [CEO Workflows](./ceo-workflows.md)
