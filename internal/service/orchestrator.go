package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"orchestrator/internal/database"
	"orchestrator/internal/executorclient"
	"orchestrator/internal/models"
	"orchestrator/internal/ws"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Orchestrator struct {
	db       *database.DB
	executor *executorclient.Client
	httpCli  *http.Client
	ws       *ws.WSManager
}

func NewOrchestrator(db *database.DB, execToken string, execTimeoutSec, healthIntervalSec int, wsManager *ws.WSManager) *Orchestrator {
	fetcher := func() ([]string, error) {
		execs, err := db.GetActiveExecutors(context.Background())
		if err != nil {
			return nil, err
		}
		var urls []string
		for _, e := range execs {
			urls = append(urls, e.URL)
		}
		return urls, nil
	}

	execClient := executorclient.NewClient(fetcher, execToken, execTimeoutSec, healthIntervalSec)

	return &Orchestrator{
		db:       db,
		executor: execClient,
		httpCli:  &http.Client{Timeout: 5 * time.Second},
		ws:       wsManager,
	}
}

func (s *Orchestrator) ListAvailableTournaments(ctx context.Context) ([]models.Tournament, error) {
	return s.db.GetTournamentsByStatus(ctx, []string{"waiting", "active"})
}

func (s *Orchestrator) GetTournamentByID(ctx context.Context, id string) (*models.Tournament, error) {
	return s.db.GetTournament(ctx, id)
}

func (s *Orchestrator) StartTournament(ctx context.Context, id string) error {
	t, err := s.db.GetTournament(ctx, id)
	if err != nil {
		return err
	}

	if t.Status == "active" || t.Status == "finished" {
		return errors.New("torneio ja iniciado ou finalizado")
	}

	now := time.Now()

	ticks := t.DurationMinutes / t.RotationConfig.PlayerMinutes
	totalHandoverSeconds := ticks * t.RotationConfig.HandoverSeconds

	baseDuration := time.Duration(t.DurationMinutes+t.RotationConfig.FinalExtraMin) * time.Minute
	handoverDuration := time.Duration(totalHandoverSeconds) * time.Second

	totalDuration := baseDuration + handoverDuration
	end := now.Add(totalDuration)

	err = s.db.UpdateTournamentStatus(ctx, id, "active", &now, &end)
	if err != nil {
		return err
	}

	s.ws.Notify(map[string]interface{}{
		"event":     "TOURNAMENT_START",
		"startTime": now,
		"endTime":   end,
		"config":    t.RotationConfig,
		"serverNow": time.Now(),
	})

	return nil
}

func (s *Orchestrator) ValidateTeam(ctx context.Context, tournamentID, teamCode string) (*models.Team, *models.CentralTeamResponse, error) {
	t, err := s.db.GetTournament(ctx, tournamentID)
	if err != nil {
		return nil, nil, err
	}

	if t.Status == "finished" {
		return nil, nil, errors.New("torneio encerrado")
	}

	url := strings.Replace(t.APIConfig.TeamValidateRoute, "{code}", teamCode, 1)
	if !strings.Contains(url, teamCode) {
		url = t.APIConfig.TeamValidateRoute + "/" + teamCode
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var centralRes models.CentralTeamResponse
	if err := json.NewDecoder(resp.Body).Decode(&centralRes); err != nil {
		return nil, nil, err
	}

	if !centralRes.Exists {
		return nil, nil, errors.New("equipe nao validada pela central")
	}

	objID, _ := bson.ObjectIDFromHex(tournamentID)
	team, err := s.db.GetTeamByCode(ctx, teamCode, objID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			newTeam := &models.Team{
				TeamCode:     teamCode,
				TournamentID: objID,
				Completed:    false,
			}
			if err := s.db.CreateTeam(ctx, newTeam); err != nil {
				return nil, nil, err
			}
			return newTeam, &centralRes, nil
		}
		return nil, nil, err
	}

	return team, &centralRes, nil
}

func (s *Orchestrator) GetTeamStatus(ctx context.Context, tournamentID, teamCode string) (*models.TeamStatusResponse, error) {
	t, err := s.db.GetTournament(ctx, tournamentID)
	if err != nil {
		return nil, err
	}

	objID, _ := bson.ObjectIDFromHex(tournamentID)
	team, err := s.db.GetTeamByCode(ctx, teamCode, objID)
	if err != nil {
		return nil, errors.New("equipe nao encontrada")
	}

	var res models.TeamStatusResponse
	res.Completed = team.Completed

	now := time.Now()
	if t.EndTime != nil {
		res.Tournament.RemainingSeconds = int(max(0, t.EndTime.Sub(now).Seconds()))
	}

	remTest, _ := s.checkCooldown(team, t, "test")
	remSubmit, _ := s.checkCooldown(team, t, "submit")
	res.Cooldown.Test = remTest
	res.Cooldown.Submit = remSubmit

	return &res, nil
}

func (s *Orchestrator) GetTournamentChallenges(ctx context.Context, tournamentID string) ([]models.Challenge, error) {
	t, err := s.db.GetTournament(ctx, tournamentID)
	if err != nil {
		return nil, err
	}

	challenges, err := s.db.GetChallengesByIDs(ctx, t.ChallengeIDs)
	if err != nil {
		return nil, err
	}

	for i := range challenges {
		challenges[i].TestCases = nil
	}

	return challenges, nil
}

func (s *Orchestrator) ProcessSubmission(ctx context.Context, req models.SubmitRequest) (*models.ExecutorResponse, int, error) {
	t, err := s.db.GetTournament(ctx, req.TournamentID)
	if err != nil {
		return nil, 0, err
	}

	now := time.Now()
	if t.Status != "active" || t.StartTime == nil || now.Before(*t.StartTime) || t.EndTime == nil || now.After(*t.EndTime) {
		return nil, 0, errors.New("torneio inativo ou fora do tempo")
	}

	objID, _ := bson.ObjectIDFromHex(req.TournamentID)
	team, err := s.db.GetTeamByCode(ctx, req.TeamCode, objID)
	if err != nil {
		return nil, 0, errors.New("equipe invalida")
	}

	if req.Type == "submit" && team.Completed {
		return nil, 0, errors.New("equipe ja concluiu o desafio")
	}

	remSec, err := s.checkCooldown(team, t, req.Type)
	if err != nil {
		return nil, remSec, err
	}

	challenge, err := s.db.GetChallenge(ctx, req.ChallengeID)
	if err != nil {
		return nil, 0, errors.New("desafio nao encontrado")
	}

	if len(challenge.TestCases) == 0 {
		return nil, 0, errors.New("desafio sem casos de teste cadastrados")
	}

	var inputs, expected []string
	for _, tc := range challenge.TestCases {
		inputs = append(inputs, tc.Input)
		expected = append(expected, tc.Expected)
	}

	payload := models.ExecutorPayload{
		Code:      req.Code,
		Language:  req.Language,
		Inputs:    inputs,
		Expected:  expected,
		Mode:      req.Type,
		TimeLimit: challenge.TimeLimitSec,
		MemLimit:  challenge.MemoryLimitMB,
	}

	startExec := time.Now()
	execRes, execURL, errExec := s.executor.Execute(ctx, payload)
	execDuration := time.Since(startExec).Milliseconds()

	if req.Type == "submit" {
		if execRes != nil {
			switch execRes.Verdict {
			case "wrong_answer":
				execRes.Message = "Wrong Answer"
				execRes.TestCases = nil
			case "accepted":
				execRes.Message = "Accepted"
				execRes.TestCases = nil
			case "compilation_error", "runtime_error", "time_limit_exceeded":
				execRes.TestCases = nil
			default:
				if execRes.Message == "" {
					execRes.Message = "Submission failed"
				}
				execRes.TestCases = nil
			}
		}
	}

	sub := &models.Submission{
		TeamID:         team.ID,
		TournamentID:   t.ID,
		ChallengeID:    challenge.ID,
		Type:           req.Type,
		Timestamp:      time.Now(),
		ExecutorURL:    execURL,
		ResponseTimeMs: execDuration,
	}

	if errExec != nil {
		sub.Verdict = "system_error"
		sub.ErrorMessage = errExec.Error()
		_ = s.db.InsertSubmission(context.Background(), sub)
		return nil, 0, errExec
	}

	sub.Verdict = execRes.Verdict
	_ = s.db.InsertSubmission(context.Background(), sub)
	_ = s.db.UpdateTeamSubmissionTime(context.Background(), team.ID, req.Type)

	if req.Type == "submit" && execRes.Verdict == "accepted" {
		updated, _ := s.db.MarkTeamCompleted(context.Background(), team.ID)
		if updated {
			go s.notifyCentralCompleted(t.APIConfig.ChallengeCompletedRoute, req.TournamentID, req.TeamCode)
		}
	}

	return execRes, 0, nil
}

func (s *Orchestrator) checkCooldown(team *models.Team, t *models.Tournament, subType string) (int, error) {
	now := time.Now()
	if subType == "test" && team.LastTestSubmissionAt != nil {
		diff := int(now.Sub(*team.LastTestSubmissionAt).Seconds())
		if diff < t.Cooldowns.TestSeconds {
			rem := t.Cooldowns.TestSeconds - diff
			return rem, errors.New("cooldown")
		}
	} else if subType == "submit" && team.LastSubmitSubmissionAt != nil {
		diff := int(now.Sub(*team.LastSubmitSubmissionAt).Seconds())
		if diff < t.Cooldowns.SubmitSeconds {
			rem := t.Cooldowns.SubmitSeconds - diff
			return rem, errors.New("cooldown")
		}
	}
	return 0, nil
}

func (s *Orchestrator) GetPracticeChallenges(ctx context.Context) ([]models.Challenge, error) {
	return s.db.GetPracticeChallenges(ctx)
}

func (s *Orchestrator) ProcessPracticeSubmission(ctx context.Context, challengeID, language, code, subType string) (*models.ExecutorResponse, error) {
	challenge, err := s.db.GetPracticeChallenge(ctx, challengeID)
	if err != nil {
		return nil, errors.New("desafio não encontrado")
	}
	if len(challenge.TestCases) == 0 {
		return nil, errors.New("desafio sem casos de teste cadastrados")
	}

	var inputs, expected []string
	if subType == "test" {
		limit := 3
		if len(challenge.TestCases) < limit {
			limit = len(challenge.TestCases)
		}
		for i := 0; i < limit; i++ {
			inputs = append(inputs, challenge.TestCases[i].Input)
			expected = append(expected, challenge.TestCases[i].Expected)
		}
	} else {
		for _, tc := range challenge.TestCases {
			inputs = append(inputs, tc.Input)
			expected = append(expected, tc.Expected)
		}
	}

	payload := models.ExecutorPayload{
		Code:      code,
		Language:  language,
		Inputs:    inputs,
		Expected:  expected,
		Mode:      subType,
		TimeLimit: challenge.TimeLimitSec,
		MemLimit:  challenge.MemoryLimitMB,
	}

	execRes, _, err := s.executor.Execute(ctx, payload)

	if subType == "submit" {
		if execRes != nil {
			switch execRes.Verdict {
			case "wrong_answer":
				execRes.Message = "Wrong Answer"
				execRes.TestCases = nil
			case "accepted":
				execRes.Message = "Accepted"
				execRes.TestCases = nil
			case "compilation_error", "runtime_error", "time_limit_exceeded":
				execRes.TestCases = nil
			default:
				if execRes.Message == "" {
					execRes.Message = "Submission failed"
				}
				execRes.TestCases = nil
			}
		}
	}

	if err != nil {
		return nil, err
	}
	return execRes, nil
}

func (s *Orchestrator) SaveCode(ctx context.Context, tournamentID bson.ObjectID, teamCode, challengeID, language, code string) error {
	return s.db.SaveCode(ctx, tournamentID, teamCode, challengeID, language, code)
}

func (s *Orchestrator) LoadCode(ctx context.Context, tournamentID bson.ObjectID, teamCode, challengeID string) (string, string, error) {
	return s.db.LoadCode(ctx, tournamentID, teamCode, challengeID)
}

func (s *Orchestrator) notifyCentralCompleted(route, tournamentID, teamCode string) {
	payload := map[string]string{
		"tournamentId": tournamentID,
		"teamCode":     teamCode,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, route, bytes.NewReader(data))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.httpCli.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	}
}
