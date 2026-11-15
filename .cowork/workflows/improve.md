---
target: pull-request
triggers: [command]
persona: gpt-6.1-sol-chatgpt-coder
reasoning: medium
---

Improve the code in the pull request without changing what it is meant to do, and push the result to its branch. Use `gh` to read the pull request and its discussion. Then check out its head branch and read the full diff locally with `git`. The comment may add guidance, such as what to focus on. Follow it, because it is the user's latest word.

Change the code only where that clearly makes it better. If it is already in good shape, leave it alone. When you do change it, run the verification the repository requires, commit, and push to the existing branch. Never push an empty commit. If someone pushed to the branch while you worked, build on their commits instead of overwriting them. After pushing changes, use `gh` to update the pull request description if it is no longer accurate. The description should always describe the pull request as a whole, as it stands now. Do not turn it into a log of what changed in each push. Keep content that is still valid and any required metadata, and change only what needs to change.

Finish with exactly one short pull request comment, posted with `gh`. If you changed the code, say what you changed and why it is better, and mention verification or scope only when it matters. If nothing needed changing, say that you looked and found nothing worth changing.

Before you edit anything, understand what the change is for. Read the surrounding code, callers, contracts, and tests, and follow the repository's guidance, such as AGENTS.md. Treat the existing code as evidence of the problem and its real constraints, not as a design you must keep.

Then set the current implementation aside and ask what you would build today for the same need. Give each concept, rule, and piece of state one clear owner, and draw boundaries so that callers do not have to remember hidden rules or coordinate steps. Judge an abstraction by how much it lets a reader set aside, not by how much code it removes. Compare that design with the pull request, reuse what fits, and change what makes the problem harder than it needs to be, even outside the changed files. A large refactor is fine when the result is clearly better, and so is changing very little.

Then look at the details. Code written quickly often hides unfinished work in local workarounds: optional fields that should be required, type casts that silence a mismatch, fallbacks for formats nothing still produces, repeated validation, guards for states that cannot happen, swallowed errors, wrappers that add nothing, comments that narrate the code, and tests that cost more than they protect. Treat each one as a signal to investigate. Trace its callers and fix the cause. Keep compatibility code that real data or callers still depend on, and keep complexity that real guarantees need.

Preserve the intended behavior. Fix bugs you find in the change and say which behavior each fix corrects, but do not add features or do unrelated cleanup. When you change a contract, carry the change through to every caller, test, and document that depends on it. Before you commit, review your own diff for accidental behavior changes and unnecessary churn.

The following repository guidance adds to the instructions above. Where the two differ, follow the repository guidance.

Use @@skill:deslop for additional cleanup guidance. Apply its principles while preserving the pull request's intended behavior and following this workflow, repository instructions, verification requirements, and required pull request comment.
