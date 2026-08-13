package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/carlosmaranje/mango/core"
)

type agentStatus struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Skills []string `json:"skills,omitempty"`
}

type agentRequest struct {
	Name string `json:"name"`
}

type taskRequest struct {
	Goal      string `json:"goal"`
	Agent     string `json:"agent,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type taskResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

// taskDTO is the public, on-the-wire representation of a task. It preserves the
// historical field names (agent_name, result) regardless of the core.Result
// shape so existing REST/SSE clients keep working.
type taskDTO struct {
	ID        string    `json:"id"`
	Goal      string    `json:"goal"`
	AgentName string    `json:"agent_name,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Status    string    `json:"status"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toTaskDTO(r *core.Result) taskDTO {
	return taskDTO{
		ID:        r.ID,
		Goal:      r.Goal,
		AgentName: r.Agent,
		SessionID: r.SessionID,
		Status:    string(r.Status),
		Result:    r.Output,
		Error:     r.Error,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/agents", s.handleAgents)
	mux.HandleFunc("/agents/start", s.handleAgentStart)
	mux.HandleFunc("/agents/stop", s.handleAgentStop)
	mux.HandleFunc("/tasks", s.handleTasks)
	mux.HandleFunc("/tasks/", s.handleTaskByID)
	mux.HandleFunc("/chat", s.handleChat)
	mux.HandleFunc("/events", s.handleEvents)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var out []agentStatus
	for _, a := range s.engine.Agents() {
		out = append(out, agentStatus{
			Name:   a.Name,
			Status: a.Status,
			Skills: a.Skills,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAgentStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req agentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.engine.StartAgent(r.Context(), req.Name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": req.Name, "status": "running"})
}

func (s *Server) handleAgentStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req agentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.engine.StopAgent(req.Name); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": req.Name, "status": "stopped"})
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req taskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(req.Goal) == "" {
			writeError(w, http.StatusBadRequest, "goal is required")
			return
		}
		result, err := s.engine.Submit(context.Background(), core.Request{
			Goal:      req.Goal,
			Agent:     req.Agent,
			SessionID: req.SessionID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, taskResponse{ID: result.ID, Status: string(result.Status)})
	case http.MethodGet:
		results := s.engine.List()
		out := make([]taskDTO, 0, len(results))
		for _, r := range results {
			out = append(out, toTaskDTO(r))
		}
		writeJSON(w, http.StatusOK, out)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/tasks/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "task id required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		result, ok := s.engine.Get(id)
		if !ok {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeJSON(w, http.StatusOK, taskResponse{
			ID:     result.ID,
			Status: string(result.Status),
			Result: result.Output,
			Error:  result.Error,
		})

	case http.MethodDelete:
		result, err := s.engine.Cancel(id)
		if err != nil {
			switch err.Error() {
			case "task not found":
				writeError(w, http.StatusNotFound, err.Error())
			default:
				writeError(w, http.StatusConflict, err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, taskResponse{ID: result.ID, Status: string(result.Status)})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleChat submits a task directly to a worker agent (bypassing the orchestrator)
// and blocks until complete, returning the result synchronously.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Goal) == "" {
		writeError(w, http.StatusBadRequest, "goal is required")
		return
	}
	// Route directly to the first non-orchestrator agent so chat bypasses
	// the orchestrator decomposition loop and goes straight to a tool-enabled worker.
	if req.Agent == "" {
		req.Agent = s.engine.DefaultAgent()
	}
	result, err := s.engine.SubmitAndWait(r.Context(), core.Request{
		Goal:      req.Goal,
		Agent:     req.Agent,
		SessionID: req.SessionID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, taskResponse{
		ID:        result.ID,
		Status:    string(result.Status),
		SessionID: result.SessionID,
		Result:    result.Output,
		Error:     result.Error,
	})
}

// handleEvents streams task and agent lifecycle events as Server-Sent Events.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	unsubscribe, events := s.engine.Subscribe()
	defer unsubscribe()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	writeSSE(w, "ping", "{}")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			data, err := marshalEvent(e)
			if err != nil {
				continue
			}
			writeSSE(w, string(e.Type), data)
			flusher.Flush()
		case <-ping.C:
			writeSSE(w, "ping", "{}")
			flusher.Flush()
		}
	}
}

// marshalEvent renders a core.Event payload in the historical SSE shape: task.*
// events carry the full task object; agent.* events carry {"name": ...}.
func marshalEvent(e core.Event) (string, error) {
	var payload any
	switch {
	case e.Task != nil:
		payload = toTaskDTO(e.Task)
	case e.Agent != "":
		payload = map[string]string{"name": e.Agent}
	default:
		payload = map[string]any{}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
