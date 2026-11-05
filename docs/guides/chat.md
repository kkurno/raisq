# Using RaisQ from your agent's chat

You don't have to learn RaisQ's commands. Type `/raisq` in your AI agent's chat, then say what you want in your own words:

```
/raisq add a contact page with our phone number and opening hours
/raisq run it
/raisq queue a menu fix and a footer fix, then start a batch
/raisq what's ready?
/raisq approve the contact page
/raisq send back the footer: the phone number should match the header
```

Your agent runs the right `raisq` commands for you and tells you what happened. The commands are still there for scripts and schedules.

## Setting it up

`raisq init` adds the skill to your project, in `.agents/skills/raisq/`. That's the folder named by the [Agent Skills](https://agentskills.io) standard, which agents such as Cursor read. Commit it so everyone on your team has it.

Some agents read skills from a folder of their own. Claude Code is one of them, so for Claude Code, also run:

```
raisq skill claude-code
```

That writes the same skill to `.claude/skills/raisq/`. Run it again if you switch agents, naming the new one. For an agent RaisQ doesn't ship a plugin for, add `skills:` to its plugin file (see [Plugins: builder](../reference/plugins.md#builder)).

After you update RaisQ, run `raisq skill` to refresh every copy in the project. `raisq doctor` tells you when one is out of date.

## What your agent does, and doesn't

Your chat agent is a go-between. It carries your words to RaisQ and RaisQ's answers back to you. It doesn't make decisions for you:

- **It approves a result only when you say so.**
- **Answers and facts come from you.** When RaisQ asks something, your agent asks you and passes on your answer. It never answers for you, even when it thinks it knows.
- **It doesn't build the task itself.** The builder AI still does the work in a sealed-off container, and RaisQ still checks it.

None of RaisQ's safety depends on your chat agent behaving well. The builder still can't change the tests that judge it, and RaisQ still never merges or deploys. The skill also never reaches the builder: RaisQ leaves it out of the builder's copy of your project, along with `.raisq/`.

## Questions

When RaisQ needs you, your agent asks you in the chat:

```
You:    /raisq add a cake order form
Agent:  RaisQ has two questions before it can queue this:
        1. Can customers order for the same day?
        2. Is there a minimum order?
You:    No same-day orders. Minimum is 500 baht.
Agent:  Queued "Add a cake order form" with 3 acceptance criteria.
```

Your agent's commands don't run in a terminal, so they can't stop and wait for your answer. RaisQ parks the task with its questions instead, and your agent answers them with your words. It's the same thing that happens in a [batch](batch.md).

## Long runs

A run takes minutes and a batch can take hours, which is longer than most agents wait for one command. Your agent starts them in the background and tells you when they've finished. If your agent can't run commands in the background, it asks you to start the run in a terminal.
