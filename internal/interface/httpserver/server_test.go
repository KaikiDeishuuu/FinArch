package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLegacyServerFailsClosed(t *testing.T) {
	srv := NewServer(":0", nil, nil, nil, nil)
	if err := srv.Run(); err == nil {
		t.Fatal("legacy unauthenticated server unexpectedly started")
	}
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/transactions", nil))
	if recorder.Code != http.StatusGone {
		t.Fatalf("legacy handler status = %d, want %d", recorder.Code, http.StatusGone)
	}
}
