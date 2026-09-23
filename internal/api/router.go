package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/andrestorresgo/backend-service/internal/config"
	"github.com/andrestorresgo/backend-service/internal/db"
)

// NewRouter constructs a chi.Mux router configured with logging, panic recovery, CORS, and health routes.
func NewRouter(cfg *config.Config, pinger db.DBPinger, auth Authenticator) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	corsOptions := cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}
	r.Use(cors.Handler(corsOptions))

	r.Get("/healthz", HealthHandler(pinger))

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", AuthLoginHandler(auth))
	})

	// Placeholder route to verify API root
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"backend-service","status":"running"}`))
	})

	return r
}
