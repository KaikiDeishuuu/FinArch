package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"finarch/internal/infrastructure/ocr"
)

func TestRunRejectsInvalidProxyConfigurationBeforeStartup(t *testing.T) {
	tests := []struct {
		name    string
		behind  string
		trusted string
		message string
	}{
		{name: "invalid opt-in", behind: "tru", trusted: "127.0.0.1/32", message: "FINARCH_BEHIND_PROXY must be exactly true or false"},
		{name: "trust-all IPv4", behind: "true", trusted: "0.0.0.0/0", message: "must not contain an all-address /0 network"},
		{name: "trust-all IPv6", behind: "true", trusted: "::/0", message: "must not contain an all-address /0 network"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FINARCH_BEHIND_PROXY", tt.behind)
			t.Setenv("FINARCH_TRUSTED_PROXY_CIDRS", tt.trusted)

			err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("run() error = %v, want proxy configuration failure containing %q", err, tt.message)
			}
		})
	}
}

func TestLoadHTTPServerConfigDefaultsAndOverrides(t *testing.T) {
	envNames := []string{
		"FINARCH_HTTP_READ_HEADER_TIMEOUT",
		"FINARCH_HTTP_READ_TIMEOUT",
		"FINARCH_HTTP_WRITE_TIMEOUT",
		"FINARCH_HTTP_IDLE_TIMEOUT",
		"FINARCH_HTTP_SHUTDOWN_TIMEOUT",
		"FINARCH_HTTP_MAX_HEADER_BYTES",
	}
	for _, name := range envNames {
		t.Setenv(name, "")
	}

	defaults := loadHTTPServerConfig()
	if defaults.readHeaderTimeout != defaultHTTPReadHeaderTimeout ||
		defaults.readTimeout != defaultHTTPReadTimeout ||
		defaults.writeTimeout != defaultHTTPWriteTimeout ||
		defaults.idleTimeout != defaultHTTPIdleTimeout ||
		defaults.shutdownTimeout != defaultHTTPShutdownTimeout ||
		defaults.maxHeaderBytes != defaultHTTPMaxHeaderBytes {
		t.Fatalf("loadHTTPServerConfig() defaults = %#v", defaults)
	}

	t.Setenv("FINARCH_HTTP_READ_HEADER_TIMEOUT", "7s")
	t.Setenv("FINARCH_HTTP_READ_TIMEOUT", "31m")
	t.Setenv("FINARCH_HTTP_WRITE_TIMEOUT", "32m")
	t.Setenv("FINARCH_HTTP_IDLE_TIMEOUT", "90s")
	t.Setenv("FINARCH_HTTP_SHUTDOWN_TIMEOUT", "4m")
	t.Setenv("FINARCH_HTTP_MAX_HEADER_BYTES", "98304")
	overrides := loadHTTPServerConfig()
	if overrides.readHeaderTimeout != 7*time.Second ||
		overrides.readTimeout != 31*time.Minute ||
		overrides.writeTimeout != 32*time.Minute ||
		overrides.idleTimeout != 90*time.Second ||
		overrides.shutdownTimeout != 4*time.Minute ||
		overrides.maxHeaderBytes != 98304 {
		t.Fatalf("loadHTTPServerConfig() overrides = %#v", overrides)
	}
}

func TestLoadHTTPServerConfigRejectsNonPositiveValues(t *testing.T) {
	t.Setenv("FINARCH_HTTP_READ_HEADER_TIMEOUT", "0")
	t.Setenv("FINARCH_HTTP_READ_TIMEOUT", "-1s")
	t.Setenv("FINARCH_HTTP_WRITE_TIMEOUT", "invalid")
	t.Setenv("FINARCH_HTTP_IDLE_TIMEOUT", "0s")
	t.Setenv("FINARCH_HTTP_SHUTDOWN_TIMEOUT", "-1m")
	t.Setenv("FINARCH_HTTP_MAX_HEADER_BYTES", "0")

	got := loadHTTPServerConfig()
	if got.readHeaderTimeout != defaultHTTPReadHeaderTimeout ||
		got.readTimeout != defaultHTTPReadTimeout ||
		got.writeTimeout != defaultHTTPWriteTimeout ||
		got.idleTimeout != defaultHTTPIdleTimeout ||
		got.shutdownTimeout != defaultHTTPShutdownTimeout ||
		got.maxHeaderBytes != defaultHTTPMaxHeaderBytes {
		t.Fatalf("loadHTTPServerConfig() invalid-value fallbacks = %#v", got)
	}
}

func TestNewProductionHTTPServer(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	config := httpServerConfig{
		readHeaderTimeout: 3 * time.Second,
		readTimeout:       4 * time.Minute,
		writeTimeout:      5 * time.Minute,
		idleTimeout:       6 * time.Second,
		shutdownTimeout:   7 * time.Second,
		maxHeaderBytes:    8192,
	}

	server := newProductionHTTPServer("127.0.0.1:0", handler, config)
	if server.Addr != "127.0.0.1:0" ||
		server.ReadHeaderTimeout != config.readHeaderTimeout ||
		server.ReadTimeout != config.readTimeout ||
		server.WriteTimeout != config.writeTimeout ||
		server.IdleTimeout != config.idleTimeout ||
		server.MaxHeaderBytes != config.maxHeaderBytes {
		t.Fatalf("newProductionHTTPServer() = %#v", server)
	}
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called || recorder.Code != http.StatusNoContent {
		t.Fatalf("handler called=%v status=%d", called, recorder.Code)
	}
}

type shutdownObservation struct {
	errAtCall   error
	hasDeadline bool
	timeLeft    time.Duration
}

type fakeManagedHTTPServer struct {
	started              chan struct{}
	stopped              chan struct{}
	shutdownObservations chan shutdownObservation
	closeCalled          chan struct{}
	listenErr            error
	shutdownErr          error
	closeErr             error
	immediate            bool
	stopOnce             sync.Once
}

func newFakeManagedHTTPServer() *fakeManagedHTTPServer {
	return &fakeManagedHTTPServer{
		started:              make(chan struct{}),
		stopped:              make(chan struct{}),
		shutdownObservations: make(chan shutdownObservation, 1),
		closeCalled:          make(chan struct{}, 1),
		listenErr:            http.ErrServerClosed,
	}
}

func (s *fakeManagedHTTPServer) ListenAndServe() error {
	close(s.started)
	if !s.immediate {
		<-s.stopped
	}
	return s.listenErr
}

func (s *fakeManagedHTTPServer) Shutdown(ctx context.Context) error {
	deadline, hasDeadline := ctx.Deadline()
	observation := shutdownObservation{errAtCall: ctx.Err(), hasDeadline: hasDeadline}
	if hasDeadline {
		observation.timeLeft = time.Until(deadline)
	}
	s.shutdownObservations <- observation
	if s.shutdownErr == nil {
		s.stop()
	}
	return s.shutdownErr
}

func (s *fakeManagedHTTPServer) Close() error {
	s.closeCalled <- struct{}{}
	s.stop()
	return s.closeErr
}

func (s *fakeManagedHTTPServer) stop() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

func TestServeHTTPServerUsesIndependentGracefulShutdownContext(t *testing.T) {
	server := newFakeManagedHTTPServer()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveHTTPServer(ctx, server, 2*time.Second) }()
	<-server.started
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveHTTPServer() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serveHTTPServer did not return after cancellation")
	}

	observation := <-server.shutdownObservations
	if observation.errAtCall != nil {
		t.Fatalf("shutdown context was already cancelled: %v", observation.errAtCall)
	}
	if !observation.hasDeadline || observation.timeLeft <= 0 || observation.timeLeft > 2*time.Second {
		t.Fatalf("shutdown deadline observation = %#v", observation)
	}
	select {
	case <-server.closeCalled:
		t.Fatal("Close called after successful graceful shutdown")
	default:
	}
}

func TestServeHTTPServerForceClosesAfterShutdownFailure(t *testing.T) {
	shutdownErr := errors.New("shutdown timed out")
	closeErr := errors.New("close failed")
	server := newFakeManagedHTTPServer()
	server.shutdownErr = shutdownErr
	server.closeErr = closeErr
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveHTTPServer(ctx, server, time.Second) }()
	<-server.started
	cancel()

	err := <-done
	if !errors.Is(err, shutdownErr) || !errors.Is(err, closeErr) {
		t.Fatalf("serveHTTPServer() error = %v, want shutdown and close errors", err)
	}
	select {
	case <-server.closeCalled:
	default:
		t.Fatal("Close was not called after failed graceful shutdown")
	}
}

func TestServeHTTPServerReturnsUnexpectedListenError(t *testing.T) {
	wantErr := errors.New("listen failed")
	server := newFakeManagedHTTPServer()
	server.immediate = true
	server.listenErr = wantErr

	err := serveHTTPServer(context.Background(), server, time.Second)
	if !errors.Is(err, wantErr) {
		t.Fatalf("serveHTTPServer() error = %v, want %v", err, wantErr)
	}
	select {
	case <-server.shutdownObservations:
		t.Fatal("Shutdown called after listener failed")
	default:
	}
}

func TestServeHTTPServerTreatsErrServerClosedAsClean(t *testing.T) {
	server := newFakeManagedHTTPServer()
	server.immediate = true
	if err := serveHTTPServer(context.Background(), server, time.Second); err != nil {
		t.Fatalf("serveHTTPServer() error = %v", err)
	}
}

func TestRunPeriodicWorkerCancellationInterruptsInitialDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runCalled := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runPeriodicWorker(ctx, time.Hour, time.Hour, func(context.Context) {
			runCalled <- struct{}{}
		})
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("periodic worker did not stop during initial delay")
	}
	select {
	case <-runCalled:
		t.Fatal("periodic task ran after cancellation")
	default:
	}
}

func TestBackgroundWorkersWaitForCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	workers := &backgroundWorkers{}
	exited := make(chan struct{})
	workers.Go(ctx, func(ctx context.Context) {
		<-ctx.Done()
		close(exited)
	})
	waited := make(chan struct{})
	go func() {
		workers.Wait()
		close(waited)
	}()

	select {
	case <-waited:
		t.Fatal("Wait returned before the worker exited")
	case <-time.After(10 * time.Millisecond):
	}
	cancel()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after cancellation")
	}
	select {
	case <-exited:
	default:
		t.Fatal("worker had not exited when Wait returned")
	}
}

func TestBuildOCRProviderSelection(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantName string
	}{
		{name: "empty", provider: "", wantName: "none"},
		{name: "none", provider: "none", wantName: "none"},
		{name: "sidecar", provider: "paddle", wantName: "paddle"},
		{name: "aistudio", provider: "paddle_aistudio", wantName: ocr.PaddleAIStudioProviderName},
		{name: "aistudio alias", provider: "aistudio", wantName: ocr.PaddleAIStudioProviderName},
		{name: "unknown", provider: "missing", wantName: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FINARCH_OCR_PROVIDER", tt.provider)
			t.Setenv("FINARCH_OCR_URL", "http://ocr.local/ocr")
			t.Setenv("FINARCH_OCR_AISTUDIO_TOKEN", "test-token")
			t.Setenv("FINARCH_OCR_AISTUDIO_MODEL", "PaddleOCR-VL-1.6")

			provider := buildOCRProvider()
			if got := provider.Name(); got != tt.wantName {
				t.Fatalf("provider.Name() = %q, want %q", got, tt.wantName)
			}
			if tt.wantName == ocr.PaddleAIStudioProviderName && !provider.Available(context.Background()) {
				t.Fatal("AIStudio provider should be available with token and model")
			}
		})
	}
}
