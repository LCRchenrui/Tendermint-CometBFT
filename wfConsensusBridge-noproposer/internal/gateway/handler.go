package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"wfconsensusbridge/internal/nacos"
)

func (g *Gateway) HandleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "message": "POST required"})
		return
	}

	var req RunRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ServiceName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "message": "s-serviceName is required"})
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "message": "s-url is required"})
		return
	}
	if strings.TrimSpace(req.Method) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "message": "s-method is required"})
		return
	}

	resp, err := g.forward(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"code": 503, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (g *Gateway) HandleServiceRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "message": "POST required"})
		return
	}
	var req nacos.RegisterInstanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GroupName == "" {
		req.GroupName = g.defaultGroup
	}
	if err := g.nacos.RegisterInstance(r.Context(), req); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"code": 502, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "message": "registered"})
}

func (g *Gateway) HandleServiceDeregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "message": "POST or DELETE required"})
		return
	}
	var req nacos.RegisterInstanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GroupName == "" {
		req.GroupName = g.defaultGroup
	}
	if err := g.nacos.DeregisterInstance(r.Context(), req); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"code": 502, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "message": "deregistered"})
}

func (g *Gateway) HandleServiceResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "message": "GET required"})
		return
	}
	serviceName := strings.TrimSpace(r.URL.Query().Get("serviceName"))
	groupName := strings.TrimSpace(r.URL.Query().Get("groupName"))
	if serviceName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "message": "serviceName is required"})
		return
	}
	if groupName == "" {
		groupName = g.defaultGroup
	}
	insts, err := g.nacos.ListInstances(r.Context(), serviceName, groupName, true)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"code": 502, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":        200,
		"message":     "ok",
		"serviceName": serviceName,
		"groupName":   groupName,
		"instances":   insts,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "message": err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
