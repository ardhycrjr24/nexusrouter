package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

func (s *Server) adminListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.db.Agents().List(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		var skills []string
		if a.Skills != "" && a.Skills != "[]" {
			_ = json.Unmarshal([]byte(a.Skills), &skills)
		}
		if skills == nil {
			skills = []string{}
		}

		out = append(out, map[string]any{
			"id":            a.ID,
			"name":          a.Name,
			"description":   a.Description,
			"system_prompt": a.SystemPrompt,
			"model":         a.Model,
			"skills":        skills,
			"created_at":    a.CreatedAt,
			"updated_at":    a.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": out})
}

func (s *Server) adminCreateAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		SystemPrompt string   `json:"system_prompt"`
		Model        string   `json:"model"`
		Skills       []string `json:"skills"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	skillsJSON := "[]"
	if len(body.Skills) > 0 {
		b, _ := json.Marshal(body.Skills)
		skillsJSON = string(b)
	}

	now := time.Now()
	a := store.Agent{
		ID:           uuid.NewString(),
		TenantID:     adminTenant,
		Name:         body.Name,
		Description:  body.Description,
		SystemPrompt: body.SystemPrompt,
		Model:        body.Model,
		Skills:       skillsJSON,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.db.Agents().Create(r.Context(), a); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": a.ID, "name": a.Name})
}

func (s *Server) adminUpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.db.Agents().Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var body struct {
		Name         *string   `json:"name"`
		Description  *string   `json:"description"`
		SystemPrompt *string   `json:"system_prompt"`
		Model        *string   `json:"model"`
		Skills       *[]string `json:"skills"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.Name != nil {
		a.Name = *body.Name
	}
	if body.Description != nil {
		a.Description = *body.Description
	}
	if body.SystemPrompt != nil {
		a.SystemPrompt = *body.SystemPrompt
	}
	if body.Model != nil {
		a.Model = *body.Model
	}
	if body.Skills != nil {
		if len(*body.Skills) > 0 {
			b, _ := json.Marshal(*body.Skills)
			a.Skills = string(b)
		} else {
			a.Skills = "[]"
		}
	}

	a.UpdatedAt = time.Now()
	if err := s.db.Agents().Update(r.Context(), a); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Agents().Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
