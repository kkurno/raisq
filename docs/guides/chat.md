# Using RaisQ from your agent's chat

You can use RaisQ by chatting with your AI agent instead of typing commands. Type `/raisq`, then say what you want in your own words. Your agent runs the matching `raisq` command for you and tells you what happened.

There's no special syntax. What matters is saying **what you want done**, and **which task** when there's more than one.

## What you can ask

Each request does one thing, the same as one `raisq` command:

| You want to | Say something like | Your agent runs |
|---|---|---|
| Set up how the project is checked | `/raisq set up this project` | several: see [Setting up a project](#setting-up-a-project) |
| Add a task | `/raisq add a contact page with our phone number` | `raisq task` |
| Build one task | `/raisq run the contact page` | `raisq run` |
| Build every queued task while you're away | `/raisq start a batch for 2 hours` | `raisq batch` |
| See what's going on | `/raisq what's ready?` | `raisq status` |
| Read a result | `/raisq show me the contact page` | `raisq review` |
| Approve it | `/raisq approve the contact page` | `raisq review --done` |
| Send it back | `/raisq send back the contact page: the phone number is wrong` | `raisq review --send-back` |
| Tell RaisQ a fact | `/raisq our phone number is 02 123 4567` | `raisq fact add` |
| See the settings | `/raisq which builder are we using?` | `raisq config` |
| Change a setting | `/raisq build with Cursor` | `raisq config builder cursor` |
| Stop running a check | `/raisq stop running the links check` | `raisq plugin remove links` |
| Find out why something won't start | `/raisq why won't it run?` | `raisq doctor` |

Type `/raisq` on its own to see what's going on and some examples.

## A whole task, from chat

```
You:    /raisq add a contact page with our phone number
Agent:  RaisQ has a question before it can queue this:
        Should the page show opening hours?
You:    yes, 9 to 5 on weekdays
Agent:  Queued add-a-contact-page with 3 acceptance criteria.
        Ran: raisq review add-a-contact-page --answer "yes, 9 to 5 on weekdays"

You:    /raisq run it
Agent:  Started add-a-contact-page in the background. I'll tell you when it's done.
        ...
Agent:  add-a-contact-page is ready. It added /contact with your phone number
        and hours. RaisQ wasn't sure about one thing: the map link.
        Approve add-a-contact-page, send it back, or leave it?

You:    approve
Agent:  Approved. The work is on branch raisq/add-a-contact-page for you to merge.
        Ran: raisq review add-a-contact-page --done
```

## Setting up a project

`raisq init` only sets up your machine. How your project is checked is worked out by your agent, because it can read your project:

```
You:    /raisq set up this project
Agent:  From your CI, here's what I'd add:
        - api-test: pnpm exec jest in apps/api, from ci.yml. Needs Postgres.
        - postgres: a test database for the checks.
        - web-lint: pnpm run lint in apps/web, from ci.yml.
        - links: RaisQ's own link check, for the website.
        Add them?
You:    yes
Agent:  RaisQ tried each check on the project as it is:
        ✓ api-test passes, so it will catch a task that breaks it.
        ! web-lint already fails before any task: 3 lint errors in
          src/legacy. It can't catch anything until those are fixed.
        ✓ links passes.
        Add api-test, postgres and links now, and leave web-lint out?
```

- **Every check comes from something your project already runs,** such as a CI step or a script, and your agent tells you which.
- **RaisQ tries each check before it's added,** and your agent shows you the result. A check that already fails, or is blocked, is worth knowing about before it judges any task.
- **Nothing is added until you agree.** Then commit `.raisq/`, so your team's tasks are checked the same way.

When your project changes, `/raisq update the checks` proposes only what's new or different.

Your agent follows a setup guide that comes with the skill, in `.agents/skills/raisq/references/setup.md`: the steps, the file formats, and the fields that keep the builder from editing your tests. It reads it only when setting up, so everyday requests stay quick.

## How your agent avoids doing the wrong thing

Your agent works out what you meant, so it's built to check rather than guess:

- **It asks when a request could mean two things.** "The contact page is wrong" could mean send it back, or add a new task to fix it, so your agent asks which.
- **It always names the task.** If "it" or "the contact page" could be more than one task, it asks which one. When it runs a task, it names that task, so it never just picks the next one in the queue.
- **It checks with you before recording a decision.** Before it approves, sends back, records a fact, changes a setting or removes a check, it says exactly what it's about to do and waits for you. In the example above, "approve" was your reply to a question that already named the task, so that counted.
- **It shows what it ran.** Each reply ends with the command, so you can see it did what you asked.
- **It never decides for you.** It doesn't approve, answer RaisQ's questions or add facts on its own, even when the answer seems obvious. When a change comes with a warning, such as running without the container, it shows you the warning and lets you decide. It also doesn't build the task itself: the builder AI still does that in its container.
- **It uses commands, and edits files only when you ask.** Commands check a change before saving it. You can still ask it to edit a file in `.raisq/` directly, such as a check's command, and it says which file first.

Even if your agent gets something wrong, RaisQ's safety doesn't depend on it. The builder still can't change the tests that judge it, RaisQ still never merges or deploys, and approving refuses a result that's out of date. The skill never reaches the builder either: RaisQ keeps it out of the builder's copy of your project, along with `.raisq/`.

## Setting it up

`raisq init` adds the skill to your project, in `.agents/skills/raisq/`. That's the folder named by the [Agent Skills](https://agentskills.io) standard, which agents such as Cursor read. Commit it so everyone on your team has it.

Some agents read skills from a folder of their own. Claude Code is one of them, so for Claude Code, also run:

```
raisq skill claude-code
```

That writes the same skill to `.claude/skills/raisq/`. If you switch agents, run it again with the new agent's name. For an agent RaisQ doesn't ship a plugin for, add `skills:` to its plugin file (see [Plugins: builder](../reference/plugins.md#builder)).

After you update RaisQ, run `raisq skill` to refresh every copy in the project. `raisq doctor` tells you when one is out of date.

## Questions and long runs

Your agent's commands don't run in a terminal, so RaisQ can't stop and wait for you to type an answer. Instead, it parks the task with its questions, the same as in a [batch](batch.md). Your agent asks you, and passes your reply on word for word.

A run takes minutes and a batch can take hours, which is longer than most agents wait for one command. Your agent starts them in the background and tells you when they've finished. If your agent can't run commands in the background, it asks you to start the run in a terminal.
