# Troubleshooting

Start with:

```
raisq doctor
```

It checks your setup and says what's wrong. Most problems are one of these.

## Setup

**"isn't running"**

Docker isn't started. Open Docker Desktop, or run `sudo systemctl start docker` on Linux, then try again.

RaisQ needs Docker or Podman for everything, including checking a change you made yourself, because the checks run in the container too. To use Podman: `raisq config isolation podman`. On a machine you're happy to throw away, `raisq config isolation none` runs everything directly on it instead.

**"not logged in on this machine"**

RaisQ uses your agent's existing login, so log in once the normal way, such as by running `claude` for Claude Code or `cursor-agent login` for Cursor.

On a Mac, the first run may ask you to allow access to the Keychain. Choose **Allow**.

**"needs \<tool\>"**

A plugin uses a tool that isn't installed on your computer. The message says how to install it.

**"no .raisq/ here"**

You're not in a RaisQ project. Go to your project's folder, or run `raisq init` there.

**"this machine isn't set up for it"**

The project uses RaisQ, but your machine has no settings yet, such as right after cloning it. Run `raisq init`: it sets up only your machine and leaves the project's shared files alone.

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

Usually the task needs to be clearer, or it needs something the agent doesn't have. Change the task's criteria in `.raisq/local/tasks/<task>/spec.yaml`, then:

```
raisq review <task> --send-back "what to do differently"
```

**A task is parked as "blocked"**

Something stopped the attempt before it could be judged, such as a check's tool missing in the container, a dependency that wouldn't install, a provider that never started, or the audit finding something the builder shouldn't have touched. The note in `raisq status` says what, and the review pack has the details. Fix that, then:

```
raisq review <task> --send-back "what you fixed"
```

**A task is parked as "needs something"**

The agent needed access to something kept out of its container, such as a database or a test API key. RaisQ never gives access automatically. See [Plugins: provides](../reference/plugins.md#provides) for how to provide it safely.

**A batch stopped early**

It hit its time limit or your AI account's usage limit. Run `raisq batch` again later, and it picks up where it left off. See [Batches](batch.md#what-a-batch-does).

**Something passed that shouldn't have**

Teach RaisQ to catch it next time:

```
raisq learn <task> "what was wrong"
```

## Still stuck?

Every attempt is saved in `.raisq/local/tasks/<task>/runs/`, including the exact prompt the agent got, what it reported, what the tests printed and what the critic said. That usually shows what went wrong.

If you think it's a bug, [open an issue](https://github.com/kkurno/raisq/issues) with what you ran, what you expected, and the output of `raisq version` and `raisq doctor`.
