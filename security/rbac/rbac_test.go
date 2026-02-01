package rbac

import (
	"context"
	"testing"
)

func TestNewRBAC(t *testing.T) {
	rbac := NewRBAC()

	if rbac == nil {
		t.Fatal("expected non-nil RBAC")
	}

	// Check default roles
	roles := rbac.ListRoles()
	if len(roles) != 4 {
		t.Errorf("expected 4 default roles, got %d", len(roles))
	}
}

func TestRBACDefaultRoles(t *testing.T) {
	rbac := NewRBAC()

	expectedRoles := []string{"admin", "user", "guest", "agent"}
	for _, name := range expectedRoles {
		role, ok := rbac.GetRole(name)
		if !ok {
			t.Errorf("expected role '%s' to exist", name)
			continue
		}
		if role.Name != name {
			t.Errorf("expected role name '%s', got '%s'", name, role.Name)
		}
	}
}

func TestRBACAddRole(t *testing.T) {
	rbac := NewRBAC()

	role := &Role{
		Name:        "custom",
		DisplayName: "Custom Role",
		Description: "A custom test role",
		Permissions: []Permission{
			{Resource: "test", Action: "read"},
		},
	}

	err := rbac.AddRole(role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, ok := rbac.GetRole("custom")
	if !ok {
		t.Fatal("expected role to be retrievable")
	}

	if retrieved.DisplayName != "Custom Role" {
		t.Errorf("expected display name 'Custom Role', got '%s'", retrieved.DisplayName)
	}
}

func TestRBACAddRoleDuplicate(t *testing.T) {
	rbac := NewRBAC()

	role := &Role{
		Name: "admin", // Already exists
	}

	err := rbac.AddRole(role)
	if err == nil {
		t.Error("expected error for duplicate role")
	}
}

func TestRBACUpdateRole(t *testing.T) {
	rbac := NewRBAC()

	role, _ := rbac.GetRole("user")
	role.Description = "Updated description"

	err := rbac.UpdateRole(role)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, _ := rbac.GetRole("user")
	if retrieved.Description != "Updated description" {
		t.Errorf("expected updated description, got '%s'", retrieved.Description)
	}
}

func TestRBACDeleteRole(t *testing.T) {
	rbac := NewRBAC()

	// Add a custom role first
	rbac.AddRole(&Role{Name: "temp"})

	err := rbac.DeleteRole("temp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := rbac.GetRole("temp")
	if ok {
		t.Error("expected role to be deleted")
	}
}

func TestRBACAddUser(t *testing.T) {
	rbac := NewRBAC()

	user := &User{
		Name:  "Test User",
		Roles: []string{"user"},
	}

	err := rbac.AddUser(user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID == "" {
		t.Error("expected user ID to be generated")
	}

	if !user.Enabled {
		t.Error("expected user to be enabled by default")
	}
}

func TestRBACGetUser(t *testing.T) {
	rbac := NewRBAC()

	user := &User{ID: "user-123", Name: "Test"}
	rbac.AddUser(user)

	retrieved, ok := rbac.GetUser("user-123")
	if !ok {
		t.Fatal("expected user to be retrievable")
	}

	if retrieved.Name != "Test" {
		t.Errorf("expected name 'Test', got '%s'", retrieved.Name)
	}
}

func TestRBACDeleteUser(t *testing.T) {
	rbac := NewRBAC()

	user := &User{ID: "user-456", Name: "Delete Me"}
	rbac.AddUser(user)

	err := rbac.DeleteUser("user-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := rbac.GetUser("user-456")
	if ok {
		t.Error("expected user to be deleted")
	}
}

func TestRBACAssignRole(t *testing.T) {
	rbac := NewRBAC()

	user := &User{ID: "user-789", Name: "Test", Roles: []string{}}
	rbac.AddUser(user)

	err := rbac.AssignRole("user-789", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, _ := rbac.GetUser("user-789")
	if len(retrieved.Roles) != 1 || retrieved.Roles[0] != "user" {
		t.Error("expected 'user' role to be assigned")
	}
}

func TestRBACRevokeRole(t *testing.T) {
	rbac := NewRBAC()

	user := &User{ID: "user-abc", Name: "Test", Roles: []string{"user", "guest"}}
	rbac.AddUser(user)

	err := rbac.RevokeRole("user-abc", "guest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, _ := rbac.GetUser("user-abc")
	for _, role := range retrieved.Roles {
		if role == "guest" {
			t.Error("expected 'guest' role to be revoked")
		}
	}
}

func TestRBACAuthorize(t *testing.T) {
	rbac := NewRBAC()

	user := &User{ID: "test-user", Name: "Test", Roles: []string{"user"}}
	rbac.AddUser(user)

	// Test allowed action
	result := rbac.Authorize(AccessRequest{
		Subject:  "test-user",
		Resource: "agent",
		Action:   "run",
	})

	if !result.Allowed {
		t.Errorf("expected action to be allowed, reason: %s", result.Reason)
	}

	// Test denied action
	result = rbac.Authorize(AccessRequest{
		Subject:  "test-user",
		Resource: "admin",
		Action:   "delete",
	})

	if result.Allowed {
		t.Error("expected action to be denied")
	}
}

func TestRBACAuthorizeAdmin(t *testing.T) {
	rbac := NewRBAC()

	user := &User{ID: "admin-user", Name: "Admin", Roles: []string{"admin"}}
	rbac.AddUser(user)

	// Admin should have access to everything
	result := rbac.Authorize(AccessRequest{
		Subject:  "admin-user",
		Resource: "anything",
		Action:   "anything",
	})

	if !result.Allowed {
		t.Errorf("expected admin to be allowed, reason: %s", result.Reason)
	}
}

func TestRBACAuthorizeDisabledUser(t *testing.T) {
	rbac := NewRBAC()

	user := &User{ID: "disabled-user", Name: "Disabled", Roles: []string{"admin"}}
	rbac.AddUser(user)

	// Disable user
	user.Enabled = false
	rbac.UpdateUser(user)

	result := rbac.Authorize(AccessRequest{
		Subject:  "disabled-user",
		Resource: "agent",
		Action:   "run",
	})

	if result.Allowed {
		t.Error("expected disabled user to be denied")
	}

	if result.Reason != "user disabled" {
		t.Errorf("expected reason 'user disabled', got '%s'", result.Reason)
	}
}

func TestRBACRoleHierarchy(t *testing.T) {
	rbac := NewRBAC()

	inherited := rbac.GetInheritedRoles("admin")

	// Admin should inherit user, guest, agent
	expectedRoles := map[string]bool{
		"admin": true,
		"user":  true,
		"guest": true,
		"agent": true,
	}

	for _, role := range inherited {
		if !expectedRoles[role] {
			t.Errorf("unexpected inherited role: %s", role)
		}
		delete(expectedRoles, role)
	}

	if len(expectedRoles) > 0 {
		t.Errorf("missing expected roles: %v", expectedRoles)
	}
}

func TestRBACPolicy(t *testing.T) {
	rbac := NewRBAC()

	policy := &Policy{
		Name:        "test-policy",
		Effect:      EffectAllow,
		Subjects:    []string{"*"},
		Resources:   []string{"test-resource"},
		Actions:     []string{"read"},
		Description: "Test policy",
	}

	err := rbac.AddPolicy(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if policy.ID == "" {
		t.Error("expected policy ID to be generated")
	}

	retrieved, ok := rbac.GetPolicy(policy.ID)
	if !ok {
		t.Fatal("expected policy to be retrievable")
	}

	if retrieved.Name != "test-policy" {
		t.Errorf("expected name 'test-policy', got '%s'", retrieved.Name)
	}
}

func TestRBACDeletePolicy(t *testing.T) {
	rbac := NewRBAC()

	policy := &Policy{ID: "policy-123", Name: "test"}
	rbac.AddPolicy(policy)

	err := rbac.DeletePolicy("policy-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := rbac.GetPolicy("policy-123")
	if ok {
		t.Error("expected policy to be deleted")
	}
}

func TestContextWithUser(t *testing.T) {
	user := &User{ID: "ctx-user", Name: "Context User"}
	ctx := context.Background()

	ctx = ContextWithUser(ctx, user)
	retrieved := UserFromContext(ctx)

	if retrieved == nil {
		t.Fatal("expected user in context")
	}

	if retrieved.ID != "ctx-user" {
		t.Errorf("expected user ID 'ctx-user', got '%s'", retrieved.ID)
	}
}

func TestUserFromContextEmpty(t *testing.T) {
	ctx := context.Background()
	user := UserFromContext(ctx)

	if user != nil {
		t.Error("expected nil user from empty context")
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern  string
		value    string
		expected bool
	}{
		{"*", "anything", true},
		{"agent", "agent", true},
		{"agent", "other", false},
		{"agent:*", "agent:run", true},
		{"agent:*", "agent:read", true},
		{"agent:*", "other:run", false},
	}

	for _, tt := range tests {
		result := matchWildcard(tt.pattern, tt.value)
		if result != tt.expected {
			t.Errorf("matchWildcard(%s, %s) = %v, expected %v",
				tt.pattern, tt.value, result, tt.expected)
		}
	}
}

func TestMemoryRBACStore(t *testing.T) {
	store := NewMemoryRBACStore()
	ctx := context.Background()

	// Test Role operations
	role := &Role{Name: "test-role"}
	err := store.SaveRole(ctx, role)
	if err != nil {
		t.Fatalf("SaveRole failed: %v", err)
	}

	retrieved, err := store.GetRole(ctx, "test-role")
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	if retrieved.Name != "test-role" {
		t.Errorf("expected name 'test-role', got '%s'", retrieved.Name)
	}

	// Test User operations
	user := &User{ID: "user-1", Name: "Test"}
	err = store.SaveUser(ctx, user)
	if err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	retrievedUser, err := store.GetUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if retrievedUser.Name != "Test" {
		t.Errorf("expected name 'Test', got '%s'", retrievedUser.Name)
	}

	// Test Policy operations
	policy := &Policy{ID: "policy-1", Name: "Test Policy"}
	err = store.SavePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("SavePolicy failed: %v", err)
	}

	retrievedPolicy, err := store.GetPolicy(ctx, "policy-1")
	if err != nil {
		t.Fatalf("GetPolicy failed: %v", err)
	}
	if retrievedPolicy.Name != "Test Policy" {
		t.Errorf("expected name 'Test Policy', got '%s'", retrievedPolicy.Name)
	}
}

func TestPolicyEffect(t *testing.T) {
	if EffectAllow != "allow" {
		t.Errorf("expected 'allow', got '%s'", EffectAllow)
	}
	if EffectDeny != "deny" {
		t.Errorf("expected 'deny', got '%s'", EffectDeny)
	}
}

func TestConditionOperators(t *testing.T) {
	tests := []struct {
		op       ConditionOperator
		expected string
	}{
		{OpEquals, "equals"},
		{OpNotEquals, "not_equals"},
		{OpContains, "contains"},
		{OpStartsWith, "starts_with"},
		{OpEndsWith, "ends_with"},
		{OpMatches, "matches"},
		{OpIn, "in"},
		{OpNotIn, "not_in"},
		{OpGreaterThan, "greater_than"},
		{OpLessThan, "less_than"},
	}

	for _, tt := range tests {
		if string(tt.op) != tt.expected {
			t.Errorf("expected '%s', got '%s'", tt.expected, tt.op)
		}
	}
}
