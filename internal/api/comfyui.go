package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/dundunHa/go-serverhttp-template/internal/comfyui"
)

type ComfyUIHandler struct {
	client *comfyui.Client
}

func NewComfyUIHandler(client *comfyui.Client) *ComfyUIHandler {
	return &ComfyUIHandler{client: client}
}

func (h *ComfyUIHandler) Register(r chi.Router) {
	r.Post("/generate", h.Generate)
	r.Get("/result/{taskID}", h.GetResult)
}

func (h *ComfyUIHandler) writeSSE(w http.ResponseWriter, event string, data interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", event, err)
	}
	for _, line := range []string{
		"event: " + event,
		"data: " + string(b),
		"",
	} {
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			return fmt.Errorf("write sse %s: %w", event, err)
		}
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func (h *ComfyUIHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var workflow map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	events, errs, err := h.client.Generate(ctx, workflow)
	if err != nil {
		log.Error().Err(err).Msg("submit ComfyUI workflow failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := h.writeSSE(w, "progress", event); err != nil {
				log.Error().Err(err).Msg("send progress failed")
				return
			}
			if event.Phase == "complete" {
				result, err := h.client.FetchResult(ctx, event.TaskID)
				if err == nil {
					if err := h.writeSSE(w, "result", result); err != nil {
						log.Error().Err(err).Msg("send result failed")
					}
				}
				return
			}
		case err := <-errs:
			log.Error().Err(err).Msg("ComfyUI progress polling failed")
			if err := h.writeSSE(w, "error", map[string]string{"error": err.Error()}); err != nil {
				log.Error().Err(err).Msg("send error failed")
			}
			return
		case <-r.Context().Done():
			log.Info().Msg("client disconnected")
			return
		case <-ctx.Done():
			log.Warn().Msg("ComfyUI generation timeout")
			if err := h.writeSSE(w, "error", map[string]string{"error": "timeout"}); err != nil {
				log.Error().Err(err).Msg("send timeout failed")
			}
			return
		}
	}
}

func (h *ComfyUIHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		http.Error(w, "taskID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.client.FetchResult(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("taskID", taskID).Msg("fetch ComfyUI result failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Error().Err(err).Str("taskID", taskID).Msg("encode ComfyUI result failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
