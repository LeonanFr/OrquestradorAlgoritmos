package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Tournament struct {
	ID              bson.ObjectID `bson:"_id,omitempty" json:"id"`
	Name            string        `bson:"name" json:"name"`
	Status          string        `bson:"status" json:"status"`
	DurationMinutes int           `bson:"duration_minutes" json:"durationMinutes"`
	RotationConfig  struct {
		PlayerMinutes   int `bson:"player_minutes" json:"playerMinutes"`
		HandoverSeconds int `bson:"handover_seconds" json:"handoverSeconds"`
		FinalExtraMin   int `bson:"final_extra_minutes" json:"finalExtraMinutes"`
	} `bson:"rotation_config" json:"rotationConfig"`
	Cooldowns struct {
		TestSeconds   int `bson:"test_seconds" json:"testSeconds"`
		SubmitSeconds int `bson:"submit_seconds" json:"submitSeconds"`
	} `bson:"cooldowns" json:"cooldowns"`
	APIConfig struct {
		TeamValidateRoute       string `bson:"team_validate_route" json:"teamValidateRoute"`
		ChallengeCompletedRoute string `bson:"challenge_completed_route" json:"challengeCompletedRoute"`
	} `bson:"api_config" json:"apiConfig"`
	ChallengeIDs []string   `bson:"challenge_ids" json:"challengeIds"`
	StartTime    *time.Time `bson:"start_time" json:"startTime"`
	EndTime      *time.Time `bson:"end_time" json:"endTime"`
}

type Team struct {
	ID                     bson.ObjectID `bson:"_id,omitempty" json:"id"`
	TeamCode               string        `bson:"team_code" json:"teamCode"`
	TournamentID           bson.ObjectID `bson:"tournament_id" json:"tournamentId"`
	Completed              bool          `bson:"completed" json:"completed"`
	CompletedAt            *time.Time    `bson:"completed_at" json:"completedAt"`
	LastTestSubmissionAt   *time.Time    `bson:"last_test_submission_at" json:"lastTestSubmissionAt"`
	LastSubmitSubmissionAt *time.Time    `bson:"last_submit_submission_at" json:"lastSubmitSubmissionAt"`
}

type Challenge struct {
	ID            string `bson:"_id" json:"id"`
	Title         string `bson:"title" json:"title"`
	Description   string `bson:"description" json:"description"`
	InputFormat   string `bson:"input_format" json:"inputFormat"`
	OutputFormat  string `bson:"output_format" json:"outputFormat"`
	Constraints   string `bson:"constraints" json:"constraints"`
	TimeLimitSec  int    `bson:"time_limit_sec" json:"timeLimitSec"`
	MemoryLimitMB int    `bson:"memory_limit_mb" json:"memoryLimitMB"`
	Samples       []struct {
		Input  string `bson:"input" json:"input"`
		Output string `bson:"output" json:"output"`
	} `bson:"samples" json:"samples"`
	TestCases []struct {
		Input    string `bson:"input" json:"input"`
		Expected string `bson:"expected" json:"expected"`
	} `bson:"test_cases" json:"testCases"`
}

type Submission struct {
	ID             bson.ObjectID `bson:"_id,omitempty" json:"id"`
	TeamID         bson.ObjectID `bson:"team_id" json:"teamId"`
	TournamentID   bson.ObjectID `bson:"tournament_id" json:"tournamentId"`
	ChallengeID    string        `bson:"challenge_id" json:"challengeId"`
	Type           string        `bson:"type" json:"type"`
	Timestamp      time.Time     `bson:"timestamp" json:"timestamp"`
	Verdict        string        `bson:"verdict" json:"verdict"`
	ExecutorURL    string        `bson:"executor_url" json:"executorUrl"`
	ResponseTimeMs int64         `bson:"response_time_ms" json:"responseTimeMs"`
	ErrorMessage   string        `bson:"error_message,omitempty" json:"errorMessage,omitempty"`
}

type ExecutorNodeDB struct {
	ID     bson.ObjectID `bson:"_id,omitempty" json:"id"`
	URL    string        `bson:"url" json:"url"`
	Active bool          `bson:"active" json:"active"`
}

type CentralTeamResponse struct {
	Exists      bool `json:"exists"`
	Integrantes []struct {
		Matricula string `json:"matricula"`
		Nome      string `json:"nome"`
	} `json:"integrantes"`
}

type ExecutorPayload struct {
	Code      string   `json:"code"`
	Language  string   `json:"language"`
	Inputs    []string `json:"inputs"`
	Expected  []string `json:"expected"`
	Mode      string   `json:"mode"`
	TimeLimit int      `json:"timeLimit"`
	MemLimit  int      `json:"memLimit"`
}

type ExecutorResponse struct {
	Verdict   string      `json:"verdict"`
	Message   string      `json:"message,omitempty"`
	TestCases interface{} `json:"testCases,omitempty"`
	TotalTime float64     `json:"totalTime,omitempty"`
}

type SubmitRequest struct {
	TournamentID string `json:"tournamentId"`
	TeamCode     string `json:"teamCode"`
	ChallengeID  string `json:"challengeId"`
	Type         string `json:"type"`
	Language     string `json:"language"`
	Code         string `json:"code"`
}

type TeamStatusResponse struct {
	Completed bool `json:"completed"`
	Cooldown  struct {
		Test   int `json:"test"`
		Submit int `json:"submit"`
	} `json:"cooldown"`
	Tournament struct {
		RemainingSeconds int `json:"remainingSeconds"`
	} `json:"tournament"`
}
