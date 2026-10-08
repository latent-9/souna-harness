// Command pillead drives the harness without any UI framework: it loads the
// provider config, runs the agent loop, and prints events to stdout.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/latent-9/souna-harness/internal/harness"
	"github.com/latent-9/souna-harness/internal/provider"
	"github.com/latent-9/souna-harness/internal/session"
	"github.com/latent-9/souna-harness/internal/tools"
)

func main() {
	configPath := flag.String("config", defaultConfigPath(), "provider config file")
	providerName := flag.String("provider", "", "provider to use (default: first configured)")
	model := flag.String("model", "", "model to use (default: first of the provider)")
	sessionID := flag.String("session", "", "session id to resume")
	workdir := flag.String("workdir", "", "working directory for tools")
	flag.Parse()

	if *workdir != "" {
		if err := os.Chdir(*workdir); err != nil {
			fmt.Fprintln(os.Stderr, "workdir:", err)
			os.Exit(1)
		}
	}

	prompt := strings.Join(flag.Args(), " ")
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "usage: pillead [-config file] [-provider name] [-model name] [-session id] prompt")
		os.Exit(2)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	reg, err := provider.NewRegistry(cfg, provider.NewOpenAI)
	if err != nil {
		fmt.Fprintln(os.Stderr, "providers:", err)
		os.Exit(1)
	}
	if *providerName == "" {
		names := reg.Names()
		if len(names) == 0 {
			fmt.Fprintln(os.Stderr, "no providers configured in", *configPath)
			os.Exit(1)
		}
		*providerName = names[0]
	}
	if *model == "" {
		p, _ := reg.Get(*providerName)
		models := p.Models()
		if len(models) == 0 {
			fmt.Fprintf(os.Stderr, "no models configured for provider %q\n", *providerName)
			os.Exit(1)
		}
		*model = models[0]
	}

	sessions, err := session.NewStore(sessionDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "sessions:", err)
		os.Exit(1)
	}

	toolReg := tools.NewRegistry(
		tools.Bash{},
		tools.Read{},
		tools.Edit{},
		tools.Grep{},
		tools.Glob{},
	)

	h := harness.New(reg, toolReg, sessions, "")

	id := *sessionID
	if id == "" {
		sess, err := sessions.Create("pillead", *providerName, *model, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "create session:", err)
			os.Exit(1)
		}
		id = sess.ID
	}
	if err := sessions.Append(id, provider.Message{Role: provider.RoleUser, Content: prompt}); err != nil {
		fmt.Fprintln(os.Stderr, "append:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	events := make(chan harness.Event, 128)
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx, id, events) }()

	var runErr error
	stdin := bufio.NewScanner(os.Stdin)
	for {
		select {
		case ev := <-events:
			printEvent(h, ev, stdin)
		case err := <-done:
			runErr = err
			for {
				select {
				case ev := <-events:
					printEvent(h, ev, stdin)
					continue
				default:
				}
				goto finish
			}
		}
	}
finish:
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "error:", runErr)
		os.Exit(1)
	}
}

func printEvent(h *harness.Harness, ev harness.Event, stdin *bufio.Scanner) {
	switch e := ev.(type) {
	case harness.TokenDelta:
		fmt.Print(e.Text)
	case harness.MessageComplete:
		fmt.Println()
	case harness.ToolUseStart:
		fmt.Printf("\n> %s\n", e.Tool)
	case harness.ToolOutput:
		out := strings.TrimRight(e.Output, "\n")
		if e.IsError {
			fmt.Printf("  error: %s\n", out)
		} else if out != "" {
			fmt.Printf("  %s\n", indent(out))
		}
	case harness.PermissionRequest:
		fmt.Printf("\n? allow %s? [y/n] ", e.Tool)
		fmt.Scanln()
		for !stdin.Scan() {
		}
		answer := strings.TrimSpace(stdin.Text())
		h.Decide(e.ID, answer == "y" || answer == "yes")
	case harness.Error:
		fmt.Fprintf(os.Stderr, "\nharness error: %v\n", e.Err)
	}
}

func indent(s string) string {
	return strings.ReplaceAll(s, "\n", "\n  ")
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "pill.json"
	}
	return filepath.Join(home, ".config", "pill", "pill.json")
}

func sessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".pill-sessions"
	}
	return filepath.Join(home, ".local", "share", "pill", "sessions")
}

func loadConfig(path string) (provider.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return provider.Config{}, err
	}
	var cfg provider.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return provider.Config{}, err
	}
	return cfg, nil
}
