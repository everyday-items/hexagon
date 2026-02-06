package devui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// builderHandler Builder API 请求处理器
type builderHandler struct {
	store    *GraphStore
	executor *BuilderExecutor
}

// newBuilderHandler 创建 Builder 处理器
func newBuilderHandler(store *GraphStore, collector *Collector) *builderHandler {
	return &builderHandler{
		store:    store,
		executor: NewBuilderExecutor(collector),
	}
}

// handleGraphs 处理图定义列表和创建
// GET  /api/builder/graphs   - 列出所有图定义
// POST /api/builder/graphs   - 创建图定义
func (h *builderHandler) handleGraphs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listGraphs(w, r)
	case http.MethodPost:
		h.createGraph(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listGraphs 列出所有图定义
func (h *builderHandler) listGraphs(w http.ResponseWriter, _ *http.Request) {
	graphs := h.store.List()
	writeSuccess(w, map[string]any{
		"graphs": graphs,
		"total":  len(graphs),
	})
}

// createGraph 创建图定义
func (h *builderHandler) createGraph(w http.ResponseWriter, r *http.Request) {
	var def GraphDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求体: "+err.Error())
		return
	}

	if def.Name == "" {
		writeError(w, http.StatusBadRequest, "图名称不能为空")
		return
	}

	created := h.store.Create(&def)
	writeJSON(w, http.StatusCreated, response{
		Success: true,
		Data:    created,
	})
}

// handleGraph 处理单个图定义的 CRUD
// GET    /api/builder/graphs/{id}  - 获取图定义
// PUT    /api/builder/graphs/{id}  - 更新图定义
// DELETE /api/builder/graphs/{id}  - 删除图定义
func (h *builderHandler) handleGraph(w http.ResponseWriter, r *http.Request) {
	// 解析图 ID（从路径中提取）
	id := extractGraphID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "图 ID 不能为空")
		return
	}

	// 检查是否是子路由（validate/execute）
	parts := strings.Split(id, "/")
	if len(parts) >= 2 {
		graphID := parts[0]
		action := parts[1]
		switch action {
		case "validate":
			h.handleValidateGraph(w, r, graphID)
			return
		case "execute":
			h.handleExecuteGraph(w, r, graphID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		h.getGraph(w, id)
	case http.MethodPut:
		h.updateGraph(w, r, id)
	case http.MethodDelete:
		h.deleteGraph(w, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// getGraph 获取图定义
func (h *builderHandler) getGraph(w http.ResponseWriter, id string) {
	def, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeSuccess(w, def)
}

// updateGraph 更新图定义
func (h *builderHandler) updateGraph(w http.ResponseWriter, r *http.Request, id string) {
	var def GraphDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求体: "+err.Error())
		return
	}

	updated, err := h.store.Update(id, &def)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeSuccess(w, updated)
}

// deleteGraph 删除图定义
func (h *builderHandler) deleteGraph(w http.ResponseWriter, id string) {
	if err := h.store.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeSuccess(w, map[string]any{"deleted": true})
}

// handleValidateGraph 验证图定义
// POST /api/builder/graphs/{id}/validate
func (h *builderHandler) handleValidateGraph(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	def, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	result := h.store.Validate(def)
	writeSuccess(w, result)
}

// handleExecuteGraph 执行图定义
// POST /api/builder/graphs/{id}/execute
func (h *builderHandler) handleExecuteGraph(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	def, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// 解析可选的初始状态
	var reqBody struct {
		InitialState map[string]any `json:"initial_state,omitempty"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
	}

	// 先验证
	validation := h.store.Validate(def)
	if !validation.Valid {
		writeError(w, http.StatusBadRequest, "图验证失败: "+strings.Join(validation.Errors, "; "))
		return
	}

	// 执行
	result, err := h.executor.Execute(r.Context(), def, reqBody.InitialState)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "图执行失败: "+err.Error())
		return
	}

	writeSuccess(w, result)
}

// handleNodeTypes 获取可用节点类型列表
// GET /api/builder/node-types
func (h *builderHandler) handleNodeTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeSuccess(w, defaultNodeTypes)
}

// extractGraphID 从 URL 路径中提取图 ID
// 路径格式: /api/builder/graphs/{id} 或 /api/builder/graphs/{id}/validate
func extractGraphID(path string) string {
	// 移除 /api/builder/graphs/ 前缀
	const prefix = "/api/builder/graphs/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.TrimSuffix(remainder, "/")
	return remainder
}
