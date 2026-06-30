// Package agent drives Claude Managed Agents (CMA) to investigate a
// monitoring alert and open a fix PR. Anthropic runs the agent loop and a
// per-session container; this package only orchestrates: ensure an
// agent+environment exist, start a session with the target repo mounted, send
// the alert as the kickoff message, and consume the event stream until the
// session goes idle/terminated.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ryuichi1208/asagao/internal/alert"
	"github.com/ryuichi1208/asagao/internal/config"
)

// githubMCPName is the MCP server name referenced by the mcp_toolset. The URL
// is GitHub's hosted Copilot MCP endpoint, which exposes create_pull_request.
const (
	githubMCPName = "github"
	githubMCPURL  = "https://api.githubcopilot.com/mcp/"
)

// systemPrompt instructs the agent how to behave for every session.
const systemPrompt = `You are an on-call SRE assistant. You receive a monitoring alert and a
checkout of the related repository mounted in your workspace. Your job:

1. Investigate the codebase to find the most likely cause of the alert.
2. Make a focused, minimal code change that addresses the cause. Do not
   refactor unrelated code or add speculative changes.
3. Create a new git branch, commit your change with a clear message, and push
   it.
4. Open a pull request using the GitHub MCP create_pull_request tool. The PR
   description must explain: the alert, your diagnosis, what you changed and
   why, and how to verify.

If you cannot confidently identify a fix, do NOT guess. Instead open a PR (or
report back) describing your investigation and the most likely areas to look,
so a human can take over. Always state what you actually verified versus what
is a hypothesis.`

// Client is the subset of the Anthropic SDK that agent uses. It exists so the
// orchestration is testable and the dependency surface is explicit.
type Client = anthropic.Client

// NewClient constructs an Anthropic client. It resolves credentials from the
// environment (ANTHROPIC_API_KEY or an `ant auth login` profile).
func NewClient() anthropic.Client {
	return anthropic.NewClient()
}

// EnsureResources makes sure an agent and environment exist, creating them if
// the corresponding IDs are absent from cfg. It returns the (possibly newly
// created) agent ID, agent version, and environment ID. Callers should persist
// these back into their config to reuse across runs rather than creating new
// resources each time.
func EnsureResources(ctx context.Context, c anthropic.Client, cfg *config.Config) (agentID string, agentVersion int64, envID string, err error) {
	envID = cfg.Anthropic.EnvironmentID
	if envID == "" {
		env, eerr := c.Beta.Environments.New(ctx, anthropic.BetaEnvironmentNewParams{
			Name: "asagao-env",
			Config: anthropic.BetaEnvironmentNewParamsConfigUnion{
				OfCloud: &anthropic.BetaCloudConfigParams{
					Networking: anthropic.BetaCloudConfigParamsNetworkingUnion{
						OfUnrestricted: &anthropic.BetaUnrestrictedNetworkParam{},
					},
				},
			},
		})
		if eerr != nil {
			return "", 0, "", fmt.Errorf("agent: create environment: %w", eerr)
		}
		envID = env.ID
	}

	agentID = cfg.Anthropic.AgentID
	agentVersion = cfg.Anthropic.AgentVersion
	if agentID == "" {
		ag, aerr := c.Beta.Agents.New(ctx, anthropic.BetaAgentNewParams{
			Name:   "asagao-fixer",
			System: anthropic.String(systemPrompt),
			Model: anthropic.BetaManagedAgentsModelConfigParams{
				ID: anthropic.BetaManagedAgentsModel(cfg.Anthropic.Model),
			},
			MCPServers: []anthropic.BetaManagedAgentsURLMCPServerParams{{
				Name: githubMCPName,
				Type: anthropic.BetaManagedAgentsURLMCPServerParamsTypeURL,
				URL:  githubMCPURL,
			}},
			Tools: []anthropic.BetaAgentNewParamsToolUnion{
				{OfAgentToolset20260401: &anthropic.BetaManagedAgentsAgentToolset20260401Params{
					Type: anthropic.BetaManagedAgentsAgentToolset20260401ParamsTypeAgentToolset20260401,
				}},
				{OfMCPToolset: &anthropic.BetaManagedAgentsMCPToolsetParams{
					Type:          anthropic.BetaManagedAgentsMCPToolsetParamsTypeMCPToolset,
					MCPServerName: githubMCPName,
				}},
			},
		})
		if aerr != nil {
			return "", 0, "", fmt.Errorf("agent: create agent: %w", aerr)
		}
		agentID = ag.ID
		agentVersion = ag.Version
	}

	return agentID, agentVersion, envID, nil
}

// RunResult summarizes a fix session.
type RunResult struct {
	SessionID string
	// Transcript is the concatenated agent.message text emitted during the run.
	Transcript string
	// StopType is the terminal stop reason / status the run ended on.
	StopType string
}

// RunFixSession starts a session for a single alert, mounts the target repo,
// sends the alert as the kickoff message, and streams events until the session
// goes idle (terminal) or terminated. The repo + clone token must be supplied
// by the caller (resolved from config + GITHUB_TOKEN). vaultIDs must reference
// a vault holding the GitHub MCP credential, or PR creation will fail.
func RunFixSession(
	ctx context.Context,
	c anthropic.Client,
	cfg *config.Config,
	agentID string,
	agentVersion int64,
	repo config.RepoMapping,
	cloneToken string,
	a *alert.NormalizedAlert,
) (*RunResult, error) {
	if cfg.Anthropic.EnvironmentID == "" {
		return nil, errors.New("agent: environment ID is empty (call EnsureResources first)")
	}
	if len(cfg.Anthropic.VaultIDs) == 0 {
		return nil, errors.New("agent: no vault_ids configured; a vault holding the GitHub MCP credential is required to create PRs")
	}
	if cloneToken == "" {
		return nil, errors.New("agent: clone token is empty; set GITHUB_TOKEN")
	}

	agentVerOpt := anthropic.Int(agentVersion)
	session, err := c.Beta.Sessions.New(ctx, anthropic.BetaSessionNewParams{
		Agent: anthropic.BetaSessionNewParamsAgentUnion{
			OfBetaManagedAgentsAgents: &anthropic.BetaManagedAgentsAgentParams{
				ID:      agentID,
				Type:    anthropic.BetaManagedAgentsAgentParamsTypeAgent,
				Version: agentVerOpt,
			},
		},
		EnvironmentID: cfg.Anthropic.EnvironmentID,
		Title:         anthropic.String(fmt.Sprintf("asagao: %s", a.Title)),
		VaultIDs:      cfg.Anthropic.VaultIDs,
		Resources: []anthropic.BetaSessionNewParamsResourceUnion{
			{OfGitHubRepository: &anthropic.BetaManagedAgentsGitHubRepositoryResourceParams{
				Type:               anthropic.BetaManagedAgentsGitHubRepositoryResourceParamsTypeGitHubRepository,
				URL:                repo.URL,
				AuthorizationToken: cloneToken,
				Checkout: anthropic.BetaManagedAgentsGitHubRepositoryResourceParamsCheckoutUnion{
					OfBranch: &anthropic.BetaManagedAgentsBranchCheckoutParam{
						Name: repo.Branch,
						Type: anthropic.BetaManagedAgentsBranchCheckoutTypeBranch,
					},
				},
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("agent: create session: %w", err)
	}

	result := &RunResult{SessionID: session.ID}

	// Stream-first: open the stream before sending the kickoff so we don't miss
	// early events.
	stream := c.Beta.Sessions.Events.StreamEvents(ctx, session.ID, anthropic.BetaSessionEventStreamParams{})
	defer stream.Close()

	if _, err := c.Beta.Sessions.Events.Send(ctx, session.ID, anthropic.BetaSessionEventSendParams{
		Events: []anthropic.BetaManagedAgentsEventParamsUnion{{
			OfUserMessage: &anthropic.BetaManagedAgentsUserMessageEventParams{
				Type: anthropic.BetaManagedAgentsUserMessageEventParamsTypeUserMessage,
				Content: []anthropic.BetaManagedAgentsUserMessageEventParamsContentUnion{{
					OfText: &anthropic.BetaManagedAgentsTextBlockParam{
						Type: anthropic.BetaManagedAgentsTextBlockTypeText,
						Text: kickoffMessage(a),
					},
				}},
			},
		}},
	}); err != nil {
		return result, fmt.Errorf("agent: send kickoff message: %w", err)
	}

	var transcript strings.Builder
	for stream.Next() {
		switch ev := stream.Current().AsAny().(type) {
		case anthropic.BetaManagedAgentsAgentMessageEvent:
			for _, block := range ev.Content {
				transcript.WriteString(block.Text)
			}
		case anthropic.BetaManagedAgentsSessionErrorEvent:
			result.Transcript = transcript.String()
			result.StopType = "error"
			return result, fmt.Errorf("agent: session error: %s", ev.Error.Message)
		case anthropic.BetaManagedAgentsSessionStatusTerminatedEvent:
			result.StopType = "terminated"
			result.Transcript = transcript.String()
			return result, nil
		case anthropic.BetaManagedAgentsSessionStatusIdleEvent:
			// Idle with a non-action stop reason is terminal for this turn.
			// requires_action means the agent is waiting on us (tool
			// confirmation / custom tool result); for the MVP we don't drive
			// those, so any idle ends the run.
			if ev.StopReason.Type == "requires_action" {
				continue
			}
			result.StopType = "idle:" + ev.StopReason.Type
			result.Transcript = transcript.String()
			return result, nil
		}
	}
	if err := stream.Err(); err != nil {
		result.Transcript = transcript.String()
		return result, fmt.Errorf("agent: stream: %w", err)
	}

	result.Transcript = transcript.String()
	return result, nil
}

// kickoffMessage renders the alert into the initial user message.
func kickoffMessage(a *alert.NormalizedAlert) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A monitoring alert fired. Investigate the mounted repository, fix the likely cause, and open a pull request.\n\n")
	fmt.Fprintf(&b, "Source: %s\n", a.Source)
	if a.ID != "" {
		fmt.Fprintf(&b, "Alert ID: %s\n", a.ID)
	}
	fmt.Fprintf(&b, "Title: %s\n", a.Title)
	if a.Severity != "" {
		fmt.Fprintf(&b, "Severity: %s\n", a.Severity)
	}
	if a.Service != "" {
		fmt.Fprintf(&b, "Service: %s\n", a.Service)
	}
	if a.Message != "" {
		fmt.Fprintf(&b, "\nMessage:\n%s\n", a.Message)
	}
	if len(a.Tags) > 0 {
		fmt.Fprintf(&b, "\nTags:\n")
		for k, v := range a.Tags {
			fmt.Fprintf(&b, "  %s=%s\n", k, v)
		}
	}
	return b.String()
}
