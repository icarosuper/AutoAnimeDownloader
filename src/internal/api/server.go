package api

import (
	"AutoAnimeDownloader/src/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"net/http"
	"sync"
	"time"

	httpSwagger "github.com/swaggo/http-swagger"
)

type Server struct {
	http.Server
	State         *daemon.State
	FileManager   *files.FileManager
	StartLoopFunc func(daemon.StartLoopPayload) func(time.Duration)

	mu                 sync.Mutex
	currentLoopControl func(time.Duration)
	cancelLoop         context.CancelFunc
}

func NewServer(port string, state *daemon.State, fileManager *files.FileManager, startLoopFunc func(daemon.StartLoopPayload) func(time.Duration)) *Server {
	server := &Server{
		State:         state,
		FileManager:   fileManager,
		StartLoopFunc: startLoopFunc,
	}

	server.Server = http.Server{
		Addr:    port,
		Handler: nil, // Will be configured in SetupRoutes
	}

	return server
}

func (s *Server) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/status", handleStatus(s))
	mux.HandleFunc("/api/v1/config", handleConfig(s))
	mux.HandleFunc("/api/v1/animes", handleAnimes(s))
	mux.HandleFunc("/api/v1/episodes", handleEpisodes(s))
	mux.HandleFunc("/api/v1/check", handleCheck(s))
	mux.HandleFunc("/api/v1/daemon/start", handleDaemonStart(s))
	mux.HandleFunc("/api/v1/daemon/stop", handleDaemonStop(s))

	mux.HandleFunc("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return mux
}

func (s *Server) Start() error {
	mux := s.SetupRoutes()
	s.Handler = ApplyMiddlewares(mux)

	logger.Logger.Info().
		Str("addr", s.Addr).
		Msg("Starting API server")

	return s.ListenAndServe()
}

func (s *Server) StartDaemonLoop() error {
	configs, err := s.FileManager.LoadConfigs()
	if err != nil {
		return err
	}

	interval := time.Duration(configs.CheckInterval) * time.Minute

	s.mu.Lock()
	if s.cancelLoop != nil {
		s.cancelLoop()
	}

	_, cancel := context.WithCancel(context.Background())
	s.cancelLoop = cancel

	loopControl := s.StartLoopFunc(daemon.StartLoopPayload{
		FileManager: s.FileManager,
		Interval:    interval,
		State:       s.State,
	})
	s.currentLoopControl = loopControl
	s.mu.Unlock()

	return nil
}

func (s *Server) StopDaemonLoop() {
	s.mu.Lock()
	hadLoop := s.cancelLoop != nil
	if s.cancelLoop != nil {
		s.cancelLoop()
		s.cancelLoop = nil
	}
	s.currentLoopControl = nil
	s.mu.Unlock()

	// Update status immediately when stopping
	// The daemon will also update it when it detects cancellation, but this ensures immediate response
	if hadLoop && s.State.GetStatus() != daemon.StatusStopped {
		s.State.SetStatus(daemon.StatusStopped)
	}
}

func (s *Server) Stop(ctx context.Context) error {
	logger.Logger.Info().Msg("Stopping API server")
	return s.Shutdown(ctx)
}
