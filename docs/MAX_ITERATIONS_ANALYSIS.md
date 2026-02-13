# Max Iterations Analysis

## Investigation Summary

**Date**: 2026-02-12
**Context**: Investigating why agents hit max_iterations (15) before completing work

## Findings

### Configuration

- **Default max iterations**: 15 (set in `internal/loom/loom.go:311`)
- **Fallback max iterations**: 25 (if not configured, `internal/worker/worker.go:640`)
- **Terminal reason**: `"max_iterations"` when exhausting all iterations

### Beads That Hit Max Iterations

| Bead ID | Task Type | Dispatches | Tokens Used | Error |
|---------|-----------|------------|-------------|-------|
| loom-6yswe | Project readiness (infrastructure) | 260 | 766,887 | context deadline exceeded |
| loom-eo8ta | Project readiness (infrastructure) | - | - | context deadline exceeded |
| loom-2joc | Investigate circular dependencies | 50 | 77,116 | context deadline exceeded |
| loom-8tw0 | Assess concurrency safety | 150 | 118,278 | context deadline exceeded |

### Root Causes

#### 1. **Infrastructure Issues** (loom-6yswe, loom-eo8ta)
- **Problem**: Agents assigned to unfixable infrastructure problems (SSH keys, missing directories)
- **Why max_iterations**: Agents tried repeatedly but couldn't fix system-level issues
- **Status**: ✅ **FIXED** by Task #8 (requires-human-config tag prevents assignment)

#### 2. **Complex Research Tasks** (loom-2joc, loom-8tw0)
- **Problem**: Code analysis tasks requiring many file reads and iterations
- **Why max_iterations**:
  - Tasks hit `context deadline exceeded` before completing naturally
  - 15 iterations insufficient for comprehensive codebase analysis
  - Large codebases need more exploration iterations

**Examples:**
- `loom-2joc`: "Investigate circular dependencies" - requires analyzing package graphs across entire codebase
- `loom-8tw0`: "Assess concurrency safety" - requires scanning many files for goroutines and race conditions

#### 3. **Context Deadline Exceeded**
- **All beads** hit `"context deadline exceeded"` error
- **Implication**: Not naturally running out of iterations, but hitting a timeout
- **Source**: Context with deadline passed to `ExecuteTask` (dispatcher.go:670)
- **Impact**: Agent loops interrupted before natural completion

## Recommendations

### 1. **Increase Max Iterations for Research Tasks** ⭐ HIGH PRIORITY

**Problem**: 15 iterations insufficient for complex analysis tasks

**Solution A - Task-Specific Limits**:
```go
// Add bead-level max_iterations override
func (d *Dispatcher) getMaxIterationsForBead(bead *models.Bead) int {
    // Check bead tags for task complexity
    if d.hasTag(bead, "code-review") || d.hasTag(bead, "analysis") {
        return 30 // More iterations for research
    }
    if d.hasTag(bead, "refactor") || d.hasTag(bead, "feature") {
        return 25 // Medium complexity
    }
    return 15 // Default for simple tasks
}
```

**Solution B - Increase Global Default**:
```go
// In internal/loom/loom.go:311
agentMgr.SetMaxLoopIterations(25) // Was: 15
```

**Recommendation**: Implement Solution A for better resource management

### 2. **Investigate Context Timeout** ⭐ HIGH PRIORITY

**Problem**: Tasks hitting "context deadline exceeded" before natural completion

**Investigation Needed**:
- Find where context deadline is set in dispatch chain
- Determine if timeout is too short for complex tasks
- Consider removing timeout or making it configurable per task type

**Search locations**:
```bash
# Find where context deadline is created
grep -r "WithTimeout\|WithDeadline" internal/dispatch/
grep -r "context.WithTimeout" cmd/
```

### 3. **Add Iteration Budget Tracking** 🔍 MEDIUM PRIORITY

**Enhancement**: Track why iterations are consumed

```go
type IterationUsage struct {
    FileReads      int
    CommandRuns    int
    LLMCalls       int
    ActionFailures int
}
```

**Benefit**: Understand where iteration budget is spent

### 4. **Implement Progressive Complexity Detection** 🔍 LOW PRIORITY

**Idea**: Start with low iterations, increase if needed

```go
// Start with 10 iterations
// If agent signals "need more time", grant +10
// Track per-bead iteration grants
```

**Benefit**: Optimize resource usage - simple tasks use fewer iterations

## Impact Assessment

### Current Impact (Pre-Fix)
- Infrastructure beads: ✅ Fixed (no longer assigned)
- Research tasks: ❌ Still problematic (hit timeouts)
- Token waste: ~900K+ tokens on failed loops

### Expected Impact (Post-Fix)

**If we increase to 25 iterations**:
- ✅ Research tasks more likely to complete
- ✅ Reduced redispatch waste
- ⚠️ Slightly higher token cost per bead
- ⚠️ Longer execution times

**If we fix context timeout**:
- ✅ Agents can work until natural completion
- ✅ Better success rate
- ⚠️ Risk of runaway agents (mitigated by max_iterations)

## Recommended Action Plan

1. ✅ **DONE**: Tag infrastructure beads to prevent agent assignment
2. ⭐ **DO NEXT**: Increase max_iterations from 15 → 25 globally
3. ⭐ **DO NEXT**: Investigate and fix context deadline timeout
4. 🔍 **LATER**: Implement task-specific iteration limits
5. 🔍 **LATER**: Add iteration usage tracking

## Testing Plan

After increasing max_iterations:

1. Monitor beads that previously hit max_iterations
2. Check if they complete successfully with 25 iterations
3. Measure token usage increase (expect ~40-60% more)
4. Verify no runaway loops occur

Success Metrics:
- `terminal_reason: max_iterations` occurrences drop by >50%
- Bead completion rate improves for research/analysis tasks
- Average dispatch_count decreases

## References

- Max iterations config: `internal/loom/loom.go:311`
- Action loop: `internal/worker/worker.go:618-950`
- Terminal reason set: `internal/worker/worker.go:943`
- Dispatch retry logic: `internal/dispatch/dispatcher.go:761-768`
