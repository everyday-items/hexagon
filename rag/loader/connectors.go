// Package loader 提供 RAG 文档加载功能
//
// 本文件实现数据连接器：
//   - GitHub: 加载 GitHub 仓库、Issues、PR
//   - Notion: 加载 Notion 页面和数据库
//   - Slack: 加载 Slack 消息和频道
//   - Database: 加载 SQL 数据库内容
//
// 设计借鉴：
//   - LlamaIndex: Data Connectors
//   - LangChain: Document Loaders
package loader

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/rag"
)

// Document 是 rag.Document 的别名
type Document = rag.Document

// ============== 错误定义 ==============

var (
	// ErrConnectorFailed 连接器失败
	ErrConnectorFailed = errors.New("connector failed")

	// ErrAuthFailed 认证失败
	ErrAuthFailed = errors.New("authentication failed")

	// ErrRateLimited 被限流
	ErrRateLimited = errors.New("rate limited")

	// ErrNotFound 未找到
	ErrNotFound = errors.New("not found")
)

// ============== 连接器接口 ==============

// Connector 数据连接器接口
type Connector interface {
	// Name 连接器名称
	Name() string

	// Load 加载文档
	Load(ctx context.Context) ([]*Document, error)
}

// ============== GitHub 连接器 ==============

// GitHubConnector GitHub 数据连接器
type GitHubConnector struct {
	token     string
	owner     string
	repo      string
	branch    string
	path      string
	loadType  GitHubLoadType
	client    *http.Client
}

// GitHubLoadType GitHub 加载类型
type GitHubLoadType int

const (
	// GitHubLoadFiles 加载文件
	GitHubLoadFiles GitHubLoadType = iota
	// GitHubLoadIssues 加载 Issues
	GitHubLoadIssues
	// GitHubLoadPRs 加载 Pull Requests
	GitHubLoadPRs
	// GitHubLoadDiscussions 加载 Discussions
	GitHubLoadDiscussions
)

// GitHubConfig GitHub 连接器配置
type GitHubConfig struct {
	// Token GitHub 访问令牌
	Token string

	// Owner 仓库所有者
	Owner string

	// Repo 仓库名称
	Repo string

	// Branch 分支名称
	Branch string

	// Path 文件路径（可选）
	Path string

	// LoadType 加载类型
	LoadType GitHubLoadType

	// FileExtensions 文件扩展名过滤（仅 LoadFiles）
	FileExtensions []string

	// IssueState Issue 状态过滤
	IssueState string // "open", "closed", "all"

	// MaxItems 最大项数
	MaxItems int
}

// NewGitHubConnector 创建 GitHub 连接器
func NewGitHubConnector(config *GitHubConfig) *GitHubConnector {
	branch := config.Branch
	if branch == "" {
		branch = "main"
	}

	return &GitHubConnector{
		token:    config.Token,
		owner:    config.Owner,
		repo:     config.Repo,
		branch:   branch,
		path:     config.Path,
		loadType: config.LoadType,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 返回连接器名称
func (gc *GitHubConnector) Name() string {
	return "github"
}

// Load 加载 GitHub 内容
func (gc *GitHubConnector) Load(ctx context.Context) ([]*Document, error) {
	switch gc.loadType {
	case GitHubLoadFiles:
		return gc.loadFiles(ctx)
	case GitHubLoadIssues:
		return gc.loadIssues(ctx)
	case GitHubLoadPRs:
		return gc.loadPRs(ctx)
	default:
		return gc.loadFiles(ctx)
	}
}

func (gc *GitHubConnector) loadFiles(ctx context.Context) ([]*Document, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		gc.owner, gc.repo, gc.path, gc.branch)

	body, err := gc.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var items []struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Type        string `json:"type"`
		DownloadURL string `json:"download_url"`
		Size        int    `json:"size"`
	}

	if err := json.Unmarshal(body, &items); err != nil {
		// 可能是单个文件
		var item struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Content     string `json:"content"`
			Encoding    string `json:"encoding"`
			DownloadURL string `json:"download_url"`
		}
		if err := json.Unmarshal(body, &item); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
		}

		content, err := gc.fetchContent(ctx, item.DownloadURL)
		if err != nil {
			return nil, err
		}

		return []*Document{{
			ID:      item.Path,
			Content: content,
			Metadata: map[string]any{
				"source": "github",
				"owner":  gc.owner,
				"repo":   gc.repo,
				"path":   item.Path,
				"branch": gc.branch,
			},
		}}, nil
	}

	var docs []*Document
	for _, item := range items {
		if item.Type != "file" {
			continue
		}

		content, err := gc.fetchContent(ctx, item.DownloadURL)
		if err != nil {
			continue
		}

		docs = append(docs, &Document{
			ID:      item.Path,
			Content: content,
			Metadata: map[string]any{
				"source": "github",
				"owner":  gc.owner,
				"repo":   gc.repo,
				"path":   item.Path,
				"branch": gc.branch,
				"size":   item.Size,
			},
		})
	}

	return docs, nil
}

func (gc *GitHubConnector) loadIssues(ctx context.Context) ([]*Document, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=all&per_page=100",
		gc.owner, gc.repo)

	body, err := gc.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var issues []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		State     string `json:"state"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Labels    []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}

	if err := json.Unmarshal(body, &issues); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}

	var docs []*Document
	for _, issue := range issues {
		content := fmt.Sprintf("# %s\n\n%s", issue.Title, issue.Body)

		labels := make([]string, len(issue.Labels))
		for i, l := range issue.Labels {
			labels[i] = l.Name
		}

		docs = append(docs, &Document{
			ID:      fmt.Sprintf("issue-%d", issue.Number),
			Content: content,
			Metadata: map[string]any{
				"source":     "github",
				"type":       "issue",
				"owner":      gc.owner,
				"repo":       gc.repo,
				"number":     issue.Number,
				"title":      issue.Title,
				"state":      issue.State,
				"author":     issue.User.Login,
				"labels":     labels,
				"created_at": issue.CreatedAt,
				"updated_at": issue.UpdatedAt,
			},
		})
	}

	return docs, nil
}

func (gc *GitHubConnector) loadPRs(ctx context.Context) ([]*Document, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=all&per_page=100",
		gc.owner, gc.repo)

	body, err := gc.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var prs []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		MergedAt  string `json:"merged_at"`
	}

	if err := json.Unmarshal(body, &prs); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}

	var docs []*Document
	for _, pr := range prs {
		content := fmt.Sprintf("# %s\n\n%s", pr.Title, pr.Body)

		docs = append(docs, &Document{
			ID:      fmt.Sprintf("pr-%d", pr.Number),
			Content: content,
			Metadata: map[string]any{
				"source":     "github",
				"type":       "pull_request",
				"owner":      gc.owner,
				"repo":       gc.repo,
				"number":     pr.Number,
				"title":      pr.Title,
				"state":      pr.State,
				"author":     pr.User.Login,
				"created_at": pr.CreatedAt,
				"updated_at": pr.UpdatedAt,
				"merged_at":  pr.MergedAt,
			},
		})
	}

	return docs, nil
}

func (gc *GitHubConnector) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if gc.token != "" {
		req.Header.Set("Authorization", "token "+gc.token)
	}

	resp, err := gc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, ErrAuthFailed
	}
	if resp.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status %d", ErrConnectorFailed, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (gc *GitHubConnector) fetchContent(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	if gc.token != "" {
		req.Header.Set("Authorization", "token "+gc.token)
	}

	resp, err := gc.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// ============== Notion 连接器 ==============

// NotionConnector Notion 数据连接器
type NotionConnector struct {
	token    string
	pageID   string
	client   *http.Client
}

// NotionConfig Notion 连接器配置
type NotionConfig struct {
	// Token Notion 集成令牌
	Token string

	// PageID 页面 ID（可选）
	PageID string

	// DatabaseID 数据库 ID（可选）
	DatabaseID string
}

// NewNotionConnector 创建 Notion 连接器
func NewNotionConnector(config *NotionConfig) *NotionConnector {
	return &NotionConnector{
		token:  config.Token,
		pageID: config.PageID,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 返回连接器名称
func (nc *NotionConnector) Name() string {
	return "notion"
}

// Load 加载 Notion 内容
func (nc *NotionConnector) Load(ctx context.Context) ([]*Document, error) {
	if nc.pageID != "" {
		return nc.loadPage(ctx, nc.pageID)
	}
	return nil, fmt.Errorf("%w: no page or database ID specified", ErrConnectorFailed)
}

func (nc *NotionConnector) loadPage(ctx context.Context, pageID string) ([]*Document, error) {
	// 获取页面块内容
	url := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", pageID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+nc.token)
	req.Header.Set("Notion-Version", "2022-06-28")

	resp, err := nc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, ErrAuthFailed
	}
	if resp.StatusCode == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status %d", ErrConnectorFailed, resp.StatusCode)
	}

	var result struct {
		Results []struct {
			Type      string `json:"type"`
			Paragraph struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"paragraph"`
			Heading1 struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"heading_1"`
			Heading2 struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"heading_2"`
			Heading3 struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"heading_3"`
			BulletedListItem struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"bulleted_list_item"`
			NumberedListItem struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"numbered_list_item"`
		} `json:"results"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}

	// 构建内容
	var content strings.Builder
	for _, block := range result.Results {
		switch block.Type {
		case "paragraph":
			for _, rt := range block.Paragraph.RichText {
				content.WriteString(rt.PlainText)
			}
			content.WriteString("\n\n")
		case "heading_1":
			content.WriteString("# ")
			for _, rt := range block.Heading1.RichText {
				content.WriteString(rt.PlainText)
			}
			content.WriteString("\n\n")
		case "heading_2":
			content.WriteString("## ")
			for _, rt := range block.Heading2.RichText {
				content.WriteString(rt.PlainText)
			}
			content.WriteString("\n\n")
		case "heading_3":
			content.WriteString("### ")
			for _, rt := range block.Heading3.RichText {
				content.WriteString(rt.PlainText)
			}
			content.WriteString("\n\n")
		case "bulleted_list_item":
			content.WriteString("- ")
			for _, rt := range block.BulletedListItem.RichText {
				content.WriteString(rt.PlainText)
			}
			content.WriteString("\n")
		case "numbered_list_item":
			content.WriteString("1. ")
			for _, rt := range block.NumberedListItem.RichText {
				content.WriteString(rt.PlainText)
			}
			content.WriteString("\n")
		}
	}

	return []*Document{{
		ID:      pageID,
		Content: content.String(),
		Metadata: map[string]any{
			"source":  "notion",
			"page_id": pageID,
		},
	}}, nil
}

// ============== Slack 连接器 ==============

// SlackConnector Slack 数据连接器
type SlackConnector struct {
	token     string
	channelID string
	client    *http.Client
}

// SlackConfig Slack 连接器配置
type SlackConfig struct {
	// Token Slack Bot Token
	Token string

	// ChannelID 频道 ID
	ChannelID string

	// Limit 消息数量限制
	Limit int
}

// NewSlackConnector 创建 Slack 连接器
func NewSlackConnector(config *SlackConfig) *SlackConnector {
	return &SlackConnector{
		token:     config.Token,
		channelID: config.ChannelID,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 返回连接器名称
func (sc *SlackConnector) Name() string {
	return "slack"
}

// Load 加载 Slack 消息
func (sc *SlackConnector) Load(ctx context.Context) ([]*Document, error) {
	url := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&limit=100",
		sc.channelID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+sc.token)

	resp, err := sc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}
	defer resp.Body.Close()

	var result struct {
		OK       bool `json:"ok"`
		Messages []struct {
			Type   string `json:"type"`
			User   string `json:"user"`
			Text   string `json:"text"`
			TS     string `json:"ts"`
		} `json:"messages"`
		Error string `json:"error"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}

	if !result.OK {
		if result.Error == "invalid_auth" {
			return nil, ErrAuthFailed
		}
		return nil, fmt.Errorf("%w: %s", ErrConnectorFailed, result.Error)
	}

	var docs []*Document
	for _, msg := range result.Messages {
		docs = append(docs, &Document{
			ID:      msg.TS,
			Content: msg.Text,
			Metadata: map[string]any{
				"source":     "slack",
				"channel_id": sc.channelID,
				"user":       msg.User,
				"timestamp":  msg.TS,
			},
		})
	}

	return docs, nil
}

// ============== SQL 数据库连接器 ==============

// DatabaseConnector SQL 数据库连接器
type DatabaseConnector struct {
	db       *sql.DB
	query    string
	columns  []string
	template string
}

// DatabaseConfig 数据库连接器配置
type DatabaseConfig struct {
	// DB 数据库连接
	DB *sql.DB

	// Query SQL 查询
	Query string

	// Columns 要提取的列
	Columns []string

	// Template 文档模板（使用 {{.ColumnName}} 语法）
	Template string
}

// NewDatabaseConnector 创建数据库连接器
func NewDatabaseConnector(config *DatabaseConfig) *DatabaseConnector {
	return &DatabaseConnector{
		db:       config.DB,
		query:    config.Query,
		columns:  config.Columns,
		template: config.Template,
	}
}

// Name 返回连接器名称
func (dc *DatabaseConnector) Name() string {
	return "database"
}

// Load 加载数据库内容
func (dc *DatabaseConnector) Load(ctx context.Context) ([]*Document, error) {
	if dc.db == nil {
		return nil, fmt.Errorf("%w: database connection is nil", ErrConnectorFailed)
	}

	rows, err := dc.db.QueryContext(ctx, dc.query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}

	var docs []*Document
	rowNum := 0

	for rows.Next() {
		// 创建值容器
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// 构建行数据
		rowData := make(map[string]any)
		for i, col := range columns {
			rowData[col] = values[i]
		}

		// 构建文档内容
		var content string
		if dc.template != "" {
			content = dc.applyTemplate(dc.template, rowData)
		} else {
			var parts []string
			for _, col := range columns {
				parts = append(parts, fmt.Sprintf("%s: %v", col, rowData[col]))
			}
			content = strings.Join(parts, "\n")
		}

		docs = append(docs, &Document{
			ID:      fmt.Sprintf("row-%d", rowNum),
			Content: content,
			Metadata: map[string]any{
				"source":   "database",
				"row":      rowNum,
				"row_data": rowData,
			},
		})

		rowNum++
	}

	return docs, nil
}

func (dc *DatabaseConnector) applyTemplate(template string, data map[string]any) string {
	result := template
	for key, value := range data {
		placeholder := fmt.Sprintf("{{.%s}}", key)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}
	return result
}

// ============== Web API 连接器 ==============

// WebAPIConnector Web API 连接器
type WebAPIConnector struct {
	client   *http.Client
	url      string
	method   string
	headers  map[string]string
	body     string
	jsonPath string
}

// WebAPIConfig Web API 连接器配置
type WebAPIConfig struct {
	// URL API 端点
	URL string

	// Method HTTP 方法
	Method string

	// Headers 请求头
	Headers map[string]string

	// Body 请求体
	Body string

	// JSONPath JSON 路径（提取数组）
	JSONPath string
}

// NewWebAPIConnector 创建 Web API 连接器
func NewWebAPIConnector(config *WebAPIConfig) *WebAPIConnector {
	method := config.Method
	if method == "" {
		method = "GET"
	}

	return &WebAPIConnector{
		client:   &http.Client{Timeout: 30 * time.Second},
		url:      config.URL,
		method:   strings.ToUpper(method),
		headers:  config.Headers,
		body:     config.Body,
		jsonPath: config.JSONPath,
	}
}

// Name 返回连接器名称
func (wc *WebAPIConnector) Name() string {
	return "web_api"
}

// Load 加载 API 数据
func (wc *WebAPIConnector) Load(ctx context.Context) ([]*Document, error) {
	var bodyReader io.Reader
	if wc.body != "" {
		bodyReader = strings.NewReader(wc.body)
	}

	req, err := http.NewRequestWithContext(ctx, wc.method, wc.url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}

	for key, value := range wc.headers {
		req.Header.Set(key, value)
	}

	resp, err := wc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, ErrAuthFailed
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status %d", ErrConnectorFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectorFailed, err)
	}

	// 解析 JSON
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		// 不是 JSON，直接返回文本
		return []*Document{{
			ID:      wc.url,
			Content: string(body),
			Metadata: map[string]any{
				"source": "web_api",
				"url":    wc.url,
			},
		}}, nil
	}

	// 提取数组数据
	items := wc.extractItems(data)

	var docs []*Document
	for i, item := range items {
		content, _ := json.MarshalIndent(item, "", "  ")
		docs = append(docs, &Document{
			ID:      fmt.Sprintf("%s-%d", wc.url, i),
			Content: string(content),
			Metadata: map[string]any{
				"source": "web_api",
				"url":    wc.url,
				"index":  i,
			},
		})
	}

	if len(docs) == 0 {
		// 返回整个响应
		content, _ := json.MarshalIndent(data, "", "  ")
		docs = append(docs, &Document{
			ID:      wc.url,
			Content: string(content),
			Metadata: map[string]any{
				"source": "web_api",
				"url":    wc.url,
			},
		})
	}

	return docs, nil
}

func (wc *WebAPIConnector) extractItems(data any) []any {
	// 如果是数组，直接返回
	if arr, ok := data.([]any); ok {
		return arr
	}

	// 如果指定了 JSON 路径，尝试提取
	if wc.jsonPath != "" {
		parts := strings.Split(wc.jsonPath, ".")
		current := data

		for _, part := range parts {
			if m, ok := current.(map[string]any); ok {
				current = m[part]
			} else {
				return nil
			}
		}

		if arr, ok := current.([]any); ok {
			return arr
		}
	}

	// 尝试常见的数组字段
	if m, ok := data.(map[string]any); ok {
		for _, key := range []string{"data", "items", "results", "records", "list"} {
			if arr, ok := m[key].([]any); ok {
				return arr
			}
		}
	}

	return nil
}
