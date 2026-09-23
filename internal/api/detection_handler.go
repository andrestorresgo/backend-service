package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// DetectionProcessor specifies the contract for handling vision detection requests.
type DetectionProcessor interface {
	ProcessDetection(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error)
}

// DetectionHandler handles POST /api/v1/detections.
func DetectionHandler(processor DetectionProcessor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if processor == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"detection service unavailable"}`))
			return
		}

		var req service.DetectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid request payload"}`))
			return
		}

		if req.EventID == "" {
			req.EventID = r.Header.Get("X-Event-ID")
		}

		result, err := processor.ProcessDetection(r.Context(), req)
		if err != nil {
			if errors.Is(err, service.ErrInvalidShape) || errors.Is(err, service.ErrConflictingShape) {
				w.WriteHeader(http.StatusBadRequest)
				resp, _ := json.Marshal(map[string]string{"error": err.Error()})
				_, _ = w.Write(resp)
				return
			}

			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"failed to process detection"}`))
			return
		}

		if !result.OK && (result.Status == service.DetectionStatusDispatched || result.Status == service.DetectionStatusDebounced) {
			result.OK = true
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}
