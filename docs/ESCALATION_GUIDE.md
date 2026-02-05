# Escalation Guide

This document describes how AgentiCorp escalates stuck beads to CEOs (human decision makers) with comprehensive context to enable quick resolution.

## Overview

When an agent becomes truly stuck (detected via [stuck detection heuristics](STUCK_DETECTION.md)), the system automatically escalates the bead to a CEO decision bead with rich context including:

- **Full investigation history**: All actions taken by agents
- **Progress analysis**: What was attempted and what's missing
- **Suggested next steps**: Specific recommendations for unblocking
- **Conversation context**: Link to full conversation history
- **Loop detection details**: Why the bead was flagged as stuck

This "graceful escalation" provides CEOs with everything needed to make informed decisions quickly.

## Escalation Triggers

A bead is escalated when:

1. **Dispatch hop limit reached** (default: 20 dispatches)
2. **AND stuck in loop** (detected by loop detector)
   - Same action repeated 3+ times
   - No progress in last 5 minutes
   - No variation in approach

Beads that reach the hop limit but are still making progress are **NOT** escalated.

## Escalation Contents

### CEO Decision Bead

A new decision bead is created with:

```
Title: CEO Decision Required
Priority: P0 (Critical)
Type: decision
Status: open
```

### Comprehensive Reason

The decision bead includes a structured reason:

```
dispatch_count=23 exceeded max_hops=20, stuck in loop: Repeated action pattern 7 times without progress

Progress Summary:
Files read: 8, modified: 1, tests: 0, commands: 0 (last: 12m ago)

Suggested Next Steps:
1. Agent made changes but didn't run tests - specify test commands or verification steps
2. Agent didn't run any commands - provide build/test/debug commands
3. Provide specific examples or reference implementations
4. Break down the task into smaller, more focused subtasks

This bead needs human attention to unblock the investigation.
```

### Bead Context

The original bead's context is enriched with:

```json
{
  "dispatch_escalated_at": "2026-02-05T20:00:00Z",
  "dispatch_escalation_reason": "dispatch_count=23 exceeded max_hops=20, stuck in loop: ...",
  "loop_detection_reason": "Repeated action pattern 7 times without progress",
  "progress_summary": "Files read: 8, modified: 1, tests: 0, commands: 0 (last: 12m ago)",
  "action_history_snapshot": "[{\"timestamp\":\"...\",\"action_type\":\"read_file\",...}]",
  "suggested_next_steps": "1. Agent made changes but didn't run tests...\n2. Agent didn't run any commands...\n",
  "conversation_session_id": "session-abc-123",
  "dispatch_escalation_decision_id": "decision-xyz-789"
}
```

## Suggested Next Steps

The system analyzes the action history and suggests specific next steps based on what was attempted:

### No File Exploration

**Pattern**: Agent hasn't read any files

**Suggestion**: "Agent hasn't explored the codebase - provide file locations or entry points"

**Action**: Tell the agent which files to examine

### Single File Focus

**Pattern**: Agent read same file repeatedly

**Suggestion**: "Agent focused on single file - suggest additional files to examine"

**Action**: Point to related files or broader context

### Read-Only Investigation

**Pattern**: Agent read files but made no changes

**Suggestion**: "Agent read files but made no changes - clarify what needs to be modified"

**Action**: Be more specific about what should change

### Changes Without Validation

**Pattern**: Agent modified files but didn't run tests

**Suggestion**: "Agent made changes but didn't run tests - specify test commands or verification steps"

**Action**: Provide test commands or acceptance criteria

### No Command Execution

**Pattern**: Agent didn't run any build/test/debug commands

**Suggestion**: "Agent didn't run any commands - provide build/test/debug commands"

**Action**: Specify commands to run for validation

### Multiple Approaches Tried

**Pattern**: Agent tried reading, editing, testing but still stuck

**Suggestion**: "Agent attempted multiple approaches - problem may require domain expertise"

**Action**: Provide domain-specific guidance or context

### General Suggestions

Always included:
- "Provide specific examples or reference implementations"
- "Break down the task into smaller, more focused subtasks"

## CEO Workflow

### 1. Review Escalation

Check the CEO decision bead:

```bash
# List CEO decision beads
bd list --type=decision --priority=0 --status=open

# View specific decision
bd show <decision-id>
```

The decision bead contains:
- Original bead ID in context
- Comprehensive reason with progress summary
- Suggested next steps

### 2. Investigate Original Bead

```bash
# View the original bead
bd show <original-bead-id>

# Check conversation history
# (Session ID is in context)

# Review action history
# (Full JSON in action_history_snapshot)
```

### 3. Analyze the Situation

Questions to consider:

- **Is the task clear?** Review bead description
- **Are requirements complete?** Check acceptance criteria
- **Is agent missing context?** What domain knowledge is needed?
- **Is task too large?** Should it be broken down?
- **Are tools/commands specified?** Does agent know how to validate?

### 4. Take Action

Choose one of:

#### A. Provide Additional Context

Update the original bead with:
- Specific file paths to examine
- Commands to run
- Examples or references
- Domain-specific knowledge

```bash
bd update <original-bead-id> --description="..."
# OR
bd update <original-bead-id> --notes="Additional context: ..."
```

Then re-open and allow to continue:

```bash
bd update <original-bead-id> --status=open
bd close <decision-id>
```

#### B. Break Down the Task

Create subtasks:

```bash
bd create --title="Subtask 1" --parent=<original-bead-id>
bd create --title="Subtask 2" --parent=<original-bead-id>
bd close <original-bead-id>
bd close <decision-id>
```

#### C. Reassign to Different Agent

```bash
bd update <original-bead-id> --assignee=<different-agent>
bd update <original-bead-id> --status=open
bd close <decision-id>
```

#### D. Close as Not Feasible

```bash
bd close <original-bead-id> --reason="Task not feasible because..."
bd close <decision-id>
```

## Example Escalations

### Example 1: Missing Test Commands

**Scenario**: Agent modified code but doesn't know how to validate

**Escalation**:
```
Progress Summary:
Files read: 5, modified: 3, tests: 0, commands: 0 (last: 15m ago)

Suggested Next Steps:
1. Agent made changes but didn't run tests - specify test commands or verification steps
2. Agent didn't run any commands - provide build/test/debug commands
```

**CEO Action**:
```bash
bd update bead-123 --notes="Run: go test ./internal/auth/... to validate changes. Expected: all tests pass, no new failures."
bd update bead-123 --status=open
```

### Example 2: Stuck on Single File

**Scenario**: Agent reading same file repeatedly

**Escalation**:
```
Progress Summary:
Files read: 1, modified: 0, tests: 0, commands: 0 (last: 20m ago)

Suggested Next Steps:
1. Agent focused on single file - suggest additional files to examine
2. Agent read files but made no changes - clarify what needs to be modified
```

**CEO Action**:
```bash
bd update bead-456 --notes="Also examine internal/auth/session.go and internal/auth/middleware.go for full context. Modify token validation logic in middleware.go."
bd update bead-456 --status=open
```

### Example 3: Complex Problem

**Scenario**: Agent tried many approaches but still stuck

**Escalation**:
```
Progress Summary:
Files read: 15, modified: 5, tests: 8, commands: 4 (last: 10m ago)

Suggested Next Steps:
1. Agent attempted multiple approaches - problem may require domain expertise
2. Review error messages and test failures for missing dependencies or configuration
```

**CEO Action**: Break down the task
```bash
bd create --title="Investigate root cause of auth failures" --parent=bead-789
bd create --title="Fix identified issue" --parent=bead-789 --depends-on=<subtask-1>
bd create --title="Add tests for edge cases" --parent=bead-789 --depends-on=<subtask-2>
bd close bead-789
```

## Monitoring Escalations

### Escalation Rate

Track how many beads are escalated vs. progressing:

```bash
# Beads that progressed past hop limit
grep "making progress" logs/dispatcher.log | wc -l

# Beads escalated as stuck
grep "stuck in loop" logs/dispatcher.log | wc -l

# Escalation rate (should be low with smart loop detection)
```

Healthy escalation rate: < 10% of beads reaching hop limit

### Common Escalation Reasons

```bash
# Most common patterns
grep "Suggested Next Steps" logs/dispatcher.log | sort | uniq -c | sort -rn
```

### Response Time

Track how long escalations wait for CEO decisions:

```bash
# Open CEO decisions
bd list --type=decision --priority=0 --status=open

# Age of oldest decision
bd show <decision-id> | grep Created
```

Target: Resolve CEO decisions within 24 hours

## Configuration

### Dispatch Hop Limit

```yaml
# config.yaml
dispatch:
  max_hops: 20  # Default, can be adjusted
```

- **Lower** (10-15): More escalations, faster stuck detection
- **Higher** (25-30): Fewer escalations, more agent autonomy

### Loop Detection Thresholds

Currently hardcoded in `internal/dispatch/loop_detector.go`:

```go
const (
    repeatThreshold = 3          // Actions must repeat 3+ times
    progressWindow  = 5 * time.Minute  // Progress within 5 minutes
    historySize     = 50         // Keep last 50 actions
)
```

Future: Make these configurable per project or agent type

## Best Practices

### For CEOs

1. **Review Promptly**: Check CEO decision beads daily
2. **Read Suggestions**: The system provides smart recommendations
3. **Be Specific**: Vague guidance leads to more escalations
4. **Update Personas**: If many agents stuck on same thing, update agent personas with that knowledge
5. **Track Patterns**: Recurring escalations indicate gaps in agent training

### For System Administrators

1. **Monitor Escalation Rate**: Should be low (< 10%)
2. **Review Resolved Escalations**: Learn what unblocked agents
3. **Adjust Hop Limit**: If too many/few escalations, tune max_hops
4. **Analyze Action Patterns**: Understand what agents are doing
5. **Improve Documentation**: Add missing context to agent personas

### For Agent Developers

1. **Record Actions**: Ensure agents log their actions properly
2. **Update Progress**: Signal progress through metrics
3. **Follow Suggestions**: System-generated next steps are usually good
4. **Request Clarification**: When stuck, clearly state what's needed
5. **Learn from Escalations**: Review resolved escalations to improve

## Integration with Other Systems

### Conversation Sessions

Escalations include conversation_session_id:

```bash
# Get full conversation context
curl http://localhost:8080/api/conversations/<session-id>
```

### Notifications

Set up alerts for CEO decisions:

```bash
# Slack webhook (example)
bd list --type=decision --priority=0 --status=open --format=json | \
  jq -r '.[] | "New CEO decision: \(.title) - \(.id)"' | \
  curl -X POST -H 'Content-type: application/json' \
       --data '{"text":"'"$(cat)"'"}' \
       $SLACK_WEBHOOK_URL
```

### Analytics

Track escalation metrics:

```sql
-- Escalations per week
SELECT
  date_trunc('week', created_at) as week,
  count(*) as escalations
FROM beads
WHERE type = 'decision' AND priority = 0
GROUP BY week
ORDER BY week DESC;

-- Most common stuck patterns
SELECT
  context->>'loop_detection_reason' as reason,
  count(*) as occurrences
FROM beads
WHERE status = 'blocked'
  AND context ? 'dispatch_escalated_at'
GROUP BY reason
ORDER BY occurrences DESC
LIMIT 10;
```

## Troubleshooting

### Too Many Escalations

**Symptom**: Many beads reaching CEO decisions

**Causes**:
- Hop limit too low
- Agent personas lack necessary context
- Tasks too complex/vague

**Solutions**:
- Increase `max_hops` in config
- Review and enhance agent personas
- Break down complex beads into subtasks

### Too Few Escalations

**Symptom**: Beads stuck for long time without escalating

**Causes**:
- Hop limit too high
- Loop detection too lenient
- Progress metrics updating incorrectly

**Solutions**:
- Decrease `max_hops` if appropriate
- Review loop detection thresholds
- Check action recording is working

### Poor Next Step Suggestions

**Symptom**: Suggestions don't match the situation

**Causes**:
- Insufficient action history
- Actions not categorized correctly
- Logic needs refinement

**Solutions**:
- Ensure agents record all actions
- Review ActionRecord types in code
- Propose improvements to suggestion logic

### CEOs Not Responding

**Symptom**: CEO decisions accumulating without resolution

**Solutions**:
- Set up notifications (Slack, email)
- Add CEO dashboard to web UI
- Schedule regular review sessions
- Assign on-call CEO rotation

## Future Enhancements

Planned improvements to escalation system:

1. **AI-Assisted Analysis**: Use LLM to analyze stuck situations and suggest solutions
2. **Escalation Playbooks**: Predefined responses for common stuck patterns
3. **Auto-Resolution**: For simple cases, automatically apply known fixes
4. **Escalation Learning**: ML to predict which beads will get stuck
5. **Interactive Debugging**: Allow CEO to step through agent actions interactively
6. **Contextual Examples**: Show similar resolved escalations for reference

## Related Documentation

- [Stuck Detection Heuristics](STUCK_DETECTION.md): How stuck loops are detected
- [Dispatch Configuration](DISPATCH_CONFIG.md): Dispatch system settings
- [Beads Workflow](BEADS_WORKFLOW.md): Overall beads system overview
- [Auto Bug Dispatch](auto-bug-dispatch.md): Automatic bug routing

## Version History

- **v1.2** (Epic 7, Task 4): Graceful escalation with comprehensive context
