package database

import (
	"context"
	"log"
	"time"

	"orchestrator/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type DB struct {
	client      *mongo.Client
	dbName      string
	tournaments *mongo.Collection
	teams       *mongo.Collection
	challenges  *mongo.Collection
	submissions *mongo.Collection
	executors   *mongo.Collection
}

func NewDB(uri string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	dbName := "algoritmos"
	return &DB{
		client:      client,
		dbName:      dbName,
		tournaments: client.Database(dbName).Collection("tournaments"),
		teams:       client.Database(dbName).Collection("teams"),
		challenges:  client.Database(dbName).Collection("challenges"),
		submissions: client.Database(dbName).Collection("submissions"),
		executors:   client.Database(dbName).Collection("executors"),
	}, nil
}

func (db *DB) GetTournament(ctx context.Context, id string) (*models.Tournament, error) {
	objID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var t models.Tournament
	err = db.tournaments.FindOne(ctx, bson.M{"_id": objID}).Decode(&t)
	return &t, err
}

func (db *DB) GetTournamentsByStatus(ctx context.Context, statuses []string) ([]models.Tournament, error) {
	coll := db.client.Database("orchestrator").Collection("tournaments")

	filter := bson.M{"status": bson.M{"$in": statuses}}

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		log.Printf("Erro na busca: %v", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []models.Tournament
	if err := cursor.All(ctx, &list); err != nil {
		log.Printf("Erro no decode: %v", err)
		return nil, err
	}

	return list, nil
}

func (db *DB) UpdateTournamentStatus(ctx context.Context, id string, status string, startTime, endTime *time.Time) error {
	objID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"start_time": startTime,
			"end_time":   endTime,
			"updated_at": time.Now(),
		},
	}
	_, err = db.tournaments.UpdateOne(ctx, bson.M{"_id": objID}, update)
	return err
}

func (db *DB) GetTeamByCode(ctx context.Context, code string, tournamentID bson.ObjectID) (*models.Team, error) {
	var t models.Team
	err := db.teams.FindOne(ctx, bson.M{"team_code": code, "tournament_id": tournamentID}).Decode(&t)
	return &t, err
}

func (db *DB) CreateTeam(ctx context.Context, team *models.Team) error {
	res, err := db.teams.InsertOne(ctx, team)
	if err == nil {
		team.ID = res.InsertedID.(bson.ObjectID)
	}
	return err
}

func (db *DB) UpdateTeamSubmissionTime(ctx context.Context, teamID bson.ObjectID, subType string) error {
	field := "last_test_submission_at"
	if subType == "submit" {
		field = "last_submit_submission_at"
	}
	update := bson.M{"$set": bson.M{field: time.Now()}}
	_, err := db.teams.UpdateOne(ctx, bson.M{"_id": teamID}, update)
	return err
}

func (db *DB) MarkTeamCompleted(ctx context.Context, teamID bson.ObjectID) (bool, error) {
	filter := bson.M{"_id": teamID, "completed": false}
	update := bson.M{"$set": bson.M{"completed": true, "completed_at": time.Now()}}
	res, err := db.teams.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0, nil
}

func (db *DB) GetChallenge(ctx context.Context, id string) (*models.Challenge, error) {
	var c models.Challenge
	err := db.challenges.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	return &c, err
}

func (db *DB) GetChallengesByIDs(ctx context.Context, ids []string) ([]models.Challenge, error) {
	cursor, err := db.challenges.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var challenges []models.Challenge
	if err := cursor.All(ctx, &challenges); err != nil {
		return nil, err
	}
	return challenges, nil
}

func (db *DB) InsertSubmission(ctx context.Context, sub *models.Submission) error {
	res, err := db.submissions.InsertOne(ctx, sub)
	if err == nil {
		sub.ID = res.InsertedID.(bson.ObjectID)
	}
	return err
}

func (db *DB) GetActiveExecutors(ctx context.Context) ([]models.ExecutorNodeDB, error) {
	cursor, err := db.executors.Find(ctx, bson.M{"active": true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var execs []models.ExecutorNodeDB
	if err := cursor.All(ctx, &execs); err != nil {
		return nil, err
	}
	return execs, nil
}

func (db *DB) Close(ctx context.Context) error {
	return db.client.Disconnect(ctx)
}
