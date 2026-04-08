package api

import (
	"encoding/json"
	"net/http"

	"github.com/charliewilco/argus/providers"
	"github.com/go-chi/chi/v5"
)

type RouterOptions struct {
	BaseURL   string
	Providers *providers.Registry
}

func NewRouter(opts RouterOptions) http.Handler {
	router := chi.NewRouter()

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"base_url": opts.BaseURL,
			"service":  "argus",
			"status":   "ok",
		})
	})

	router.Post("/webhooks/{provider}/{tenantID}/{connectionID}", notImplemented("webhooks"))
	router.Get("/oauth/{provider}/connect", notImplemented("oauth connect"))
	router.Get("/oauth/{provider}/callback", notImplemented("oauth callback"))

	router.Route("/api", func(r chi.Router) {
		r.Get("/connections", notImplemented("list connections"))
		r.Post("/connections", notImplemented("create connection"))
		r.Delete("/connections/{id}", notImplemented("delete connection"))

		r.Get("/pipelines", notImplemented("list pipelines"))
		r.Post("/pipelines", notImplemented("create pipeline"))
		r.Put("/pipelines/{id}", notImplemented("update pipeline"))
		r.Delete("/pipelines/{id}", notImplemented("delete pipeline"))

		r.Get("/events", notImplemented("list events"))
		r.Get("/events/{id}/replay", notImplemented("replay event"))

		r.Get("/providers", func(w http.ResponseWriter, r *http.Request) {
			if opts.Providers == nil {
				writeJSON(w, http.StatusOK, []providers.Metadata{})
				return
			}

			writeJSON(w, http.StatusOK, opts.Providers.Metadata())
		})
		r.Get("/providers/{name}/schema", notImplemented("provider schema"))
		r.Get("/providers/{name}/options/{field}", notImplemented("provider options"))
	})

	return router
}

func notImplemented(operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error":     "not_implemented",
			"operation": operation,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
