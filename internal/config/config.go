// Package config loads asagao configuration from a YAML file plus the
// environment. Secrets (ANTHROPIC_API_KEY, GITHUB_TOKEN) live in the
// environment only and are never read from YAML.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level asagao configuration.
type Config struct {
	Anthropic AnthropicConfig        `yaml:"anthropic"`
	Repos     map[string]RepoMapping `yaml:"repos"`
	Server    ServerConfig           `yaml:"server"`
}

// AnthropicConfig holds the Managed Agents resource IDs. The API key itself
// is read from ANTHROPIC_API_KEY by the SDK and is intentionally absent here.
type AnthropicConfig struct {
	// AgentID / AgentVersion identify a pre-created agent. EnsureAgent fills
	// these in on first run; persist them back into the YAML to reuse the
	// agent across runs instead of creating a new one each time.
	AgentID      string `yaml:"agent_id"`
	AgentVersion int64  `yaml:"agent_version"`
	// EnvironmentID identifies a pre-created cloud environment. EnsureAgent
	// fills this in on first run if empty.
	EnvironmentID string `yaml:"environment_id"`
	// VaultIDs reference vaults holding the GitHub MCP OAuth credential
	// required for PR creation. Must be created out-of-band (see README).
	VaultIDs []string `yaml:"vault_ids"`
	// Model is the Claude model the agent uses. Defaults to claude-opus-4-8.
	Model string `yaml:"model"`
}

// RepoMapping maps a service name to the GitHub repository the agent should
// operate on.
type RepoMapping struct {
	URL    string `yaml:"url"`    // https://github.com/owner/repo
	Branch string `yaml:"branch"` // base branch; defaults to "main"
}

// ServerConfig configures the webhook HTTP server.
type ServerConfig struct {
	Addr string `yaml:"addr"` // listen address, e.g. ":8080"
}

const (
	// DefaultModel is used when AnthropicConfig.Model is empty.
	DefaultModel = "claude-opus-4-8"
	// DefaultServerAddr is used when ServerConfig.Addr is empty.
	DefaultServerAddr = ":8080"
	// DefaultBranch is used when a RepoMapping.Branch is empty.
	DefaultBranch = "main"
)

// Load reads and parses the YAML config at path, then applies defaults.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	c.applyDefaults()
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Anthropic.Model == "" {
		c.Anthropic.Model = DefaultModel
	}
	if c.Server.Addr == "" {
		c.Server.Addr = DefaultServerAddr
	}
	for name, repo := range c.Repos {
		if repo.Branch == "" {
			repo.Branch = DefaultBranch
			c.Repos[name] = repo
		}
	}
}

// RepoFor resolves the repository to operate on for a given service name. If
// the service is empty or unmapped, it falls back to a repo mapped under the
// special key "default". It returns ok=false when nothing matches.
func (c *Config) RepoFor(service string) (RepoMapping, bool) {
	if service != "" {
		if r, ok := c.Repos[service]; ok {
			return r, true
		}
	}
	if r, ok := c.Repos["default"]; ok {
		return r, true
	}
	return RepoMapping{}, false
}
