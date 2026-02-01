/**
 * Hexagon Dev UI
 *
 * 实时调试界面，通过 SSE 接收事件并展示
 */

// ============================================================================
// 全局状态
// ============================================================================

const state = {
    events: [],
    selectedEvent: null,
    eventSource: null,
    connected: false,
    paused: false,
    streamContent: {},  // LLM 流式内容聚合
    metrics: {
        totalEvents: 0,
        agentRuns: 0,
        llmCalls: 0,
        toolCalls: 0,
        retrieverRuns: 0,
        errors: 0
    }
};

// 事件类型配置
const EVENT_CONFIG = {
    'agent.start': { icon: '🚀', label: 'Agent 开始', category: 'agent' },
    'agent.end': { icon: '✅', label: 'Agent 结束', category: 'agent' },
    'llm.request': { icon: '🤖', label: 'LLM 请求', category: 'llm' },
    'llm.stream': { icon: '💬', label: 'LLM 流式', category: 'llm' },
    'llm.response': { icon: '📝', label: 'LLM 响应', category: 'llm' },
    'tool.call': { icon: '🔧', label: '工具调用', category: 'tool' },
    'tool.result': { icon: '📦', label: '工具结果', category: 'tool' },
    'retriever.start': { icon: '🔍', label: '检索开始', category: 'retriever' },
    'retriever.end': { icon: '📚', label: '检索结束', category: 'retriever' },
    'graph.start': { icon: '📊', label: '图开始', category: 'graph' },
    'graph.node': { icon: '⬡', label: '图节点', category: 'graph' },
    'graph.end': { icon: '🏁', label: '图结束', category: 'graph' },
    'state.change': { icon: '🔄', label: '状态变更', category: 'state' },
    'error': { icon: '❌', label: '错误', category: 'error' }
};

// ============================================================================
// DOM 元素
// ============================================================================

const elements = {
    connectionStatus: document.getElementById('connectionStatus'),
    eventList: document.getElementById('eventList'),
    detailView: document.getElementById('detailView'),
    detailTitle: document.getElementById('detailTitle'),
    eventTypeFilter: document.getElementById('eventTypeFilter'),
    eventCount: document.getElementById('eventCount'),
    lastEventTime: document.getElementById('lastEventTime'),
    pauseBtn: document.getElementById('pauseBtn'),
    pauseIcon: document.getElementById('pauseIcon'),
    clearBtn: document.getElementById('clearBtn'),
    streamModal: document.getElementById('streamModal'),
    streamContent: document.getElementById('streamContent'),
    closeStreamModal: document.getElementById('closeStreamModal'),
    // 指标
    metricTotalEvents: document.getElementById('metricTotalEvents'),
    metricAgentRuns: document.getElementById('metricAgentRuns'),
    metricLLMCalls: document.getElementById('metricLLMCalls'),
    metricToolCalls: document.getElementById('metricToolCalls'),
    metricRetrieverRuns: document.getElementById('metricRetrieverRuns'),
    metricErrors: document.getElementById('metricErrors'),
    uptime: document.getElementById('uptime')
};

// ============================================================================
// SSE 连接
// ============================================================================

function connect() {
    if (state.eventSource) {
        state.eventSource.close();
    }

    const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:';
    const host = window.location.host;
    const url = `${protocol}//${host}/events`;

    console.log('Connecting to SSE:', url);

    state.eventSource = new EventSource(url);

    state.eventSource.onopen = () => {
        console.log('SSE connected');
        state.connected = true;
        updateConnectionStatus('connected', '已连接');
    };

    state.eventSource.onerror = (err) => {
        console.error('SSE error:', err);
        state.connected = false;
        updateConnectionStatus('disconnected', '已断开');

        // 尝试重连
        setTimeout(() => {
            if (!state.connected) {
                console.log('Attempting to reconnect...');
                connect();
            }
        }, 3000);
    };

    // 监听所有事件类型
    const eventTypes = Object.keys(EVENT_CONFIG);
    eventTypes.forEach(type => {
        state.eventSource.addEventListener(type, handleEvent);
    });

    // 连接成功事件
    state.eventSource.addEventListener('connected', (e) => {
        console.log('Connected:', e.data);
    });
}

function handleEvent(e) {
    if (state.paused) return;

    try {
        const event = JSON.parse(e.data);
        event.type = e.type;  // 确保类型正确

        // 处理 LLM 流式事件 - 聚合内容
        if (event.type === 'llm.stream') {
            const runId = event.data?.run_id || event.id;
            if (!state.streamContent[runId]) {
                state.streamContent[runId] = '';
            }
            state.streamContent[runId] += event.data?.content || '';

            // 更新已选中的事件显示
            if (state.selectedEvent?.data?.run_id === runId) {
                updateDetailView(state.selectedEvent);
            }
        }

        // 添加到事件列表
        state.events.unshift(event);

        // 限制事件数量
        if (state.events.length > 1000) {
            state.events.pop();
        }

        // 更新指标
        updateMetrics(event);

        // 更新 UI
        renderEventList();
        updateFooter();
    } catch (err) {
        console.error('Failed to parse event:', err, e.data);
    }
}

// ============================================================================
// UI 更新
// ============================================================================

function updateConnectionStatus(status, text) {
    elements.connectionStatus.className = `connection-status ${status}`;
    elements.connectionStatus.querySelector('.status-text').textContent = text;
}

function renderEventList() {
    const filter = elements.eventTypeFilter.value;
    const filteredEvents = filter
        ? state.events.filter(e => e.type === filter)
        : state.events;

    if (filteredEvents.length === 0) {
        elements.eventList.innerHTML = `
            <div class="empty-state">
                <span class="empty-icon">📭</span>
                <p>等待事件...</p>
            </div>
        `;
        return;
    }

    const html = filteredEvents.slice(0, 100).map((event, index) => {
        const config = EVENT_CONFIG[event.type] || { icon: '📌', label: event.type, category: 'unknown' };
        const time = formatTime(event.timestamp);
        const title = getEventTitle(event);
        const isNew = index === 0;
        const isSelected = state.selectedEvent?.id === event.id;

        return `
            <div class="event-item ${isNew ? 'new' : ''} ${isSelected ? 'selected' : ''}"
                 data-id="${event.id}"
                 onclick="selectEvent('${event.id}')">
                <div class="event-icon ${config.category}">${config.icon}</div>
                <div class="event-content">
                    <div class="event-title">${escapeHtml(title)}</div>
                    <div class="event-meta">
                        <span class="event-type">${config.label}</span>
                        <span class="event-time">${time}</span>
                    </div>
                </div>
            </div>
        `;
    }).join('');

    elements.eventList.innerHTML = html;
}

function getEventTitle(event) {
    const data = event.data || {};

    switch (event.type) {
        case 'agent.start':
            return `Agent: ${data.input || data.run_id || 'unknown'}`;
        case 'agent.end':
            return `完成 (${data.duration_ms}ms)`;
        case 'llm.request':
            return `${data.model || 'LLM'}: 请求中...`;
        case 'llm.stream':
            return data.content?.substring(0, 50) || '...';
        case 'llm.response':
            return `${data.model}: ${data.total_tokens} tokens`;
        case 'tool.call':
            return `调用: ${data.tool_name}`;
        case 'tool.result':
            return `${data.tool_name}: ${data.error ? '失败' : '成功'}`;
        case 'retriever.start':
            return `检索: ${data.query?.substring(0, 30)}...`;
        case 'retriever.end':
            return `找到 ${data.doc_count} 个文档`;
        case 'error':
            return data.message?.substring(0, 50) || '错误';
        default:
            return event.type;
    }
}

function selectEvent(id) {
    const event = state.events.find(e => e.id === id);
    if (!event) return;

    state.selectedEvent = event;
    renderEventList();
    updateDetailView(event);
}

function updateDetailView(event) {
    const config = EVENT_CONFIG[event.type] || { icon: '📌', label: event.type };
    elements.detailTitle.textContent = `${config.icon} ${config.label}`;

    const data = event.data || {};
    let html = `
        <div class="detail-section">
            <div class="detail-section-title">基本信息</div>
            <div class="detail-content">
                <div class="detail-row">
                    <span class="detail-label">事件 ID</span>
                    <span class="detail-value">${event.id}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">类型</span>
                    <span class="detail-value">${event.type}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">时间</span>
                    <span class="detail-value">${formatDateTime(event.timestamp)}</span>
                </div>
    `;

    if (event.trace_id) {
        html += `
                <div class="detail-row">
                    <span class="detail-label">Trace ID</span>
                    <span class="detail-value">${event.trace_id}</span>
                </div>
        `;
    }

    if (event.agent_id) {
        html += `
                <div class="detail-row">
                    <span class="detail-label">Agent ID</span>
                    <span class="detail-value">${event.agent_id}</span>
                </div>
        `;
    }

    html += `
            </div>
        </div>
    `;

    // 事件数据
    if (Object.keys(data).length > 0) {
        html += `
            <div class="detail-section">
                <div class="detail-section-title">事件数据</div>
                <div class="detail-content">${formatJSON(data)}</div>
            </div>
        `;
    }

    // LLM 流式内容
    if (event.type === 'llm.request' || event.type === 'llm.stream' || event.type === 'llm.response') {
        const runId = data.run_id || event.id;
        const streamedContent = state.streamContent[runId];
        if (streamedContent) {
            html += `
                <div class="detail-section">
                    <div class="detail-section-title">流式输出</div>
                    <div class="detail-content">${escapeHtml(streamedContent)}</div>
                </div>
            `;
        }
    }

    elements.detailView.innerHTML = html;
}

function updateMetrics(event) {
    state.metrics.totalEvents++;

    switch (event.type) {
        case 'agent.start':
            state.metrics.agentRuns++;
            break;
        case 'llm.request':
            state.metrics.llmCalls++;
            break;
        case 'tool.call':
            state.metrics.toolCalls++;
            break;
        case 'retriever.start':
            state.metrics.retrieverRuns++;
            break;
        case 'error':
            state.metrics.errors++;
            break;
    }

    elements.metricTotalEvents.textContent = state.metrics.totalEvents;
    elements.metricAgentRuns.textContent = state.metrics.agentRuns;
    elements.metricLLMCalls.textContent = state.metrics.llmCalls;
    elements.metricToolCalls.textContent = state.metrics.toolCalls;
    elements.metricRetrieverRuns.textContent = state.metrics.retrieverRuns;
    elements.metricErrors.textContent = state.metrics.errors;
}

function updateFooter() {
    elements.eventCount.textContent = `${state.events.length} 个事件`;

    if (state.events.length > 0) {
        const lastEvent = state.events[0];
        elements.lastEventTime.textContent = `最后更新: ${formatTime(lastEvent.timestamp)}`;
    }
}

// 定期更新运行时间
function updateUptime() {
    fetch('/api/metrics')
        .then(res => res.json())
        .then(data => {
            if (data.success && data.data.uptime_seconds) {
                const seconds = data.data.uptime_seconds;
                const hours = Math.floor(seconds / 3600);
                const minutes = Math.floor((seconds % 3600) / 60);
                const secs = seconds % 60;
                elements.uptime.textContent =
                    `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
            }
        })
        .catch(err => console.error('Failed to fetch metrics:', err));
}

// ============================================================================
// 事件处理
// ============================================================================

elements.eventTypeFilter.addEventListener('change', renderEventList);

elements.pauseBtn.addEventListener('click', () => {
    state.paused = !state.paused;
    elements.pauseIcon.textContent = state.paused ? '▶️' : '⏸️';
    updateConnectionStatus(
        state.paused ? 'paused' : 'connected',
        state.paused ? '已暂停' : '已连接'
    );
});

elements.clearBtn.addEventListener('click', () => {
    state.events = [];
    state.selectedEvent = null;
    state.streamContent = {};
    state.metrics = {
        totalEvents: 0,
        agentRuns: 0,
        llmCalls: 0,
        toolCalls: 0,
        retrieverRuns: 0,
        errors: 0
    };
    renderEventList();
    updateMetrics({ type: '' });
    updateFooter();
    elements.detailView.innerHTML = `
        <div class="empty-state">
            <span class="empty-icon">👈</span>
            <p>选择一个事件查看详情</p>
        </div>
    `;
});

elements.closeStreamModal.addEventListener('click', () => {
    elements.streamModal.classList.remove('active');
});

// ============================================================================
// 工具函数
// ============================================================================

function formatTime(timestamp) {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });
}

function formatDateTime(timestamp) {
    const date = new Date(timestamp);
    return date.toLocaleString('zh-CN');
}

function formatJSON(obj) {
    try {
        return escapeHtml(JSON.stringify(obj, null, 2));
    } catch (e) {
        return String(obj);
    }
}

function escapeHtml(str) {
    if (typeof str !== 'string') {
        str = String(str);
    }
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// ============================================================================
// 初始化
// ============================================================================

function init() {
    console.log('Hexagon Dev UI initializing...');
    connect();
    updateUptime();
    setInterval(updateUptime, 1000);
}

// 页面加载完成后初始化
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
