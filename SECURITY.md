# Security

RaisQ runs AI agents with their permission prompts off, so its isolation is a security boundary. Please report anything that gets through it.

## Reporting

Please don't open a public issue for a vulnerability. Report it privately through [GitHub's private vulnerability reporting](https://github.com/kkurno/raisq/security/advisories/new).

Include what you ran, what happened, and what you expected. We'll reply within a week.

## What counts

A vulnerability is any way an agent, a check, or text the agent wrote can:

- read or write files on the host outside the task's checkout and out folder
- reach a credential it shouldn't have: SSH keys, git credentials, cloud logins, test secrets while it's building, or another tool's tokens from a shared login store
- get a write back to the host through the copied login or a read-only mount
- change what judges it: the tests or check config as they were before the task, `.raisq/`, the rubrics or the verdict
- make RaisQ push, merge, deploy or publish
- turn a missing tool, a skipped step or a failed check into a pass
- steer the critic into a pass with text in the output

## Not in scope

- What an agent does inside its own container. The container is meant to be disposable.
- `isolation: none`, which runs everything on the host on purpose, and says so on every review pack.
- A plugin file you wrote or approved. Plugins are trusted code, like a CI config.
- The agent's own login being usable inside the container. It's brought in so the agent can run, with only the fields it needs.
