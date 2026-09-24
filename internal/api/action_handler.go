package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// ActionsProvider specifies the contract for querying recent system actions.
type ActionsProvider interface {
	GetRecentActions(ctx context.Context, limit int) ([]service.ActionRecord, error)
}

// ActionHandler handles GET /api/v1/actions returning recent system action logs.
func ActionHandler(provider ActionsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if provider == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "actions service unavailable",
			})
			return
		}

		limit := 50
		if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
			if l, err := strconv.Atoi(limitParam); err == nil && l > 0 {
				limit = l
			}
		}

		actions, err := provider.GetRecentActions(r.Context(), limit)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to retrieve system action logs",
			})
			return
		}

		if actions == nil {
			actions = []service.ActionRecord{}
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"actions": actions,
			"count":   len(actions),
		})
	}
}
