// cortex-cli — main entry point.
//
// cortex-cli is a fork of vix (https://github.com/get-vix/vix). We keep
// the entire `internal/ui/` tree (bubbletea + lipgloss + glamour) so the
// TUI is identical to vix's, then replace vix's `vixd` daemon with an
// in-process session and a cortex-aware provider layer that talks to
// Cortex, OpenAI, Anthropic, or Ollama.
//
// The entrypoint lives under cmd/cortex/ following standard Go layout.
// This makes it easier to add future subcommands (e.g. `cortex config`)
// and keeps the root clean.
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
	"github.com/Mateooo93/cortex-cli/internal/provider/codex"
	"github.com/Mateooo93/cortex-cli/internal/swarm"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func init() {
	// Register the ChatGPT-subscription (codex) provider so provider.New
	// can resolve `provider == "codex"` without a config-key API key.
	codex.Register()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run contains the real main logic. It is extracted so the top-level
// main stays tiny (easier to test, easier to add subcommands later)
// and all error handling funnels through one place.
func run(args []string) error {
	fs := flag.NewFlagSet("cortex", flag.ExitOnError)
	versionFlag := fs.Bool("version", false, "Print version and exit")
	modelFlag := fs.String("m", "", "Override the active model (e.g. cortex, openai, anthropic, ollama)")
	workdir := fs.String("workdir", "", "Set the working directory for this session")
	prompt := fs.String("p", "", "Run a single prompt non-interactively (headless mode)")
	testMode := fs.Bool("test", false, "Fill chat with fake data for UI testing")
	listModels := fs.Bool("list-models", false, "List available models and exit")
	_ = fs.Parse(args)

	if *versionFlag {
		fmt.Println("cortex " + Version)
		return nil
	}

	// Load user config early so we can handle --list-models before anything else
	cortexCfg, err := cortexconfig.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
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
		return nil
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
		return fmt.Errorf("loading vix config: %w", err)
	}

	// Set the UI version
	ui.Version = Version

	// Headless mode (-p): one-shot prompt
	if *prompt != "" {
		return runHeadless(cortexCfg, cwd, *prompt, *modelFlag)
	}

	// Interactive TUI
	client := cortexdaemon.NewSessionClient("")
	if err := client.Connect(cwd, "", cortexCfg.DefaultModel, false, true, true, false); err != nil {
		return fmt.Errorf("starting session: %w", err)
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
			return fmt.Errorf("no API key found for the selected provider. Set the provider API key in ~/.cortex/config.yaml or the matching environment variable")
		}
	}

	// Reconnect with the saved key (if we just saved it)
	if err := client.Connect(cwd, "", cortexCfg.DefaultModel, false, true, true, false); err != nil {
		return fmt.Errorf("reconnecting session: %w", err)
	}

	enableWrite := cortexCfg.Tools.AllowWrite
	enableDir := true
	model := ui.NewModel(vixCfg, cortexCfg, client, *testMode, "", enableWrite, enableDir)

	p := tea.NewProgram(model)
	ui.SetProgram(p)

	// Always try to leave the terminal usable. Bubble Tea restores state on
	// graceful quit; this defer covers races and abnormal exits.
	defer ui.RestoreTerminal()

	// SIGINT/SIGTERM must go through Bubble Tea so alt-screen, mouse capture,
	// and raw mode are restored. os.Exit bypasses that cleanup and leaves the
	// shell unresponsive.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		p.Send(ui.SignalQuitMsg{})
		// Second signal: force shutdown but still run renderer/TTY cleanup.
		<-sigCh
		p.Kill()
	}()

	if _, err := p.Run(); err != nil {
		return err
	}

	// Self-update: the TUI quit cleanly after /update; re-exec the new binary
	// now that bubbletea has restored the terminal.
	if exe, argv, ok := ui.TakePendingRestart(); ok {
		client.SendClose()
		args := append([]string{exe}, argv...)
		if err := syscall.Exec(exe, args, os.Environ()); err != nil {
			cmd := exec.Command(exe, argv...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = os.Environ()
			if runErr := cmd.Run(); runErr != nil {
				return fmt.Errorf("restart after update: %w", runErr)
			}
		}
	}
	return nil
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

var _ = time.Second
var _ = context.Background
var _ = swarm.New
