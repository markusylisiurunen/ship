# Ship

`ship` is a CLI tool for opinionated way to deploy applications to a VPS on Hetzner. It contains two main components:

- `agent`: A Go binary which will be downloaded on the VPS and which will execute the actual operations on the VPS. Entry point at `cmd/agent`.
- `client`: A Go binary which orchestrates what happens on the server. Most often, it executes the `agent` binary with set configuration. Entry point at `cmd/client`.

Create disposable files with `mktemp`, or `mktemp -d` for a directory, instead of inventing paths.

## Agent setup

`.agents/managed.json` identifies centrally managed skills, prompts, configuration, and the marked section below. Change their sources in the agents repository rather than editing managed content directly. Follow each skill's activation rules. Use `github-workflow` for Git and GitHub work and `clear-writing` for prose.

`.cowork/scripts/` is repo-owned. Keep lifecycle hooks safe for dirty, non-default worktrees and preserve Git state. Do not add recurring schedules unless requested.

<!-- agents:subagents:start -->

## Subagent model selection

When neither the user nor active instructions request a subagent model or reasoning effort, omit the model override so the subagent uses the same model and settings as you. A model allowlist alone is not a request to select a model.

When a model is requested without an effort, use its default below. When an effort is explicitly requested, preserve it. If the environment cannot select the requested combination, explain the limitation rather than silently substituting another model.

For Claude subagents, prefer the Claude Code models, which use subscription limits instead of paid API usage. Use the direct Anthropic API only when the user or active instructions ask for it, so a plain request for Sonnet, Opus, or Haiku means the Claude Code model. If Claude Code is unavailable or out of limits, say so instead of switching to the API.

- Luna: `openai-codex/gpt-6-luna`; allowed efforts: low, medium, high, xhigh; default: `medium`.
- Terra: `openai-codex/gpt-6.1-terra`; allowed efforts: low, medium, high; default: `medium`.
- Sol: `openai-codex/gpt-6.1-sol`; allowed efforts: low, medium, high; default: `medium`.
- Astra: `openai-codex/gpt-6-astra`; allowed efforts: low, medium, high; default: `low`.
- Sonnet: `anthropic/claude-sonnet-5-5`; allowed efforts: low, medium, high; default: `medium`.
- Claude Code Sonnet: `claude-code/claude-sonnet-5-5`; allowed efforts: low, medium, high; default: `medium`.
- Claude Code Opus: `claude-code/claude-opus-5-5`; allowed efforts: low, medium, high; default: `medium`.
- Claude Code Haiku: `claude-code/claude-haiku-5-5`; allowed efforts: low, medium, high; default: `medium`.
- Opus: `anthropic/claude-opus-5-5`; allowed efforts: low, medium, high; default: `medium`.

Sonnet, Claude Code Sonnet, Claude Code Opus, Claude Code Haiku, and Opus may be selected as model overrides only when explicitly requested by the user or active instructions. Do not select them on your own. This does not prevent inheriting the parent model when no override is requested.
<!-- agents:subagents:end -->
