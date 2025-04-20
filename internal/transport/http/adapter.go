package httpserver

import (
	"context"
	"net/http"
)

type HandlerFunc[Req any, Res any] func(ctx context.Context, req Req) (Res, *APIError)

func Adapter[Req any, Res any](h HandlerFunc[Req, Res]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		if apiErr := BindRequest(r, &req); apiErr != nil {
			writeError(w, apiErr)
			return
		}
		res, apiErr := h(r.Context(), req)
		if apiErr != nil {
			writeError(w, apiErr)
			return
		}
		writeSuccess(w, res)
	}
}
