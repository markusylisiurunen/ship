# Ship

`ship` is a CLI tool for opinionated way to deploy applications to a VPS on Hetzner. It contains two main components:

- `agent`: A Go binary which will be downloaded on the VPS and which will execute the actual operations on the VPS. Entry point at `cmd/agent`.
- `client`: A Go binary which orchestrates what happens on the server. Most often, it executes the `agent` binary with set configuration. Entry point at `cmd/client`.

Create disposable files with `mktemp`, or `mktemp -d` for a directory, instead of inventing paths.

<!-- agents:testing:start -->

## Testing

Tests are a trade-off. A test is worth keeping when what it protects outweighs what it costs to maintain. The factors below help weigh that, and each has exceptions, so use judgment where a case doesn't fit.

- **It protects more** when a break would go unnoticed, such as wrong output for some inputs or quietly lost data, when the consequence is serious, and when reading the code doesn't settle the behavior, because many rules interact or a past bug was subtle. A mistake that fails loudly and at once, such as an app that won't start or a type error, needs no test.
- **It costs more** when it needs mocks or elaborate setup, depends on wording, call order, or internal structure, or is slow or flaky. A table of inputs and expected outputs against a stable interface is cheap, so code with many interacting cases, such as a parser, can have many of them. Several cases that check the same behavior add cost and nothing else.

Treat the tests as one suite that describes the code as it is now. When a change alters behavior, update, replace, or remove the tests that covered the old behavior, so the suite reads as if it had been written for the new code. A test about the change itself, such as one asserting that something no longer happens, usually describes history, which the commit message already records. Some changes do work of their own that is worth checking, though, such as a migration that moves existing data to a new shape.

Agents tend to add tests by default, so the suite grows even when a change replaces behavior instead of adding it. Start from no new test, and add one only when you can say what it protects that the suite doesn't already. Checking a change while you make it is part of the work and does not need to leave a test behind.

This repository's own guidance may set a different bar for some areas. Follow it where it does.

<!-- agents:testing:end -->

<!-- agents:writing:start -->

## Writing

Use skill `clear-writing` for all text, including its part on writing for AI models when the reader is a model. Write for the person or model who reads the text, in the concepts that reader has.

- Say each thing once, where its reader needs it. Keep inventories of implementation details in their authoritative sources.
- This repository's own guidance may name its writing surfaces and their readers, such as user-facing documentation or product copy. Follow it where it applies.

<!-- agents:writing:end -->

## Agent setup

`.agents/managed.json` identifies centrally managed skills, prompts, configuration, and the marked sections in this file. Change their sources in the agents repository rather than editing managed content directly. Follow each skill's activation rules. Use `github-workflow` for Git and GitHub work.

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
