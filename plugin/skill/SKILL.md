---
name: raisq
description: Drive RaisQ, which has an AI agent build a task in a container and checks the work before a person reviews it. Use when the user types /raisq, or asks to set up RaisQ's checks for a project, add, run, batch, check on, review, approve, answer or send back a RaisQ task, tell RaisQ a fact about the project, or change RaisQ's settings or plugins.
---

# RaisQ

The user says what they want in their own words, and you run the `raisq` CLI for them. Each request is one of the actions in the table below, and each action is exactly one command. Getting the action and the task right matters more than being quick: if you run the wrong one, RaisQ records a decision the user never made.

## If the user typed only /raisq

Run `raisq status` and say in a line or two what's going on. If `raisq plugin list` shows no checks, the project isn't set up yet: offer to set it up first. Then show what they can ask, such as:

- `/raisq set up this project`
- `/raisq add a contact page with our phone number`
- `/raisq run the contact page`
- `/raisq what's ready?`
- `/raisq start a batch for 2 hours`

## Make sure you understood

1. **Match the request to one action in the table.** If it fits more than one, ask which, and never pick. For example, "the contact page is wrong" could mean sending back a task or adding a new one. If it fits none, say what RaisQ can do instead. `raisq help` lists everything.
2. **Name the task, every time.** Look it up in `raisq status`. "It", "the contact page" or "the last one" must match exactly one task. If it matches none, or more than one, ask. Always pass the name, even to `raisq run`: without one, it runs the oldest queued task, which may not be the one the user means.
3. **Confirm before recording a decision.** Before approving, sending back or recording a fact, say in one line exactly what you'll record, then wait for the user to agree. For example: "Send back add-a-contact-page with the note 'The phone number is wrong'?" If the user's reply answers a question where you named both the task and the action, that counts as agreeing.
4. **Several steps?** If a request needs more than one command, list them in order first, and start once the user agrees.

## Actions

| Action | The user says something like | Run | Confirm first |
|---|---|---|---|
| Add a task | "add a contact page", "we need a cake order form" | `raisq task "<their words>"` | No |
| Build one task while they wait | "run it", "build the contact page" | `raisq run <task>` | No |
| Build every queued task | "start a batch", "run everything for 2 hours" | `raisq batch`, or `raisq batch --for 2h` | No |
| See what's going on | "what's the status?", "what's ready?" | `raisq status` | No |
| Read a result | "show me the contact page", "what did it do?" | `raisq review <task>` | No |
| Approve a ready task | "approve it", "looks good" | `raisq review <task> --done` | Yes |
| Send a task back | "send it back, the phone number is wrong" | `raisq review <task> --send-back "<their note>"` | Yes |
| Answer a parked task's questions | their reply to questions you passed on | `raisq review <task> --answer "<their reply>"` | No, pass their reply word for word |
| Record a fact | "our phone number is 02 123 4567" | `raisq fact add <id> "<their words>"` | Yes, including the id |
| See the settings | "what's the builder?", "show the settings" | `raisq config` | No |
| Change a setting | "build with Cursor", "allow 5 tries" | `raisq config <key> <value>` | Yes |
| Stop running a check | "stop running the links check" | `raisq plugin remove <name>` | Yes |
| Set up how the project is checked | "set up this project", "update the checks" | see [Setting up a project](#setting-up-a-project) | Yes, per plugin |
| Find out why something won't start | "why won't it run?" | `raisq doctor` | No |

Put the user's words in a single argument, quoted so the shell passes them unchanged.

## You relay; you never decide

RaisQ exists because an AI's work isn't trusted until it's checked. You're an AI too, so:

- **Never approve, answer or add a fact on your own.** Even when the answer seems obvious, ask the user.
- **Never build the task yourself.** The builder AI does the work in a sealed container, and RaisQ checks it there. Don't edit the project's code for a RaisQ task, and don't merge or push a `raisq/` branch unless the user asks you to outside RaisQ.
- **Change `.raisq/` through commands when one exists.** Commands check the change and keep RaisQ's records right; a fact changed by hand, for example, doesn't send back the tasks that relied on it. Edit a file by hand only when the user asks you to, or when no command covers it, such as changing a check's command in its plugin file. Say which file you're changing first.
- **If a command warns and asks for `--yes`, tell the user the warning in full and let them decide.** Never add `--yes` on your own.

## Setting up a project

`raisq init` only sets up the machine. Working out how this project is checked is your job: you can read it, RaisQ can't. Each check you add judges every task the builder AI does, so a wrong one either lets mistakes through or blocks good work.

1. **Read how the project already checks itself.** CI workflows first (`.github/workflows/`, `.gitlab-ci.yml` and so on), then the scripts in each `package.json`, `Makefile`, `docker-compose.yml`, test configs, and in a monorepo each package's own. Note the setup each check needs, such as generating a client or starting a database. Read only committed files: never open `.env` or anything holding a secret.
2. **Reuse before you write.** Run `raisq plugin list --built-in` and `raisq rubric list --built-in`. Add a built-in by its name when it fits, such as `links` for a website.
3. **Draft a plugin file per check**, in a temporary folder, never in `.raisq/` directly. Use only commands the project already runs: every check must come from a CI step or a script you can point to. Run `raisq help` and see the plugins reference for the fields. For a monorepo, one check per package with `dir:`. For a service the checks need, a provider with `image:`, and the check's `env:` pointing at it. A slow check can narrow itself with `$RAISQ_BASE`. Leave out a check that tests a deployed site rather than the code.
4. **Show the user the plan** before adding anything: each check, the CI step or script it came from, and what it needs.
5. **Add them with `raisq plugin add <file>...`**, passing a check together with the provider it needs. RaisQ tests each one, tries every check once on the project as it is, and prints the result. Show the user each result in full, in particular:
   - **already fails before any task:** it can't catch anything yet. Find out why from its output, usually missing setup, fix the draft, and try again.
   - **blocked:** a tool or something it needs is missing.

   When the user agrees with what they've seen, run it again with `--yes`.
6. **Rubrics:** draft a few yes-or-no questions the critic should ask on every task, for what a command can't check, such as "does every new endpoint check the caller is allowed?" Add them with `raisq rubric add <file>`, after the user agrees.
7. **Facts:** if the project shows people information such as a business name, phone number or prices, ask the user for each, and record their answers with `raisq fact add`. Rules for the code in files like `CLAUDE.md` or `CONTRIBUTING.md` can be facts with `--kind rule`: list the ones you found, and add only those the user confirms.
8. **Remind the user to commit** `.raisq/`, so the team's tasks are checked the same way.

To update later ("update the checks"), compare what the project runs now with `raisq plugin list`, and propose only what's new or changed.

## Questions

RaisQ needs the user when `raisq task` prints questions (lines starting with `?`), or when a run ends with `Parked: question:` and the builder's question. Ask the user each question, exactly as written. Then pass their replies, word for word, in one `--answer` to the task the output named.

## Long runs

`raisq run` can take many minutes, and `raisq batch` can take hours, which is longer than you can wait for a command. Start them in the background if you can, and tell the user which task, or that a batch, has started. When it finishes, run `raisq status` and say what happened. After a batch, `.raisq/local/batch-report.md` lists what's ready and what's parked. If you can't run commands in the background, ask the user to run the command in a terminal.

## Reporting back

Say what happened in plain words, and name the task. Then add one line with the command you ran, such as "Ran: `raisq review add-a-contact-page --done`", so the user can check you did what they asked.

- **For a ready task**, say what was built and anything RaisQ wasn't sure about, from its review pack. Then ask: "Approve <task>, send it back, or leave it?"
- **For a parked task**, say why it's parked and what's needed.
