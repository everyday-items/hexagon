// Package loader 提供 RAG 系统的文档加载器
//
// 本文件实现额外的文档加载器：
//   - GitHubLoader: GitHub 仓库加载器
//   - YAMLLoader: YAML 文件加载器
//   - CompositeLoader: 组合加载器
//   - S3Loader: AWS S3 加载器
//   - DatabaseLoader: 数据库加载器
//   - NotionLoader: Notion 文档加载器
//   - SlackLoader: Slack 消息加载器
package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
	"github.com/everyday-items/hexagon/rag"
)

// ============== GitHubLoader ==============

// GitHubLoader GitHub 仓库加载器
type GitHubLoader struct {
	owner      string
	repo       string
	branch     string
	path       string   // 仓库内路径
	extensions []string // 文件扩展名过滤
	token      string   // GitHub token（可选）
	httpClient *http.Client
}

// GitHubOption GitHub 加载器选项
type GitHubOption func(*GitHubLoader)

// WithGitHubBranch 设置分支
func WithGitHubBranch(branch string) GitHubOption {
	return func(l *GitHubLoader) {
		l.branch = branch
	}
}

// WithGitHubPath 设置仓库内路径
func WithGitHubPath(path string) GitHubOption {
	return func(l *GitHubLoader) {
		l.path = path
	}
}

// WithGitHubExtensions 设置文件扩展名过滤
func WithGitHubExtensions(exts []string) GitHubOption {
	return func(l *GitHubLoader) {
		l.extensions = exts
	}
}

// WithGitHubToken 设置 GitHub token
func WithGitHubToken(token string) GitHubOption {
	return func(l *GitHubLoader) {
		l.token = token
	}
}

// NewGitHubLoader 创建 GitHub 加载器
func NewGitHubLoader(owner, repo string, opts ...GitHubOption) *GitHubLoader {
	l := &GitHubLoader{
		owner:      owner,
		repo:       repo,
		branch:     "main",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 GitHub 仓库内容
func (l *GitHubLoader) Load(ctx context.Context) ([]rag.Document, error) {
	// 获取仓库内容树
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1",
		l.owner, l.repo, l.branch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if l.token != "" {
		req.Header.Set("Authorization", "Bearer "+l.token)
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch repo tree: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status)
	}

	var treeResp struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"tree"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&treeResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// 过滤并加载文件
	var docs []rag.Document
	for _, item := range treeResp.Tree {
		if item.Type != "blob" {
			continue
		}

		// 路径过滤
		if l.path != "" && !strings.HasPrefix(item.Path, l.path) {
			continue
		}

		// 扩展名过滤
		if len(l.extensions) > 0 {
			ext := filepath.Ext(item.Path)
			matched := false
			for _, e := range l.extensions {
				if ext == e || ext == "."+e {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// 加载文件内容
		content, err := l.loadFile(ctx, item.Path)
		if err != nil {
			// 记录加载失败的文件（便于排查问题），继续处理其他文件
			fmt.Fprintf(os.Stderr, "[WARN] hexagon/rag/loader: 加载 GitHub 文件 %s 失败: %v\n", item.Path, err)
			continue
		}

		doc := rag.Document{
			ID:      util.GenerateID("doc"),
			Content: content,
			Source:  fmt.Sprintf("github.com/%s/%s/%s", l.owner, l.repo, item.Path),
			Metadata: map[string]any{
				"loader":    "github",
				"owner":     l.owner,
				"repo":      l.repo,
				"branch":    l.branch,
				"path":      item.Path,
				"file_name": filepath.Base(item.Path),
			},
			CreatedAt: time.Now(),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// loadFile 加载单个文件
func (l *GitHubLoader) loadFile(ctx context.Context, path string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		l.owner, l.repo, l.branch, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	if l.token != "" {
		req.Header.Set("Authorization", "Bearer "+l.token)
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch file failed: %s", resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// Name 返回加载器名称
func (l *GitHubLoader) Name() string {
	return "GitHubLoader"
}

var _ rag.Loader = (*GitHubLoader)(nil)

// ============== YAMLLoader ==============

// YAMLLoader YAML 文件加载器
type YAMLLoader struct {
	path       string
	contentKey string
	isMultiDoc bool // 是否为多文档 YAML
}

// YAMLOption YAML 加载器选项
type YAMLOption func(*YAMLLoader)

// WithYAMLContentKey 设置内容键
func WithYAMLContentKey(key string) YAMLOption {
	return func(l *YAMLLoader) {
		l.contentKey = key
	}
}

// WithMultiDoc 设置为多文档模式
func WithMultiDoc(multi bool) YAMLOption {
	return func(l *YAMLLoader) {
		l.isMultiDoc = multi
	}
}

// NewYAMLLoader 创建 YAML 加载器
func NewYAMLLoader(path string, opts ...YAMLOption) *YAMLLoader {
	l := &YAMLLoader{
		path:       path,
		contentKey: "content",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 YAML 文件
func (l *YAMLLoader) Load(ctx context.Context) ([]rag.Document, error) {
	content, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("read YAML file: %w", err)
	}

	// YAML 加载需要 yaml 库，这里简化处理
	// 将 YAML 内容直接作为文档
	doc := rag.Document{
		ID:      util.GenerateID("doc"),
		Content: string(content),
		Source:  l.path,
		Metadata: map[string]any{
			"loader":    "yaml",
			"file_path": l.path,
			"file_name": filepath.Base(l.path),
		},
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// Name 返回加载器名称
func (l *YAMLLoader) Name() string {
	return "YAMLLoader"
}

var _ rag.Loader = (*YAMLLoader)(nil)

// ============== CompositeLoader ==============

// CompositeLoader 组合加载器
// 可以组合多个加载器
type CompositeLoader struct {
	loaders []rag.Loader
}

// NewCompositeLoader 创建组合加载器
func NewCompositeLoader(loaders ...rag.Loader) *CompositeLoader {
	return &CompositeLoader{
		loaders: loaders,
	}
}

// Load 加载所有加载器的文档
func (l *CompositeLoader) Load(ctx context.Context) ([]rag.Document, error) {
	var allDocs []rag.Document
	for _, loader := range l.loaders {
		docs, err := loader.Load(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", loader.Name(), err)
		}
		allDocs = append(allDocs, docs...)
	}
	return allDocs, nil
}

// AddLoader 添加加载器
func (l *CompositeLoader) AddLoader(loader rag.Loader) {
	l.loaders = append(l.loaders, loader)
}

// Name 返回加载器名称
func (l *CompositeLoader) Name() string {
	return "CompositeLoader"
}

var _ rag.Loader = (*CompositeLoader)(nil)

// ============== S3Loader ==============

// S3Loader AWS S3 加载器
type S3Loader struct {
	bucket     string
	prefix     string
	region     string
	extensions []string
	// 实际使用时需要注入 S3 客户端
}

// S3Option S3 加载器选项
type S3Option func(*S3Loader)

// WithS3Prefix 设置前缀
func WithS3Prefix(prefix string) S3Option {
	return func(l *S3Loader) {
		l.prefix = prefix
	}
}

// WithS3Region 设置区域
func WithS3Region(region string) S3Option {
	return func(l *S3Loader) {
		l.region = region
	}
}

// WithS3Extensions 设置文件扩展名过滤
func WithS3Extensions(exts []string) S3Option {
	return func(l *S3Loader) {
		l.extensions = exts
	}
}

// NewS3Loader 创建 S3 加载器
func NewS3Loader(bucket string, opts ...S3Option) *S3Loader {
	l := &S3Loader{
		bucket: bucket,
		region: "us-east-1",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 加载 S3 对象
// 注意：这是一个占位实现，实际使用需要注入 AWS SDK
func (l *S3Loader) Load(ctx context.Context) ([]rag.Document, error) {
	// 占位实现 - 实际需要使用 aws-sdk-go
	return nil, fmt.Errorf("S3Loader requires AWS SDK integration; bucket: %s, prefix: %s", l.bucket, l.prefix)
}

// Name 返回加载器名称
func (l *S3Loader) Name() string {
	return "S3Loader"
}

var _ rag.Loader = (*S3Loader)(nil)

// ============== DatabaseLoader ==============

// DatabaseLoader 数据库加载器
type DatabaseLoader struct {
	driver       string
	dsn          string
	query        string
	contentCol   string
	metadataCols []string
}

// DatabaseOption 数据库加载器选项
type DatabaseOption func(*DatabaseLoader)

// WithDBQuery 设置查询语句
func WithDBQuery(query string) DatabaseOption {
	return func(l *DatabaseLoader) {
		l.query = query
	}
}

// WithDBContentColumn 设置内容列
func WithDBContentColumn(col string) DatabaseOption {
	return func(l *DatabaseLoader) {
		l.contentCol = col
	}
}

// WithDBMetadataColumns 设置元数据列
func WithDBMetadataColumns(cols []string) DatabaseOption {
	return func(l *DatabaseLoader) {
		l.metadataCols = cols
	}
}

// NewDatabaseLoader 创建数据库加载器
func NewDatabaseLoader(driver, dsn string, opts ...DatabaseOption) *DatabaseLoader {
	l := &DatabaseLoader{
		driver:     driver,
		dsn:        dsn,
		contentCol: "content",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 从数据库加载文档
// 注意：这是一个占位实现，实际使用需要注入数据库驱动
func (l *DatabaseLoader) Load(ctx context.Context) ([]rag.Document, error) {
	// 占位实现 - 实际需要使用 database/sql
	return nil, fmt.Errorf("DatabaseLoader requires database driver; driver: %s", l.driver)
}

// Name 返回加载器名称
func (l *DatabaseLoader) Name() string {
	return "DatabaseLoader"
}

var _ rag.Loader = (*DatabaseLoader)(nil)

// ============== NotionLoader ==============

// NotionLoader Notion 文档加载器
type NotionLoader struct {
	apiKey     string
	databaseID string
	pageID     string
	httpClient *http.Client
}

// NotionOption Notion 加载器选项
type NotionOption func(*NotionLoader)

// WithNotionDatabaseID 设置 Notion 数据库 ID
func WithNotionDatabaseID(id string) NotionOption {
	return func(l *NotionLoader) {
		l.databaseID = id
	}
}

// WithNotionPageID 设置 Notion 页面 ID
func WithNotionPageID(id string) NotionOption {
	return func(l *NotionLoader) {
		l.pageID = id
	}
}

// NewNotionLoader 创建 Notion 加载器
func NewNotionLoader(apiKey string, opts ...NotionOption) *NotionLoader {
	l := &NotionLoader{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 从 Notion 加载文档
func (l *NotionLoader) Load(ctx context.Context) ([]rag.Document, error) {
	if l.pageID != "" {
		return l.loadPage(ctx, l.pageID)
	}
	if l.databaseID != "" {
		return l.loadDatabase(ctx, l.databaseID)
	}
	return nil, fmt.Errorf("either pageID or databaseID must be specified")
}

// loadPage 加载单个页面
func (l *NotionLoader) loadPage(ctx context.Context, pageID string) ([]rag.Document, error) {
	url := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", pageID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Notion-Version", "2022-06-28")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Notion API error: %s", resp.Status)
	}

	var result struct {
		Results []struct {
			Type      string `json:"type"`
			Paragraph struct {
				RichText []struct {
					PlainText string `json:"plain_text"`
				} `json:"rich_text"`
			} `json:"paragraph"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var content strings.Builder
	for _, block := range result.Results {
		if block.Type == "paragraph" {
			for _, text := range block.Paragraph.RichText {
				content.WriteString(text.PlainText)
			}
			content.WriteString("\n")
		}
	}

	doc := rag.Document{
		ID:      util.GenerateID("doc"),
		Content: content.String(),
		Source:  fmt.Sprintf("notion://page/%s", pageID),
		Metadata: map[string]any{
			"loader":  "notion",
			"page_id": pageID,
		},
		CreatedAt: time.Now(),
	}

	return []rag.Document{doc}, nil
}

// loadDatabase 加载数据库中的所有页面
func (l *NotionLoader) loadDatabase(ctx context.Context, databaseID string) ([]rag.Document, error) {
	url := fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", databaseID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query database: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Notion API error: %s", resp.Status)
	}

	var result struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var docs []rag.Document
	for _, page := range result.Results {
		pageDocs, err := l.loadPage(ctx, page.ID)
		if err != nil {
			continue // 跳过失败的页面
		}
		docs = append(docs, pageDocs...)
	}

	return docs, nil
}

// Name 返回加载器名称
func (l *NotionLoader) Name() string {
	return "NotionLoader"
}

var _ rag.Loader = (*NotionLoader)(nil)

// ============== SlackLoader ==============

// SlackLoader Slack 消息加载器
type SlackLoader struct {
	token      string
	channelID  string
	limit      int
	httpClient *http.Client
}

// SlackOption Slack 加载器选项
type SlackOption func(*SlackLoader)

// WithSlackChannelID 设置频道 ID
func WithSlackChannelID(id string) SlackOption {
	return func(l *SlackLoader) {
		l.channelID = id
	}
}

// WithSlackLimit 设置消息数量限制
func WithSlackLimit(limit int) SlackOption {
	return func(l *SlackLoader) {
		l.limit = limit
	}
}

// NewSlackLoader 创建 Slack 加载器
func NewSlackLoader(token string, opts ...SlackOption) *SlackLoader {
	l := &SlackLoader{
		token:      token,
		limit:      100,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load 从 Slack 加载消息
func (l *SlackLoader) Load(ctx context.Context) ([]rag.Document, error) {
	if l.channelID == "" {
		return nil, fmt.Errorf("channelID must be specified")
	}

	url := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&limit=%d",
		l.channelID, l.limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+l.token)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK       bool `json:"ok"`
		Messages []struct {
			Text string `json:"text"`
			User string `json:"user"`
			TS   string `json:"ts"`
		} `json:"messages"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("Slack API error: %s", result.Error)
	}

	var docs []rag.Document
	for _, msg := range result.Messages {
		doc := rag.Document{
			ID:      util.GenerateID("doc"),
			Content: msg.Text,
			Source:  fmt.Sprintf("slack://%s/%s", l.channelID, msg.TS),
			Metadata: map[string]any{
				"loader":     "slack",
				"channel_id": l.channelID,
				"user":       msg.User,
				"timestamp":  msg.TS,
			},
			CreatedAt: time.Now(),
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// Name 返回加载器名称
func (l *SlackLoader) Name() string {
	return "SlackLoader"
}

var _ rag.Loader = (*SlackLoader)(nil)
