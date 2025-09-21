# Ship

`ship` is a CLI tool for opinionated way to deploy applications to a VPS on Hetzner. It contains two main components:

- `agent`: A Go binary which will be downloaded on the VPS and which will execute the actual operations on the VPS. Entry point at `cmd/agent`.
- `client`: A Go binary which orchestrates what happens on the server. Most often, it executes the `agent` binary with set configuration. Entry point at `cmd/client`.
