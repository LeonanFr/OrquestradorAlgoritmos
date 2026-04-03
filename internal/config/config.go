package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                   string
	MongoURI               string
	RequestTimeoutSec      int
	HealthCheckIntervalSec int
	APIAuthToken           string
	ExecutorAuthToken      string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	reqTimeout, err := strconv.Atoi(os.Getenv("REQUEST_TIMEOUT_SEC"))
	if err != nil {
		reqTimeout = 30
	}

	hcInterval, err := strconv.Atoi(os.Getenv("HEALTH_CHECK_INTERVAL_SEC"))
	if err != nil {
		hcInterval = 10
	}

	return &Config{
		Port:                   port,
		MongoURI:               mongoURI,
		RequestTimeoutSec:      reqTimeout,
		HealthCheckIntervalSec: hcInterval,
		APIAuthToken:           os.Getenv("API_AUTH_TOKEN"),
		ExecutorAuthToken:      os.Getenv("EXECUTOR_AUTH_TOKEN"),
	}
}
