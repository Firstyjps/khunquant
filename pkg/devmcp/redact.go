package devmcp

import (
	"encoding/json"
	"strings"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
)

// redactConfig returns a JSON representation of cfg with ALL secrets masked.
// SecureString fields are auto-masked by their MarshalJSON (→ "[NOT_HERE]").
// Plain-string secret fields must be explicitly overwritten.
func redactConfig(cfg *config.Config) (string, error) {
	// Marshal the config (SecureStrings auto-become "[NOT_HERE]")
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}

	// Unmarshal to a map for selective field redaction
	var configMap map[string]interface{}
	if err := json.Unmarshal(b, &configMap); err != nil {
		return "", err
	}

	// Redact known plain-string secret paths
	redactPlainStringSecrets(configMap)

	// Re-marshal and return
	result, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return "", err
	}

	// Additional pass-through filtering for any accidentally-exposed secrets
	// Build a replacer from the actual config values
	replacements := buildSecretReplacements(cfg)
	if len(replacements) > 0 {
		replacer := strings.NewReplacer(replacements...)
		return replacer.Replace(string(result)), nil
	}

	return string(result), nil
}

// redactPlainStringSecrets overwrites known plain-string secret fields in the
// unmarshaled config map with "[REDACTED]".
func redactPlainStringSecrets(m map[string]interface{}) {
	// Redact tools.web.* APIKeys
	if tools, ok := m["tools"].(map[string]interface{}); ok {
		if web, ok := tools["web"].(map[string]interface{}); ok {
			redactWebSecrets(web)
		}
		if skills, ok := tools["skills"].(map[string]interface{}); ok {
			redactSkillsSecrets(skills)
		}
	}

	// Redact providers.* api_keys (legacy)
	if providers, ok := m["providers"].(map[string]interface{}); ok {
		redactProviderSecrets(providers)
	}
}

// redactWebSecrets redacts api_key and api_keys fields from web search tool configs
func redactWebSecrets(web map[string]interface{}) {
	for _, toolName := range []string{"brave", "tavily", "perplexity", "glm"} {
		if toolCfg, ok := web[toolName].(map[string]interface{}); ok {
			if _, hasKey := toolCfg["api_key"]; hasKey {
				toolCfg["api_key"] = "[REDACTED]"
			}
			if _, hasKeys := toolCfg["api_keys"]; hasKeys {
				// Redact all entries in api_keys array
				if keys, ok := toolCfg["api_keys"].([]interface{}); ok {
					for i := range keys {
						keys[i] = "[REDACTED]"
					}
				}
			}
		}
	}
}

// redactSkillsSecrets redacts token and auth_token from skills config
func redactSkillsSecrets(skills map[string]interface{}) {
	if github, ok := skills["github"].(map[string]interface{}); ok {
		if _, hasToken := github["token"]; hasToken {
			github["token"] = "[REDACTED]"
		}
	}

	if registries, ok := skills["registries"].(map[string]interface{}); ok {
		if clawhub, ok := registries["clawhub"].(map[string]interface{}); ok {
			if _, hasToken := clawhub["auth_token"]; hasToken {
				clawhub["auth_token"] = "[REDACTED]"
			}
		}
	}
}

// redactProviderSecrets redacts api_key and api_base from provider configs
func redactProviderSecrets(providers map[string]interface{}) {
	// providers is a map of provider name -> config
	for _, provCfg := range providers {
		if cfg, ok := provCfg.(map[string]interface{}); ok {
			if _, hasKey := cfg["api_key"]; hasKey {
				cfg["api_key"] = "[REDACTED]"
			}
		}
	}
}

// buildSecretReplacements constructs a list of old->new pairs for secret values
// that might leak into the config JSON. This catches any values that weren't
// caught by the field-based redaction above.
func buildSecretReplacements(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}

	var replacements []string

	// Add non-empty secret values to the replacer
	// This ensures that even if a secret appears elsewhere (by accident),
	// it will be masked
	addIfNotEmpty := func(val string) {
		if val != "" && len(val) > 4 {
			// Only add replacements for reasonably long secrets to avoid
			// masking common words
			replacements = append(replacements, val, "[REDACTED]")
		}
	}

	// Collect secrets from Tools.Web
	if cfg.Tools.Web.Brave.APIKey != "" {
		addIfNotEmpty(cfg.Tools.Web.Brave.APIKey)
	}
	for _, k := range cfg.Tools.Web.Brave.APIKeys {
		addIfNotEmpty(k)
	}

	if cfg.Tools.Web.Tavily.APIKey != "" {
		addIfNotEmpty(cfg.Tools.Web.Tavily.APIKey)
	}
	for _, k := range cfg.Tools.Web.Tavily.APIKeys {
		addIfNotEmpty(k)
	}

	if cfg.Tools.Web.Perplexity.APIKey != "" {
		addIfNotEmpty(cfg.Tools.Web.Perplexity.APIKey)
	}
	for _, k := range cfg.Tools.Web.Perplexity.APIKeys {
		addIfNotEmpty(k)
	}

	if cfg.Tools.Web.GLMSearch.APIKey != "" {
		addIfNotEmpty(cfg.Tools.Web.GLMSearch.APIKey)
	}

	// Collect secrets from Skills
	if cfg.Tools.Skills.Github.Token != "" {
		addIfNotEmpty(cfg.Tools.Skills.Github.Token)
	}

	if cfg.Tools.Skills.Registries.ClawHub.AuthToken != "" {
		addIfNotEmpty(cfg.Tools.Skills.Registries.ClawHub.AuthToken)
	}

	// Collect secrets from legacy Providers (each provider is a field)
	if cfg.Providers.Anthropic.APIKey != "" {
		addIfNotEmpty(cfg.Providers.Anthropic.APIKey)
	}
	if cfg.Providers.OpenAI.APIKey != "" {
		addIfNotEmpty(cfg.Providers.OpenAI.APIKey)
	}
	if cfg.Providers.Groq.APIKey != "" {
		addIfNotEmpty(cfg.Providers.Groq.APIKey)
	}
	if cfg.Providers.Gemini.APIKey != "" {
		addIfNotEmpty(cfg.Providers.Gemini.APIKey)
	}
	if cfg.Providers.OpenRouter.APIKey != "" {
		addIfNotEmpty(cfg.Providers.OpenRouter.APIKey)
	}
	if cfg.Providers.Zhipu.APIKey != "" {
		addIfNotEmpty(cfg.Providers.Zhipu.APIKey)
	}
	if cfg.Providers.VLLM.APIKey != "" {
		addIfNotEmpty(cfg.Providers.VLLM.APIKey)
	}
	if cfg.Providers.Nvidia.APIKey != "" {
		addIfNotEmpty(cfg.Providers.Nvidia.APIKey)
	}
	if cfg.Providers.LiteLLM.APIKey != "" {
		addIfNotEmpty(cfg.Providers.LiteLLM.APIKey)
	}
	if cfg.Providers.Ollama.APIKey != "" {
		addIfNotEmpty(cfg.Providers.Ollama.APIKey)
	}

	return replacements
}

// redactPayload scrubs sensitive data from a string (e.g. message content).
func redactPayload(s string, cfg *config.Config) string {
	if cfg == nil {
		return s
	}
	return cfg.FilterSensitiveData(s)
}
