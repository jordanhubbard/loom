# Agent Actions Reference

This document describes the action schema for AgentiCorp agents. Agents use these actions to interact with the codebase, run commands, manage beads, and execute tests.

## Action Envelope Format

All agent responses must be valid JSON matching this schema:

```json
{
  "actions": [
    {
      "type": "action_name",
      // action-specific fields
    }
  ],
  "notes": "Optional notes about the actions"
}
```

## Available Actions

### File Operations

#### read_file

Read the contents of a file.

```json
{
  "type": "read_file",
  "path": "src/main.go"
}
```

**Fields:**
- `path` (required): Path to file relative to project root

**Returns:**
- `path`: Absolute file path
- `content`: File contents
- `size`: File size in bytes

#### write_file

Write content to a file (creates or overwrites).

```json
{
  "type": "write_file",
  "path": "src/config.json",
  "content": "{\"key\": \"value\"}"
}
```

**Fields:**
- `path` (required): Path to file
- `content` (required): File contents to write

**Returns:**
- `path`: Absolute file path
- `bytes_written`: Number of bytes written

#### read_code

Alias for `read_file` (legacy, prefer `read_file`).

#### edit_code

Apply a unified diff patch to files.

```json
{
  "type": "edit_code",
  "path": "src/main.go",
  "patch": "--- a/src/main.go\n+++ b/src/main.go\n@@ -10,7 +10,7 @@\n-\told code\n+\tnew code"
}
```

**Fields:**
- `path` (required): Path to file being patched
- `patch` (required): Unified diff in git format

**Returns:**
- `output`: Patch application output

#### apply_patch

Apply a multi-file unified diff patch.

```json
{
  "type": "apply_patch",
  "patch": "diff --git a/file1.go b/file1.go\n..."
}
```

**Fields:**
- `patch` (required): Multi-file unified diff

**Returns:**
- `output`: Patch application results

#### read_tree

List directory contents recursively.

```json
{
  "type": "read_tree",
  "path": "src",
  "max_depth": 3,
  "limit": 100
}
```

**Fields:**
- `path` (optional): Directory path (default: ".")
- `max_depth` (optional): Maximum recursion depth
- `limit` (optional): Maximum entries to return

**Returns:**
- `entries`: Array of file/directory entries

#### search_text

Search for text in files using grep.

```json
{
  "type": "search_text",
  "path": "src",
  "query": "TODO",
  "limit": 50
}
```

**Fields:**
- `path` (optional): Directory to search (default: ".")
- `query` (required): Search pattern
- `limit` (optional): Maximum matches to return

**Returns:**
- `matches`: Array of matching lines with file/line info

### Command Execution

#### run_command

Execute a shell command in the project directory.

```json
{
  "type": "run_command",
  "command": "npm install",
  "working_dir": "frontend"
}
```

**Fields:**
- `command` (required): Shell command to execute
- `working_dir` (optional): Working directory for command

**Returns:**
- `command_id`: Execution ID for tracking
- `exit_code`: Command exit code

**Security:** Commands are executed in a sandboxed environment with resource limits.

#### run_tests

Execute project tests and return structured results.

```json
{
  "type": "run_tests",
  "test_pattern": "TestCalculator",
  "framework": "go",
  "timeout_seconds": 300
}
```

**Fields:**
- `test_pattern` (optional): Pattern to filter tests (e.g., "TestFoo*", "test_bar")
- `framework` (optional): Test framework to use ("go", "jest", "pytest", "npm")
- `timeout_seconds` (optional): Maximum execution time in seconds

**Returns:**
```json
{
  "framework": "go",
  "success": false,
  "exit_code": 1,
  "timed_out": false,
  "duration": "1.234s",
  "summary": {
    "total": 10,
    "passed": 8,
    "failed": 2,
    "skipped": 0
  },
  "tests": [
    {
      "name": "TestCalculate",
      "package": "github.com/user/pkg",
      "status": "fail",
      "duration": "0.123s",
      "error": "Expected 100, got 99",
      "stack_trace": "..."
    }
  ],
  "raw_output": "full test output..."
}
```

**Framework Auto-Detection:**

If `framework` is not specified, the system auto-detects based on:

- **Go**: Presence of `go.mod` or `*_test.go` files
- **Jest**: `package.json` with `jest` dependency
- **npm**: Generic `package.json` without specific framework
- **pytest**: `pytest.ini`, `pyproject.toml`, or `test_*.py` files

**Examples:**

Run all tests with auto-detection:
```json
{
  "type": "run_tests"
}
```

Run specific Go tests:
```json
{
  "type": "run_tests",
  "test_pattern": "TestDatabase",
  "framework": "go"
}
```

Run Jest tests with custom timeout:
```json
{
  "type": "run_tests",
  "test_pattern": "should handle errors",
  "framework": "jest",
  "timeout_seconds": 600
}
```

Run pytest with pattern:
```json
{
  "type": "run_tests",
  "test_pattern": "test_api",
  "framework": "pytest"
}
```

**Test-Fix-Retest Pattern:**

```json
{
  "actions": [
    {"type": "run_tests"},
    {"type": "read_file", "path": "src/calculator.go"},
    {"type": "edit_code", "path": "src/calculator.go", "patch": "..."},
    {"type": "run_tests", "test_pattern": "TestCalculate"}
  ],
  "notes": "Fixed off-by-one error in Calculate function"
}
```

### Git Operations

#### git_status

Get git status for the project.

```json
{
  "type": "git_status"
}
```

**Returns:**
- `output`: Git status output

#### git_diff

Get git diff for the project.

```json
{
  "type": "git_diff"
}
```

**Returns:**
- `output`: Git diff output

### Bead Management

#### create_bead

Create a new bead (issue/task).

```json
{
  "type": "create_bead",
  "bead": {
    "title": "Fix authentication bug",
    "description": "Users cannot login after password reset",
    "project_id": "proj-123",
    "priority": 1,
    "type": "bug",
    "tags": ["security", "urgent"]
  }
}
```

**Fields:**
- `bead` (required): Bead payload object
  - `title` (required): Bead title
  - `project_id` (required): Project identifier
  - `description` (optional): Detailed description
  - `priority` (optional): Priority level (0-4, where 0=critical, 4=backlog)
  - `type` (optional): Bead type ("task", "bug", "feature", "epic")
  - `tags` (optional): Array of tags

**Returns:**
- `bead_id`: Created bead identifier

#### close_bead

Mark a bead as complete.

```json
{
  "type": "close_bead",
  "bead_id": "bead-abc-123",
  "reason": "Fixed authentication bug and added tests"
}
```

**Fields:**
- `bead_id` (required): Bead identifier
- `reason` (optional): Closure reason/notes

**Returns:**
- `bead_id`: Closed bead identifier

#### escalate_ceo

Escalate a bead to CEO for decision.

```json
{
  "type": "escalate_ceo",
  "bead_id": "bead-abc-123",
  "reason": "Need architectural decision on database choice",
  "returned_to": "agent-456"
}
```

**Fields:**
- `bead_id` (required): Bead to escalate
- `reason` (optional): Escalation reason
- `returned_to` (optional): Agent to return decision to

**Returns:**
- `decision_id`: Created decision bead identifier

#### approve_bead

Approve a bead in a workflow.

```json
{
  "type": "approve_bead",
  "bead_id": "bead-abc-123",
  "reason": "Code review passed, tests are comprehensive"
}
```

**Fields:**
- `bead_id` (required): Bead to approve
- `reason` (optional): Approval reason

**Returns:**
- `bead_id`: Approved bead identifier

#### reject_bead

Reject a bead in a workflow.

```json
{
  "type": "reject_bead",
  "bead_id": "bead-abc-123",
  "reason": "Missing error handling in new code"
}
```

**Fields:**
- `bead_id` (required): Bead to reject
- `reason` (required): Rejection reason

**Returns:**
- `bead_id`: Rejected bead identifier
- `reason`: Rejection reason

### Communication

#### ask_followup

Ask the user a follow-up question.

```json
{
  "type": "ask_followup",
  "question": "Should I use PostgreSQL or MySQL for the database?"
}
```

**Fields:**
- `question` (required): Question to ask

**Note:** This creates a bead with the question for the user to answer.

## Action Results

All actions return a result with this structure:

```json
{
  "action_type": "run_tests",
  "status": "executed",
  "message": "tests executed",
  "metadata": {
    // action-specific result data
  }
}
```

**Status Values:**
- `executed`: Action completed successfully
- `error`: Action failed with error

## Multi-Action Patterns

### Sequential Actions

Execute multiple actions in order:

```json
{
  "actions": [
    {"type": "read_file", "path": "src/main.go"},
    {"type": "write_file", "path": "src/config.json", "content": "{}"},
    {"type": "run_command", "command": "go build"}
  ]
}
```

### Test-Driven Development

```json
{
  "actions": [
    {"type": "write_file", "path": "src/calculator_test.go", "content": "..."},
    {"type": "run_tests", "test_pattern": "TestAdd"},
    {"type": "write_file", "path": "src/calculator.go", "content": "..."},
    {"type": "run_tests", "test_pattern": "TestAdd"}
  ],
  "notes": "TDD: Write failing test, implement code, verify test passes"
}
```

### Debug and Fix

```json
{
  "actions": [
    {"type": "run_tests"},
    {"type": "read_file", "path": "src/auth.go"},
    {"type": "edit_code", "path": "src/auth.go", "patch": "..."},
    {"type": "run_tests", "test_pattern": "TestAuth"}
  ],
  "notes": "Fixed nil pointer dereference in authentication handler"
}
```

### Comprehensive Code Change

```json
{
  "actions": [
    {"type": "git_status"},
    {"type": "search_text", "query": "TODO", "path": "src"},
    {"type": "write_file", "path": "src/feature.go", "content": "..."},
    {"type": "run_tests"},
    {"type": "git_status"}
  ],
  "notes": "Implemented new feature with tests"
}
```

## Error Handling

If an action fails, the result will have `status: "error"`:

```json
{
  "action_type": "run_tests",
  "status": "error",
  "message": "test runner not configured"
}
```

**Common Errors:**

- `"file not found"`: File doesn't exist
- `"patch failed"`: Patch couldn't be applied
- `"test runner not configured"`: Test execution unavailable
- `"command execution failed"`: Shell command returned non-zero exit
- `"bead creator not configured"`: Bead operations unavailable

**Error Recovery:**

When tests fail, the agent should:
1. Analyze the test failure output
2. Read relevant source files
3. Make targeted fixes
4. Re-run tests to verify

Example:
```json
{
  "actions": [
    {"type": "run_tests"},
    // Test fails, agent analyzes output
    {"type": "read_file", "path": "src/parser.go"},
    {"type": "edit_code", "path": "src/parser.go", "patch": "..."},
    {"type": "run_tests", "test_pattern": "TestParser"}
  ],
  "notes": "Fixed parser to handle edge case"
}
```

## Best Practices

### 1. Always Run Tests After Code Changes

```json
{
  "actions": [
    {"type": "write_file", "path": "src/feature.go", "content": "..."},
    {"type": "run_tests"}
  ]
}
```

### 2. Use Specific Test Patterns When Debugging

Instead of:
```json
{"type": "run_tests"}
```

Use:
```json
{"type": "run_tests", "test_pattern": "TestSpecificFunction"}
```

### 3. Include Meaningful Notes

```json
{
  "actions": [...],
  "notes": "Fixed off-by-one error in pagination logic (lines 45-47)"
}
```

### 4. Check Git Status Before Major Changes

```json
{
  "actions": [
    {"type": "git_status"},
    {"type": "git_diff"},
    // ... make changes ...
  ]
}
```

### 5. Read Before Writing

```json
{
  "actions": [
    {"type": "read_file", "path": "src/config.go"},
    {"type": "edit_code", "path": "src/config.go", "patch": "..."}
  ]
}
```

### 6. Use Appropriate Timeouts for Tests

```json
{
  "type": "run_tests",
  "test_pattern": "TestIntegration",
  "timeout_seconds": 600
}
```

## Validation Rules

The action schema validates:

1. **Required Fields**: Each action type has required fields that must be present
2. **Unknown Fields**: Strict decoding rejects unknown fields
3. **Action Array**: At least one action must be present
4. **Type Values**: Action type must be one of the defined constants

## Related Documentation

- [Test Execution Design](TEST_EXECUTION_DESIGN.md) - Test runner architecture
- [Conversation Architecture](CONVERSATION_ARCHITECTURE.md) - Multi-turn workflows
- [Worker Integration](../internal/worker/README.md) - How actions are executed

## Implementation

The action schema is implemented in:

- `internal/actions/schema.go` - Action types and validation
- `internal/actions/router.go` - Action execution routing
- `internal/actions/testrunner_adapter.go` - Test execution integration

## Contributing

When adding new actions:

1. Add constant to `schema.go`
2. Add fields to `Action` struct if needed
3. Add validation in `validateAction()`
4. Add router case in `executeAction()`
5. Write unit and integration tests
6. Update this documentation

## License

See [LICENSE](../LICENSE) for details.
