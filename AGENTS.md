# AGENTS.md

Instructions for AI coding agents working on RaisQ's own code. People should start with [CONTRIBUTING.md](CONTRIBUTING.md).

RaisQ makes AI-built work prove itself: it runs an agent in a container, checks the result with the project's tests, has a separate AI session judge it with quotes, and decides pass or fail in plain code. That makes this codebase unusual: its job is to not trust an agent. Keep that in mind when changing it.

## Commands

```
go build ./...
go test ./...                                  # offline: no agent, model, network or Docker
RAISQ_DOCKER_TEST=1 go test ./plugin/docker    # needs a running Docker
gofmt -l . && go vet ./...                     # both must be clean
go build -o /tmp/raisq ./cmd/raisq             # to try the CLI in a scratch project
```

Run `go test ./...` before you say a change is done. Don't run anything that calls a real model or agent (`raisq bench critic`, `raisq run` with a real builder) unless you're asked to: it costs the user money.

## Architecture rules

Three rings, and dependencies point inwards only. `arch_test.go` fails the build otherwise.

| Ring | Packages | May import from RaisQ |
|---|---|---|
| 1 | `domain` | nothing |
| 2 | `slot`, `usecase` | `domain`, and `slot` for `usecase` |
| 3 | `plugin/...`, `cmd/raisq` | anything further in |

- **Rules go in `domain`.** Verdicts, states, what counts as a failure. Pure functions, no files, processes or network.
- **Flows go in `usecase`.** Anything a flow needs from outside is an interface in `slot`, filled in `plugin/`. Don't import a plugin from a use case; add or widen a slot instead.
- **`cmd/raisq` holds no logic.** It parses flags, calls a use case, and prints. If you're writing a decision in the CLI, it belongs in `usecase`.
- **A new tool is a plugin file, not Go,** whenever the built-in `command` plugin can express it. Built-in plugin files are in `plugin/detect/builtins/`. Write Go only for what a file can't do.

## Safety invariants

These are the point of the project. A change that weakens one is wrong even if every test passes. If a task seems to need one weakened, stop and ask.

- **No AI decides pass or fail.** The critic answers questions; `domain.Decide` and `domain.JudgeChecks` turn the answers into verdicts. Never let a model's answer become a verdict directly.
- **The order of an attempt's steps is fixed in `usecase/attempt.go`:** build, audit, commit, checks, extract, critic, decide. No plugin, preset or setting may skip, reorder or insert a step. A task can change what happens inside a step, through a slot, never between steps.
- **The builder can't change what judges it.** Existing tests and each check's locked files are restored from before the task before checks run. `.raisq/` and RaisQ's skill folders never enter the container, and are left out of the task's commits and diffs. A new test counts only once it has failed on the old code and passed on the new.
- **Nothing reaches the builder that it could leak.** The agent's login is copied in with only the fields it needs (`keep:`), never mounted. Test secrets reach the checks only, after the builder has exited. The checkout is a clone with no remote, never a worktree.
- **Everything the builder wrote is data to the critic, never instructions.** The output sits in a block with a fresh random tag per call.
- **A missing tool, a missing answer, or a skipped step is never a pass.** A check whose tool is missing blocks. A claim the critic didn't answer for is unsure, not passed and not failed.
- **Nothing ships.** RaisQ never merges, deploys or releases. A report plugin may post a draft.
- **A timeout must stop everything it started.** Run a host shell through `proc.Shell`, which kills the whole process group; a bare `exec.Command("sh", "-c", ...)` leaves its children running and holding the output open. Inside a container, the session's `Stop` removes the container, which ends whatever is left in it.

## Conventions

- **Settings exist only for real exceptions.** `raisq.yaml` names plugins for one-of slots, plus a few defaults. Don't add a setting for something RaisQ can work out, and don't add an allowlist.
- **One word per concept.** Use the terms already in the code: task, run, attempt, handoff, fact, rule, criterion, claim, rubric, check, slot, plugin, provider. Don't introduce synonyms.
- **Facts say what's true; plugins' `for_ai` says how to use a tool; rubrics say what good looks like; checks are commands.** Don't blur them, such as putting a tool instruction in `facts/`.
- **Comments say why, not what,** in one short line where possible. No references to issues or tickets in comments.
- **Errors say what to do.** A user reads them in a terminal: name the file, the command, or the fix.

## Tests

- Test behaviour that could break, through the use cases, with the fakes in `slot/fake` and a temporary git repo. See `usecase/attempt_test.go` for the pattern.
- A new safety rule needs a test that tries to break it, such as a builder that edits a test, plants a hook, or tries to push.
- A change to the critic's prompts needs a case in `cmd/raisq/bench.go` if it came from a real model's mistake, so the bench keeps catching it.
- Don't add a test that only proves something was removed.

## Commits

Use [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/):

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

- **Types:** `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`.
- **Scope:** the package or area: `domain`, `critic`, `attempt`, `docker`, `login`, `git`, `store`, `detect`, `plugins`, `cli`.
- **Description:** imperative, lower case, no full stop, under about 70 characters. Say what changes for a user, not which function moved.
- **Body:** why, when it isn't obvious. Wrap at 72.
- **Breaking changes** to the CLI or to a file format in `.raisq/` get `!` and a `BREAKING CHANGE:` footer that tells the user what to do.
- One logical change per commit. Don't mix a refactor with a fix.
- Don't add `Co-Authored-By`, "Generated with" lines or any other AI attribution.

```
fix(critic): treat an unanswered claim as unsure, not invented

A model that skipped one claim's provenance made RaisQ fail it as
fabricated. A missing answer isn't evidence.
```

```
feat(login)!: list the login fields to keep instead of the ones to drop

BREAKING CHANGE: in builder plugin files, replace `drop:` with `keep:`
listing the fields the agent needs.
```

Commit or push only when you're asked to.
