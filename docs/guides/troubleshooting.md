# Troubleshooting

Start with:

```
raisq doctor
```

It checks your setup and says what's wrong. Most problems are one of these.

## Setup

**"isn't running"**

Docker isn't started. Open Docker Desktop, or run `sudo systemctl start docker` on Linux, then try again.

If you can't use Docker, you can still make changes by hand and let RaisQ check them. See [Working with tasks: do it yourself](tasks.md#no-agent-do-it-yourself).

**"not logged in on this machine"**

RaisQ uses your agent's existing login, so log in once the normal way:

```
claude        # for Claude Code
agent login   # for Cursor
```

On a Mac, the first run may ask you to allow access to the Keychain. Choose **Allow**.

**"needs \<tool\>"**

A plugin uses a tool that isn't installed on your computer. The message says how to install it.

**"no .raisq/raisq.yaml here"**

You're not in a RaisQ project. Go to your project's folder, or run `raisq init` there.

**"the critic must be a different model from the builder"**

The AI that checks the work can't be the same model that wrote it, because it would share the same blind spots. Open `.raisq/raisq.yaml` and change `critic:` to a different model's plugin.

## Running

**"make the change on branch raisq/\<task\> first"**

You used `raisq run --check`, which checks a change you made yourself, but the task's branch doesn't exist yet. Create it, make your change, and commit:

```
git switch -c raisq/<task>
```

**A task is parked as "stuck" or "out of attempts"**

The agent tried and couldn't get it right. Read the review pack to see what kept failing:

```
raisq review <task>
```

Usually the task needs to be clearer, or it needs something the agent doesn't have. Change the task's criteria in `.raisq/tasks/<task>/spec.yaml`, then:

```
raisq review <task> --send-back "what to do differently"
```

**A task is parked as "needs something"**

The agent needed access to something kept out of its container, such as a database or a test API key. RaisQ never gives access automatically. See [Plugins: provides](../reference/plugins.md#provides) for how to provide it safely.

**A batch stopped early**

It hit its time limit or your AI account's usage limit. Run `raisq batch` again later, and it picks up where it left off.

**Something passed that shouldn't have**

Teach RaisQ to catch it next time:

```
raisq learn <task> "what was wrong"
```

## Still stuck?

Every attempt is saved in `.raisq/tasks/<task>/runs/`, including the exact prompt the agent got, what it reported, what the tests printed and what the critic said. That usually shows what went wrong.

If you think it's a bug, [open an issue](https://github.com/kkurno/raisq/issues) with what you ran, what you expected, and the output of `raisq version` and `raisq doctor`.
