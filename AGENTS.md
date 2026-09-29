# Ship

`ship` is a CLI tool for opinionated way to deploy applications to a VPS on Hetzner. It contains two main components:

- `agent`: A Go binary which will be downloaded on the VPS and which will execute the actual operations on the VPS. Entry point at `cmd/agent`.
- `client`: A Go binary which orchestrates what happens on the server. Most often, it executes the `agent` binary with set configuration. Entry point at `cmd/client`.

## Subagents

When the user asks to use GPT-6.1 Sol or GPT-6 Luna for a subagent without specifying a reasoning effort, use `openai-codex/gpt-6.1-sol:medium` or `openai-codex/gpt-6-luna:high`, respectively. When the user asks to use GPT-6 Astra without specifying a reasoning effort, use `openai-codex/gpt-6-astra:medium`.
