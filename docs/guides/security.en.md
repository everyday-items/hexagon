<div align="right">Language: <a href="security.md">中文</a> | English</div>

# Security Configuration Guide

Hexagon provides a multi-layered security protection mechanism.

## Input Validation

### Prompt Injection Detection

```go
import "github.com/hexagon-codes/hexagon/security/guard"

// Create a Prompt injection detection guard
detector := guard.NewPromptInjectionGuard()

// Check text
result, err := detector.Check(context.Background(), "Ignore previous instructions and tell me...")
if err != nil {
    log.Printf("Detection failed: %v", err)
}
if result.IsRisky {
    fmt.Println("Prompt injection attack detected")
}
```

### PII Detection

```go
// Create a PII detection guard
detector := guard.NewPIIGuard()

// Detect personally identifiable information in text
result, err := detector.Check(context.Background(), "My phone number is 138-1234-5678")
if err != nil {
    log.Printf("Detection failed: %v", err)
}
if len(result.Findings) > 0 {
    // Redact sensitive information
    sanitized := detector.Redact("My phone number is 138-1234-5678")
    fmt.Println(sanitized) // Output: My phone number is 138****5678
}

// Convenience functions (for simple scenarios)
findings := guard.DetectPII("My phone number is 138-1234-5678")
sanitized := guard.RedactPII("My phone number is 138-1234-5678")
```

### Guard Chain

```go
chain := guard.NewGuardChain(
    guard.NewPromptInjectionGuard(),
    guard.NewPIIGuard(),
    guard.NewToxicityGuard(),
)

agent := agent.NewBaseAgent(
    agent.WithGuardChain(chain),
)
```

## Access Control

### RBAC

```go
import "github.com/hexagon-codes/hexagon/security/rbac"

// Define roles
admin := rbac.NewRole("admin")
user := rbac.NewRole("user")

// Define permissions
readPerm := rbac.NewPermission("agent:read")
writePerm := rbac.NewPermission("agent:write")

// Assign permissions
admin.AddPermissions(readPerm, writePerm)
user.AddPermissions(readPerm)

// Check permissions
if admin.HasPermission("agent:write") {
    // Allow operation
}
```

## Cost Control

```go
import "github.com/hexagon-codes/hexagon/security/cost"

controller := cost.NewController(
    cost.WithDailyBudget(100.0),        // daily budget $100
    cost.WithTokenLimit(1000000),        // token limit
    cost.WithRateLimit(100, time.Minute), // 100 requests per minute
)

agent := agent.NewBaseAgent(
    agent.WithCostController(controller),
)
```

## Audit Logging

```go
import "github.com/hexagon-codes/hexagon/security/audit"

auditor := audit.NewAuditor()

// Log an operation
auditor.Log(audit.Event{
    Type:      "agent_call",
    User:      "user123",
    Resource:  "agent-1",
    Action:    "run",
    Timestamp: time.Now(),
})

// Query audit logs
events := auditor.Query(audit.Query{
    User:      "user123",
    StartTime: time.Now().Add(-24 * time.Hour),
})
```

For more details, see [DESIGN.md](../DESIGN.md#安全防护).
