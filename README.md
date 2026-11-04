# RaisQ

Say it like "rescue". AI-built work isn't trusted until it can prove what it says.

RaisQ lets AI agents do project work while nobody is watching, then proves each result against a written spec before a person reviews it. You describe the work. An AI builds it, isolated from the rest of your machine. RaisQ checks it with your tests and with a second AI that must show its evidence. You read only what needs a person.

**Status:** v0.1. Everything in the plan is built, and both spikes pass with real models: Claude Code builds headless in Docker, and Claude judges as the critic. See [What's proven](#whats-proven).

## Install

Needs Go 1.23 or later to build, then nothing but git at run time.

```
go build -o ~/bin/raisq ./cmd/raisq
```

To run tasks unattended you also need Docker (or Podman) and an agent CLI you're logged into, such as Claude Code or Codex.

## Use

```
raisq init                          set up the project, once
raisq task "Add a cake order form"  describe a piece of work; raisq asks about what's unclear
raisq run [task]                    build and check the next queued task, or the one you name
raisq batch                         build and check every queued task, while you're away
raisq review [task]                 read what needs a person, then decide
```

`raisq run` is watched: if the builder has a question, you're asked right away. `raisq batch` never waits: a question parks the task and the next one starts. A batch stops at its time limit or at the agent's usage limit, and resumes from where it stopped.

Without an agent, build by hand and let RaisQ verify it:

```
git switch -c raisq/<task>    # make the change, commit
raisq run --check <task>
```

From a spec tool or a tracker:

```
raisq task --spec specs/001-cake-orders/spec.md   # Spec Kit, OpenSpec, any Markdown spec
raisq task --from SHOP-123                        # through a spec-source plugin
```

Now and then:

| Command | When |
|---|---|
| `raisq status` | each task's state, and facts due for rechecking |
| `raisq fact add`, `list`, `confirm` | a fact changes; `list --kind rule`, `--tag inferred`, `--said-by Mai`, `--due` |
| `raisq review <task>` with `--done`, `--send-back "why"`, `--answer "..."` or `--overrule <lesson>` | deciding |
| `raisq learn <task> "what was wrong"`, optionally `--check "cmd" --bad <commit> --good <commit>` | a mistake got through; it becomes a check or a rubric |
| `raisq learn` | lessons to retire, and a sample of done tasks to spot-check |
| `raisq doctor` | something isn't working |
| `raisq bench critic` | before trusting a model as the critic |
| `raisq slots`, `raisq plugin list`, `new`, `add`, `test` | plugins |

### Logging the agent in

Nothing to set up: log in to your agent CLI once, the way you normally do, and RaisQ brings that login into each container for one attempt. On macOS it reads Claude Code's login from the Keychain, and macOS asks once to allow it; on Linux it reads the login file. Only the fields the agent needs go in. The refresh token and any other tool's tokens kept alongside, such as MCP servers', stay on your machine, so what's inside expires on its own. A login about to expire is renewed first. `raisq doctor` says if it can't find one. An `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` in your environment is used instead, when set.

## What's proven

| Proven here | How |
|---|---|
| The verdict rules, the stuck rule, expiry, fact revisions | Unit tests, offline |
| The whole loop, fail then fix forward, with a handoff | Use-case tests with fake plugins and real git |
| A builder can't pass by editing a test or a check's config, or by adding a test that tests nothing | Use-case tests, and a real vitest project in Docker |
| An invented claim fails; a claim against a changed fact fails | Use-case tests; the bakery fixtures in Docker, from both the diff and the served pages |
| A wrong phone number on a page fails the built-in contacts check | The web fixture, with a real site served in Docker |
| A removed export fails the Go API check, unless the spec says `changes_api: true` | A Go library fixture in Docker |
| A planted git hook fails the audit; the checkout has nothing to push to; the builder never sees `.raisq/` | Git plugin and use-case tests |
| The checkout is writable, the spec read-only, your home invisible, and a write to the login inside never reaches the host | Docker test against the real daemon (`RAISQ_DOCKER_TEST=1`) |
| Every command in the container runs as an unprivileged user, so agents that refuse to skip prompts as root still run | The vitest and web fixtures in Docker |
| The Keychain login reaches the container without the refresh token or other tools' tokens, and Claude Code answers inside | A probe against this Mac's real login |
| Changing a fact sends back only the tasks that relied on it; a stale result can't be marked done | Use-case tests and the bakery fixture |
| A big task splits into parts on one branch, then the whole is checked | Use-case test and the fixture |
| A watched question is asked and settled into the spec; an unwatched one parks | Use-case tests |
| A converted spec stops on a dropped line or an invented quote | Use-case tests |
| A lesson's check counts only if it fails on the mistake and passes on the fix | Use-case test |
| The critic's data block can't be closed by the builder's text | Use-case test |
| The rings import only inwards | `arch_test.go` |

### The two spikes, with real models

**S1, the critic** (`raisq bench critic --runs 3`, Claude Opus through the `claude` CLI, no tools): **30 of 30** planted mistakes caught, **0 of 27** true claims failed, every quote found in the output. The planted mistakes include a wrong phone number, a stretched lead time, hours that don't match, a free-delivery promise nobody made, a "since 1998" nobody said, and a comment telling reviewers everything is confirmed.

**S2, the builder** (Claude Sonnet through Claude Code, headless in Docker, logged in from this Mac's Keychain with only its own login fields): a four-criterion task on the bakery fixture **passed in 2 attempts**. Attempt 1 built the order page and passed every check and criterion, but its form showed nothing after submitting, which the `web/form-states` rubric failed. Attempt 2 got that one failure as its handoff, added a status message and a handler, and passed. The provenance it had already settled was reused, so attempt 2's critic made one call instead of two. The review pack listed two choices the spec didn't cover, including that the new handler acknowledges an order without storing it.

Running the real agent found four things the scripted ones couldn't, all fixed and added to the bench: the critic judged text that was on a page before the task; it treated a form's own messages ("Order sent.") as claims about the business; it called a claim that dropped one word ("Cake") a contradiction; and a hung model call outlived its timeout, because killing `sh` didn't kill the agent it started.

## The plan

Everything is in [docs/raisq-build-plan.md](docs/raisq-build-plan.md). Start with:

- **Section 1:** the design concepts every decision is judged against
- **Section 3:** what using RaisQ looks like
- **Section 12:** the two spikes to run first, and what done looks like
- **Appendix B:** the decisions made so far, and why

## The code

```
domain/       ring 1: the rules. Facts, tasks, checks, verdicts. Imports nothing.
usecase/      ring 2: the flows. The attempt's order of steps lives here.
slot/         ring 2: the interfaces the flows need from outside, and fakes for tests
plugin/       ring 3: git, store, docker, agent, command (plugin files), builtin, detect, registry
cmd/raisq/    ring 3: the CLI
```

CI is in `.github/workflows/`. `docs/examples/raisq-batch.yml` is a scheduled batch for a project that uses RaisQ.

## The idea in one paragraph

An AI that re-reads its own work shares the blind spots that produced it. So RaisQ keeps checking separate from building. Every attempt is a fresh AI session. Plain code runs the tests and decides pass or fail, and a test the AI wrote counts only once it has failed on the old code and passed on the new. A different AI model, which sees only the spec and the output, must quote its evidence for every fact. Failures go back as exact instructions, a limited number of times. People define what's correct, answer what the AI couldn't find out, and decide only the hard cases. Every mistake found later becomes a new check, so RaisQ gets stricter over time.
