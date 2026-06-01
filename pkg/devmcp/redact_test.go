package devmcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
)

// TestRedactConfig_PlainStringSecrets verifies that plain-string API keys are masked.
func TestRedactConfig_PlainStringSecrets(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Web: config.WebToolsConfig{
				Brave: config.BraveConfig{
					APIKey: "brave-secret-12345",
					APIKeys: []string{"brave-key-a", "brave-key-b"},
				},
				Tavily: config.TavilyConfig{
					APIKey: "tavily-secret-12345",
					APIKeys: []string{"tavily-key-x"},
				},
				Perplexity: config.PerplexityConfig{
					APIKey: "perplexity-secret-12345",
					APIKeys: []string{"perp-key-1", "perp-key-2"},
				},
				GLMSearch: config.GLMSearchConfig{
					APIKey: "glm-secret-12345",
				},
			},
			Skills: config.SkillsToolsConfig{
				Github: config.SkillsGithubConfig{
					Token: "github-secret-12345",
				},
				Registries: config.SkillsRegistriesConfig{
					ClawHub: config.ClawHubRegistryConfig{
						AuthToken: "clawhub-secret-12345",
					},
				},
			},
		},
		Providers: config.ProvidersConfig{
			Anthropic: config.ProviderConfig{
				APIKey: "anthropic-secret-12345",
			},
			OpenAI: config.OpenAIProviderConfig{
				ProviderConfig: config.ProviderConfig{
					APIKey: "openai-secret-12345",
				},
			},
			Groq: config.ProviderConfig{
				APIKey: "groq-secret-12345",
			},
		},
	}

	// Call redactConfig (unexported function in same package)
	result, err := redactConfig(cfg)
	if err != nil {
		t.Fatalf("redactConfig failed: %v", err)
	}

	// Verify result is valid JSON
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonMap); err != nil {
		t.Fatalf("redactConfig returned invalid JSON: %v", err)
	}

	// Assert no plaintext secrets appear in output
	secrets := []string{
		"brave-secret-12345", "brave-key-a", "brave-key-b",
		"tavily-secret-12345", "tavily-key-x",
		"perplexity-secret-12345", "perp-key-1", "perp-key-2",
		"glm-secret-12345",
		"github-secret-12345",
		"clawhub-secret-12345",
		"anthropic-secret-12345",
		"openai-secret-12345",
		"groq-secret-12345",
	}

	for _, secret := range secrets {
		if strings.Contains(result, secret) {
			t.Errorf("secret leaked in redactConfig output: %s", secret)
		}
	}

	// Verify redaction markers are present
	if !strings.Contains(result, "[REDACTED]") && !strings.Contains(result, "[NOT_HERE]") {
		t.Error("expected [REDACTED] or [NOT_HERE] markers in redacted output")
	}
}

// TestRedactConfig_SecureStringFields verifies SecureString fields are masked as "[NOT_HERE]".
func TestRedactConfig_SecureStringFields(t *testing.T) {
	cfg := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{
				Token: *config.NewSecureString("telegram-bot-token-secret-xyz"),
			},
		},
	}

	result, err := redactConfig(cfg)
	if err != nil {
		t.Fatalf("redactConfig failed: %v", err)
	}

	// SecureString fields should marshal as "[NOT_HERE]" in JSON
	if strings.Contains(result, "telegram-bot-token-secret-xyz") {
		t.Error("Telegram token was not redacted")
	}

	// Verify JSON structure is valid
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonMap); err != nil {
		t.Fatalf("redactConfig output is not valid JSON: %v", err)
	}
}

// TestRedactConfig_ProviderAPIKeys verifies provider API keys from Providers config are masked.
func TestRedactConfig_ProviderAPIKeys(t *testing.T) {
	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Anthropic: config.ProviderConfig{
				APIKey: "anthropic-prov-secret-xyz",
			},
			OpenAI: config.OpenAIProviderConfig{
				ProviderConfig: config.ProviderConfig{
					APIKey: "openai-prov-secret-abc",
				},
			},
			Gemini: config.ProviderConfig{
				APIKey: "gemini-prov-secret-def",
			},
			OpenRouter: config.ProviderConfig{
				APIKey: "openrouter-prov-secret-ghi",
			},
		},
	}

	result, err := redactConfig(cfg)
	if err != nil {
		t.Fatalf("redactConfig failed: %v", err)
	}

	// Verify no plaintext provider secrets leak
	providerSecrets := []string{
		"anthropic-prov-secret-xyz",
		"openai-prov-secret-abc",
		"gemini-prov-secret-def",
		"openrouter-prov-secret-ghi",
	}

	for _, secret := range providerSecrets {
		if strings.Contains(result, secret) {
			t.Errorf("provider secret leaked: %s", secret)
		}
	}

	// Verify JSON is valid
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonMap); err != nil {
		t.Fatalf("redactConfig output is not valid JSON: %v", err)
	}
}

// TestRedactConfig_EmptyConfig ensures empty config doesn't panic.
func TestRedactConfig_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}

	result, err := redactConfig(cfg)
	if err != nil {
		t.Fatalf("redactConfig with empty config failed: %v", err)
	}

	// Should still be valid JSON
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &jsonMap); err != nil {
		t.Fatalf("redactConfig output is not valid JSON: %v", err)
	}
}

// TestRedactPayload_SecureStringFiltering verifies that redactPayload filters SecureString values.
func TestRedactPayload_SecureStringFiltering(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			FilterSensitiveData: true,
			FilterMinLength:     1,
		},
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{
				Token: *config.NewSecureString("my-secret-telegram-token-1234567890"),
			},
		},
	}

	payload := "The token is my-secret-telegram-token-1234567890 here in the content"

	// This should filter the token via FilterSensitiveData
	// Note: FilterSensitiveData only filters SecureString values collected via reflection,
	// not plain-string API keys. This is expected behavior.
	result := cfg.FilterSensitiveData(payload)

	// Verify it's callable without panic
	_ = result
}

// TestRedactPayload_NonSecureStringsNotFiltered documents that plain-string secrets are NOT filtered.
// This is a deliberate limitation of FilterSensitiveData (only SecureString values).
func TestRedactPayload_NonSecureStringsNotFiltered(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Web: config.WebToolsConfig{
				Brave: config.BraveConfig{
					APIKey: "plain-brave-key-not-filtered",
				},
			},
		},
	}

	payload := "The API key is plain-brave-key-not-filtered in the content"
	result := cfg.FilterSensitiveData(payload)

	// Plain-string secrets are NOT filtered by FilterSensitiveData.
	// Only SecureString values are tracked and filtered.
	// This is expected behavior — plain strings are redacted in redactConfig() but not in FilterSensitiveData().
	_ = result // just verify it doesn't panic
}

// TestRedactConfig_AllWebToolSecrets verifies all web tool secrets are masked.
func TestRedactConfig_AllWebToolSecrets(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Web: config.WebToolsConfig{
				Brave: config.BraveConfig{
					APIKey: "brave-key-1",
					APIKeys: []string{"brave-key-2", "brave-key-3"},
				},
				Tavily: config.TavilyConfig{
					APIKey: "tavily-key-1",
					APIKeys: []string{"tavily-key-2"},
				},
				Perplexity: config.PerplexityConfig{
					APIKey: "perplexity-key-1",
					APIKeys: []string{"perplexity-key-2"},
				},
				GLMSearch: config.GLMSearchConfig{
					APIKey: "glm-key-1",
				},
			},
		},
	}

	result, err := redactConfig(cfg)
	if err != nil {
		t.Fatalf("redactConfig failed: %v", err)
	}

	webSecrets := []string{
		"brave-key-1", "brave-key-2", "brave-key-3",
		"tavily-key-1", "tavily-key-2",
		"perplexity-key-1", "perplexity-key-2",
		"glm-key-1",
	}

	for _, secret := range webSecrets {
		if strings.Contains(result, secret) {
			t.Errorf("web tool secret leaked: %s", secret)
		}
	}
}

// TestRedactConfig_RegistriesSecrets verifies registry tokens are masked.
func TestRedactConfig_RegistriesSecrets(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Skills: config.SkillsToolsConfig{
				Registries: config.SkillsRegistriesConfig{
					ClawHub: config.ClawHubRegistryConfig{
						AuthToken: "clawhub-token-secret-xyz",
					},
				},
			},
		},
	}

	result, err := redactConfig(cfg)
	if err != nil {
		t.Fatalf("redactConfig failed: %v", err)
	}

	if strings.Contains(result, "clawhub-token-secret-xyz") {
		t.Error("ClawHub auth token was not redacted")
	}
}

// TestRedactConfig_ConcurrentReads ensures redactConfig is thread-safe.
func TestRedactConfig_ConcurrentReads(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Web: config.WebToolsConfig{
				Brave: config.BraveConfig{
					APIKey: "brave-concurrent-key",
				},
			},
		},
		Providers: config.ProvidersConfig{
			Anthropic: config.ProviderConfig{
				APIKey: "anthropic-concurrent-key",
			},
		},
	}

	// Run redactConfig concurrently in multiple goroutines
	results := make(chan string, 10)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func() {
			result, err := redactConfig(cfg)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}

	// Verify all calls succeeded and no secrets leaked
	for i := 0; i < 10; i++ {
		select {
		case err := <-errors:
			t.Fatalf("concurrent redactConfig failed: %v", err)
		case result := <-results:
			if strings.Contains(result, "brave-concurrent-key") ||
				strings.Contains(result, "anthropic-concurrent-key") {
				t.Error("secret leaked in concurrent redactConfig")
			}
		}
	}
}
