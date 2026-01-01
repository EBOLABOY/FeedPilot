package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/netutil"
	"notebook-podcast-automator/internal/workflow"
)

type Server struct {
	runMu sync.Mutex

	stateMu      sync.RWMutex
	running      bool
	lastStarted  time.Time
	lastFinished time.Time
	lastError    string
	lastResult   *workflow.Result

	currentStage   string
	currentMessage string
}

type RunRequest struct {
	InputURL            string `json:"input_url,omitempty"`
	MaxEntries          int    `json:"max_entries,omitempty"`
	AudioPrompt         string `json:"audio_prompt,omitempty"`
	ProjectEmoji        string `json:"project_emoji,omitempty"`
	PollIntervalSeconds int    `json:"poll_interval_seconds,omitempty"`
	PollTimeoutSeconds  int    `json:"poll_timeout_seconds,omitempty"`
	DownloadsDir        string `json:"downloads_dir,omitempty"`
	KeepLocalAudio      bool   `json:"keep_local_audio,omitempty"`

	StatePath       string `json:"state_path,omitempty"`
	StateMaxEntries int    `json:"state_max_entries,omitempty"`

	FilterMode            string   `json:"filter_mode,omitempty"`
	FilterBlockKeywords   []string `json:"filter_block_keywords,omitempty"`
	FilterAllowKeywords   []string `json:"filter_allow_keywords,omitempty"`
	FilterMinContentChars int      `json:"filter_min_content_chars,omitempty"`
	FilterStrict          bool     `json:"filter_strict,omitempty"`
	FilterLLMBaseURL      string   `json:"filter_llm_base_url,omitempty"`
	FilterLLMModel        string   `json:"filter_llm_model,omitempty"`
	FilterLLMMaxChars     int      `json:"filter_llm_max_chars,omitempty"`
	FilterLLMTimeoutSec   int      `json:"filter_llm_timeout_seconds,omitempty"`
}

type Event struct {
	At      time.Time `json:"at"`
	Stage   string    `json:"stage"`
	Message string    `json:"message"`
}

type RunResponse struct {
	OK     bool             `json:"ok"`
	Noop   bool             `json:"noop,omitempty"`
	Result *workflow.Result `json:"result,omitempty"`
	Events []Event          `json:"events,omitempty"`
	Error  string           `json:"error,omitempty"`
}

type StatusResponse struct {
	OK           bool             `json:"ok"`
	Running      bool             `json:"running"`
	LastStarted  *time.Time       `json:"last_started,omitempty"`
	LastFinished *time.Time       `json:"last_finished,omitempty"`
	LastError    string           `json:"last_error,omitempty"`
	LastNoop     bool             `json:"last_noop,omitempty"`
	LastResult   *workflow.Result `json:"last_result,omitempty"`

	CurrentStage   string `json:"current_stage,omitempty"`
	CurrentMessage string `json:"current_message,omitempty"`
}

func Run() error {
	_ = godotenv.Load()
	netutil.MaybeSetLocalProxy()

	srv := &Server{}
	srv.startSchedulerIfConfigured()
	mux := http.NewServeMux()
	mux.HandleFunc("/status", srv.handleStatus)
	mux.HandleFunc("/run", srv.handleRun)
	mux.HandleFunc("/generate", srv.handleRun)

	httpSrv := &http.Server{
		Addr:              httpAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("[http] listening on %s", httpSrv.Addr)
	return httpSrv.ListenAndServe()
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	s.stateMu.RLock()
	running := s.running
	lastStarted := s.lastStarted
	lastFinished := s.lastFinished
	lastError := s.lastError
	lastResult := s.lastResult
	currentStage := s.currentStage
	currentMessage := s.currentMessage
	s.stateMu.RUnlock()

	var startedPtr *time.Time
	var finishedPtr *time.Time
	if !lastStarted.IsZero() {
		t := lastStarted
		startedPtr = &t
	}
	if !lastFinished.IsZero() {
		t := lastFinished
		finishedPtr = &t
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		OK:             true,
		Running:        running,
		LastStarted:    startedPtr,
		LastFinished:   finishedPtr,
		LastError:      lastError,
		LastNoop:       lastResult != nil && lastResult.Noop,
		LastResult:     lastResult,
		CurrentStage:   currentStage,
		CurrentMessage: currentMessage,
	})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	if !s.runMu.TryLock() {
		writeError(w, http.StatusConflict, "already_running")
		return
	}
	defer s.runMu.Unlock()

	var req RunRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	res, events, err := s.executeRun(r.Context(), req, func(stage, message string) {
		log.Printf("[workflow] %s: %s", stage, message)
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, RunResponse{
			OK:     false,
			Events: events,
			Error:  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, RunResponse{
		OK:     true,
		Noop:   res.Noop,
		Result: &res,
		Events: events,
	})
}

func (s *Server) executeRun(ctx context.Context, req RunRequest, logf func(stage, message string)) (workflow.Result, []Event, error) {
	cfg := workflow.Config{
		InputURL:       defaultInputURL(req.InputURL),
		MaxEntries:     req.MaxEntries,
		AudioPrompt:    req.AudioPrompt,
		ProjectEmoji:   req.ProjectEmoji,
		DownloadsDir:   req.DownloadsDir,
		KeepLocalAudio: req.KeepLocalAudio,

		StatePath:       req.StatePath,
		StateMaxEntries: req.StateMaxEntries,

		FilterMode:            req.FilterMode,
		FilterBlockKeywords:   req.FilterBlockKeywords,
		FilterAllowKeywords:   req.FilterAllowKeywords,
		FilterMinContentChars: req.FilterMinContentChars,
		FilterStrict:          req.FilterStrict,
		FilterLLMBaseURL:      req.FilterLLMBaseURL,
		FilterLLMModel:        req.FilterLLMModel,
		FilterLLMMaxChars:     req.FilterLLMMaxChars,
	}
	if req.PollIntervalSeconds > 0 {
		cfg.PollInterval = time.Duration(req.PollIntervalSeconds) * time.Second
	}
	if req.PollTimeoutSeconds > 0 {
		cfg.PollTimeout = time.Duration(req.PollTimeoutSeconds) * time.Second
	}
	if req.FilterLLMTimeoutSec > 0 {
		cfg.FilterLLMTimeout = time.Duration(req.FilterLLMTimeoutSec) * time.Second
	}

	var events []Event
	progress := func(stage, message string) {
		events = append(events, Event{
			At:      time.Now(),
			Stage:   stage,
			Message: message,
		})
		s.stateMu.Lock()
		s.currentStage = stage
		s.currentMessage = message
		s.stateMu.Unlock()
		if logf != nil {
			logf(stage, message)
		}
	}

	started := time.Now()
	s.stateMu.Lock()
	s.running = true
	s.lastStarted = started
	s.lastError = ""
	s.lastResult = nil
	s.currentStage = ""
	s.currentMessage = ""
	s.stateMu.Unlock()

	res, err := workflow.Run(ctx, cfg, workflow.ProgressFunc(progress))

	finished := time.Now()
	s.stateMu.Lock()
	s.running = false
	s.lastFinished = finished
	if err != nil {
		s.lastError = err.Error()
		s.lastResult = nil
	} else {
		s.lastError = ""
		s.lastResult = &res
	}
	s.stateMu.Unlock()

	if err != nil {
		return workflow.Result{}, events, err
	}
	return res, events, nil
}

func httpAddr() string {
	if v := strings.TrimSpace(os.Getenv("NPA_HTTP_ADDR")); v != "" {
		return v
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = strings.TrimSpace(os.Getenv("NPA_PORT"))
	}
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err == nil {
		return ":" + port
	}
	if strings.HasPrefix(port, ":") {
		return port
	}
	return ":" + port
}

func defaultInputURL(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if v := strings.TrimSpace(os.Getenv("NPA_INPUT_URL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("NLM_INPUT_TARGET")); v != "" {
		return v
	}
	return "http://192.168.100.3:10082/atom"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": code,
	})
}
