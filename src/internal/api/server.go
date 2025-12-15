package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	httpSwagger "github.com/swaggo/http-swagger"
)

// FileManagerInterface defines the interface for file management operations
// This interface matches daemon.FileManagerInterface
type FileManagerInterface interface {
	LoadConfigs() (*files.Config, error)
	SaveConfigs(config *files.Config) error
	LoadSavedEpisodes() ([]files.EpisodeStruct, error)
	SaveEpisodesToFile(episodes []files.EpisodeStruct) error
	DeleteEpisodesFromFile(episodeIds []int) error
	DeleteEmptyFolders(savePath string, completedAnimeSaveFolder string) error
}

type Server struct {
	http.Server
	State         *daemon.State
	FileManager   FileManagerInterface
	StartLoopFunc func(daemon.StartLoopPayload) func(time.Duration)
	WSManager     *WebSocketManager

	mu                 sync.Mutex
	currentLoopControl func(time.Duration)
	cancelLoop         context.CancelFunc
}

func NewServer(port string, state *daemon.State, fileManager FileManagerInterface, startLoopFunc func(daemon.StartLoopPayload) func(time.Duration)) *Server {
	wsManager := NewWebSocketManager()
	
	// Set state getter for WebSocket manager
	wsManager.SetStateGetter(func() (daemon.Status, time.Time, bool) {
		return state.GetAll()
	})
	
	server := &Server{
		State:         state,
		FileManager:   fileManager,
		StartLoopFunc: startLoopFunc,
		WSManager:     wsManager,
	}

	server.Server = http.Server{
		Addr:    port,
		Handler: nil, // Will be configured in SetupRoutes
	}

	return server
}

func (s *Server) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// API routes - apply JSON middleware only to API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/v1/status", handleStatus(s))
	apiMux.HandleFunc("/api/v1/config", handleConfig(s))
	apiMux.HandleFunc("/api/v1/animes", handleAnimes(s))
	apiMux.HandleFunc("/api/v1/episodes", handleEpisodes(s))
	apiMux.HandleFunc("/api/v1/check", handleCheck(s))
	apiMux.HandleFunc("/api/v1/daemon/start", handleDaemonStart(s))
	apiMux.HandleFunc("/api/v1/daemon/stop", handleDaemonStop(s))
	
	// WebSocket route (no JSON middleware)
	mux.HandleFunc("/api/v1/ws", s.handleWebSocket())
	
	// Apply middlewares to API routes
	mux.Handle("/api/", ApplyMiddlewares(apiMux))
	mux.Handle("/swagger/", ApplyMiddlewares(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)))

	// Static files (frontend) - apply only CORS and logging, not JSON middleware
	staticHandler := corsMiddleware(loggingMiddleware(s.handleStaticFiles()))
	mux.Handle("/", staticHandler)

	return mux
}

func (s *Server) handleStaticFiles() http.HandlerFunc {
	// Get the frontend dist directory
	// Try multiple possible paths
	var frontendDir string
	
	// Get current working directory first
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	
	// Try paths relative to current working directory
	possiblePaths := []string{
		filepath.Join(cwd, "src", "internal", "frontend", "dist"),
		"src/internal/frontend/dist",
		"./src/internal/frontend/dist",
		filepath.Join(filepath.Dir(os.Args[0]), "..", "src", "internal", "frontend", "dist"),
		filepath.Join(filepath.Dir(os.Args[0]), "src", "internal", "frontend", "dist"),
	}
	
	for _, path := range possiblePaths {
		if absPath, err := filepath.Abs(path); err == nil {
			if info, err := os.Stat(absPath); err == nil && info.IsDir() {
				// Verify it contains index.html
				indexPath := filepath.Join(absPath, "index.html")
				if _, err := os.Stat(indexPath); err == nil {
					frontendDir = absPath
					break
				}
			}
		}
	}
	
	// Check if dist directory exists
	if frontendDir == "" {
		logger.Logger.Warn().
			Msg("Frontend dist directory not found, serving placeholder")
		
		return func(w http.ResponseWriter, r *http.Request) {
			// Don't serve static files for API routes
			if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/swagger/") {
				http.NotFound(w, r)
				return
			}
			
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
				<!DOCTYPE html>
				<html>
				<head><title>Auto Anime Downloader</title></head>
				<body>
					<h1>Frontend not built</h1>
					<p>Please run <code>npm run build</code> in <code>src/internal/frontend/</code> directory.</p>
				</body>
				</html>
			`))
		}
	}

	logger.Logger.Info().
		Str("dir", frontendDir).
		Msg("Serving frontend from directory")

	fileServer := http.FileServer(http.Dir(frontendDir))

	return func(w http.ResponseWriter, r *http.Request) {
		// Don't serve static files for API routes
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/swagger/") {
			http.NotFound(w, r)
			return
		}

		path := r.URL.Path
		
		// Check if the requested file exists
		fullPath := filepath.Join(frontendDir, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			// File exists, serve it
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing: if the path doesn't exist and doesn't have an extension,
		// serve index.html
		if !strings.Contains(path, ".") {
			indexPath := filepath.Join(frontendDir, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
		}

		// Fallback to file server
		fileServer.ServeHTTP(w, r)
	}
}

func (s *Server) Start() error {
	mux := s.SetupRoutes()
	s.Handler = mux

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

func (s *Server) handleWebSocket() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.WSManager.HandleWebSocket(w, r)
	}
}

func (s *Server) Stop(ctx context.Context) error {
	logger.Logger.Info().Msg("Stopping API server")
	
	// Close WebSocket connections
	if s.WSManager != nil {
		s.WSManager.Close()
	}
	
	return s.Shutdown(ctx)
}
