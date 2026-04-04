package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"orchestrator/internal/api"
	"orchestrator/internal/config"
	"orchestrator/internal/database"
	"orchestrator/internal/service"
	"orchestrator/internal/ws"

	"github.com/gorilla/mux"
)

func main() {
	cfg := config.Load()

	db, err := database.NewDB(cfg.MongoURI)
	if err != nil {
		log.Fatalf("falha ao conectar no mongo: %v", err)
	}
	defer func() {
		_ = db.Close(context.Background())
	}()

	wsManager := ws.NewWSManager()
	go wsManager.StartBroadcasting()

	svc := service.NewOrchestrator(db, cfg.ExecutorAuthToken, cfg.RequestTimeoutSec, cfg.HealthCheckIntervalSec, wsManager)
	handler := api.NewHandler(svc)

	r := mux.NewRouter()
	handler.RegisterRoutes(r)
	r.HandleFunc("/ws", wsManager.HandleConnections)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: api.CORSMiddleware(r),
	}

	go func() {
		log.Printf("Orquestrador rodando na porta %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("erro no servidor: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Encerrando orquestrador...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("erro ao encerrar servidor: %v", err)
	}
}
