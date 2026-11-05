# Plugins

Everything outside RaisQ's rules is a plugin in a **slot**. A slot is an interface RaisQ defines, such as "make the change" or "run a check", and a plugin fills it. RaisQ's rules never depend on a plugin, so swapping your agent, container tool or test runner never changes how a result is judged.

Most plugins are a YAML file in `.raisq/plugins/` describing a tool you already have. A new tool needs a file, not a new RaisQ release.

## Slots

`raisq slots` lists them:

| Slot | Job | Picked by |
|---|---|---|
| `builder` | Make the change: an agent CLI | `raisq config builder <name>` |
| `model` | The critic, and every AI job with no tools | `raisq config critic <name>` |
| `isolation` | Keep each attempt off your machine | `raisq config isolation <docker\|podman\|none>` |
| `checks` | Pass, fail or a number, from a command | every file with this slot runs |
| `provides` | Something a check needs: a site, a database, a test secret | by name, when a check asks for it |
| `extractor` | Turn the result into text for the critic | every file with this slot runs |
| `spec-source` | Turn an issue id into a task | the one whose `matches:` fits |
| `report` | Deliver review packs and batch reports | every file with this slot runs |

`checkout` (git) and `store` (files in `.raisq/`) are built in.

Every file needs `name` and `slot`. Everything else depends on the slot.

## checks

A check is a command. Exit 0 passes, and what it prints goes to the agent's handoff and your review pack.

```yaml
# .raisq/plugins/unit-tests.yaml
name: unit-tests
slot: checks
priority: 20
run: npm test
run_one: npx vitest run {file}
locks: ["vitest.config.*"]
tests: ["**/*.test.*", "**/*.spec.*"]
```

| Field | What |
|---|---|
| `run` | The command, run in the container |
| `priority` | Lower runs first; equal ones run alphabetically |
| `run_one` | Runs one test file, `{file}`. With it, RaisQ can prove a test the agent added fails on the old code. Without it, new tests aren't proven. |
| `tests` | Globs of this check's test files: restored from before the task, and the ones the agent added are proven |
| `locks` | Globs of files that configure the check: restored from before the task, so the agent can't loosen it |
| `needs` | Providers to start first, such as `[site]` |

A check whose result is a number:

```yaml
name: lighthouse
slot: checks
run: npx lighthouse http://localhost:3000 --output=json --output-path=stdout --chrome-flags="--headless --no-sandbox"
needs: [site]
result: {from: json, path: categories.performance.score, scale: 100}
pass_at: 90
median: 3          # run 3 times, the median counts
```

RaisQ never skips a check because of which files changed: it can't know what a change affects. A slow check can narrow itself with its own tool, such as `vitest --changed`.

### Built-in checks

These ship inside RaisQ. Use them with `run: builtin:<name>`:

| `run:` | What it fails on |
|---|---|
| `builtin:contacts` | A phone number or email on any page that isn't in your facts. Needs `site`, and `port:` says which port it serves on. |
| `builtin:links` | An internal link that doesn't resolve. Needs `site`, and `port:` says which port it serves on. |
| `builtin:go-api` | A Go library's exported name removed or changed, unless the task's spec says `changes_api: true`. Adding is fine. |

## provides

What a check or extractor `needs:` by name. Nothing starts unless something asked for it, and nothing here reaches the agent while it's building.

A service started inside the container after the agent has exited:

```yaml
name: site
slot: provides
run: npm run build && npm run start
port: 3000
ready: http://localhost:3000
```

`port` is where it listens, and `ready` is a URL RaisQ waits for before running the checks that need it.

A secret for the checks only, read on your machine and given to the checks as environment after the agent has exited:

```yaml
name: env.test
slot: provides
from: host:.env.test
```

The agent never has it, so it can't send it anywhere. If the agent needs to run tests while working, give it `.env.test.example` and tell it in the task that real values arrive at check time.

A read-only reference for the agent itself, with nothing secret in it:

```yaml
name: design-system
slot: provides
for: builder
from: host:../design-system:ro
```

It's mounted read-only while the agent builds, so the agent can't change it. With `isolation: none` there's no container and nothing is mounted. `raisq doctor` refuses one that looks like a secret.

## builder

The builder can be any AI coding agent that has a command-line tool and can run without asking for permission. RaisQ ships plugin files for the ones tested so far, and `raisq init` sets them up when it finds them:

| Agent | Plugin | Login brought in |
|---|---|---|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `claude-code` | the login file on Linux, the Keychain on macOS |
| [Cursor](https://cursor.com/cli) | `cursor` | the Keychain on macOS |

For any other agent, write a plugin file like the one below, or let `raisq plugin add <agent> --slot builder` draft one from its `--help`, then choose it with `raisq config builder <name>`. The critic is a `model` plugin and works the same way: any AI that can answer a prompt with no tools.

Here's the built-in Claude Code plugin as an example:

```yaml
name: claude-code
slot: builder
vendor: anthropic
model: sonnet
tool:
  find: claude --version
  install_hint: npm i -g @anthropic-ai/claude-code
# Runs inside the container, unattended, as an unprivileged user.
run: claude -p --model sonnet --dangerously-skip-permissions < {prompt}
install: npm i -g @anthropic-ai/claude-code
# Claude Code reads a project's skills here, not in the standard .agents/skills.
skills: .claude/skills
# Your existing login, copied in for one attempt and never mounted: a file on
# Linux, the Keychain on macOS (the first time, macOS asks to allow it). Only
# the fields below go in. The refresh token, and any other tool's tokens kept
# alongside, such as MCP servers', stay on this machine, so what goes in
# expires on its own and reaches nothing but Claude.
login:
  - to: /home/raisq/.claude/.credentials.json
    file: ~/.claude/.credentials.json
    command: security find-generic-password -s "Claude Code-credentials" -w
    keep:
      - claudeAiOauth.accessToken
      - claudeAiOauth.expiresAt
      - claudeAiOauth.scopes
      - claudeAiOauth.subscriptionType
      - claudeAiOauth.rateLimitTier
    expires_at: claudeAiOauth.expiresAt
    refresh: claude -p "Reply with ok" >/dev/null
# Used instead, when set: an API key, or a token from `claude setup-token`.
login_env: [ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN]
image: node:22-bookworm
```

| Field | What |
|---|---|
| `run` | Runs in the container, in the project. `{prompt}` is the prompt file. It must write `/raisq/out/result.json` before it exits. |
| `vendor`, `model` | So review packs can say when the critic is the builder's vendor or model |
| `tool` | `find`: a command that shows the agent is installed on your machine, such as `claude --version`. `install_hint`: how a person installs it, which `raisq doctor` shows when it's missing. |
| `install` | Installs the agent in a fresh container, when the image lacks it |
| `image` | The container image |
| `login` | Where the agent's login is on your machine, and where it goes inside |
| `login_env` | Variables passed in when set. If any is set, it's used instead of `login`, and the login isn't copied in. |
| `skills` | Where the agent reads a project's skills, if not the standard `.agents/skills`. `raisq skill <agent>` writes RaisQ's skill there too. Claude Code's is `.claude/skills`. |

Each `login` entry:

| Field | What |
|---|---|
| `to` | Where the agent expects it in the container, as a file |
| `env` | The variable it goes in instead, for an agent that reads its login from the environment, such as Cursor's `CURSOR_AUTH_TOKEN` |
| `file` | Where it is on your machine. A folder is copied whole, so use a folder only when the agent keeps nothing else in it. |
| `command` | Prints it, when it's not a file, such as the macOS Keychain. Used when `file` isn't there. |
| `keep` | The JSON fields that go in, and nothing else. A login store can hold other tools' tokens, so this lists what to keep, never what to drop. |
| `expires_at` | The field with the login's expiry |
| `refresh` | Renews the login on your machine when it would expire mid-attempt |

The login is copied into each container, never mounted, so nothing the agent writes there reaches your machine. A login that goes in as a variable is passed through a private file, never on a command line, and only the access token goes in: Cursor's refresh token stays in your Keychain.

### What the agent reports

The agent writes `/raisq/out/result.json`:

```json
{"status": "done", "summary": "Added the order page"}
```

`status` is `done`, `failed`, `question` (with `"question": "..."`) or `needs-something` (with `"needs": "..."`). It may also write `/raisq/out/choices.json`, a list of `{"what": "...", "why": "..."}` for decisions the spec didn't make. The prompt tells the agent all of this.

## model

The critic, and every other AI job with no tools, such as writing a spec or drafting a plugin. It runs on your machine, reads the prompt on stdin, and answers with JSON on stdout. It can be any AI that does that. The built-in `claude-code-critic` and `cursor-critic` are the ones tested so far.

```yaml
name: claude-code-critic
slot: model
vendor: anthropic
model: opus
run: claude -p --model {model} --tools "" --output-format text
```

It must have no tools: it judges what it's shown and nothing else. It can be any model, even the builder's, but a different one shares fewer of its blind spots, and every review pack says when they match. However good a model is said to be, measure it on your setup before trusting it: `raisq bench critic --runs 3` gives it planted mistakes and fails it below the bar.

## extractor

Turns the result into text for the critic. `{base}` and `{head}` are the commits before and after the task. Print nothing when there's nothing for it in this result.

```yaml
name: pdf-text
slot: extractor
run: git diff --name-only {base} {head} -- '*.pdf' | xargs -r -n1 pdftotext -layout - -
```

The `git-diff` extractor is always on. `run: builtin:page-text` adds the text of every page the task changed, as a user reads it, with the changed lines marked. It needs `site`.

## spec-source

Turns an issue id into the text of a task, for `raisq task --from <id>`. It runs on your machine, so its token never enters a container.

```yaml
name: jira
slot: spec-source
matches: "[A-Z]+-[0-9]+"
run: jira issue view {id} --plain
```

What comes in this way is marked as coming from outside, so the agent treats it as a description of the work, not as instructions.

## report

Gets each review pack on stdin, with `RAISQ_TASK`, `RAISQ_STATE` and `RAISQ_BRANCH` set. It runs on your machine. It may post a draft or a note, never merge or deploy. If it can't reach its service, the local review pack is complete anyway.

```yaml
name: draft-pr
slot: report
run: '[ "$RAISQ_STATE" = ready ] && git push -q origin "$RAISQ_BRANCH" && gh pr create --draft --head "$RAISQ_BRANCH" --title "$RAISQ_TASK" --body-file -'
```

## Telling the agent how to use a tool

Any plugin can add instructions to the agent's prompt:

```yaml
for_ai:
  when: Changing anything a user sees in the browser
  how: Add or update a Playwright test for every page you change.
  never: Never update screenshots to make a test pass.
```

They go with the plugin: remove it with `raisq plugin remove <name>`, and the instruction goes too.

## Writing a plugin

```
raisq plugin list                     # every plugin file in the project
raisq plugin new checks my-check      # a starting file for a slot, with <...> to fill
raisq plugin test my-check            # checks it against its slot's contract
raisq plugin add mytool --slot checks # a model drafts it from `mytool --help`, tests it, asks before saving
raisq plugin remove my-check          # stop using one
```

A plugin for one task only goes in `.raisq/local/tasks/<task>/plugins/`.

A plugin fills its slot and nothing more. It can't skip a step, change a verdict or reorder the attempt, and one that runs your project's code runs inside the container.
