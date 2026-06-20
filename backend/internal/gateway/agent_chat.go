package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleAgentChatCompletions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// 1. Fetch Agent
	agent, err := s.db.Agents().Get(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// 2. Read Original Request
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// 3. Override Model
	if agent.Model != "" {
		req["model"] = agent.Model
	}

	// 4. Build Full System Prompt
	fullSystemPrompt := agent.SystemPrompt

	// Append skills if any
	if agent.Skills != "" && agent.Skills != "[]" {
		var skillIDs []string
		_ = json.Unmarshal([]byte(agent.Skills), &skillIDs)
		if len(skillIDs) > 0 {
			allSkills := s.loadSkills(r)
			skillMap := make(map[string]skill)
			for _, sk := range allSkills {
				if sk.Enabled {
					skillMap[sk.ID] = sk
				}
			}
			for _, sid := range skillIDs {
				if sk, ok := skillMap[sid]; ok {
					fullSystemPrompt += "\n\n" + sk.Prompt
				}
			}
		}
	}

	// 5. Inject System Prompt into Messages
	if fullSystemPrompt != "" {
		messages, ok := req["messages"].([]any)
		if !ok {
			messages = []any{}
		}

		systemMsg := map[string]any{
			"role":    "system",
			"content": fullSystemPrompt,
		}

		// Prepend system message
		newMessages := append([]any{systemMsg}, messages...)
		req["messages"] = newMessages
	}

	// 6. Serialize and Proxy to handleOpenAIChat
	newBody, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rewrite request")
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))

	// Forward to standard chat handler
	s.handleOpenAIChat(w, r)
}
