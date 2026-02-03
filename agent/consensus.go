package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/everyday-items/hexagon/internal/util"
)

// ConsensusStrategy 共识策略
type ConsensusStrategy int

const (
	// ConsensusMajority 多数投票
	ConsensusMajority ConsensusStrategy = iota

	// ConsensusUnanimous 全票通过
	ConsensusUnanimous

	// ConsensusWeighted 加权投票
	ConsensusWeighted

	// ConsensusAverage 平均值（用于数值决策）
	ConsensusAverage

	// ConsensusBorda Borda 计数法（用于排序）
	ConsensusBorda

	// ConsensusFirst 采用第一个响应
	ConsensusFirst

	// ConsensusBest 采用最佳响应（根据评分）
	ConsensusBest
)

// String 返回策略名称
func (s ConsensusStrategy) String() string {
	switch s {
	case ConsensusMajority:
		return "majority"
	case ConsensusUnanimous:
		return "unanimous"
	case ConsensusWeighted:
		return "weighted"
	case ConsensusAverage:
		return "average"
	case ConsensusBorda:
		return "borda"
	case ConsensusFirst:
		return "first"
	case ConsensusBest:
		return "best"
	default:
		return "unknown"
	}
}

// Vote 投票
type Vote struct {
	// AgentID 投票 Agent ID
	AgentID string `json:"agent_id"`

	// AgentName 投票 Agent 名称
	AgentName string `json:"agent_name"`

	// Value 投票值
	Value any `json:"value"`

	// Weight 权重（用于加权投票）
	Weight float64 `json:"weight"`

	// Score 评分（用于最佳选择）
	Score float64 `json:"score"`

	// Reason 投票理由
	Reason string `json:"reason"`

	// Ranking 排序（用于 Borda 计数）
	Ranking []any `json:"ranking,omitempty"`

	// Timestamp 投票时间
	Timestamp time.Time `json:"timestamp"`

	// Metadata 元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ConsensusResult 共识结果
type ConsensusResult struct {
	// ID 结果 ID
	ID string `json:"id"`

	// Strategy 使用的策略
	Strategy ConsensusStrategy `json:"strategy"`

	// Decision 最终决策
	Decision any `json:"decision"`

	// Confidence 置信度（0-1）
	Confidence float64 `json:"confidence"`

	// Votes 所有投票
	Votes []Vote `json:"votes"`

	// VoteCount 投票统计
	VoteCount map[string]int `json:"vote_count"`

	// Participation 参与率
	Participation float64 `json:"participation"`

	// Reached 是否达成共识
	Reached bool `json:"reached"`

	// Reason 结果说明
	Reason string `json:"reason"`

	// Duration 耗时
	Duration time.Duration `json:"duration"`

	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// ConsensusConfig 共识配置
type ConsensusConfig struct {
	// Strategy 共识策略
	Strategy ConsensusStrategy

	// Threshold 阈值（用于多数投票，默认 0.5）
	Threshold float64

	// Timeout 超时时间
	Timeout time.Duration

	// MinParticipation 最小参与率（默认 0.5）
	MinParticipation float64

	// Weights Agent 权重（用于加权投票）
	Weights map[string]float64

	// Scorer 评分函数（用于最佳选择）
	Scorer func(vote Vote) float64

	// Validator 投票验证函数
	Validator func(vote Vote) bool

	// AllowAbstain 允许弃权
	AllowAbstain bool
}

// DefaultConsensusConfig 返回默认配置
func DefaultConsensusConfig() ConsensusConfig {
	return ConsensusConfig{
		Strategy:         ConsensusMajority,
		Threshold:        0.5,
		Timeout:          30 * time.Second,
		MinParticipation: 0.5,
		Weights:          make(map[string]float64),
		AllowAbstain:     true,
	}
}

// ConsensusProtocol 共识协议
type ConsensusProtocol struct {
	// Config 配置
	config ConsensusConfig

	// Network 所属网络
	network *AgentNetwork

	// ActivePolls 进行中的投票
	activePolls sync.Map // pollID -> *Poll
}

// Poll 投票会话
type Poll struct {
	// ID 投票 ID
	ID string

	// Question 问题
	Question string

	// Options 选项（可选）
	Options []any

	// Voters 投票者列表
	Voters []string

	// Votes 收到的投票
	Votes []Vote

	// StartedAt 开始时间
	StartedAt time.Time

	// Closed 是否关闭
	Closed bool

	mu sync.Mutex
}

// NewConsensusProtocol 创建共识协议
func NewConsensusProtocol(network *AgentNetwork, opts ...ConsensusOption) *ConsensusProtocol {
	p := &ConsensusProtocol{
		config:  DefaultConsensusConfig(),
		network: network,
	}

	for _, opt := range opts {
		opt(&p.config)
	}

	return p
}

// ConsensusOption 共识配置选项
type ConsensusOption func(*ConsensusConfig)

// WithConsensusStrategy 设置共识策略
func WithConsensusStrategy(strategy ConsensusStrategy) ConsensusOption {
	return func(c *ConsensusConfig) {
		c.Strategy = strategy
	}
}

// WithConsensusThreshold 设置阈值
func WithConsensusThreshold(threshold float64) ConsensusOption {
	return func(c *ConsensusConfig) {
		c.Threshold = threshold
	}
}

// WithConsensusTimeout 设置超时
func WithConsensusTimeout(timeout time.Duration) ConsensusOption {
	return func(c *ConsensusConfig) {
		c.Timeout = timeout
	}
}

// WithMinParticipation 设置最小参与率
func WithMinParticipation(rate float64) ConsensusOption {
	return func(c *ConsensusConfig) {
		c.MinParticipation = rate
	}
}

// WithAgentWeights 设置 Agent 权重
func WithAgentWeights(weights map[string]float64) ConsensusOption {
	return func(c *ConsensusConfig) {
		c.Weights = weights
	}
}

// WithScorer 设置评分函数
func WithScorer(scorer func(vote Vote) float64) ConsensusOption {
	return func(c *ConsensusConfig) {
		c.Scorer = scorer
	}
}

// Propose 发起提案
func (p *ConsensusProtocol) Propose(ctx context.Context, question string, options []any) (*ConsensusResult, error) {
	return p.ProposeToAgents(ctx, question, options, nil)
}

// ProposeToAgents 向指定 Agent 发起提案
func (p *ConsensusProtocol) ProposeToAgents(ctx context.Context, question string, options []any, agentIDs []string) (*ConsensusResult, error) {
	startTime := time.Now()

	// 确定投票者
	voters := agentIDs
	if len(voters) == 0 {
		agents := p.network.ListOnlineAgents()
		voters = make([]string, len(agents))
		for i, a := range agents {
			voters[i] = a.ID()
		}
	}

	if len(voters) == 0 {
		return nil, fmt.Errorf("no voters available")
	}

	// 创建投票会话
	poll := &Poll{
		ID:        util.GenerateID("poll"),
		Question:  question,
		Options:   options,
		Voters:    voters,
		Votes:     make([]Vote, 0),
		StartedAt: startTime,
	}
	p.activePolls.Store(poll.ID, poll)
	defer p.activePolls.Delete(poll.ID)

	// 收集投票
	votes, err := p.collectVotes(ctx, poll)
	if err != nil {
		return nil, err
	}

	// 计算结果
	result := p.calculateResult(votes, len(voters), startTime)
	result.ID = poll.ID

	return result, nil
}

// collectVotes 收集投票
func (p *ConsensusProtocol) collectVotes(ctx context.Context, poll *Poll) ([]Vote, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	votes := make([]Vote, 0, len(poll.Voters))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, voterID := range poll.Voters {
		wg.Add(1)
		go func(agentID string) {
			defer wg.Done()

			vote, err := p.requestVote(ctx, poll, agentID)
			if err != nil {
				return
			}

			// 验证投票
			if p.config.Validator != nil && !p.config.Validator(*vote) {
				return
			}

			mu.Lock()
			votes = append(votes, *vote)
			mu.Unlock()
		}(voterID)
	}

	// 等待所有投票或超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	return votes, nil
}

// requestVote 请求投票
func (p *ConsensusProtocol) requestVote(ctx context.Context, poll *Poll, agentID string) (*Vote, error) {
	agent, ok := p.network.GetAgent(agentID)
	if !ok {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// 构建投票请求
	input := Input{
		Query: fmt.Sprintf("Please vote on the following question:\n\n%s\n\nOptions: %v\n\nRespond with your choice and reasoning.", poll.Question, poll.Options),
		Context: map[string]any{
			"poll_id":    poll.ID,
			"vote_type":  "consensus",
			"options":    poll.Options,
			"voter_role": agent.Role().Name,
		},
	}

	output, err := agent.Run(ctx, input)
	if err != nil {
		return nil, err
	}

	// 解析投票
	vote := &Vote{
		AgentID:   agentID,
		AgentName: agent.Name(),
		Value:     p.parseVoteValue(output.Content, poll.Options),
		Weight:    p.getWeight(agentID),
		Reason:    output.Content,
		Timestamp: time.Now(),
		Metadata:  output.Metadata,
	}

	// 如果有评分函数，计算分数
	if p.config.Scorer != nil {
		vote.Score = p.config.Scorer(*vote)
	}

	return vote, nil
}

// parseVoteValue 解析投票值
//
// 检查 LLM 返回的内容中是否包含任何选项，如果包含则返回该选项。
func (p *ConsensusProtocol) parseVoteValue(content string, options []any) any {
	// 简单实现：检查内容中是否包含选项（子字符串匹配）
	for _, opt := range options {
		optStr := fmt.Sprintf("%v", opt)
		if strings.Contains(content, optStr) {
			return opt
		}
	}

	// 默认返回内容本身
	return content
}

// getWeight 获取 Agent 权重
func (p *ConsensusProtocol) getWeight(agentID string) float64 {
	if weight, ok := p.config.Weights[agentID]; ok {
		return weight
	}
	return 1.0
}

// calculateResult 计算共识结果
func (p *ConsensusProtocol) calculateResult(votes []Vote, totalVoters int, startTime time.Time) *ConsensusResult {
	result := &ConsensusResult{
		Strategy:      p.config.Strategy,
		Votes:         votes,
		VoteCount:     make(map[string]int),
		Participation: float64(len(votes)) / float64(totalVoters),
		Timestamp:     time.Now(),
		Duration:      time.Since(startTime),
	}

	// 检查最小参与率
	if result.Participation < p.config.MinParticipation {
		result.Reached = false
		result.Reason = fmt.Sprintf("participation %.2f%% below minimum %.2f%%",
			result.Participation*100, p.config.MinParticipation*100)
		return result
	}

	// 根据策略计算结果
	switch p.config.Strategy {
	case ConsensusMajority:
		p.calculateMajority(result)
	case ConsensusUnanimous:
		p.calculateUnanimous(result)
	case ConsensusWeighted:
		p.calculateWeighted(result)
	case ConsensusAverage:
		p.calculateAverage(result)
	case ConsensusBorda:
		p.calculateBorda(result)
	case ConsensusFirst:
		p.calculateFirst(result)
	case ConsensusBest:
		p.calculateBest(result)
	}

	return result
}

// calculateMajority 多数投票
func (p *ConsensusProtocol) calculateMajority(result *ConsensusResult) {
	// 统计投票
	for _, vote := range result.Votes {
		key := fmt.Sprintf("%v", vote.Value)
		result.VoteCount[key]++
	}

	// 找出最高票
	var maxCount int
	var winner string
	for value, count := range result.VoteCount {
		if count > maxCount {
			maxCount = count
			winner = value
		}
	}

	// 检查是否达到阈值
	ratio := float64(maxCount) / float64(len(result.Votes))
	if ratio >= p.config.Threshold {
		result.Reached = true
		result.Decision = winner
		result.Confidence = ratio
		result.Reason = fmt.Sprintf("'%s' won with %.1f%% votes", winner, ratio*100)
	} else {
		result.Reached = false
		result.Reason = fmt.Sprintf("no option reached %.1f%% threshold", p.config.Threshold*100)
	}
}

// calculateUnanimous 全票通过
func (p *ConsensusProtocol) calculateUnanimous(result *ConsensusResult) {
	if len(result.Votes) == 0 {
		result.Reached = false
		result.Reason = "no votes received"
		return
	}

	firstValue := fmt.Sprintf("%v", result.Votes[0].Value)
	unanimous := true

	for _, vote := range result.Votes {
		key := fmt.Sprintf("%v", vote.Value)
		result.VoteCount[key]++
		if key != firstValue {
			unanimous = false
		}
	}

	if unanimous {
		result.Reached = true
		result.Decision = result.Votes[0].Value
		result.Confidence = 1.0
		result.Reason = "unanimous consensus reached"
	} else {
		result.Reached = false
		result.Reason = "votes are not unanimous"
	}
}

// calculateWeighted 加权投票
func (p *ConsensusProtocol) calculateWeighted(result *ConsensusResult) {
	weightedCounts := make(map[string]float64)
	totalWeight := 0.0

	for _, vote := range result.Votes {
		key := fmt.Sprintf("%v", vote.Value)
		weightedCounts[key] += vote.Weight
		totalWeight += vote.Weight
		result.VoteCount[key]++
	}

	// 找出最高加权票
	var maxWeight float64
	var winner string
	for value, weight := range weightedCounts {
		if weight > maxWeight {
			maxWeight = weight
			winner = value
		}
	}

	ratio := maxWeight / totalWeight
	if ratio >= p.config.Threshold {
		result.Reached = true
		result.Decision = winner
		result.Confidence = ratio
		result.Reason = fmt.Sprintf("'%s' won with %.1f%% weighted votes", winner, ratio*100)
	} else {
		result.Reached = false
		result.Reason = fmt.Sprintf("no option reached %.1f%% weighted threshold", p.config.Threshold*100)
	}
}

// calculateAverage 平均值
func (p *ConsensusProtocol) calculateAverage(result *ConsensusResult) {
	var sum float64
	var count int

	for _, vote := range result.Votes {
		if num, ok := toFloat64(vote.Value); ok {
			sum += num
			count++
		}
	}

	if count == 0 {
		result.Reached = false
		result.Reason = "no numeric votes received"
		return
	}

	avg := sum / float64(count)
	result.Reached = true
	result.Decision = avg
	result.Confidence = float64(count) / float64(len(result.Votes))
	result.Reason = fmt.Sprintf("average value: %.2f from %d votes", avg, count)
}

// calculateBorda Borda 计数法
func (p *ConsensusProtocol) calculateBorda(result *ConsensusResult) {
	scores := make(map[string]int)

	for _, vote := range result.Votes {
		if vote.Ranking == nil {
			continue
		}

		n := len(vote.Ranking)
		for i, item := range vote.Ranking {
			key := fmt.Sprintf("%v", item)
			scores[key] += n - i - 1 // 第一名得 n-1 分，最后一名得 0 分
		}
	}

	// 找出最高分
	var maxScore int
	var winner string
	for value, score := range scores {
		if score > maxScore {
			maxScore = score
			winner = value
		}
		result.VoteCount[value] = score
	}

	if winner != "" {
		result.Reached = true
		result.Decision = winner
		result.Confidence = float64(maxScore) / float64(len(result.Votes)*len(result.Votes[0].Ranking))
		result.Reason = fmt.Sprintf("'%s' won with Borda score %d", winner, maxScore)
	} else {
		result.Reached = false
		result.Reason = "no rankings provided"
	}
}

// calculateFirst 第一个响应
func (p *ConsensusProtocol) calculateFirst(result *ConsensusResult) {
	if len(result.Votes) == 0 {
		result.Reached = false
		result.Reason = "no votes received"
		return
	}

	// 按时间排序
	sorted := make([]Vote, len(result.Votes))
	copy(sorted, result.Votes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	first := sorted[0]
	result.Reached = true
	result.Decision = first.Value
	result.Confidence = 1.0
	result.Reason = fmt.Sprintf("first response from %s", first.AgentName)

	for _, vote := range result.Votes {
		key := fmt.Sprintf("%v", vote.Value)
		result.VoteCount[key]++
	}
}

// calculateBest 最佳响应
func (p *ConsensusProtocol) calculateBest(result *ConsensusResult) {
	if len(result.Votes) == 0 {
		result.Reached = false
		result.Reason = "no votes received"
		return
	}

	// 按分数排序
	sorted := make([]Vote, len(result.Votes))
	copy(sorted, result.Votes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	best := sorted[0]
	result.Reached = true
	result.Decision = best.Value
	result.Confidence = best.Score
	result.Reason = fmt.Sprintf("best response from %s with score %.2f", best.AgentName, best.Score)

	for _, vote := range result.Votes {
		key := fmt.Sprintf("%v", vote.Value)
		result.VoteCount[key]++
	}
}

// toFloat64 转换为 float64
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

// ============== Aggregation Functions ==============

// AggregateOutputs 聚合多个 Agent 输出
func AggregateOutputs(outputs []Output, strategy ConsensusStrategy) (*ConsensusResult, error) {
	votes := make([]Vote, len(outputs))
	for i, output := range outputs {
		votes[i] = Vote{
			AgentID:   output.Metadata["agent_id"].(string),
			Value:     output.Content,
			Timestamp: time.Now(),
		}
	}

	config := DefaultConsensusConfig()
	config.Strategy = strategy

	p := &ConsensusProtocol{config: config}
	return p.calculateResult(votes, len(votes), time.Now()), nil
}

// VoteWithReason 创建带理由的投票
func VoteWithReason(agentID string, value any, reason string) Vote {
	return Vote{
		AgentID:   agentID,
		Value:     value,
		Reason:    reason,
		Weight:    1.0,
		Timestamp: time.Now(),
	}
}

// RankedVote 创建排序投票
func RankedVote(agentID string, ranking []any) Vote {
	return Vote{
		AgentID:   agentID,
		Ranking:   ranking,
		Weight:    1.0,
		Timestamp: time.Now(),
	}
}
