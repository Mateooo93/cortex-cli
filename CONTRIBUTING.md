# Contributing to cortex-cli

Thank you for your interest in contributing to cortex-cli! This guide will
help you get started.

cortex-cli is a fork of [vix](https://github.com/get-vix/vix). Most
contributions happen in the cortex-specific packages (`internal/cortexconfig`,
`internal/provider`, `internal/session`, `internal/swarm`, `internal/tools`,
and the `main.go` entry point). Changes to `internal/ui/` should be minimal
and sync-friendly with vix upstream.

## Priority Areas

We value contributions in this order:

1. **Bug fixes** — especially crashes, data loss, and stack overflows
2. **Cortex provider integration** — better support for the Cortex gateway
3. **Cross-platform compatibility** — macOS, Linux, and Windows support
4. **Security hardening** — shell injection, prompt injection, and
   privilege escalation prevention
5. **Performance and robustness** — error handling, retry logic, and
   resource management
6. **New tools and skills** — broadly useful additions
7. **Documentation** — fixes and clarifications

## Development Setup

### Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- An API key for your LLM provider (Cortex, OpenAI, Anthropic, or Ollama)

### Getting Started

1. Fork and clone the repository:

```bash
git clone https://github.com/<your-username>/cortex-cli.git
cd cortex-cli
```

2. Build the project:

```bash
go build ./...
```

3. Run the tests:

```bash
go test ./...
```

4. (Optional) Build the local `cortex` binary:

```bash
go build -o bin/cortex .
./bin/cortex chat
```

## Development Workflow

1. Create a branch from `main`:

```bash
git checkout -b your-feature-name
```

2. Make your changes, ensuring the code compiles and tests pass.

3. Commit using conventional commit messages:

```
fix(tools): prevent stack overflow in bash tool on recursive scripts
feat(provider): add streaming support for Cortex gateway
docs: clarify setup instructions
```

4. Push your branch and open a pull request against `main`.

## Commit Attribution

If you use an AI coding agent to help with your contribution, please credit
it as a co-author by appending a trailer to your commit messages:

```
Co-authored-by: cortex-cli <Mateooo93@users.noreply.github.com>
```

## Code Guidelines

- **Keep it simple** — avoid over-engineering. The right amount of
  complexity is the minimum needed for the current task.
- **Security matters** — sanitize inputs, avoid shell injection, and
  validate at system boundaries.
- **Comments** — explain intent, not implementation. If the code needs a
  comment to explain what it does, consider rewriting it.
- **Test your changes** — add or update tests when fixing bugs or adding
  features.
- **No unnecessary dependencies** — prefer the standard library when
  reasonable.

## Sync strategy with vix upstream

When vix ships improvements to `internal/ui/`, port them in with a focused
merge. Avoid making non-essential changes in `internal/ui/` so future
upstream syncs stay small. Cortex-specific behaviour belongs in
`internal/cortexconfig/`, `internal/provider/`, `internal/session/`,
`internal/swarm/`, or `internal/tools/`.

## Pull Request Process

- Keep PRs focused on a single change.
- Provide a clear description of what your change does and why.
- Ensure all tests pass before requesting review.
- Be responsive to feedback during code review.

## Reporting Issues

When opening an issue, please include:

- Steps to reproduce the problem
- Expected vs actual behaviour
- Your environment (OS, Go version, cortex-cli version)
- Relevant logs or stack traces

## License

By contributing to cortex-cli, you agree that your contributions will be
licensed under the [GNU Affero General Public License v3.0](LICENSE).
