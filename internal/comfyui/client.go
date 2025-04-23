package comfyui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go-serverhttp-template/internal/config"
)

// Client 与 ComfyUI 交互的 HTTP 客户端
type Client struct {
	host         string
	httpClient   *http.Client
	retryCount   int
	pollInterval time.Duration
}

// NewClient 使用全局加载的 ComfyUIConfig 构造
func NewClient(conf config.ComfyUIConfig) *Client {
	return &Client{
		host:         conf.Host,
		httpClient:   &http.Client{Timeout: conf.Timeout},
		retryCount:   conf.RetryCount,
		pollInterval: conf.PollInterval,
	}
}

// Generate 提交工作流并轮询进度，返回事件通道和错误通道
func (c *Client) Generate(ctx context.Context, workflow map[string]interface{}) (<-chan ProgressEvent, <-chan error, error) {
	// --- 1. Submit ---
	payload, err := json.Marshal(WorkflowRequest{WorkflowJSON: workflow})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal workflow payload: %w", err)
	}
	submitURL := c.host + "/api/submit-workflow"

	var (
		taskID  string
		lastErr error
	)
	for i := 0; i < c.retryCount; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, submitURL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("submit status %d: %s", resp.StatusCode, string(body))
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		var out struct {
			TaskID string `json:"task_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, nil, err
		}
		taskID = out.TaskID
		break
	}
	if taskID == "" {
		return nil, nil, fmt.Errorf("submit failed after %d retries: %w", c.retryCount, lastErr)
	}

	// --- 2. Poll Progress ---
	events := make(chan ProgressEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		statusURL := fmt.Sprintf("%s/api/status/%s", c.host, taskID)
		ticker := time.NewTicker(c.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case <-ticker.C:
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
				resp, err := c.httpClient.Do(req)
				if err != nil {
					errs <- err
					return
				}
				if resp.StatusCode != http.StatusOK {
					errs <- fmt.Errorf("status http %d", resp.StatusCode)
					resp.Body.Close()
					return
				}
				var ev ProgressEvent
				if err := json.NewDecoder(resp.Body).Decode(&ev); err != nil {
					errs <- err
					resp.Body.Close()
					return
				}
				resp.Body.Close()
				events <- ev
				if ev.Phase == "complete" || ev.Phase == "error" {
					return
				}
			}
		}
	}()

	return events, errs, nil
}

// FetchResult 根据 taskID 获取最终结果
func (c *Client) FetchResult(ctx context.Context, taskID string) (ResultData, error) {
	url := fmt.Sprintf("%s/api/result/%s", c.host, taskID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ResultData{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ResultData{}, fmt.Errorf("fetch status %d: %s", resp.StatusCode, string(body))
	}
	var rd ResultData
	if err := json.NewDecoder(resp.Body).Decode(&rd); err != nil {
		return ResultData{}, err
	}
	return rd, nil
}
