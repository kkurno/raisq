# Batches: run tasks while you're away

A **batch** runs every queued task, one after another, with nobody watching. Queue up work at the end of the day, start a batch, and come back to finished work that has already been tested and reviewed.

```
raisq task "Add a contact page"
raisq task "Show opening hours on the home page"
raisq task "Fix the menu on mobile"
raisq batch
```

## How it's different from `raisq run`

| | `raisq run` | `raisq batch` |
|---|---|---|
| Runs | One task | Every queued task, in order |
| If the 🤖 builder AI has a question | ⚙️ RaisQ asks 🧑 you right away | The task is parked, and the next one starts |
| You're | Watching | Away |

Both build, test and review each task the same way. The only difference is what happens when the builder AI needs you.

## What a batch does

1. **Checks everything first.** ⚙️ RaisQ runs `raisq doctor` before the first task. If Docker, a login or a tool is missing, the batch stops right there, not hours later at task seven.
2. **Runs each queued task in turn**, oldest first. Each one gets its full set of tries, just like `raisq run`.
3. **Never waits for you.** If a task needs you, it's parked and the next task starts. One stuck task doesn't hold up the rest.
4. **Stops cleanly.** It stops starting new tasks when:
   - its time limit is up (8 hours by default)
   - your AI account hits its usage limit
   - you press Ctrl-C

   The task it was working on goes back to the queue, so nothing is lost.
5. **Ends with a report** of what's ready and what's parked. It's printed, saved in `.raisq/batch-report.md`, and sent to any [report plugin](../reference/plugins.md#report) you've set up.

There's one more reason a batch stops: if ⚙️ RaisQ finds the builder AI touched something it shouldn't have, such as a git hook, it stops the whole batch, because that means the setup isn't safe.

## The report

```
# Batch report, 2026-11-04 08:00

## Ready for review (2)
- add-a-contact-page
- show-opening-hours-on-the-home-page

## Parked (1)
- fix-the-menu-on-mobile: question: Should the menu be a dropdown or a side drawer?
```

The report covers what this batch did. For every task's state, including ones parked earlier, run `raisq status`.

## When you're back

```
raisq status                                     # see everything
raisq review add-a-contact-page                  # read a finished task
raisq review add-a-contact-page --done           # approve it
raisq review fix-the-menu-on-mobile --answer "A side drawer"
```

Answering a parked task puts it back in the queue. Run `raisq batch` again, and it continues with whatever is still queued.

## Setting the time limit

For one batch:

```
raisq batch --for 2h
```

For every batch, in `.raisq/raisq.yaml`:

```yaml
batch: 6h
```

The limit is when the batch stops *starting* tasks. A task that's already running is allowed to finish.

## Running a batch on a schedule

A batch is one command, so anything that can run a command on a schedule can run it: `cron`, a CI job, or a machine you leave on overnight.

On a machine that only exists to run batches, such as a CI runner, you can set `isolation: none` in `raisq.yaml` to skip Docker. Only do this on a machine you're happy to throw away, because the builder AI then runs directly on it. Every review pack says when this was the case.

## Tips

- **Small tasks finish more often.** A clear task with a few acceptance criteria rarely parks. A vague one usually comes back with a question.
- **Answer questions when you write the task.** `raisq task` asks about anything unclear up front, which is what lets a batch run without you.
- **Tell RaisQ the facts.** If the builder AI knows your phone number and opening hours, it doesn't have to ask or guess. See [Facts](facts.md).
- **Tasks run one after another**, so a batch takes about as long as its tasks added up.
