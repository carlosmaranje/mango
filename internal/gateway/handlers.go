package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/carlosmaranje/mango/internal/orchestrator"
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
	for _, a := range s.registry.List() {
		status := "stopped"
		if runner, ok := s.runners[a.Name]; ok && runner.IsRunning() {
			status = "running"
		}
		out = append(out, agentStatus{
			Name:   a.Name,
			Status: status,
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
	runner, ok := s.runners[req.Name]
	if !ok {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err := runner.Start(r.Context()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if s.bus != nil {
		s.bus.Emit(orchestrator.Event{Type: "agent.started", Payload: map[string]string{"name": req.Name}})
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
	runner, ok := s.runners[req.Name]
	if !ok {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	runner.Stop()
	if s.bus != nil {
		s.bus.Emit(orchestrator.Event{Type: "agent.stopped", Payload: map[string]string{"name": req.Name}})
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
		task, err := s.dispatcher.Submit(context.Background(), req.Goal, req.Agent, req.SessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, taskResponse{ID: task.ID, Status: task.Status})
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.dispatcher.List())
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
		task, ok := s.dispatcher.Get(id)
		if !ok {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeJSON(w, http.StatusOK, taskResponse{
			ID:     task.ID,
			Status: task.Status,
			Result: task.Result,
			Error:  task.Error,
		})

	case http.MethodDelete:
		task, err := s.dispatcher.Cancel(id)
		if err != nil {
			switch err.Error() {
			case "task not found":
				writeError(w, http.StatusNotFound, err.Error())
			default:
				writeError(w, http.StatusConflict, err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, taskResponse{ID: task.ID, Status: task.Status})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleChat submits a task and blocks until it completes, returning the result synchronously.
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
	task, err := s.dispatcher.Submit(r.Context(), req.Goal, req.Agent, req.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := s.dispatcher.Wait(r.Context(), task.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, taskResponse{
		ID:        result.ID,
		Status:    result.Status,
		SessionID: result.SessionID,
		Result:    result.Result,
		Error:     result.Error,
	})
}

// handleEvents streams task and agent lifecycle events as Server-Sent Events.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "event bus not available")
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

	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	writeSSE(w, "ping", "{}")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			data, err := json.Marshal(e.Payload)
			if err != nil {
				continue
			}
			writeSSE(w, e.Type, string(data))
			flusher.Flush()
		case <-ping.C:
			writeSSE(w, "ping", "{}")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
