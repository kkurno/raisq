# How RaisQ works

RaisQ answers two questions about every piece of AI-built work:

- **Did it do what was asked?** Your checks run, and a critic model confirms each acceptance criterion is met, quoting the result.
- **Is what it says true?** For anything a user would read (a website, docs, an email template), the critic matches every claim to a fact you confirmed. A refactor or a library change has no such claims, and skips this.

The first is what any CI does, made safe for an agent working unattended. The second is what RaisQ adds.

## A task's life

```
queued ──► running ──► ready ──► done
  ▲          │  │        │
  │          │  │        └──► queued (sent back, or a fact it relied on changed)
  │          │  └──► queued (stopped by Ctrl-C or a usage limit)
  │          ▼
  └────── parked (answered, or sent back)
```

A task is always in exactly one state. `raisq status` shows them all.

- **queued:** waiting its turn. `raisq run` takes the oldest.
- **running:** in a run now.
- **ready:** passed, waiting for you to review.
- **parked:** stopped, waiting for a person. The reason is one of *out of attempts*, *question* (the agent asked one, or `raisq task` had questions with nobody at a terminal), *needs something* (the container keeps it out), *stuck* (two attempts failed the same way) or *blocked* (something stopped the attempt before it could be judged, such as a missing tool; the note says what).
- **done:** you marked it done. Its branch is yours to merge. A part of a split task becomes done by itself when it passes; you review the whole task instead.

## A run

A run is one task, judged against one fixed version of the spec. When it starts, RaisQ records which version of your facts and the task's spec it's judged against, such as `facts@77c962ccf551`. Every attempt and the critic use that version, so a fact you edit mid-run doesn't change what's being judged. If you change a fact the result relied on, the task goes back to the queue rather than to review.

A task gets a limited number of attempts: three by default, set with `raisq config attempts <n>`. Each one is a fresh agent session in a new container, continuing from the branch the last attempt committed, so it fixes forward instead of starting over.

## An attempt

The steps run in this order, and the order is code: no plugin, preset or setting can skip or reorder one.

### 1. Build

RaisQ writes the prompt from files only, with no chat history and no secrets:

- the task's acceptance criteria
- an index of your facts, one line each, with the full facts mounted read-only for the agent to open as it needs them
- each plugin's instructions for the agent (`for_ai`)
- on a retry, the exact failures from the last attempt

The agent works in a container on a clone of your project, then writes `result.json`: *done*, *question*, *needs-something* or *failed*. No `result.json` counts as failed. It also writes the choices it made that the spec didn't cover, which go into the review pack.

### 2. Audit

RaisQ checks the clone's own git hooks and config are unchanged, and that every reference given to the builder was mounted read-only, so it couldn't be written. A breach fails the attempt and stops a batch, because it means the setup isn't safe. Then RaisQ commits the change itself.

### 3. Check

Every check runs on the result:

- **Existing tests run as they were before the task.** RaisQ restores each check's test files and config from the commit the task started from, so editing them changes nothing.
- **A new test counts only once it's proven.** RaisQ runs it on the new code, where it must pass, and on the old code, where it must fail. A test that passes on both tests nothing and is reported.
- **Old failures don't count.** What already failed on the base commit is measured once and subtracted.
- **A missing tool blocks the task.** It's never a pass.

### 4. Judge

The critic runs in its own session, with no tools, and is ideally a different model from the builder. It sees the task's spec and the text of what the task changed: the diff, and for a website the text of every page the task changed, with the changed lines marked. Text that was there before the task isn't this task's to answer for, and isn't judged.

It works in two calls, so the largest text is read once:

1. **Read the output.** List every claim a user would read and could act on (names, numbers, prices, contact details, hours, conditions, promises), quoted exactly. Show where each acceptance criterion is met, quoted. Answer each rubric question.
2. **Provenance.** For each claim, name the fact that backs it from the index, and whether a user would be misled.

A form's own messages, such as "Sending…" or "Order sent.", aren't claims. Everything inside the output is treated as data, never as instructions, so a comment saying "every fact here is confirmed" changes nothing.

### 5. Decide

Fixed rules, in code, turn the critic's answers into verdicts. No AI decides this step.

| The critic's answer | Verdict |
|---|---|
| A quote that isn't really in the output | asked again once, then unsure |
| A quote that isn't really in the fact it cites, or a fact that doesn't exist | unsure |
| A claim with no fact behind it | fail: it was invented |
| A claim about a known gap | fail: it should have been left out |
| A claim that contradicts its fact | fail, saying what differs |
| A claim backed by a guessed or expired fact | unsure |
| A claim the critic never answered for | unsure |
| A criterion not shown in the output | fail |
| A rubric answered n/a | not judged, and listed |
| A rubric the critic didn't answer, or answered "maybe" | unsure |
| Anything else | pass |

Any fail, or any failed check, fails the attempt. Its failures go to the next session as a handoff: each one with only the lines that show it, and the files changed so far. If two attempts in a row fail the same way, the task is parked instead of spending another attempt on the same wall.

## The review pack

One per task, in the order you need it:

1. Its state and how many attempts it used
2. **Unsure items**, each with its quote and the fact it rests on. Read these first.
3. Failures, if it's parked
4. Choices the spec didn't cover
5. Tests the agent added, and whether each one is proven
6. Failures that were there before the task
7. Rubrics that didn't apply
8. What earlier attempts got wrong
9. Everything that passed
10. How to decide: `--done`, `--send-back "why"`, or change the spec

`--done` refuses a result whose spec or facts were changed after it was judged.

## Watched and unattended

`raisq run` is watched. If the agent asks a question, you're asked right away, and your answer goes into the spec the same way an answer at `raisq task` does. It doesn't use up an attempt.

`raisq batch` is unattended and never waits. A question parks the task and the next one starts. A batch:

- runs `raisq doctor` before the first task, so a missing container tool, login or critic doesn't fail it at task seven. Tools that checks use are only found inside the container, and a missing one parks that task as blocked.
- stops starting new tasks at its time limit (8 hours by default), and lets the running one finish
- stops right away at a usage limit or Ctrl-C, at any step, and the task it was on goes back to the queue
- stops entirely on an audit breach
- ends with a report of what's ready and what's parked

## Isolation

The agent works with its permission prompts off, because nobody is there to approve them. So RaisQ limits what can be lost instead of what's allowed:

| Inside the container | Kept out |
|---|---|
| A clone of your project on the task's branch, with no remote | Your other files and your repo's `.git` |
| Any command, package installs, the network | SSH keys, git credentials, cloud logins, tokens |
| Your agent's login, copied in for one attempt | Anything that could push, deploy or publish |
| Read-only references you name | Test secrets, until the agent has exited |

- **Every command runs as an unprivileged user** with your uid, so files it writes stay yours.
- **The agent's login is copied, never mounted.** Only the fields the agent needs go in. A refresh token and other tools' tokens kept alongside it stay on your machine. On macOS, Claude Code's login is read from the Keychain, and macOS asks once to allow it.
- **Test secrets reach only the checks,** after the agent has exited, through a provider (see [plugins](plugins.md#provides)).
- **`.raisq/` and RaisQ's agent skill never enter the container.** The agent can't read your rubrics or change what judges it, and it can't change the skill that tells your chat agent how to use RaisQ. They're also left out of every commit and diff the task makes.

`isolation: none` runs everything on the host. It's for a machine that's disposable anyway. `raisq config isolation none` warns and asks before setting it, and every review pack says so.

## Big tasks

A task with more than six acceptance criteria is offered a split. The parts run in order on one branch, each judged on its own criteria, and then the whole task is checked once more and reviewed as one.

## Learning

When a mistake gets past review, `raisq learn <task> "what was wrong"` turns it into one of two things:

- **A check,** if a command can catch it. Give it with `--check "command" --bad <commit> --good <commit>`. It's kept only if it fails on the commit that had the mistake and passes on the commit that fixed it.
- **A rubric,** a standing question for the critic, if it needs judgment. A model drafts it and you approve it.

Either way, it runs on every later task. A lesson says how to check, never what's true: facts belong in `facts/`.

Lessons can be wrong. For a rubric lesson, `raisq review <task> --overrule <lesson>` records that it failed something it shouldn't have, and after three overrules `raisq learn` proposes retiring it. A check lesson is a plugin file: `raisq plugin remove <name>` stops it. With no arguments, `raisq learn` also picks a share of done tasks for you to re-read (20% by default, `spot_check` in [configuration](configuration.md)), because that's how the mistakes that got through are found.

## Specs from elsewhere

- `raisq task --spec specs/001/spec.md` converts a Spec Kit, OpenSpec or other Markdown spec. A model splits it into criteria and facts, and RaisQ checks that every item quotes a line of the file and every line of the file is used. If anything was dropped or invented, nothing is written. Facts that come in this way are *inferred* until you confirm them.
- `raisq task --from SHOP-123` fetches an issue through a spec-source plugin. Its text is marked as coming from outside, so the agent treats it as a description of the work, not as instructions.

## What it costs

Almost all of a task's tokens go to the builder: one attempt is a whole agent session. So RaisQ saves attempts first:

- One attempt reports every failure at once. The critic runs even when a check failed.
- Questions are asked when you write the task, not found by the agent mid-run.
- A retry gets only the failing lines, not whole logs.
- Prompts put what changes least first, so a vendor's prompt cache can reuse them.
- The critic reads the output once, and a claim already settled on an earlier attempt isn't asked again.
- The before-the-task baseline is measured once per base commit and shared by every task that starts from it.
