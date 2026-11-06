# Configuration

`raisq init` writes the folders and this machine's settings. The checks, rubrics and facts come from setting up the project in your agent's chat (`/raisq set up this project`), or from the commands in [Changing the setup](#changing-the-setup). You can change any of it afterwards, with those commands or by editing the files.

```
.raisq/
  facts/          what's true about the project          shared
  plugins/        the checks and tools RaisQ runs        shared
  rubrics/        standing questions for the critic      shared
  local/          this machine's: never committed
    config.yaml     which agent and container tool to use here
    tasks/          your tasks, their runs and review packs
    snapshots/      the facts as each run saw them
    baselines/      what already failed before a task
    batch-report.md your last batch
```

**Shared** is how the project's work is judged, so everyone's is judged the same way: commit it like any other file. A lesson one person teaches RaisQ is then caught for everyone.

**`local/`** is who runs RaisQ on this machine, and the work in progress. It has its own `.gitignore`, so nothing in it is ever committed, however RaisQ grows. Tasks are personal, like local branches: their runs and `raisq/<task>` branches exist only on your machine. To share a piece of work with your team, put it in your issue tracker or a spec file, which [RaisQ can read](how-it-works.md#specs-from-elsewhere).

`init` also writes RaisQ's agent skill to `.agents/skills/raisq/SKILL.md`, so you can use RaisQ from your agent's chat. Commit it too, so your teammates' agents find it. It's written by RaisQ, so don't edit it: `raisq skill` overwrites it. See [Using RaisQ from chat](../guides/chat.md).

**Joining a project that uses RaisQ:** after cloning it, run `raisq init`. It sets up only your machine: it finds the agent and container tool you have, writes `local/config.yaml`, and leaves the shared files alone. Until then, every command says to run it.

**`raisq init` is safe to run again,** such as after updating RaisQ. It fills in only what's missing, such as the agents' plugin files or the skill, and refreshes an old skill. It never changes your settings, or a file that's there.

**A project set up before `local/` existed** is moved on the first command: `raisq.yaml` becomes `local/config.yaml`, and tasks, fact history, baselines and the batch report move into `local/`. RaisQ says what it moved. Commit the removals git then shows under `.raisq/`.

## local/config.yaml

This machine's settings. Change them with [`raisq config`](#changing-the-setup); each person has their own.

```yaml
builder: claude-code
critic: claude-code-critic
isolation: docker
```

Each line names a plugin for a slot that takes exactly one, because a choice has to be made:

| Key | What | Notes |
|---|---|---|
| `builder` | The plugin that builds | A file in `plugins/` with `slot: builder` |
| `critic` | The plugin that judges, and does every other AI job with no tools | A file with `slot: model`. Any model works. If it's the builder's vendor or model, every review pack says so. |
| `isolation` | Where attempts run | `docker`, `podman`, or `none` for a disposable machine |

Defaults you can override, if you need to:

| Key | Default | What |
|---|---|---|
| `attempts` | `3` | Tries per task, the first included, before it's parked. A task stops sooner if two tries in a row fail the same way. |
| `batch` | `8h` | How long a batch keeps starting new tasks |
| `split_over` | `6` | Criteria a task may have before `raisq task` offers to split it |
| `spot_check` | `0.2` | Share of done tasks `raisq learn` picks for you to re-read |
| `keep_artifacts` | `30d` | How long old runs are kept. Older ones are removed when a batch starts. The latest run of every task, and every run of a parked task, are always kept. |

There's deliberately no list of checks, no setup or start command, and no allowlist of hosts or commands. Checks are plugin files, the agent installs what it needs, and anything that enters the container goes through a provider.

## Changing the setup

Change what `init` chose with commands. They check a change before saving it, so a typo can't stop the next run:

| To | Run |
|---|---|
| See every setting, defaults included | `raisq config` |
| Change one | `raisq config <key> <value>`, such as `raisq config builder cursor` or `raisq config attempts 5` |
| Put one back to its default | `raisq config --unset <key>` |
| Add checks or providers | `raisq plugin add <file or built-in>...`: each is tested, and every check is tried once on the project first. `raisq plugin new <slot> <name>` writes a file to fill in; `raisq plugin list --built-in` lists RaisQ's own. |
| Add rubrics | `raisq rubric new <area>` writes a file to fill in; `raisq rubric add <file or set>` adds it, such as `raisq rubric add web` |
| Stop running a check | `raisq plugin remove <name>` |
| Add or change a fact | `raisq fact add <id> "text"` |

What the commands take care of:

- **Choosing a built-in agent brings in its plugin file.** `raisq config builder cursor` copies RaisQ's `cursor.yaml` into `plugins/`, for you to commit and edit. A plugin file the project already has is never overwritten.
- **A change that removes a safeguard asks first.** `raisq config isolation none` explains what it means and asks you to confirm. Without a terminal, such as from your agent's chat, it needs `--yes`.
- **Nothing a run depends on can be removed.** `raisq plugin remove` refuses the builder, the critic, the isolation in use, and a provider a check still needs. Removing a [lesson](../guides/tasks.md#learning-from-mistakes) check says which task it came from, and that its mistake won't be checked any more.
- **A running task isn't affected.** It keeps the settings it started with, and a change applies from the next run.
- **Your comments in `local/config.yaml` are kept.**

You can still edit any file by hand when you know what you're changing, for example a check's command in its plugin file. For facts, prefer `raisq fact`: it sends back the ready tasks that relied on a fact that changed, which an edit by hand doesn't. Marking such a task done is still refused, because its result no longer matches the facts.

## facts/

What's true about your project, one file per topic. RaisQ reads every `.yaml` under `facts/`, however you arrange it.

```yaml
# .raisq/facts/orders.yaml
- id: orders/delivery-radius
  text: Delivery is offered within 5 km
  kind: terms
  tag: confirmed
  said_by: Mai (owner)
  said_on: 2026-11-03

- id: orders/hours
  text: Tue–Sun 8:00–18:00
  kind: hours
  tag: inferred
  needs: stakeholder

- id: orders/minimum
  tag: gap

- id: orders/phone-required
  text: Every order must have a phone number
  kind: rule
  tag: confirmed
```

| Field | What |
|---|---|
| `id` | A name, such as `orders/delivery-radius`. Not a path: moving the file changes nothing. |
| `text` | The fact. What the agent is told and what the critic quotes. |
| `tag` | *confirmed* (a person said so), *inferred* (a guess, or a draft nobody checked) or *gap* (known to be unknown) |
| `kind` | `contact`, `hours`, `price`, `terms` or `rule`. Sets how often it must be reconfirmed. |
| `said_by`, `said_on` | Who said it, and when. A fact you give is recorded with your name from `git config user.name`, so a teammate reading it knows who said so. |
| `needs: stakeholder` | Someone else, such as a client, must confirm it. It stays inferred until `raisq fact confirm <id> --by "<who>"` records who did. |
| `recheck` | Override how often it must be reconfirmed, such as `90d` or `12w` |
| `see` | Another fact's id, so a fact shown in two places is written once |

**How the tag changes a verdict:** a claim backed by a confirmed fact passes. Backed by an inferred one, or a confirmed one past its recheck date, it's unsure and shown to you first. A claim about a gap fails: the agent was told to leave it out.

**Confirmations expire** for facts that change in the world. By default, contact details every 180 days, hours and terms every 90, prices every 30. A `rule` never expires. `raisq status` lists what's due.

**Rules are facts too.** "All copy in British English", "use the design system's components", "don't refactor what the task didn't ask for": constraints the output must respect go here with `kind: rule`, and the critic judges them.

**Nothing is lost.** Git keeps every version of a fact, with who changed it and when. Each run also takes a snapshot of the facts in `local/snapshots/`, so what it's judged against can't change under it.

**When a teammate changes a fact you relied on,** you get the change with `git pull`. A ready task that relied on the old version can't be marked done, because its result no longer matches the facts: send it back to rebuild it.

Use `raisq fact` rather than editing by hand, so a change finds the tasks that relied on it:

```
raisq fact add business/phone "02-123-4567" --kind contact
raisq fact confirm business/hours
raisq fact list --kind rule
raisq fact list --tag inferred
raisq fact list --due
```

A project may have no facts at all. A library has no phone number; the critic then checks criteria and rubrics only.

## local/tasks/

One folder per task, written by `raisq task`:

```
local/tasks/order-page/
  spec.yaml       its acceptance criteria
  state.yaml      queued, running, ready, parked or done, and why
  runs/           every run: what it was judged against, each attempt's prompt, result, checks, verdicts and handoff
  review.md       the latest review pack
  plugins/        optional: a plugin for this task only
```

```yaml
# .raisq/local/tasks/order-page/spec.yaml
name: order-page
title: Add a cake order page
criteria:
  - {id: c1, text: There is a page at /order with a form to order a cake}
  - {id: c2, text: The form asks for a delivery date}
  - {id: c3, text: The order page says how much notice an order needs}
```

A spec says what must be true when the task is done, never how to do it. `changes_api: true` lets a task break a Go library's public API, which the `go-api` check otherwise fails.

## plugins/

One file per tool RaisQ runs. See [plugins](plugins.md).

## rubrics/

Standing questions the critic asks of every task: what good looks like for this kind of project, which no task's criteria would say.

```yaml
# .raisq/rubrics/web.yaml
- id: web/no-placeholder
  ask: Does any text a user would see look like placeholder or sample text?
  fail_if: yes
- id: web/form-states
  ask: Does every form show what happened after submitting, success or error?
  fail_if: no
```

The critic answers *yes*, *no* or *unsure* with a quote, or *n/a* when the output has nothing the question applies to. A question belongs here when a command can't answer it. If a command can, make it a check.

Lessons from `raisq learn` that need judgment land in `rubrics/lessons.yaml`. Lessons a command can check land in `plugins/` as `lesson-<name>.yaml`. Either way, a `lesson:` line says which task they came from.
