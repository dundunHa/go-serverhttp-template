package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"go-serverhttp-template/internal/comfyui"
)

// ComfyUIHandler 处理与 ComfyUI 交互的 HTTP 请求
type ComfyUIHandler struct {
	client *comfyui.Client
}

// NewComfyUIHandler 创建 ComfyUI 处理器
func NewComfyUIHandler(client *comfyui.Client) *ComfyUIHandler {
	return &ComfyUIHandler{client: client}
}

// Register 注册 ComfyUI 相关路由
func (h *ComfyUIHandler) Register(r chi.Router) {
	r.Post("/generate", h.Generate)
	r.Get("/result/{taskID}", h.GetResult)
}

// 新增：统一 SSE 写入及错误处理
func (h *ComfyUIHandler) writeSSE(w http.ResponseWriter, event string, data interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", event, err)
	}
	msgs := []string{
		"event: " + event,
		"data: " + string(b),
		"",
	}
	for _, m := range msgs {
		if _, err := w.Write([]byte(m + "\n")); err != nil {
			return fmt.Errorf("write sse %s: %w", event, err)
		}
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Generate 处理 POST /api/v1/comfyui/generate 请求
// 接收工作流 JSON 并启动生成任务，使用 SSE 推送进度
func (h *ComfyUIHandler) Generate(w http.ResponseWriter, r *http.Request) {
	// 1. 解析工作流 JSON
	var workflow map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 2. 创建带超时的 Context (10 分钟)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	// 3. 调用 Generate
	events, errs, err := h.client.Generate(ctx, workflow)
	if err != nil {
		log.Error().Err(err).Msg("提交 ComfyUI 生成任务失败")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. SSE 推送进度
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 避免 Nginx 缓冲

	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "流式传输不支持", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := h.writeSSE(w, "progress", event); err != nil {
				log.Error().Err(err).Msg("发送 progress 失败")
				return
			}

			// 如果已完成，也发送最终结果
			if event.Phase == "complete" {
				result, err := h.client.FetchResult(ctx, event.TaskID)
				if err == nil {
					if err := h.writeSSE(w, "result", result); err != nil {
						log.Error().Err(err).Msg("发送 result 失败")
					}
				}
				return
			}
		case err := <-errs:
			log.Error().Err(err).Msg("ComfyUI 进度监控错误")
			errData := map[string]string{"error": err.Error()}
			if err := h.writeSSE(w, "error", errData); err != nil {
				log.Error().Err(err).Msg("发送 error 失败")
			}
			return
		case <-r.Context().Done(): // 客户端断开连接
			log.Info().Msg("客户端断开连接")
			return
		case <-ctx.Done(): // 超时
			log.Warn().Msg("ComfyUI 生成任务超时")
			errData := map[string]string{"error": "timeout"}
			if err := h.writeSSE(w, "error", errData); err != nil {
				log.Error().Err(err).Msg("发送 timeout 错误失败")
			}
			return
		}
	}
}

// GetResult 处理 GET /api/v1/comfyui/result/{taskID} 请求
// 直接返回指定任务 ID 的生成结果
func (h *ComfyUIHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		http.Error(w, "缺少 taskID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.client.FetchResult(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("taskID", taskID).Msg("获取 ComfyUI 结果失败")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Error().Err(err).Str("taskID", taskID).Msg("Encode JSON 失败")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
