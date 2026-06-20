package gateway

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/connectors"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

// modelEntry is one entry in a /v1/models listing, in the OpenAI shape plus
// NexusRouter extensions (provider, kind, dimensions).
type modelEntry struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	OwnedBy    string `json:"owned_by"`
	Provider   string `json:"provider,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// handleListModels reports targetable models: the tenant's chains (as virtual
// models) plus every catalogued LLM model in provider/model form. This lets a
// client discover what it can pass in the `model` field.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	tenantID := tenantOf(key)

	data := make([]modelEntry, 0, 64)

	// Chains are exposed as "combo" models, matching the upstream convention:
	// a combo chains multiple providers with auto-fallback and is callable by
	// its bare name (and via the chain: prefix). owned_by:"combo" lets client
	// tools surface them distinctly from single-provider models.
	chains, err := s.chains.ListByTenant(r.Context(), tenantID)
	if err == nil {
		for _, c := range chains {
			data = append(data, modelEntry{
				ID: c.Name, Object: "model", OwnedBy: "combo", Kind: string(core.ServiceLLM), Name: c.Name,
			})
		}
	}

	// Static catalog models.
	for _, pm := range connectors.ModelsByKind(core.ServiceLLM) {
		data = append(data, modelEntry{
			ID:      pm.Provider + "/" + pm.Model.ID,
			Object:  "model",
			OwnedBy: pm.Provider,
			Provider: pm.Provider,
			Kind:    string(core.ServiceLLM),
			Name:    pm.Model.Name,
		})
	}

	// Live model discovery: for providers with a LiveModelSource and connected
	// accounts, fetch the live catalog and merge (live models supplement, not
	// replace, the static catalog).
	liveModels := s.fetchLiveModels(r.Context(), tenantID)
	for provider, models := range liveModels {
		for _, lm := range models {
			data = append(data, modelEntry{
				ID:      provider + "/" + lm.ID,
				Object:  "model",
				OwnedBy: provider,
				Provider: provider,
				Kind:    string(lm.Kind),
				Name:    lm.Name,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleListModelsByKind serves GET /v1/models/{kind}: it lists every model of
// the requested service kind (llm, embedding, image, stt, tts, search, fetch)
// across the provider catalog, plus a special "chains" view.
func (s *Server) handleListModelsByKind(w http.ResponseWriter, r *http.Request) {
	kindParam := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "kind")))

	// "chains" is a convenience view of the tenant's routing chains.
	if kindParam == "chains" {
		s.handleListModels(w, r)
		return
	}

	kind := core.ServiceKind(kindParam)
	if !core.ValidServiceKind(kind) {
		writeError(w, http.StatusBadRequest, "unknown model kind: "+kindParam)
		return
	}

	pairs := connectors.ModelsByKind(kind)
	data := make([]modelEntry, 0, len(pairs))
	for _, pm := range pairs {
		data = append(data, modelEntry{
			ID:         pm.Provider + "/" + pm.Model.ID,
			Object:     "model",
			OwnedBy:    pm.Provider,
			Provider:   pm.Provider,
			Kind:       string(pm.Model.Kind),
			Name:       pm.Model.Name,
			Dimensions: pm.Model.Dimensions,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "kind": kindParam, "data": data})
}

// fetchLiveModels queries providers that support live model discovery, using
// the first connected account's credentials. Returns a map of provider →
// models. Errors are silently skipped (live discovery is best-effort).
func (s *Server) fetchLiveModels(ctx context.Context, tenantID string) map[string][]connectors.ModelSpec {
	if s.accounts == nil || s.vault == nil {
		return nil
	}
	result := map[string][]connectors.ModelSpec{}

	// Check each provider that has a live model source.
	for provider, src := range map[string]connectors.LiveModelSource{
		"kiro": connectors.GetLiveModelSource("kiro"),
	} {
		if src == nil {
			continue
		}
		accs, err := s.accounts.ListByProvider(ctx, tenantID, provider)
		if err != nil || len(accs) == 0 {
			continue
		}
		// Use the first non-disabled account.
		var acc store.Account
		for _, a := range accs {
			if !a.Disabled {
				acc = a
				break
			}
		}
		if acc.ID == "" {
			continue
		}
		creds, err := s.vault.Open(acc)
		if err != nil {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		models, err := src.ListModels(probeCtx, creds)
		cancel()
		if err != nil || len(models) == 0 {
			continue
		}
		result[provider] = models
	}
	return result
}

// handleModelInfo serves GET /v1/models/info?id=<provider/model>: it returns
// metadata for a single model (kind, dimensions, provider, name).
func (s *Server) handleModelInfo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id query parameter is required")
		return
	}

	provider, model, ok := strings.Cut(id, "/")
	if !ok || provider == "" || model == "" {
		writeError(w, http.StatusBadRequest, "id must be in provider/model form")
		return
	}

	spec, found := connectors.FindModel(provider, model)
	if !found {
		writeError(w, http.StatusNotFound, "unknown model: "+id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"provider":   provider,
		"model":      spec.ID,
		"name":       spec.Name,
		"kind":       string(spec.Kind),
		"dimensions": spec.Dimensions,
	})
}