package llm

import (
	"fmt"
	"log/slog"
	"os"

	pb "microchat.ai/proto"
)

// NewProvider creates a provider based on the model type
func NewProvider(model pb.Model, logger *slog.Logger) Provider {
	// Check if we're in development mode for Echo provider
	isDev := os.Getenv("APP_ENV") == "development"

	switch model {
	case pb.Model_GEMINI_2_5_FLASH_LITE:
		provider, err := NewGeminiProvider()
		if err != nil {
			logger.Warn("failed to create Gemini provider, falling back to Echo", "error", err)
			return NewEchoProvider()
		}
		return provider
	case pb.Model_ECHO:
		if !isDev {
			logger.Warn("Echo provider requested in production environment, falling back to Gemini", "model", model.String())
			provider, err := NewGeminiProvider()
			if err != nil {
				logger.Error("failed to create Gemini fallback provider", "error", err)
				return NewEchoProvider() // Last resort
			}
			return provider
		}
		logger.Info("using Echo provider for development", "model", model.String())
		return NewEchoProvider()
	default:
		// Default to Gemini for unknown models in production, Echo in development
		if isDev {
			logger.Info("unknown model in development, using Echo provider", "model", model.String())
			return NewEchoProvider()
		} else {
			logger.Warn("unknown model in production, falling back to Gemini", "model", model.String())
			provider, err := NewGeminiProvider()
			if err != nil {
				logger.Error("failed to create Gemini fallback provider", "error", err)
				return NewEchoProvider() // Last resort
			}
			return provider
		}
	}
}

// GetProviderName returns a human-readable name for the model
func GetProviderName(model pb.Model) string {
	switch model {
	case pb.Model_GEMINI_2_5_FLASH_LITE:
		return "Gemini-2.5-Flash-Lite"
	case pb.Model_ECHO:
		return "Echo (Dev/Test)"
	default:
		return fmt.Sprintf("Unknown Model %d", int(model))
	}
}
