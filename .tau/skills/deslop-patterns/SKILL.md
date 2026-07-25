---
name: "deslop-patterns"
description: "Apply Ship-specific compatibility, contract, ownership, and regression patterns during deslop cleanup. Use only alongside the deslop skill in the Ship repository. Trigger: explicit."
---

# Ship deslop patterns

## Compatibility and contract drift

Ship is released infrastructure tooling, and other repositories download its binaries. Before removing compatibility code, check release history and consumers rather than assuming a contract never shipped.

Preserve CLI commands, flags, defaults, release asset names, remote directory layouts, `.ship` conventions, volume and secret paths, and deployed symlink behavior unless an intentional migration updates every producer and consumer. Trace changes across local CLI input, archive and configuration generation, SSH invocation, remote agent parsing, reconciliation, and filesystem effects; these layers are common drift points.

Collapse identical independently declared representations across those layers into one owned type. Keep separate boundary types only when their meanings differ, and make each conversion explicit and symmetric. Derive remote paths, release locations, and other reproducible values from canonical configuration at their owner instead of plumbing duplicates through client payloads, agent input, and reconciliation state.

## Ownership and regression surfaces

- Keep cloud and SSH orchestration on the client side and privileged machine-state changes on the agent and reconciliation side. Move validation or shared data only when its owner remains unambiguous across the local-to-remote boundary.
- Keep reconcilers idempotent. Re-running machine setup, maintenance, or deployment must converge without duplicating state, corrupting a release, or locking out access.
- Treat firewall and SSH configuration, user ownership and permissions, secrets, release directories and symlinks, and Docker/Caddy state as high-risk surfaces. Preserve checks and ordering whose purpose is outage or data-loss prevention, especially installing required firewall allow rules before first enablement.
- Validate values at the boundary that owns them before they reach remote paths or shell construction. Do not scatter defensive checks downstream, but do not mistake externally supplied values for trusted internal state.
- Keep resource cleanup with the layer that acquired the file, archive writer, temporary path, SSH session, or transfer client. Preserve useful error context when failures cross filesystem, archive, cloud, SSH, or remote-command boundaries.
- Similar remote actions do not automatically share ownership or failure semantics. Extract common setup only when the abstraction preserves those distinctions.
- Preserve comments that explain safety-critical ordering or external command constraints; remove comments that merely narrate the implementation.
