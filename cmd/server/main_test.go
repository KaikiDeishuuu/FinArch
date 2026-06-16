package main

import (
	"context"
	"testing"

	"finarch/internal/infrastructure/ocr"
)

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
