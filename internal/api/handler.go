package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"orchestrator/internal/models"
	"orchestrator/internal/service"

	"github.com/gorilla/mux"
)

type Handler struct {
	svc *service.Orchestrator
}

func NewHandler(svc *service.Orchestrator) *Handler {
	return &Handler{svc: svc}
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ws" {
			next.ServeHTTP(w, r)
			return
		}

		expectedToken := strings.TrimSpace(os.Getenv("API_AUTH_TOKEN"))
		authHeader := r.Header.Get("Authorization")

		if expectedToken != "" {
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				log.Printf("AUTH_ERROR: Header ausente ou prefixo Bearer faltando")
				http.Error(w, "missing or invalid token", http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			token = strings.TrimSpace(token)

			if token != expectedToken {
				log.Printf("AUTH_MISMATCH: Esperado(len:%d) [%s] | Recebido(len:%d) [%s]",
					len(expectedToken), expectedToken, len(token), token)

				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.Use(h.authMiddleware)

	r.HandleFunc("/health", h.healthCheck).Methods(http.MethodGet, http.MethodHead)
	r.HandleFunc("/admin/tournament/start", h.startTournament).Methods(http.MethodPost)
	r.HandleFunc("/api/team/validate", h.validateTeam).Methods(http.MethodPost)
	r.HandleFunc("/api/team/status", h.getTeamStatus).Methods(http.MethodGet)
	r.HandleFunc("/api/submit", h.submitCode).Methods(http.MethodPost)
	r.HandleFunc("/api/challenges", h.getChallenges).Methods(http.MethodGet)
	r.HandleFunc("/api/tournaments", h.listTournaments).Methods(http.MethodGet)
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte("OK"))
	}
}

func (h *Handler) listTournaments(w http.ResponseWriter, r *http.Request) {
	tList, err := h.svc.ListAvailableTournaments(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(tList)
}

func (h *Handler) startTournament(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		TournamentID string `json:"tournamentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.StartTournament(r.Context(), payload.TournamentID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (h *Handler) validateTeam(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		TournamentID string `json:"tournamentId"`
		TeamCode     string `json:"teamCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	team, centralInfo, err := h.svc.ValidateTeam(r.Context(), payload.TournamentID, payload.TeamCode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	response := map[string]interface{}{
		"team": map[string]interface{}{
			"id":          team.ID.Hex(),
			"code":        team.TeamCode,
			"completed":   team.Completed,
			"integrantes": centralInfo.Integrantes,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) getTeamStatus(w http.ResponseWriter, r *http.Request) {
	tID := r.URL.Query().Get("tournamentId")
	code := r.URL.Query().Get("teamCode")

	if tID == "" || code == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	status, err := h.svc.GetTeamStatus(r.Context(), tID, code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *Handler) getChallenges(w http.ResponseWriter, r *http.Request) {
	tID := r.URL.Query().Get("tournamentId")
	if tID == "" {
		http.Error(w, "missing tournamentId", http.StatusBadRequest)
		return
	}

	challenges, err := h.svc.GetTournamentChallenges(r.Context(), tID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(challenges)
}

func (h *Handler) submitCode(w http.ResponseWriter, r *http.Request) {
	var req models.SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	execRes, remSec, err := h.svc.ProcessSubmission(r.Context(), req)
	if err != nil {
		if remSec > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":            "cooldown",
				"remainingSeconds": remSec,
			})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(execRes)
}
