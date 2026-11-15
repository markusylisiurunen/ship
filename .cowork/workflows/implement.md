---
target: issue
triggers: [command]
persona: gpt-6.1-sol-chatgpt-coder
reasoning: medium
---

Implement the issue and open a pull request for it. Before you start, read the issue and all of its comments with `gh`. The discussion defines what to build, so follow any direction or plan it has settled on. The comment that started this task may add guidance of its own. Follow it, because it is the user's latest word. Other branches or pull requests for this issue may already exist. Do not reuse or build on them, because this is an independent implementation.

Nobody will answer questions while you work, so do not stop to ask. Make reasonable decisions that fit the issue's intent and the repository's conventions, and keep track of the ones that matter.

Create a branch, implement the issue, run the verification the repository requires, commit, and push. Then open a pull request with `gh` that links the issue. Its title and description are the lasting record of the change, so describe the change as a whole, including the decisions about its behavior that you made where the discussion left room, and leave out how the work went. Further discussion happens on the pull request. If you have open questions, assumptions the reviewer should confirm, or verification failures or gaps, post them in one pull request comment after opening it. Do not comment on the issue. The link from the pull request is enough. Open the pull request ready for review by default. Use draft status only when the user requests it or the implementation is incomplete or has known blocking defects. Verification failures or gaps do not by themselves require draft status.

Before you change anything, read the code the issue touches and the repository's guidance, such as AGENTS.md. Use existing behavior, tests, and patterns to understand where the change belongs.

Implement the issue completely and leave the code in a coherent state. Follow existing patterns when they fit, and change them when they no longer serve the new behavior. Fix bugs you run into when they affect the result, but leave unrelated cleanup alone, and mention any work beyond the issue in the pull request. Add or update tests the way the repository already does.

Before you commit, review your own diff for correctness, security problems, and changes you did not intend.

The following repository guidance adds to the instructions above. Where the two differ, follow the repository guidance.

Inspect @@skill:deslop for additional guidance on evolving abstractions, preserving clean boundaries, and leaving affected code coherent. Apply relevant principles while implementing the issue, but do not perform a separate deslop pass or broaden the workflow into a general cleanup task.
