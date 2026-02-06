package handlers

import (
	"net/http"
	"time"
)

type adminRunnerResponse struct {
	RunnerID    int64   `json:"runner_id"`
	Name        string  `json:"name"`
	Environment string  `json:"environment"`
	Status      string  `json:"status"`
	LastSeenAt  *string `json:"last_seen_at,omitempty"`
}

type listAdminRunnersResponse struct {
	Runners []adminRunnerResponse `json:"runners"`
}

// ListRunners lists all registered runners (admin-only route).
func (h *Handlers) ListRunners(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	runners, err := h.store.ListRunners(r.Context())
	if err != nil {
		h.logger.Error("list runners", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	resp := listAdminRunnersResponse{Runners: make([]adminRunnerResponse, 0, len(runners))}
	for _, runner := range runners {
		rr := adminRunnerResponse{
			RunnerID:    runner.ID,
			Name:        runner.Name,
			Environment: runner.Environment,
			Status:      runner.Status,
		}
		if runner.LastSeenAt != nil {
			s := runner.LastSeenAt.Format(time.RFC3339)
			rr.LastSeenAt = &s
		}
		resp.Runners = append(resp.Runners, rr)
	}

	writeJSON(w, http.StatusOK, resp)
}
