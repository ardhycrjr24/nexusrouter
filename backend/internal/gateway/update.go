package gateway

import (
	"net/http"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/update"
)

// versionString returns the running build version, defaulting to "dev" when the
// gateway was constructed without an injected version (e.g. local `go run`).
func (s *Server) versionString() string {
	if s.version == "" {
		return "dev"
	}
	return s.version
}

// adminUpdateCheck reports whether a newer NexusRouter release is available on
// GitHub, along with its changelog. The result is cached by the checker, so
// repeated dashboard loads do not hammer GitHub's API. When no checker is
// configured the handler returns just the current version with checked=false.
func (s *Server) adminUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.updates == nil {
		writeJSON(w, http.StatusOK, &update.Info{Current: s.versionString(), Checked: false})
		return
	}
	info := s.updates.Check(r.Context())
	writeJSON(w, http.StatusOK, info)
}