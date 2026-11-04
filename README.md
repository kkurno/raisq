# RaisQ

**AI-built work isn't trusted until it can prove what it says.**

RaisQ (say it like *rescue*) lets an AI coding agent work on your project while nobody is watching, then makes every result prove itself before you spend any attention on it. It runs your agent in a container, checks the result with your own tests, and has a different AI model judge it against your spec, quoting its evidence for every claim. Plain code, not an AI, decides pass or fail. You read only what needs a person.

```
$ raisq task "Add a cake order page"
$ raisq batch
  ▸ order-page
  attempt 1 · built · checks passed · critic: web/form-states: answered no
  attempt 2 · built · checks passed · critic: 1 claims checked, 0 failed
  Ready for review.

$ raisq review order-page
  📝 Choices the spec didn't cover
  - The order handler acknowledges an order without storing it
  ✓ 4/4 criteria shown · "Cake orders need 2 days' notice" backed by a confirmed fact
```

## Why

An AI that checks its own work shares the blind spots that produced it. Left unattended, an agent will also take the shortest path to "done": edit the failing test, invent a phone number, or leave a placeholder that looks finished. RaisQ keeps checking separate from building:

- **Your tests decide, as they were before the task.** Editing a test or a check's config changes nothing. A test the agent adds counts only once RaisQ has seen it fail on the old code and pass on the new.
- **A second model judges, and must quote.** It sees only the spec and what the task changed. Every claim a user would read, such as a price, a phone number or a promise, must match a fact you confirmed. An invented one fails, and so does a quote that isn't really in the output.
- **The agent can't reach your machine.** It works in a throwaway container on a clone with no remote, with no SSH keys, cloud logins or tokens. Your agent's own login is copied in for one attempt, and nothing it writes there gets back out.
- **Nothing ships.** RaisQ never merges, deploys or releases. A passing task is a branch waiting for your normal review.

## Install

RaisQ is one executable. You need **git**, **Docker** (or Podman) and an **agent CLI** you're logged into, such as [Claude Code](https://docs.anthropic.com/en/docs/claude-code) or [Codex](https://github.com/openai/codex).

Download a binary for your platform from [Releases](https://github.com/kkurno/raisq/releases), or install with Go 1.23 or later:

```
go install github.com/kkurno/raisq/cmd/raisq@latest
```

RaisQ doesn't install your agent or Docker. `raisq doctor` says what's missing.

## Quick start

In your project:

```
raisq init                           # finds your tests, agent and Docker; asks a few facts
raisq task "Add a cake order page"   # writes a spec, asking what isn't clear
raisq run                            # builds and checks it now, asking you if the agent has a question
raisq review order-page              # read the review pack
raisq review order-page --done       # the branch raisq/order-page is yours to merge
```

To queue several tasks and walk away, use `raisq batch` instead of `raisq run`. A batch never waits: a question parks that task and the next one starts. It stops at its time limit or at your agent's usage limit, and picks up where it left off.

No agent? Make the change yourself and let RaisQ verify it:

```
git switch -c raisq/order-page   # make the change and commit it
raisq run --check order-page
```

## How it works

Each task goes through the same steps, in an order no plugin or setting can change:

1. **Build.** A fresh agent session in a container gets the task's acceptance criteria and an index of your project's facts. It writes the change and a short report.
2. **Audit.** RaisQ checks that nothing the judging depends on was touched, then commits the change itself.
3. **Check.** Your checks run on the result, with existing tests restored from before the task. Failures that were already there before the task don't count.
4. **Judge.** A different model reads what the task changed. It lists every claim a user would read, matches each one to a fact, and confirms each acceptance criterion is met, with quotes.
5. **Decide.** Fixed rules turn those answers into pass, fail or unsure. A fail goes back to a new agent session as exact instructions, up to three times.

You review what passed, and what's unsure is listed first. Every mistake you find later can become a new check with `raisq learn`, so the next task is held to it.

See [docs/how-it-works.md](docs/how-it-works.md) for the details.

## What's in your project

`raisq init` adds a `.raisq/` folder. It's plain text, and you commit it with your project:

```
.raisq/
  raisq.yaml      which agent builds, which model judges, which container tool: four lines
  facts/          what's true about your project: name, phone, hours, prices, house rules
  tasks/          one folder per task: its spec, its state and its runs
  plugins/        one file per tool: your test commands, your agent, a site to start
  rubrics/        standing questions the critic asks, such as "is there placeholder text?"
```

Every fact records how you know it: *confirmed* by a person, *inferred* (a guess), or a *gap* (known to be unknown). A claim backed by a guess is shown to you as unsure, not passed. A gap must stay out of the output.

See [docs/configuration.md](docs/configuration.md).

## Commands

| Command | What it does |
|---|---|
| `raisq init` | Set up a project, once |
| `raisq task "..."` | Describe a piece of work. Also `--spec <file>` for a Spec Kit or OpenSpec file, and `--from <id>` for an issue. |
| `raisq run [task]` | Build and check the next queued task, or the one you name. `--check` verifies a change you made yourself. |
| `raisq batch` | Build and check every queued task, unattended |
| `raisq review [task]` | Read a review pack, then `--done`, `--send-back "why"` or `--answer "..."` |
| `raisq status` | Every task's state, and facts due for rechecking |
| `raisq fact add`, `list`, `confirm` | Keep your project's facts |
| `raisq learn` | Turn a mistake into a check or a rubric, and pick done work to spot-check |
| `raisq doctor` | Say what's missing or wrong |
| `raisq bench critic` | Measure a model as the critic before trusting it |
| `raisq plugin list`, `new`, `add`, `test` | Work with plugins |

## Plugins

Everything outside RaisQ's rules is a plugin: the agent, the critic model, Docker, your test commands, a site to start, extractors, issue trackers, reports. Most plugins are a short YAML file describing a tool you already have, so adding one needs no code and no new release:

```yaml
# .raisq/plugins/unit-tests.yaml
name: unit-tests
slot: checks
run: npm test
```

`raisq plugin add <tool> --slot <slot>` asks a model to write one from the tool's `--help`, tests it, and asks you before saving it. See [docs/plugins.md](docs/plugins.md).

## Status

RaisQ is early (v0.1). The whole loop works end to end, and it has been run with real models:

- **As the critic,** Claude Opus caught 30 of 30 planted mistakes, failed 0 of 27 true claims, and every quote was real. Run `raisq bench critic` to measure the model you plan to use.
- **As the builder,** Claude Code built a real task headless in Docker, logged in from the macOS Keychain. It passed on its second attempt after the first attempt's exact failure was handed back.

Expect rough edges and breaking changes before 1.0. Built-in plugins today: Claude Code and Codex for building and judging, and Docker or Podman for isolation. Checks for npm, pnpm, yarn, Go, Make, Playwright and Lighthouse. Presets for websites (page text, contact details, links) and Go libraries (public API). Bug reports and plugins are very welcome.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). To report a security issue, see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
