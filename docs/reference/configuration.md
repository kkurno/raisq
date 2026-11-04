# Configuration

`raisq init` writes everything below. Most projects never edit more than `facts/`.

```
.raisq/
  raisq.yaml
  facts/
  tasks/
  plugins/
  rubrics/
```

All of it is plain text, meant to be committed. Run history, pinned versions and baselines are local and listed in `.raisq/.gitignore`.

## raisq.yaml

```yaml
preset: web
builder: claude-code
critic: claude-code-critic
isolation: docker
```

Each line names a plugin for a slot that takes exactly one, because a choice has to be made:

| Key | What | Notes |
|---|---|---|
| `preset` | The kind of project `init` found, which picked the starting plugins and rubrics | `web` adds page text, contact-detail and link checks, and website rubrics. `library-go` adds the public API check. A list when a project is more than one kind. After `init`, the files are what count. |
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
| `keep_artifacts` | `30d` | How long old runs are kept. The latest run of every task, and every run of a parked task, are always kept. |

There's deliberately no list of checks, no setup or start command, and no allowlist of hosts or commands. Checks are plugin files, the agent installs what it needs, and anything that enters the container goes through a provider.

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
| `said_by`, `said_on` | Who said it, and when |
| `needs: stakeholder` | You can't confirm this one yourself; it stays inferred until the stakeholder has |
| `recheck` | Override how often it must be reconfirmed, such as `90d` or `12w` |
| `see` | Another fact's id, so a fact shown in two places is written once |

**How the tag changes a verdict:** a claim backed by a confirmed fact passes. Backed by an inferred one, or a confirmed one past its recheck date, it's unsure and shown to you first. A claim about a gap fails: the agent was told to leave it out.

**Confirmations expire** for facts that change in the world. By default, contact details every 180 days, hours and terms every 90, prices every 30. A `rule` never expires. `raisq status` lists what's due.

**Rules are facts too.** "All copy in British English", "use the design system's components", "don't refactor what the task didn't ask for": constraints the output must respect go here with `kind: rule`, and the critic judges them.

**Nothing is overwritten.** Every change is a new version, and the old one is kept so a run can read the version it started on.

Use `raisq fact` rather than editing by hand, so a change finds the tasks that relied on it:

```
raisq fact add business/phone "02-123-4567" --kind contact
raisq fact confirm business/hours
raisq fact list --kind rule
raisq fact list --tag inferred
raisq fact list --due
```

A project may have no facts at all. A library has no phone number; the critic then checks criteria and rubrics only.

## tasks/

One folder per task, written by `raisq task`:

```
tasks/order-page/
  spec.yaml       its acceptance criteria
  state.yaml      queued, running, ready, parked or done, and why
  runs/           every run: what it was judged against, each attempt's prompt, result, checks, verdicts and handoff
  review.md       the latest review pack
  plugins/        optional: a plugin for this task only
```

```yaml
# .raisq/tasks/order-page/spec.yaml
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

Lessons from `raisq learn` land in `rubrics/lessons.yaml` with a `lesson:` line saying which task they came from.
