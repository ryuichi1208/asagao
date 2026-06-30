// Command asagao turns monitoring alerts into auto-generated fix PRs.
//
// Subcommands:
//
//	asagao serve --config config.yaml
//	    Start the webhook receiver (POST /webhook/{provider}).
//
//	asagao run --config config.yaml --provider datadog --file alert.json
//	    Process a single alert from a file (for E2E testing / debugging).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ryuichi1208/asagao/internal/agent"
	"github.com/ryuichi1208/asagao/internal/config"
	"github.com/ryuichi1208/asagao/internal/provider"
	"github.com/ryuichi1208/asagao/internal/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("asagao: ")

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `asagao — alert-driven auto-PR agent

Usage:
  asagao serve --config config.yaml
  asagao run   --config config.yaml --provider <name> --file <alert.json>

Providers: `+fmt.Sprint(provider.Names())+`
`)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", "config.yaml", "path to YAML config")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := agent.NewClient()
	agentID, agentVersion, envID, err := agent.EnsureResources(ctx, client, cfg)
	if err != nil {
		return err
	}
	cfg.Anthropic.AgentID = agentID
	cfg.Anthropic.AgentVersion = agentVersion
	cfg.Anthropic.EnvironmentID = envID
	log.Printf("using agent=%s version=%d environment=%s", agentID, agentVersion, envID)
	log.Printf("persist these IDs into %s to reuse them across restarts", *cfgPath)

	srv := server.New(cfg, client, agentID, agentVersion)
	log.Printf("listening on %s", cfg.Server.Addr)
	return http.ListenAndServe(cfg.Server.Addr, srv.Handler())
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", "config.yaml", "path to YAML config")
	providerName := fs.String("provider", "", "provider name (e.g. datadog, grafana)")
	file := fs.String("file", "", "path to an alert payload JSON file")
	_ = fs.Parse(args)

	if *providerName == "" || *file == "" {
		return fmt.Errorf("run: --provider and --file are required (providers: %v)", provider.Names())
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	p, ok := provider.Get(*providerName)
	if !ok {
		return fmt.Errorf("run: unknown provider %q (registered: %v)", *providerName, provider.Names())
	}

	raw, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("run: read alert file: %w", err)
	}

	a, err := p.Normalize(raw)
	if err != nil {
		return fmt.Errorf("run: normalize: %w", err)
	}
	log.Printf("normalized alert: source=%s title=%q service=%q severity=%q", a.Source, a.Title, a.Service, a.Severity)

	repo, ok := cfg.RepoFor(a.Service)
	if !ok {
		return fmt.Errorf("run: no repo mapping for service %q and no default mapping", a.Service)
	}

	ctx := context.Background()
	client := agent.NewClient()
	agentID, agentVersion, envID, err := agent.EnsureResources(ctx, client, cfg)
	if err != nil {
		return err
	}
	cfg.Anthropic.EnvironmentID = envID

	res, err := agent.RunFixSession(ctx, client, cfg, agentID, agentVersion, repo, os.Getenv("GITHUB_TOKEN"), a)
	if err != nil {
		return err
	}
	log.Printf("session %s ended (stop=%s)", res.SessionID, res.StopType)
	if res.Transcript != "" {
		fmt.Println("\n--- agent transcript ---")
		fmt.Println(res.Transcript)
	}
	return nil
}
