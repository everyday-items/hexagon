// Package database 提供数据库查询工具
//
// 支持 SQL 数据库查询操作。
package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/everyday-items/ai-core/tool"
)

// DatabaseTool 数据库工具
type DatabaseTool struct {
	db       *sql.DB
	readOnly bool
}

// Option 数据库工具选项
type Option func(*DatabaseTool)

// WithReadOnly 设置为只读模式
func WithReadOnly(readOnly bool) Option {
	return func(t *DatabaseTool) {
		t.readOnly = readOnly
	}
}

// NewDatabaseTool 创建数据库工具
func NewDatabaseTool(db *sql.DB, opts ...Option) *DatabaseTool {
	t := &DatabaseTool{
		db:       db,
		readOnly: true, // 默认只读模式，安全第一
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// QueryInput 查询输入
type QueryInput struct {
	SQL    string `json:"sql" description:"SQL 查询语句"`
	Params []any  `json:"params,omitempty" description:"查询参数"`
}

// QueryOutput 查询输出
type QueryOutput struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Count   int      `json:"count"`
}

// Tools 返回数据库工具集合
func (t *DatabaseTool) Tools() []tool.Tool {
	tools := []tool.Tool{
		// 查询工具
		tool.NewFunc(
			"db_query",
			"执行 SQL 查询并返回结果",
			func(ctx context.Context, input QueryInput) (QueryOutput, error) {
				return t.query(ctx, input.SQL, input.Params...)
			},
		),
	}

	// 如果不是只读模式，添加执行工具
	if !t.readOnly {
		tools = append(tools, tool.NewFunc(
			"db_execute",
			"执行 SQL 语句 (INSERT/UPDATE/DELETE)",
			func(ctx context.Context, input QueryInput) (struct {
				RowsAffected int64 `json:"rows_affected"`
			}, error) {
				result, err := t.db.ExecContext(ctx, input.SQL, input.Params...)
				if err != nil {
					return struct {
						RowsAffected int64 `json:"rows_affected"`
					}{}, fmt.Errorf("执行 SQL 失败: %w", err)
				}

				affected, _ := result.RowsAffected()
				return struct {
					RowsAffected int64 `json:"rows_affected"`
				}{RowsAffected: affected}, nil
			},
		))
	}

	return tools
}

// query 执行查询
func (t *DatabaseTool) query(ctx context.Context, query string, args ...any) (QueryOutput, error) {
	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return QueryOutput{}, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return QueryOutput{}, fmt.Errorf("获取列名失败: %w", err)
	}

	// 读取数据
	var result [][]any
	for rows.Next() {
		// 创建接收器
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// 扫描行
		if err := rows.Scan(valuePtrs...); err != nil {
			return QueryOutput{}, fmt.Errorf("扫描行失败: %w", err)
		}

		// 转换为字符串
		row := make([]any, len(columns))
		for i, v := range values {
			if v == nil {
				row[i] = nil
			} else {
				// 简单转换，实际使用时可能需要更复杂的类型处理
				row[i] = v
			}
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return QueryOutput{}, fmt.Errorf("遍历结果失败: %w", err)
	}

	return QueryOutput{
		Columns: columns,
		Rows:    result,
		Count:   len(result),
	}, nil
}

// QuickQuery 快速查询辅助函数
func QuickQuery(ctx context.Context, db *sql.DB, query string, args ...any) (QueryOutput, error) {
	t := NewDatabaseTool(db)
	return t.query(ctx, query, args...)
}
