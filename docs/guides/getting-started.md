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

Download the file for your computer from [Releases](https://github.com/kkurno/raisq/releases) and put it somewhere on your `PATH`, or install it with Go:

```
go install github.com/kkurno/raisq/cmd/raisq@latest
```

Check it works:

```
raisq version
```

## 2. Set up your project

Go to your project's folder and run:

```
cd my-website
raisq init
```

RaisQ looks at your project and tells you what it found:

```
  Found: web app · npm · claude-code · Docker
  Preset: web   Builder: claude-code   Critic: claude-code-critic   Isolation: docker
  Plugin files: build, claude-code, claude-code-critic, contacts, links, page-text, site
  note: one agent CLI: the critic is from the same vendor as the builder, and every review pack says so
  ? Write these into .raisq/? You can change any of it afterwards [y/N]
    > y
```

- **Builder** is the agent that writes code.
- **Critic** is the second AI that checks it.
- **Plugin files** are the tools RaisQ found: your build command, how to start your site, and some checks for websites.

Type `y`. If something isn't what you want, such as the builder, accept it anyway and [change it afterwards](#changing-what-init-chose): typing `n` writes nothing at all. RaisQ then asks a few questions about your project, such as its name and phone number. These become **facts**, which is how RaisQ catches an agent making things up. Press Enter to skip one you don't know.

RaisQ creates a `.raisq/` folder in your project. Commit it: the facts, checks and rubrics in it are how your project's work is judged, for everyone on the team. `.raisq/local/` inside it is yours alone, with your settings and tasks, and git ignores it. A teammate who clones the project runs `raisq init` once to set up their own machine.

Run `raisq doctor` to make sure everything is ready:

```
  ✓ everything RaisQ needs is here
```

If it reports a problem, see [Troubleshooting](troubleshooting.md).

### Changing what init chose

Everything `init` wrote can be changed later:

```
raisq config                     # see every setting
raisq config builder cursor      # build with Cursor instead
raisq plugin remove links        # stop running the links check
```

[Configuration: changing the setup](../reference/configuration.md#changing-the-setup) lists everything you can change.

## 3. Describe a task

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

## 4. Run it

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

## 5. Review the result

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

## 6. Finish

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
