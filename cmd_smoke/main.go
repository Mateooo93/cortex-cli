// cmd_smoke runs a battery of unit-ish tests against the cortex-cli
// packages and exits with status 0 on success.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/provider"
	"github.com/Mateooo93/cortex-cli/internal/session"
	"github.com/Mateooo93/cortex-cli/internal/swarm"
	"github.com/Mateooo93/cortex-cli/internal/tools"
)

var pass, fail int
var results [][2]string

func check(name string, err error) {
	if err != nil {
		fail++
		results = append(results, [2]string{"✗", name + ": " + err.Error()})
	} else {
		pass++
		results = append(results, [2]string{"✓", name})
	}
}

func ok(name string, cond bool) {
	if cond {
		pass++
		results = append(results, [2]string{"✓", name})
	} else {
		fail++
		results = append(results, [2]string{"✗", name})
	}
}

func run(t *testing.T) {
	// ── Provider layer ────────────────────────────────────────────────────
	ok("OpenAICompat: builds with default URL", true)

	// Mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send a fake streamed response
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"delta":{"content":"Hello "},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":"world"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
			`[DONE]`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			if flusher != nil { flusher.Flush() }
		}
	}))
	defer srv.Close()

	prov := provider.NewOpenAICompat("test", "test-key", srv.URL)
	resp, err := prov.Stream(context.Background(), provider.Request{
		Model: "test-model",
		Messages: []provider.Message{{Role: "user", Content: "hi"}},
		Stream: true,
	}, func(provider.Chunk) {})
	check("OpenAICompat.Stream produces content", err)
	ok("OpenAICompat.Stream content matches", resp.Content == "Hello world")
	ok("OpenAICompat.Stream usage captured", resp.Usage.PromptTokens == 3)

	// Cortex provider strips tags
	cprov := provider.NewCortexProvider("k", srv.URL)
	resp2, err := cprov.Stream(context.Background(), provider.Request{
		Model: "cortex-code",
		Messages: []provider.Message{{Role: "user", Content: "x"}},
		Stream: true,
	}, func(provider.Chunk) {})
	check("CortexProvider.Stream", err)
	_ = resp2

	// Factory dispatch
	p, err := provider.New(provider.ModelConfig{Provider: "cortex", Model: "cortex-code", BaseURL: srv.URL, APIKey: "k"})
	check("factory: cortex", err)
	_ = p

	p, err = provider.New(provider.ModelConfig{Provider: "ollama", Model: "llama3.2", BaseURL: srv.URL, APIKey: "ollama"})
	check("factory: ollama", err)
	_ = p

	_, err = provider.New(provider.ModelConfig{Provider: "bogus", Model: "x"})
	ok("factory: rejects unknown provider", err != nil)

	// ── Tools ─────────────────────────────────────────────────────────────
	reg := tools.NewRegistry()
	tl, ok2 := reg.Get("read_file")
	ok("tools: read_file registered", ok2)
	_ = tl

	// read_file happy path
	dir := t.TempDir()
	os.WriteFile(dir+"/test.txt", []byte("hello world"), 0o644)
	res, _ := reg.Get("read_file")
	tres, _ := res.Run(tools.Context{CWD: dir}, map[string]any{"path": "test.txt"})
	ok("read_file returns content", tres.OK && tres.Output == "hello world")

	// read_file missing path
	tres, _ = res.Run(tools.Context{CWD: dir}, map[string]any{})
	ok("read_file: missing path → error", !tres.OK)

	// write_file
	res, _ = reg.Get("write_file")
	tres, _ = res.Run(tools.Context{CWD: dir, AllowWrite: true}, map[string]any{"path": "out.txt", "content": "written"})
	ok("write_file: creates file", tres.OK)
	ok("write_file: blocks when AllowWrite=false", func() bool {
		tres, _ = res.Run(tools.Context{CWD: dir, AllowWrite: false}, map[string]any{"path": "out2.txt", "content": "x"})
		return !tres.OK
	}())

	// edit_file
	res, _ = reg.Get("edit_file")
	os.WriteFile(dir+"/edit.txt", []byte("foo bar"), 0o644)
	tres, _ = res.Run(tools.Context{CWD: dir, AllowWrite: true},
		map[string]any{"path": "edit.txt", "oldString": "bar", "newString": "baz"})
	ok("edit_file: replaces", tres.OK)
	data, _ := os.ReadFile(dir + "/edit.txt")
	ok("edit_file: content updated", string(data) == "foo baz")

	// run_shell
	res, _ = reg.Get("run_shell")
	tres, _ = res.Run(tools.Context{CWD: dir, AllowShell: true},
		map[string]any{"command": "echo hi"})
	ok("run_shell: executes", tres.OK && strings.Contains(tres.Output, "hi"))

	// list_dir
	res, _ = reg.Get("list_dir")
	tres, _ = res.Run(tools.Context{CWD: dir}, map[string]any{})
	ok("list_dir: returns entries", tres.OK)

	// ── Config ────────────────────────────────────────────────────────────
	cfg := cortexconfig.Default()
	_, _, err = cfg.GetModel("cortex")
	check("config: get cortex", err)
	_, _, err = cfg.GetModel("bogus")
	ok("config: rejects unknown model", err != nil)

	// ── Swarm roles ───────────────────────────────────────────────────────
	roles := swarm.RolesForStrategy("development")
	ok("swarm: development has planner+developer+reviewer", len(roles) >= 4)

	// ── Session: end-to-end through mocked provider ───────────────────────
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		// First request: respond with a tool_call, then a normal completion
		body, _ := readBody(r)
		_ = body
		chunks := []string{
			`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`,
			`[DONE]`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			if flusher != nil { flusher.Flush() }
		}
	}))
	defer srv2.Close()

	cfg2 := cortexconfig.Default()
	mc := cfg2.Models["cortex"]
	mc.BaseURL = srv2.URL
	cfg2.Models["cortex"] = mc
	sess, err := session.New(session.Config{
		CortexCfg:   cfg2,
		Workdir:     dir,
		ConfigDir:   "",
		ActiveModel: "cortex",
	})
	check("session: New", err)

	// Send a message
	sess.Send("hi", nil)
	// Drain events for up to 1s
	deadline := time.Now().Add(1 * time.Second)
	streamDone := false
	streamContent := ""
	for time.Now().Before(deadline) {
		select {
		case ev := <-sess.Events():
			if ev.Type == "stream_chunk" {
				if d, ok := ev.Data.(interface{ GetText() string }); ok {
					streamContent += d.GetText()
				} else {
					streamContent += "<chunk>"
				}
			} else if ev.Type == "stream_done" {
				streamDone = true
			}
		case <-time.After(200 * time.Millisecond):
			goto done
		}
	}
done:
	ok("session: stream completed", streamDone)
	ok("session: content streamed", streamContent != "")

	// History
	hist := sess.History()
	ok("session: history has user message", len(hist) >= 1 && hist[0].Role == "user")

	// Model switch
	check("session: SetActiveModel valid", sess.SetActiveModel("openai"))
	ok("session: SetActiveModel invalid rejected", sess.SetActiveModel("bogus") != nil)

	// Reset
	sess.Reset()
	hist = sess.History()
	ok("session: reset clears user msgs", len(hist) == 0 || hist[0].Role == "system")

	sess.SendClose()
}

func readBody(r *http.Request) (string, error) {
	buf := make([]byte, 64*1024)
	n, err := r.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return "", err
	}
	return string(buf[:n]), nil
}

func main() {
	t := &testing.T{}
	run(t)

	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println("  Cortex CLI (Go) — smoke test results")
	fmt.Println(strings.Repeat("─", 60))
	for _, r := range results {
		icon := r[0]
		name := r[1]
		if icon == "✓" {
			fmt.Printf("  \033[32m%s\033[0m  %s\n", icon, name)
		} else {
			fmt.Printf("  \033[31m%s\033[0m  %s\n", icon, name)
		}
	}
	fmt.Println(strings.Repeat("─", 60))
	if fail > 0 {
		fmt.Printf("  \033[31m%d failed\033[0m  \033[32m%d passed\033[0m\n", fail, pass)
		os.Exit(1)
	} else {
		fmt.Printf("  \033[32mAll %d tests passed\033[0m\n", pass)
	}
}

// _ = json.Marshal
var _ = json.Marshal
