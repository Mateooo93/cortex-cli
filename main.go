// cortex-cli — main entry point.
//
// cortex-cli is a fork of vix (https://github.com/get-vix/vix). We keep
// the entire `internal/ui/` tree (bubbletea + lipgloss + glamour) so the
// TUI is identical to vix's, then replace vix's `vixd` daemon with an
// in-process session and a cortex-aware provider layer that talks to
// Cortex, OpenAI, Anthropic, or Ollama.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mattn/go-isatty"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/ui"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	cortexdaemon "github.com/Mateooo93/cortex-cli/internal/daemon" // wraps session
	"github.com/Mateooo93/cortex-cli/internal/swarm"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	modelFlag := flag.String("m", "", "Override the active model (e.g. cortex, openai, anthropic, ollama)")
	workdir := flag.String("workdir", "", "Set the working directory for this session")
	prompt := flag.String("p", "", "Run a single prompt non-interactively (headless mode)")
	testMode := flag.Bool("test", false, "Fill chat with fake data for UI testing")
	listModels := flag.Bool("list-models", false, "List available models and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("cortex " + Version)
		return
	}

	// Load user config early so we can handle --list-models before anything else
	cortexCfg, err := cortexconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if *listModels {
		fmt.Println("Available models:")
		for name, m := range cortexCfg.Models {
			active := " (default)"
			if name != cortexCfg.DefaultModel {
				active = ""
			}
			fmt.Printf("  %s%s: %s / %s @ %s\n", name, active, m.Provider, m.Model, m.BaseURL)
		}
		return
	}

	// Wire the daemon stub's global config loader
	cortexdaemon.SetGlobalConfigLoader(func() *cortexconfig.Config { return cortexCfg })

	if *modelFlag != "" {
		cortexCfg.DefaultModel = *modelFlag
	}

	// Resolve workdir
	cwd, _ := os.Getwd()
	if *workdir != "" {
		cwd = *workdir
	}

	// vixCfg is the vix-style config the (forked) UI needs; cortexCfg is
	// the cortex-specific config (providers, swarm settings, etc).
	vixCfg, err := config.Load(false, cwd, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Set the UI version
	ui.Version = Version

	// Headless mode (-p): one-shot prompt
	if *prompt != "" {
		if err := runHeadless(cortexCfg, cwd, *prompt, *modelFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Interactive TUI
	client := cortexdaemon.NewSessionClient("")
	if err := client.Connect(cwd, "", cortexCfg.DefaultModel, false, true, true, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
		os.Exit(1)
	}
	defer client.SendClose()

	// Pre-flight: if no API key, prompt for one
	if !hasUsableKey(cortexCfg) {
		if isatty.IsTerminal(os.Stdin.Fd()) {
			key := promptAPIKey()
			if key != "" {
				if _, mc, err := cortexCfg.GetModel(cortexCfg.DefaultModel); err == nil && mc != nil {
					cortexCfg.SetProviderAPIKey(mc.Provider, key)
					_ = cortexconfig.Save(cortexCfg)
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: no API key found for the selected provider. Set the provider API key in ~/.cortex/config.yaml or the matching environment variable.\n")
			os.Exit(1)
		}
	}

	// Reconnect with the saved key (if we just saved it)
	if err := client.Connect(cwd, "", cortexCfg.DefaultModel, false, true, true, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
		os.Exit(1)
	}

	enableWrite := cortexCfg.Tools.AllowWrite
	enableDir := true
	model := ui.NewModel(vixCfg, cortexCfg, client, *testMode, "", enableWrite, enableDir)

	p := tea.NewProgram(model)
	ui.SetProgram(p)

	// Handle SIGINT/SIGTERM by persisting the latest chat scrollback
	// to disk before letting the TUI exit. The TUI's own Ctrl+C dialog
	// already calls PersistSessions on confirm, but if the user hits
	// Ctrl+C twice in a row (or the terminal sends a raw signal that
	// bypasses the TUI's key handler), the only path out is this
	// signal goroutine — without the persist call below, the user's
	// most recent turn would be lost on every hard kill.
	//
	// We poll for the signal in a tight loop so the user can still
	// get out of a wedged session with a single Ctrl+C. Bubbletea
	// also catches Ctrl+C as a keypress, so the in-TUI confirm
	// dialog is still the preferred path; this is a backstop.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		// Best-effort flush. Any error here is non-recoverable
		// anyway since the user is force-killing us.
		model.PersistSessions()
		client.SendClose()
		os.Exit(0)
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runHeadless runs a single prompt and prints the result.
func runHeadless(cortexCfg *cortexconfig.Config, cwd, prompt, modelName string) error {
	client := cortexdaemon.NewSessionClient("")
	if err := client.Connect(cwd, "", modelName, false, true, true, true); err != nil {
		return err
	}
	defer client.SendClose()

	// Send input
	if err := client.SendInput(prompt, nil); err != nil {
		return err
	}
	// Drain events; print text chunks to stdout
	for ev := range client.Events() {
		switch ev.Type {
		case "stream_chunk":
			if m, ok := ev.Data.(protocol.EventStreamChunk); ok {
				fmt.Print(m.Text)
			}
		case "stream_done":
			fmt.Println()
			return nil
		case "error":
			if m, ok := ev.Data.(protocol.EventError); ok {
				return fmt.Errorf("%s", m.Message)
			}
		}
	}
	return nil
}

// hasUsableKey reports whether the selected provider can be used without
// prompting for a key.
func hasUsableKey(cfg *cortexconfig.Config) bool {
	if cfg == nil {
		return false
	}
	_, mc, err := cfg.GetModel(cfg.DefaultModel)
	if err != nil || mc == nil {
		return false
	}
	providerName := cortexconfig.NormalizeProviderName(mc.Provider)
	if !cortexconfig.ProviderNeedsAPIKey(providerName) {
		return true
	}
	if mc.APIKey != "" {
		return true
	}
	if pc, ok := cfg.ProviderConfig(providerName); ok && pc.APIKey != "" {
		return true
	}
	if envVar := cortexconfig.ProviderEnvVar(providerName); envVar != "" && os.Getenv(envVar) != "" {
		return true
	}
	return false
}

func promptAPIKey() string {
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	fmt.Println(warn.Render("No API key found."))
	fmt.Print(prompt.Render("Enter your API key: "))
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// Helper to avoid unused imports
var _ = exec.Command
var _ = time.Second
var _ = context.Background
var _ = swarm.New
