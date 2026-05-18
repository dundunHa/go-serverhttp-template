package comfyui

import "time"

// WorkflowRequest 提交给 ComfyUI 的工作流结构
type WorkflowRequest struct {
	WorkflowJSON map[string]interface{} `json:"workflow_json"`
}

// ProgressEvent 描述 ComfyUI 返回的进度事件
type ProgressEvent struct {
	TaskID    string    `json:"task_id"`
	Phase     string    `json:"phase"`    // e.g. "submitted","rendering","complete","error"
	Progress  float32   `json:"progress"` // 0.0-1.0
	Timestamp time.Time `json:"timestamp"`
}

// ResultData 最终生成结果（图片/视频）
type ResultData struct {
	TaskID string   `json:"task_id"`
	Images []string `json:"images"`
	Video  string   `json:"video"`
}
