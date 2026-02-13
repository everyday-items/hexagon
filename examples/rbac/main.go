// Package main 演示 Hexagon RBAC 角色权限控制
//
// RBAC 系统支持：
//   - 角色管理: 创建/删除角色，设置权限
//   - 用户管理: 创建用户，分配角色
//   - 权限检查: 基于角色的资源访问控制
//   - 角色层级: 角色继承机制
//
// 运行方式:
//
//	go run ./examples/rbac/
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/everyday-items/hexagon/security/rbac"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== 示例 1: 基本角色和权限 ===")
	runBasicRBAC(ctx)

	fmt.Println("\n=== 示例 2: 角色层级继承 ===")
	runRoleHierarchy(ctx)

	fmt.Println("\n=== 示例 3: 自定义角色 ===")
	runCustomRoles(ctx)
}

// runBasicRBAC 演示基本角色和权限控制
func runBasicRBAC(ctx context.Context) {
	r := rbac.NewRBAC()

	// 查看默认角色
	roles := r.ListRoles()
	fmt.Printf("  默认角色: %d 个\n", len(roles))
	for _, role := range roles {
		fmt.Printf("    - %s (%s): %d 个权限\n", role.Name, role.DisplayName, len(role.Permissions))
	}

	// 添加用户并分配角色
	err := r.AddUser(ctx, &rbac.User{
		ID:   "user-alice",
		Name: "Alice",
	})
	if err != nil {
		log.Fatalf("添加用户失败: %v", err)
	}

	err = r.AssignRole(ctx, "user-alice", "user")
	if err != nil {
		log.Fatalf("分配角色失败: %v", err)
	}
	fmt.Println("  已创建用户 Alice，角色: user")

	// 权限检查
	tests := []struct {
		resource, action string
	}{
		{"agent", "run"},
		{"agent", "read"},
		{"admin", "delete"},
		{"tool", "execute"},
	}

	for _, tt := range tests {
		result := r.Authorize(rbac.AccessRequest{
			Subject:  "user-alice",
			Resource: tt.resource,
			Action:   tt.action,
		})
		status := "允许"
		if !result.Allowed {
			status = "拒绝"
		}
		fmt.Printf("    %s.%s → %s (%s)\n", tt.resource, tt.action, status, result.Reason)
	}
}

// runRoleHierarchy 演示角色层级继承
func runRoleHierarchy(ctx context.Context) {
	r := rbac.NewRBAC()

	// admin 继承 user 和 guest 的权限
	inherited := r.GetInheritedRoles("admin")
	fmt.Printf("  admin 继承的角色: %v\n", inherited)

	// 添加 admin 用户
	r.AddUser(ctx, &rbac.User{ID: "user-admin", Name: "Admin"})
	r.AssignRole(ctx, "user-admin", "admin")

	// admin 可以访问所有资源（通配符 *）
	result := r.Authorize(rbac.AccessRequest{
		Subject:  "user-admin",
		Resource: "system",
		Action:   "shutdown",
	})
	fmt.Printf("  admin 访问 system.shutdown: %v\n", result.Allowed)

	// 添加 guest 用户
	r.AddUser(ctx, &rbac.User{ID: "user-guest", Name: "Guest"})
	r.AssignRole(ctx, "user-guest", "guest")

	// guest 只能读取
	readResult := r.Authorize(rbac.AccessRequest{
		Subject:  "user-guest",
		Resource: "agent",
		Action:   "read",
	})
	writeResult := r.Authorize(rbac.AccessRequest{
		Subject:  "user-guest",
		Resource: "agent",
		Action:   "run",
	})
	fmt.Printf("  guest 读取 agent: %v, 运行 agent: %v\n", readResult.Allowed, writeResult.Allowed)
}

// runCustomRoles 演示自定义角色
func runCustomRoles(ctx context.Context) {
	r := rbac.NewRBAC()

	// 创建数据分析师角色
	err := r.AddRole(ctx, &rbac.Role{
		Name:        "analyst",
		DisplayName: "数据分析师",
		Description: "可以查询和分析数据，但不能修改",
		Permissions: []rbac.Permission{
			{Resource: "data", Action: "read"},
			{Resource: "data", Action: "query"},
			{Resource: "report", Action: "create"},
			{Resource: "report", Action: "read"},
		},
	})
	if err != nil {
		log.Fatalf("创建角色失败: %v", err)
	}
	fmt.Println("  已创建角色: analyst")

	// 分配给用户
	r.AddUser(ctx, &rbac.User{ID: "user-bob", Name: "Bob"})
	r.AssignRole(ctx, "user-bob", "analyst")

	// 测试权限
	tests := []struct {
		resource, action string
	}{
		{"data", "read"},
		{"data", "query"},
		{"data", "write"},
		{"report", "create"},
		{"report", "delete"},
	}

	for _, tt := range tests {
		result := r.Authorize(rbac.AccessRequest{
			Subject:  "user-bob",
			Resource: tt.resource,
			Action:   tt.action,
		})
		status := "允许"
		if !result.Allowed {
			status = "拒绝"
		}
		fmt.Printf("    %s.%s → %s\n", tt.resource, tt.action, status)
	}
}
