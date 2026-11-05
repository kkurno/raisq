---
name: raisq
description: Drive RaisQ, which has an AI agent build a task in a container and checks the work before a person reviews it. Use when the user types /raisq, or asks to add, run, batch, check on, review, approve, answer or send back a RaisQ task, to tell RaisQ a fact about the project, or to change RaisQ's settings or plugins.
---

# RaisQ

The user says what they want in their own words, and you run the `raisq` CLI for them. Each request is one of the actions in the table below, and each action is exactly one command. Getting the action and the task right matters more than being quick: if you run the wrong one, RaisQ records a decision the user never made.

## If the user typed only /raisq

Run `raisq status` and say in a line or two what's going on. Then show what they can ask, such as:

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
| Find out why something won't start | "why won't it run?" | `raisq doctor` | No |

Put the user's words in a single argument, quoted so the shell passes them unchanged.

## You relay; you never decide

RaisQ exists because an AI's work isn't trusted until it's checked. You're an AI too, so:

- **Never approve, answer or add a fact on your own.** Even when the answer seems obvious, ask the user.
- **Never build the task yourself.** The builder AI does the work in a sealed container, and RaisQ checks it there. Don't edit the project's code for a RaisQ task, and don't merge or push a `raisq/` branch unless the user asks you to outside RaisQ.
- **Change `.raisq/` through commands when one exists.** Commands check the change and keep RaisQ's records right; a fact changed by hand, for example, doesn't send back the tasks that relied on it. Edit a file by hand only when the user asks you to, or when no command covers it, such as changing a check's command in its plugin file. Say which file you're changing first.
- **If a command warns and asks for `--yes`, tell the user the warning in full and let them decide.** Never add `--yes` on your own.

## Questions

When a command prints questions (lines starting with `?`), RaisQ needs the user before it can go on. Ask the user each one, exactly as written. Then pass their replies, word for word, in one `--answer` to the task the command named.

## Long runs

`raisq run` can take many minutes, and `raisq batch` can take hours, which is longer than you can wait for a command. Start them in the background if you can, and tell the user which task, or that a batch, has started. When it finishes, run `raisq status` and say what happened. After a batch, `.raisq/batch-report.md` lists what's ready and what's parked. If you can't run commands in the background, ask the user to run the command in a terminal.

## Reporting back

Say what happened in plain words, and name the task. Then add one line with the command you ran, such as "Ran: `raisq review add-a-contact-page --done`", so the user can check you did what they asked.

- **For a ready task**, say what was built and anything RaisQ wasn't sure about, from its review pack. Then ask: "Approve <task>, send it back, or leave it?"
- **For a parked task**, say why it's parked and what's needed.
