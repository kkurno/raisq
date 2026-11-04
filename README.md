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

```
  you describe a task
         │
         ▼
  ┌─────────────┐      ┌──────────────┐      ┌───────────────┐
  │ 1. Build    │ ───► │ 2. Test      │ ───► │ 3. Review     │
  │ an AI agent │      │ your tests   │      │ a second AI   │
  │ writes code │      │ run on it    │      │ checks facts  │
  └─────────────┘      └──────────────┘      └───────────────┘
         ▲                                           │
         │            something's wrong?             │
         └──────── "here's exactly what to fix" ◄────┤
                                                     │ all good
                                                     ▼
                                          you review the result
```

1. **Build.** The agent writes the code in a sealed-off container, so it can't touch anything else on your computer.
2. **Test.** RaisQ runs your project's tests. The agent can't cheat by editing them: RaisQ always runs the tests as they were before the agent started.
3. **Review.** A second AI reads the changes and checks every fact against the facts you gave RaisQ. A made-up phone number fails.

If any step fails, the agent gets another try (up to three) with a precise list of what was wrong. Then the work waits for you.

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
| Queue several tasks and walk away | [Working with tasks](docs/guides/tasks.md) |
| Tell RaisQ the facts about my project | [Facts](docs/guides/facts.md) |
| Fix something that isn't working | [Troubleshooting](docs/guides/troubleshooting.md) |
| Use a different AI, tool or test runner | [Plugins](docs/reference/plugins.md) |
| See every setting | [Configuration](docs/reference/configuration.md) |
| Understand exactly how a result is judged | [How it works](docs/reference/how-it-works.md) |

## Is it safe?

The agent runs in a container with only a copy of your project. It can't see your SSH keys, cloud logins or other files, and it can't push to git. RaisQ never merges, deploys or publishes: everything waits for you. See [how isolation works](docs/reference/how-it-works.md#isolation).

## Contributing

Bug reports, plugins and doc fixes are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md), and [SECURITY.md](SECURITY.md) to report a security issue.

## License

[MIT](LICENSE)
