# Setting up a project for RaisQ

Read this when the user asks to set up RaisQ for a project, or to update its checks. Everything here is done through the `raisq` CLI, which tests what you add before it can judge any task.

## What you're building, and why it matters

RaisQ has a builder AI make each task in a container, then decides pass or fail with plain code. What it runs to decide is what you set up here:

- **Checks** are commands. Exit 0 passes. They judge every task, so a wrong one either lets mistakes through or fails good work.
- **Providers** are what a check needs first, such as a database or the site running.
- **Rubrics** are yes-or-no questions a separate AI answers about each task's result, with a quote, for what no command can check.
- **Facts** are what's true about the project, such as the business's phone number, or a rule like "all copy is in British English". The critic checks the result against them, so the builder can't make them up.

RaisQ doesn't tell the builder how to work in the project. The builder follows the project's own instructions, as any agent does: its `AGENTS.md` or `CLAUDE.md`, and its agent skills. Those are the project's, not RaisQ's.

## Steps

0. **Check the machine first:** `raisq doctor`. RaisQ needs Docker or Podman running, both for the builder and for the services checks need. If it reports a problem, tell the user and stop until it's fixed. Never change `isolation` to get past it: `isolation: none` runs the builder directly on the user's machine, and is the user's decision alone.
1. **Read how the project already checks itself.** Read only committed files (`git ls-files` lists them); never open `.env`, `credentials/` or anything else that holds a secret. In this order:
   - CI: `.github/workflows/*.yml`, `.gitlab-ci.yml`, `.circleci/`, `Jenkinsfile`. This is the best source: it shows each check and the setup it needs.
   - `package.json` scripts, in each package of a monorepo (`pnpm-workspace.yaml`, `workspaces`, `turbo.json`, `nx.json` list them), `Makefile`, `go.mod`, `pyproject.toml`, `Cargo.toml`.
   - `docker-compose.yml`, for the services tests need.
   - Test configs (`jest.config.*`, `vitest.config.*`, `playwright.config.*`, `pytest.ini`), for where tests live.
   - `CLAUDE.md`, `AGENTS.md`, `CONTRIBUTING.md`, `RULES.md`, for rules about the code.
2. **Reuse before you write:** `raisq plugin list --built-in` and `raisq rubric list --built-in`. A built-in is added by its name.
3. **Draft the files in a folder outside `.raisq/`**, such as `.raisq-setup/`. `raisq plugin new <slot> <name>` and `raisq rubric new <name>` write a starting file there, with every field explained.
4. **Show the user the plan** before adding anything: each check, the CI step or script it came from, the files it protects, and what it needs.
5. **Add with `raisq plugin add <file>...`**, a check together with the providers it needs. Read what it prints, show the user, and act on it (see [Trial results](#trial-results)). When the user agrees, run the same command with `--yes`.
6. **Add rubrics with `raisq rubric add <file>`**, after the user agrees with each question.
7. **Ask the user for facts** and record their words with `raisq fact add`.
8. **Delete the drafts folder**, and remind the user to commit `.raisq/`.

## Checks

**A check runs one command the project already has,** such as a `package.json` script or a `make` target, plus simple setup before it like `prisma generate && …`. The project owns how it's tested, so people, CI and RaisQ all run the same thing. A check is never a program written for RaisQ:

- **No services in a check.** Never start a database, a container or a server inside `run`. Put it in a provider with `image:` or `run:` (see [Providers](#providers)) and name it in `needs:`. `raisq plugin add` refuses a check that runs `docker run`, `docker compose` or `podman run`.
- **No command to call?** If CI runs several steps inline, or no script covers what's needed, propose adding one to the project, such as a `test:migrations` script in `apps/api/package.json`. Show the user the change and make it only once they agree, like any change to their code. Then the check calls it.
- **Never invent a check.** One RaisQ can't trace to the project's CI or scripts is a guess about how the project should be tested.

```yaml
name: api-test                 # unique; one check per package and job: api-test, web-lint
slot: checks
description: The API's unit tests, from ci.yml's "api" job.
dir: apps/api                  # the folder it runs in; tests, locks and {file} are relative to it
run: pnpm exec prisma generate && pnpm exec jest
run_one: pnpm exec jest {file} # runs one test file
tests: ["**/*.spec.ts"]        # this check's test files
locks: ["jest.config.*", "test-utils/**"]
env: {DATABASE_URL: "postgresql://postgres:test@postgres:5432/app"}
needs: [postgres]
priority: 20                   # lower runs first: build 10, lint and types 15, tests 20, e2e 30
```

The fields that protect the project, and must be right:

- **`tests`:** the glob of this check's test files. **Before the checks run, RaisQ puts every file matching `tests` or `locks` back to how it was before the task.** That's what stops the builder passing by editing a test. A check with `tests` and `locks` both empty protects nothing, and `raisq plugin add` says so. That's right for a build; for a check that runs tests or reads a config, it's a hole.
- **`locks`:** files that decide how the check judges: its config, setup files, fixtures, snapshots. Restored the same way, so the builder can't loosen the check.
- **`run_one`:** runs a single test file. With it, a test the builder adds is run on the old code, where it must fail, and on the new, where it must pass; one that passes on both tests nothing and is reported. Without it, new tests aren't proven.

Also:

- **Setup goes in `run`,** such as `prisma generate`. RaisQ installs dependencies itself from the lockfile (npm, pnpm, yarn, Go), with the Node version and package manager the project names.
- **A slow check can narrow itself** with `$RAISQ_BASE`, the commit the task started from: `turbo run test --filter="...[$RAISQ_BASE]"`. It's empty on the run that measures what already failed, so the check must still work in full without it. RaisQ runs every check either way.
- **Leave out** checks that test a deployed site rather than the code, need real credentials, or deploy, publish or push anything.
- **`env` is committed,** so never put a secret in it. A secret goes in a provider with `from: host:<file>`, read on the user's machine and given to the checks only, after the builder has exited.

## Providers

A service in its own container, beside the checks, reachable at its name:

```yaml
name: postgres
slot: provides
image: postgres:16             # the version the project uses, from docker-compose.yml or CI
env: {POSTGRES_PASSWORD: test, POSTGRES_DB: app}
ready: pg_isready -h postgres  # a command that exits 0, or a URL, when it's ready
```

The project's own app, started in the checks' container, for checks that test it running:

```yaml
name: site
slot: provides
run: pnpm build && pnpm start
port: 3000
ready: http://localhost:3000
```

A file of test settings or secrets on the user's machine, given to the checks as environment:

```yaml
name: env.test
slot: provides
from: host:.env.test           # must be git-ignored; the builder never sees it
```

A check names what it needs in `needs:`, and RaisQ starts only those, after the builder has exited.

## Rubrics

A rubric is a question with a yes-or-no answer the critic must quote evidence for. Use one only for what no command can check. A file is a list:

```yaml
- id: api/auth-guard           # area/name, unique in the project
  ask: Does every endpoint this task adds or changes check that the caller is allowed?
  fail_if: no                  # the answer that fails the task: yes or no
- id: web/i18n
  ask: Does any text a user would see appear in code instead of the message files?
  fail_if: yes
```

Write questions about the task's result, answerable from its diff or pages. Keep each one to a single thing. A few good rubrics beat many vague ones: each is asked on every task.

## Facts

```
raisq fact add business/phone "02 123 4567" --kind contact
raisq fact add style/british-english "All copy a user reads is in British English" --kind rule
raisq fact add orders/minimum --gap
```

- **Kinds:** `contact`, `hours`, `price`, `terms` (rechecked every 180, 90, 30 and 90 days) and `rule` (never expires).
- **Only the user says what's true.** Ask for each fact and record their words. If they don't know one, add it with `--gap`: the builder is then told to leave it out instead of inventing it.
- **Rules from `CLAUDE.md` and similar files:** a rule about what the code or its output must be ("no console.log in committed code", "every page has a title") is a fact with `--kind rule`, and the critic checks each task against it. Instructions about how to work ("run the tests before committing", "use Playwright for anything in the browser") aren't facts: they belong in the project's `AGENTS.md`, `CLAUDE.md` or an agent skill, which the builder already reads. If the project has none and the user wants some, propose adding them there, as a change to their project. List the rules you found, and add only the ones the user confirms.

## Trial results

`raisq plugin add` runs each check once on the project as it is, in a fresh container, before adding it:

- **passes:** it will catch a task that breaks it. Good.
- **already fails before any task:** it can't catch anything yet, because RaisQ only counts new failures. Read its output. Usually setup is missing: a client to generate, a service to start, an environment variable. Fix the draft and add it again. If the project really is failing there, tell the user: they may want to fix the project first, or add the check anyway and know it protects nothing yet.
- **blocked:** a tool or something it needs isn't there, such as a provider that never became ready. Every task would be blocked by it, so fix it before adding. If a provider can't start because Docker or Podman isn't running, or the machine runs with `isolation: none`, tell the user and stop. Don't work around it by starting services in the check.

Nothing is added until the command runs with `--yes`, which you do only after the user has seen these results and agreed.

## Updating later

When the user asks to update the checks, compare what the project runs now (step 1) with `raisq plugin list` and `raisq rubric list`, and propose only what's new, changed or gone. A check that should change is removed with `raisq plugin remove <name>` and added again, so it's tried again.
