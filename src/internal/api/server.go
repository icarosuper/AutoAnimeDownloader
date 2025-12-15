package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/frontend"
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"io"
	"io/fs"
	"net/http"
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
	// Use embedded frontend filesystem
	// The embed includes dist/*, so we need to use fs.Sub to get the dist directory
	distFS, err := fs.Sub(frontend.DistFS, "dist")
	if err != nil {
		logger.Logger.Fatal().
			Err(err).
			Msg("Failed to create sub filesystem for frontend dist")
	}

	logger.Logger.Info().
		Msg("Serving frontend from embedded filesystem")

	fileServer := http.FileServer(http.FS(distFS))

	return func(w http.ResponseWriter, r *http.Request) {
		// Don't serve static files for API routes
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/swagger/") {
			http.NotFound(w, r)
			return
		}

		path := r.URL.Path
		
		// For SPA routing: serve index.html for paths without file extensions
		if path == "/" || (!strings.Contains(path, ".") && path != "/") {
			// Check if the requested path exists as a file
			if path != "/" {
				file, err := distFS.Open(strings.TrimPrefix(path, "/"))
				if err == nil {
					// File exists, serve it normally
					file.Close()
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			// File doesn't exist or it's root path, serve index.html for SPA routing
			// Read and serve the file directly to avoid redirect loops
			file, err := distFS.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer file.Close()
			
			// Set content type
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			
			// Copy file content to response
			_, err = io.Copy(w, file)
			if err != nil {
				logger.Logger.Error().Err(err).Msg("Failed to serve index.html")
			}
			return
		}

		// Serve the file normally for paths with extensions
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
