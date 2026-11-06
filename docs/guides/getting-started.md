# Getting started

This guide takes you from nothing to your first finished task, in about 15 minutes. It uses a small website as the example, but the steps are the same for any project.

## Before you start

You need three things on your computer:

| What | Why | Check it's working |
|---|---|---|
| [git](https://git-scm.com) | RaisQ puts each task's work on its own branch | `git --version` |
| [Docker](https://www.docker.com/get-started/) | The agent works inside a container, away from your files | `docker info` |
| An AI coding agent with a command-line tool, logged in | It writes the code, and a second model checks it | Its own version command, such as `claude --version` |

RaisQ works with any AI coding agent. [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [Cursor](https://cursor.com/cli) are tested and set up automatically; for any other, see [adding an agent](../reference/plugins.md#builder). This guide uses Claude Code in its examples.

Your project should also be a git repository with its work committed, and ideally have some tests. RaisQ works without tests, but it can check much less.

## 1. Install RaisQ

RaisQ runs on macOS and Linux. Run:

```
curl -fsSL https://raw.githubusercontent.com/kkurno/raisq/main/install.sh | sh
```

It downloads the right file for your computer, checks it against the release's checksum, and puts it in a folder that's already on your `PATH`, such as `~/.local/bin`. If none of those folders is on your `PATH`, it says the one line to add. Check it works:

```
raisq version
```

If you see `command not found`, open a new terminal and try again, or follow the line the installer printed.

**With Go instead:** `go install github.com/kkurno/raisq/cmd/raisq@latest`. Go puts it in `~/go/bin`, which often isn't on your `PATH`, so add it once: `echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc`, then open a new terminal.

**To update,** run the installer again, or `go install` again if you used Go. `raisq update` prints the line. Then run `raisq init` in each project: it refreshes the chat skill and fills in anything new, and changes nothing you set.

## 2. Set up your project

In your project's folder, run:

```
raisq init
```

RaisQ finds the agent and container tool on your computer:

```
  Found: claude-code · Docker
  Builder: claude-code   Critic: claude-code-critic   Isolation: docker
  note: one agent CLI: the critic is from the same vendor as the builder, and every review pack says so
  ? Set up RaisQ here with these? You can change any of it afterwards [y/N]
    > y
```

- **Builder** is the agent that writes code.
- **Critic** is the second AI that checks it.

Type `y`. RaisQ creates a `.raisq/` folder and adds its skill to `.agents/skills/raisq/`, so you can use it from your agent's chat. If you use Claude Code, also run `raisq skill claude-code`, because Claude Code reads skills from its own folder.

## 3. Teach RaisQ how to check your project

`init` doesn't know yet how your project is tested. Working that out means reading your CI, your scripts and your docs, which your AI agent is good at. Open your agent's chat in the project and type:

```
/raisq set up this project
```

Your agent reads how the project already checks itself, then proposes:

- **checks:** the commands that test your work, such as your tests, linter and build, each taken from your CI or scripts
- **what they need,** such as a database to run against
- **rubrics:** yes-or-no questions the critic asks on every task, for what a command can't check
- **facts:** your business name, phone number and other details it asks you for, so RaisQ catches an agent making them up

Before anything is added, RaisQ tries each check once on your project as it is, and your agent shows you the result. A check that already fails before any task can't catch anything yet, so your agent fixes it first. Nothing is added until you agree.

Commit `.raisq/`: the checks, facts and rubrics in it are how your project's work is judged, for everyone on the team. `.raisq/local/` inside it is yours alone, with your settings and tasks, and git ignores it. A teammate who clones the project runs `raisq init` once to set up their own machine.

Then run `raisq doctor` to make sure everything is ready:

```
  ✓ everything RaisQ needs is here
```

If it reports a problem, see [Troubleshooting](troubleshooting.md).

**Without an agent chat,** you can add checks yourself. `raisq plugin new checks unit-tests` writes `unit-tests.yaml` in the folder you're in, with every field explained. Fill it in, then `raisq plugin add unit-tests.yaml` tries it on your project and adds it. See [Plugins](../reference/plugins.md).

### Changing the setup later

```
raisq config                     # see every setting
raisq config builder cursor      # build with Cursor instead
raisq plugin remove links        # stop running the links check
```

Or ask your agent: `/raisq update the checks` after your project changes. [Configuration: changing the setup](../reference/configuration.md#changing-the-setup) lists everything you can change.

## 4. Describe a task

```
raisq task "Add a cake order page"
```

RaisQ turns your sentence into a list of things that must be true when the task is done, called **acceptance criteria**. If something is unclear, it asks:

```
  ? Should customers be able to order for the same day?
    > no
  Queued: add-a-cake-order-page (4 acceptance criteria)
    c1  There is a page at /order with a form to order a cake
    c2  The form asks for a delivery date
    c3  The order page says how much notice an order needs
    c4  The home page links to the order page
```

Write tasks the way you'd explain them to a colleague. Smaller tasks work better than big ones.

## 5. Run it

```
raisq run
```

RaisQ starts the agent and shows each attempt:

```
  Running add-a-cake-order-page: "Add a cake order page"
  Spec: 4 acceptance criteria · 5 facts · facts@77c962ccf551
  attempt 1 · built · checks passed · critic: 1 claims checked, 1 failed, 0 unsure · web/form-states: Does every form show what happened after submitting, success or error?: answered no
  attempt 2 · built · checks passed · critic: 1 claims checked, 0 failed, 0 unsure
  Ready for review.
  Review: raisq review add-a-cake-order-page
```

Here's what happened:

- **Attempt 1:** the agent built the page and the tests passed. But the critic noticed the form showed nothing after you pressed "Order", so the attempt failed.
- **Attempt 2:** the agent was told exactly that, added a "Thank you, your order was received" message, and everything passed.

Each attempt takes a few minutes. If the agent has a question while it works, RaisQ asks you right there in the terminal.

## 6. Review the result

```
raisq review add-a-cake-order-page
```

The review pack shows what needs your attention first:

```
## 📝 Choices the spec didn't cover

- The order handler acknowledges an order without storing it (the spec didn't say where orders go)

If one of these matters, add it to the spec.

## ✓ Passed

- "Cake orders need 2 days' notice." · backed by a confirmed fact
- c1: There is a page at /order with a form to order a cake · shown in the output
- c2: The form asks for a delivery date · shown in the output
...
```

- **Unsure** items, if there are any, come first. These are things RaisQ couldn't decide, such as a claim based on a fact you haven't confirmed.
- **Choices the spec didn't cover** are decisions the agent made on its own. Here, it accepts orders but doesn't save them anywhere. That's worth knowing before you ship.
- **Passed** lists every check and fact it confirmed.

To see the actual code, look at the branch:

```
git diff main raisq/add-a-cake-order-page
```

## 7. Finish

If you're happy:

```
raisq review add-a-cake-order-page --done
```

Then merge the branch `raisq/add-a-cake-order-page` the way you normally would, for example with a pull request.

If it's not right, send it back with what's wrong, and it goes back in the queue:

```
raisq review add-a-cake-order-page --send-back "Save orders to orders.json"
```

## What's next

- [Using RaisQ from chat](chat.md): do all of this by typing `/raisq` and what you want in your agent's chat.
- [Working with tasks](tasks.md): queue several tasks and run them while you're away.
- [Facts](facts.md): tell RaisQ what's true about your project, so it can catch made-up details.
- [Plugins](../reference/plugins.md): add a test runner or tool RaisQ didn't find.
