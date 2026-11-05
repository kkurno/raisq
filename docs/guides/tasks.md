# Working with tasks

A **task** is one piece of work, such as "Add a contact page" or "Fix the footer on mobile". This guide covers everything you do with tasks after [getting started](getting-started.md).

## Writing a good task

```
raisq task "Add a contact page with our phone number and opening hours"
```

RaisQ turns this into **acceptance criteria**: a short list of things that must be true when the work is done. The agent works toward them, and the critic checks each one.

Tips:

- **Say what, not how.** "Add a contact page" is better than "create contact.html with a div". The agent decides how.
- **Keep it small.** One page or one feature per task. A task with too many criteria is offered a split into smaller parts automatically.
- **Answer the questions.** When RaisQ asks something, it's because the agent would otherwise have to guess.

To see or change a task's criteria, open `.raisq/tasks/<task>/spec.yaml`.

### From a spec file or an issue

If you already write specs with a tool like Spec Kit or OpenSpec:

```
raisq task --spec specs/001-contact/spec.md
```

If you've set up an issue tracker plugin:

```
raisq task --from SHOP-123
```

## Running tasks

### One task, while you watch

```
raisq run                 # the oldest queued task
raisq run add-contact     # a specific one
```

If the agent has a question, you're asked right away.

### Many tasks, while you're away

Queue up several tasks, then:

```
raisq batch
```

A batch runs every queued task, one after another, and never waits for you: if the agent has a question, that task is set aside (*parked*) and the next one starts. It ends with a report of what's ready and what's waiting for you. See [Batches](batch.md).

## Checking on tasks

```
raisq status
```

```
  ready    add-contact
  parked   fix-footer          question: Should the footer be the same on every page?
  queued   add-menu
```

Every task is in one of five states:

| State | Means | What to do |
|---|---|---|
| **queued** | Waiting its turn | Nothing. `raisq run` or `raisq batch` will pick it up. |
| **running** | The agent is working on it now | Wait |
| **ready** | It passed every check | Review it |
| **parked** | It stopped and needs you | Read why, then answer or send it back |
| **done** | You approved it | Merge its branch |

## Reviewing

```
raisq review                 # lists everything that needs you
raisq review add-contact     # reads one task's review pack
```

Then decide:

| Command | When |
|---|---|
| `raisq review add-contact --done` | It's right. Merge the branch `raisq/add-contact`. |
| `raisq review add-contact --send-back "Use the phone number from the footer"` | It's not right. Your note is added to the task, and it's queued again. |
| `raisq review fix-footer --answer "Yes, the same everywhere"` | It's parked with a question. Your answer goes into the task's criteria, or into the facts when it's something true about the project, and it's queued again. Only for a question: any other parked task is sent back. |

## Why a task gets parked

| Reason | Means | Fix |
|---|---|---|
| question | The agent needed a decision only you can make, or `raisq task` had questions and nobody was at a terminal to answer them | `--answer` it |
| out of attempts | It used all its tries (three by default; `raisq config attempts <n>` changes it) and still didn't pass | Read the review pack to see what kept failing. Make the task clearer, then `--send-back`. |
| stuck | Two tries failed in exactly the same way | Usually the task needs something it doesn't have. Read the failures, then `--send-back`. |
| needs something | The agent needed access it doesn't have, such as a database or an API key | Provide it (see [Plugins: provides](../reference/plugins.md#provides)), then `--send-back` |
| blocked | Something stopped the attempt before it could be judged: a check's tool missing in the container, a dependency that wouldn't install, a provider that never started, or the audit finding something the builder shouldn't have touched | The note says what. Fix it, then `--send-back`. |

## No agent? Do it yourself

You can make a change by hand and still let RaisQ check it:

```
git switch -c raisq/add-contact     # make the change and commit it
git switch main
raisq run --check add-contact
```

The checks still run in the container, so this needs Docker or Podman like any run. If it fails, the task is parked with the review pack: fix it on the branch, then `raisq review add-contact --send-back "what you fixed"` and `raisq run --check add-contact` again.

## Learning from mistakes

If you find a mistake RaisQ let through, teach it to catch that mistake next time:

```
raisq learn add-contact "The phone number on mobile was different from desktop"
```

RaisQ turns it into a new check that runs on every future task. You approve it first.
