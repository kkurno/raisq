# RaisQ

**Let an AI agent code for you, and only read the results that are worth your time.**

You give RaisQ a task. It has an AI coding agent build it, then checks the work before you ever see it. If the work is wrong, RaisQ tells the agent exactly what to fix and lets it try again. You only review work that has already passed.

Say it like *rescue*.

## The problem it solves

AI agents are fast, but you can't trust them unwatched. An agent trying to finish a task might:

- edit a failing test so it passes, instead of fixing the code
- make up a phone number, a price or an opening time
- leave placeholder text that looks finished

If you have to check every line it writes, you haven't saved much time. RaisQ does that checking for you.

## How it works

Four actors take part:

- 🧑 **You** say what you want and make the final call.
- 🤖 **Builder AI**, the AI coding agent you use, writes the code.
- 🤖 **Reviewer AI**, a different AI model, checks the builder's work.
- ⚙️ **RaisQ** runs everything in between, and decides pass or fail with plain code.

```mermaid
flowchart TD
    task(["🧑 You<br/>describe a task"])
    build["🤖 Builder AI<br/><b>1. Build</b>"]
    test["⚙️ RaisQ<br/><b>2. Test</b>"]
    review["🤖 Reviewer AI<br/><b>3. Review</b>"]
    passed{"⚙️ RaisQ<br/><b>4. Passed?</b>"}
    left{"⚙️ RaisQ<br/>Tries left?"}
    ready(["🧑 You<br/>Ready: review it"])
    parked(["🧑 You<br/>Parked: step in"])
    done(["🧑 You<br/>Done: merge it"])

    task --> build --> test --> review --> passed
    passed -- "Yes" --> ready
    passed -- "No" --> left
    left -. "Yes, try again" .-> build
    left -- "No" --> parked
    build -- "Needs you" --> parked
    ready -- "Approve" --> done
    ready -. "Send back" .-> build
    parked -. "Answer or<br/>send back" .-> build
```

1. **Build.** 🤖 The builder AI writes the code in a sealed-off container that ⚙️ RaisQ sets up, away from the rest of your computer.
2. **Test.** ⚙️ RaisQ runs your project's tests as they were before the agent started, so the builder can't pass by editing them.
3. **Review.** 🤖 The reviewer AI checks that each requirement is met and that each fact, like a price or a phone number, matches what 🧑 you told RaisQ. It must quote what it found.
4. **Passed?** ⚙️ RaisQ decides, with plain code. If the work failed and there are tries left, RaisQ sends it back to the 🤖 builder AI to try again, with exactly what was wrong. If the builder AI needs you while it works, the task is parked straight away.

Every run ends **ready** or **parked**, and either way it's 🧑 your turn. Approve it and it's done, or answer it or send it back with a note, and it goes back to the queue to be built again:

| | When | What 🧑 you do |
|---|---|---|
| **Ready** | It passed everything | Review it, then merge it or send it back |
| **Parked** | The 🤖 builder AI asked a question, or needs access it doesn't have, like a test database | Answer it, or give it access |
| **Parked** | No tries left: it reached the limit (3 by default), or failed the same way twice in a row | Read what failed, make the task clearer, and send it back |
| **Parked** | A tool a test needs isn't installed | Install it. `raisq doctor` says what's missing. |

A question parks a task only when nobody is there to answer it. With `raisq run`, RaisQ asks you right away. With `raisq batch`, it sets the task aside and moves on to the next one, so you come back to a list. [More on parked tasks](docs/guides/tasks.md#why-a-task-gets-parked).

## Batches: queue tasks and walk away

`raisq run` does one task while you watch. `raisq batch` does every queued task, one after another, while you're away:

```
raisq task "Add a contact page"
raisq task "Fix the menu on mobile"
raisq batch
```

A batch checks your setup before it starts, so it doesn't fail halfway through the night. It never waits for you: a task that needs you is parked, and the next one starts. It stops cleanly at its time limit (8 hours by default) or your AI account's usage limit, and `raisq batch` picks up where it left off. When you're back, a report lists what's ready to review and what needs you. [More on batches](docs/guides/batch.md).

## Principles

RaisQ is built on a few ideas. They're why it works the way it does.

- **Nothing grades its own work.** An AI checking its own code shares the blind spots that wrote it. So a different AI does the review, and plain code, not an AI, makes the final pass or fail call.
- **Show the evidence.** The reviewer must quote the exact words it's judging, and RaisQ checks those words are really there. A verdict without evidence isn't a verdict.
- **You decide what's true.** Facts about your project come from you. Anything the AI guessed is marked as a guess, and anything nobody knows is kept out instead of made up.
- **Safe by design, not by settings.** The agent works in a sealed-off container, can't change the tests that judge it, and can't push, merge or deploy. None of that needs configuring. Running without the container takes one deliberate setting, and every result then says so.
- **Your time is for decisions.** RaisQ handles retries and routine checks, and brings you only what needs a person: what it wasn't sure about, and the choices the agent made on its own.
- **It gets stricter over time.** When a mistake slips through, you can turn it into a new check, so it's caught on every task after.
- **Use the tools you have.** RaisQ works with any AI agent and any test runner. Each one is a small plugin file, so nothing is locked to one vendor.

## Quick start

You need [git](https://git-scm.com), [Docker](https://www.docker.com/get-started/), and an AI coding agent with a command-line tool, logged in. RaisQ works with any agent; [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [Cursor](https://cursor.com/cli) are the ones tested so far, and [any other can be added](docs/reference/plugins.md#builder).

**1. Install RaisQ.** Download it from [Releases](https://github.com/kkurno/raisq/releases), or with Go:

```
go install github.com/kkurno/raisq/cmd/raisq@latest
```

**2. Set up your project.** In your project's folder:

```
raisq init
```

RaisQ finds your tests and your agent, and asks a few questions about your project.

**3. Give it a task.**

```
raisq task "Add a contact page"
```

RaisQ turns your sentence into a clear list of what "done" means, and asks you about anything unclear.

**4. Let it work.**

```
raisq run
```

**5. Look at the result.**

```
raisq review add-a-contact-page
```

You'll see what the agent did, anything it wasn't sure about, and every check it passed. If you're happy:

```
raisq review add-a-contact-page --done
```

The work is on its own git branch, `raisq/add-a-contact-page`. Merge it like any other branch. RaisQ never merges or deploys anything itself.

The [getting started guide](docs/guides/getting-started.md) walks through all of this with a real example.

## What next

| I want to… | Read |
|---|---|
| Try it step by step, with an example | [Getting started](docs/guides/getting-started.md) |
| Queue several tasks and walk away | [Batches](docs/guides/batch.md) |
| Write, run and review tasks | [Working with tasks](docs/guides/tasks.md) |
| Tell RaisQ the facts about my project | [Facts](docs/guides/facts.md) |
| Fix something that isn't working | [Troubleshooting](docs/guides/troubleshooting.md) |
| Use a different AI, tool or test runner | [Plugins](docs/reference/plugins.md) |
| See every setting | [Configuration](docs/reference/configuration.md) |
| Understand exactly how a result is judged | [How it works](docs/reference/how-it-works.md) |

## Is it safe?

The agent runs in a container with only a copy of your project. It can't see your SSH keys, cloud logins or other files, and it can't push to git. Your agent's login is copied in for one attempt, and nothing it writes there gets back to your computer. See [how isolation works](docs/reference/how-it-works.md#isolation).

## Contributing

Bug reports, plugins and doc fixes are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md), and [SECURITY.md](SECURITY.md) to report a security issue.

## License

[MIT](LICENSE)
