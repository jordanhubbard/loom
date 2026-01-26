# AgentiCorp Motivation System

The Motivation System is the core mechanism by which AgentiCorp proactively initiates work rather than waiting for external commands. Motivations are human-readable triggers that cause agents to wake and begin work when the system would otherwise be idle.

## Overview

Unlike traditional task queues where work is pushed externally, AgentiCorp's motivation system allows agents to be **pulled** into action by internal triggers. This creates a self-driving system that:

- Wakes agents when their expertise is needed
- Responds to calendar events (deadlines, quarters, months)
- Reacts to system state changes (idle detection, threshold breaches)
- Integrates with external events (GitHub webhooks, releases)

## Core Concepts

### Motivations

A **Motivation** is a trigger that can wake an agent or create work. Each motivation has:

| Field | Description |
|-------|-------------|
| `type` | Category: `calendar`, `event`, `threshold`, `idle`, `external` |
| `condition` | Specific trigger: `deadline_approach`, `system_idle`, etc. |
| `agent_role` | Which agent role should be triggered |
| `priority` | 0-100, higher = more important |
| `cooldown` | Minimum time between triggers |
| `create_bead` | Whether to create a stimulus bead |
| `wake_agent` | Whether to wake the agent directly |

### Motivation Types

#### Calendar Motivations
Time-based triggers driven by dates and schedules.

```
Conditions:
- deadline_approach   # Beads/milestones approaching due date
- deadline_passed     # Overdue items detected
- scheduled_interval  # Recurring schedule (hourly, daily, etc.)
- quarter_boundary    # Start of calendar quarter (Jan, Apr, Jul, Oct)
- month_boundary      # Start of calendar month
```

#### Event Motivations
System event-driven triggers.

```
Conditions:
- bead_created        # New bead created
- bead_status_changed # Bead status updated
- bead_completed      # Bead closed
- decision_pending    # Decision awaiting resolution
- decision_resolved   # Decision made
- release_published   # New release published
```

#### Threshold Motivations
Metric-based triggers when thresholds are crossed.

```
Conditions:
- cost_exceeded       # Spending over budget
- coverage_dropped    # Test coverage below threshold
- test_failure        # CI/CD test failures
- velocity_drop       # Team velocity decreased
```

#### Idle Motivations
Activity-based triggers for proactive work.

```
Conditions:
- system_idle         # Entire system idle (triggers CEO)
- agent_idle          # Specific agent idle
- project_idle        # Project has no active work
```

#### External Motivations
Triggers from external systems.

```
Conditions:
- github_issue_opened   # New GitHub issue
- github_comment_added  # Comment on issue/PR
- github_pr_opened      # New pull request
- webhook_received      # Generic webhook
```

## Default Motivations by Role

### CEO
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| System Idle - Strategic Review | idle | system_idle | 90 |
| Decision Pending - Executive Approval | event | decision_pending | 95 |
| Quarterly Business Review | calendar | quarter_boundary | 80 |

### CFO
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Budget Threshold Exceeded | threshold | cost_exceeded | 85 |
| Monthly Financial Review | calendar | month_boundary | 75 |
| System Idle - Cost Optimization | idle | system_idle | 50 |

### Project Manager
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Deadline Approaching | calendar | deadline_approach | 80 |
| Deadline Passed | calendar | deadline_passed | 90 |
| Velocity Drop Detected | threshold | velocity_drop | 70 |

### Engineering Manager
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Deadline Approaching - Technical | calendar | deadline_approach | 75 |
| Test Failure Detected | threshold | test_failure | 85 |
| Coverage Drop Detected | threshold | coverage_dropped | 60 |

### QA Engineer
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Bead Completed - QA Review | event | bead_completed | 70 |
| Release Approaching - QA Sweep | calendar | deadline_approach | 80 |
| Test Failure - Investigation | threshold | test_failure | 85 |

### Public Relations Manager
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Release Published - Announcement | event | release_published | 80 |
| GitHub Issue - Community Response | external | github_issue_opened | 60 |
| GitHub Comment - Community Engagement | external | github_comment_added | 50 |

### Product Manager
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Milestone Complete - Feature Review | event | bead_completed | 70 |
| Quarterly Planning | calendar | quarter_boundary | 75 |
| GitHub Issue - Feature Request Triage | external | github_issue_opened | 65 |

### DevOps Engineer
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Release Approaching - Infrastructure Prep | calendar | deadline_approach | 80 |
| Test Failure - Pipeline Investigation | threshold | test_failure | 90 |
| System Idle - Infrastructure Maintenance | idle | system_idle | 40 |

### Documentation Manager
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Feature Completed - Documentation Update | event | bead_completed | 60 |
| Release Approaching - Docs Review | calendar | deadline_approach | 70 |
| System Idle - Documentation Improvements | idle | system_idle | 30 |

### Code Reviewer
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Pull Request Opened - Code Review | external | github_pr_opened | 85 |
| Bead In Progress - Review Check | event | bead_status_changed | 50 |

### Housekeeping Bot
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| System Idle - Cleanup | idle | system_idle | 20 |
| Daily Maintenance | calendar | scheduled_interval | 25 |

### Decision Maker
| Motivation | Type | Condition | Priority |
|------------|------|-----------|----------|
| Decision Pending - Resolution | event | decision_pending | 85 |
| Project Idle - Decision Review | idle | project_idle | 60 |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Motivation Engine                         │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌───────────────┐    ┌───────────────┐    ┌─────────────┐ │
│  │   Registry    │───▶│   Evaluators  │───▶│   Actions   │ │
│  │ (Motivations) │    │ (5 types)     │    │ (Wake/Bead) │ │
│  └───────────────┘    └───────────────┘    └─────────────┘ │
│         │                    │                     │        │
│         ▼                    ▼                     ▼        │
│  ┌───────────────┐    ┌───────────────┐    ┌─────────────┐ │
│  │  Cooldowns &  │    │ State Provider│    │  Event Bus  │ │
│  │   History     │    │  (System Data)│    │ (Pub/Sub)   │ │
│  └───────────────┘    └───────────────┘    └─────────────┘ │
│                                                              │
└─────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│              Temporal Orchestration                          │
├─────────────────────────────────────────────────────────────┤
│  Heartbeat Workflow ──▶ Motivation Activity ──▶ Agent Wake  │
└─────────────────────────────────────────────────────────────┘
```

### Components

1. **Registry** (`internal/motivation/registry.go`)
   - Stores all motivation definitions
   - Tracks cooldowns and trigger history
   - Indexes motivations by role and project

2. **Engine** (`internal/motivation/engine.go`)
   - Evaluates motivations on each tick
   - Fires motivations when conditions are met
   - Respects cooldown periods

3. **Evaluators** (`internal/motivation/evaluators.go`)
   - CalendarEvaluator: Time-based conditions
   - EventEvaluator: System event conditions
   - ThresholdEvaluator: Metric-based conditions
   - IdleEvaluator: Activity-based conditions
   - ExternalEvaluator: External event conditions

4. **Idle Detector** (`internal/motivation/idle_detector.go`)
   - Monitors system activity
   - Detects idle states for system, projects, and agents
   - Configurable thresholds

5. **Temporal Integration** (`internal/temporal/activities/motivation.go`)
   - Activity for heartbeat workflow integration
   - Publishes motivation events to event bus

## API Endpoints

### List Motivations
```http
GET /api/v1/motivations
GET /api/v1/motivations?agent_role=ceo
GET /api/v1/motivations?type=idle
GET /api/v1/motivations?active=true
```

### Get Motivation
```http
GET /api/v1/motivations/{id}
```

### Create Motivation
```http
POST /api/v1/motivations
Content-Type: application/json

{
  "name": "Custom Trigger",
  "type": "calendar",
  "condition": "scheduled_interval",
  "agent_role": "housekeeping-bot",
  "cooldown_minutes": 60,
  "priority": 50,
  "wake_agent": true,
  "parameters": {
    "interval": "1h"
  }
}
```

### Update Motivation
```http
PUT /api/v1/motivations/{id}
Content-Type: application/json

{
  "priority": 75,
  "enabled": true
}
```

### Delete Motivation
```http
DELETE /api/v1/motivations/{id}
```

Note: Built-in motivations cannot be deleted.

### Enable/Disable Motivation
```http
POST /api/v1/motivations/{id}/enable
POST /api/v1/motivations/{id}/disable
```

### Manual Trigger
```http
POST /api/v1/motivations/{id}/trigger
```

### Trigger History
```http
GET /api/v1/motivations/history
GET /api/v1/motivations/history?limit=50
```

### Idle State
```http
GET /api/v1/motivations/idle
```

### List Roles and Their Motivations
```http
GET /api/v1/motivations/roles
```

### Register Defaults
```http
POST /api/v1/motivations/defaults
```

## Temporal DSL Integration

Agents can register motivations via the Temporal DSL:

```markdown
<temporal>
MOTIVATION: CustomDeadlineAlert
  TYPE: calendar
  CONDITION: deadline_approach
  AGENT_ROLE: project-manager
  PRIORITY: 80
  COOLDOWN: 120
  WAKE_AGENT: true
  PARAMETERS: {"days_threshold": 3}
END
</temporal>
```

## Configuration

Default configuration (`internal/motivation/types.go`):

```go
EvaluationInterval: 30 * time.Second  // How often to check
DefaultCooldown:    5 * time.Minute   // Default cooldown
MaxTriggersPerTick: 10                // Max triggers per cycle
IdleThreshold:      30 * time.Minute  // System idle threshold
EnabledByDefault:   true              // New motivations active
```

Idle thresholds (`internal/motivation/idle_detector.go`):

```go
SystemIdleThreshold:  30 * time.Minute  // Wake CEO
ProjectIdleThreshold: 15 * time.Minute  // Project review
AgentIdleThreshold:   5 * time.Minute   // Agent available
```

## Database Schema

The motivation system uses three tables:

```sql
-- Motivation definitions
CREATE TABLE motivations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  condition TEXT NOT NULL,
  status TEXT DEFAULT 'active',
  agent_role TEXT,
  cooldown_period_ns INTEGER,
  priority INTEGER DEFAULT 50,
  ...
);

-- Trigger history
CREATE TABLE motivation_triggers (
  id TEXT PRIMARY KEY,
  motivation_id TEXT NOT NULL,
  triggered_at DATETIME,
  result TEXT,
  ...
);

-- Milestones for deadline tracking
CREATE TABLE milestones (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  due_date DATETIME NOT NULL,
  ...
);
```

## Best Practices

1. **Set Appropriate Cooldowns**: Prevent trigger storms by setting reasonable cooldowns.

2. **Use Priority Wisely**: Higher priority motivations fire first when multiple trigger simultaneously.

3. **Consider Dependencies**: Some motivations (GitHub-based) require webhook integration.

4. **Test with Manual Triggers**: Use the API to manually trigger motivations during development.

5. **Monitor Trigger History**: Review `/api/v1/motivations/history` to understand firing patterns.

## Troubleshooting

### Motivation Not Firing

1. Check if motivation is enabled: `GET /api/v1/motivations/{id}`
2. Check cooldown status (may be in cooldown)
3. Verify condition is being met (check state provider)
4. Review trigger history for errors

### Too Many Triggers

1. Increase cooldown period
2. Reduce max triggers per tick in config
3. Disable low-priority motivations

### Agent Not Waking

1. Verify `wake_agent: true` on motivation
2. Check agent exists and is assigned to correct role
3. Verify agent is not already working
