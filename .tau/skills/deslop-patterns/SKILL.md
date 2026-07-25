---
name: "deslop-patterns"
description: "Apply Ship-specific Go CLI, remote-execution, reconciliation, compatibility, safety, and verification patterns during deslop cleanup. Use only alongside the deslop skill in the Ship repository. Trigger: explicit."
---

# Ship deslop patterns

Use these patterns alongside `deslop`, `AGENTS.md`, and the Go reference from the shared skill.

## Product and compatibility

Ship is a released deployment CLI with two binaries:

- `client` runs locally or in CI and orchestrates Hetzner, SSH, archive upload, secrets, and remote agent invocation.
- `agent` runs on the VPS, reconciles machine state, performs maintenance, and activates application releases.

Preserve CLI command names, flags, defaults, release asset names, remote directory layouts, `.ship` file conventions, volume and secret paths, and deployed symlink behavior unless an intentional migration updates every consumer. Other repositories download released Ship binaries and depend on these contracts.

## Boundaries and safety

- Keep cloud API and SSH orchestration under `internal/client`; keep privileged machine and deployment operations under `internal/agent` and `internal/reconcile`.
- Keep reconcilers idempotent. Re-running `machine up`, maintenance, or deployment must converge safely instead of duplicating state or locking out access.
- Treat firewall, SSH daemon, user ownership, permissions, secrets, symlinks, release directories, and Docker/Caddy operations as destructive boundaries. Apparent checks and ordering often prevent outages or data loss.
- Preserve the firewall sequence that installs required allow rules before first enablement.
- Validate every value used in remote paths or shell commands before interpolation. Prefer argument arrays where possible and keep unavoidable shell construction narrow.
- Keep resource ownership explicit. Close local files, SSH sessions, SCP clients, ZIP writers, and temporary paths at the layer that acquired them.
- Preserve useful wrapped errors across cloud, filesystem, archive, SSH, and remote-command boundaries.
- Do not run commands that create machines, connect to production hosts, set secrets, deploy applications, alter firewall rules, or perform maintenance during cleanup.

## Code shape

Keep `cmd/` as thin binary entrypoints, command behavior in `internal/client` or `internal/agent`, machine convergence in `internal/reconcile`, and shared constants narrowly scoped. Do not add an abstraction merely because two remote actions have similar setup; extract only when ownership and failure semantics remain clear.

Comments that explain safety-critical ordering or external command behavior may be valuable even when the code is readable. Remove narrative comments, but preserve reasons that prevent destructive simplification.

## Tests and verification

The current repository has little automated coverage. Add tests only for pure parsing, reconciliation planning, archive behavior, path validation, or other logic that can be exercised without cloud or root side effects. Do not create mocks that merely restate CLI wiring.

Run:

```sh
gofmt -w <changed-go-files>
go vet ./...
go test ./...
go build ./...
```

Never use real Hetzner, SSH, Docker, firewall, secret, or deployment operations as verification.
