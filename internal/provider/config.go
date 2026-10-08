package provider

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Config is the provider configuration loaded from the user's config file.
type Config struct {
	Providers map[string]ProviderConfig `json:"providers"`
}

// ProviderConfig is one provider entry. APIKey supports {env:VAR}
// substitution so keys are never stored in the config file itself.
type ProviderConfig struct {
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key"`
	Models  []string `json:"models"`
}

var envRef = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// ResolveKey substitutes {env:VAR} references in s. Unset variables
// resolve to the empty string.
func ResolveKey(s string) string {
	return envRef.ReplaceAllStringFunc(s, func(m string) string {
		name := envRef.FindStringSubmatch(m)[1]
		return os.Getenv(name)
	})
}

// Validate checks that every provider entry has a base URL and that all
// {env:VAR} references resolve to set variables.
func (c Config) Validate() error {
	for name, pc := range c.Providers {
		if strings.TrimSpace(pc.BaseURL) == "" {
			return fmt.Errorf("provider %q: base_url is required", name)
		}
		for _, m := range envRef.FindAllStringSubmatch(pc.APIKey, -1) {
			if os.Getenv(m[1]) == "" {
				return fmt.Errorf("provider %q: environment variable %s is not set", name, m[1])
			}
		}
	}
	return nil
}
