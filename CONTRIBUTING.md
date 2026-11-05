# Contributing

Thanks for helping. Bug reports, plugins, and fixes to the docs are all welcome.

## Getting started

You need Go 1.23 or later and git. Docker is needed only for the container tests.

```
git clone https://github.com/kkurno/raisq
cd raisq
go build ./...
go test ./...
RAISQ_DOCKER_TEST=1 go test ./plugin/docker   # against a running Docker
```

`go test ./...` needs no agent, no model and no network. Every slot has a fake, so the whole loop from task to verdict runs offline.

To try a change end to end, build the binary and run it in a scratch project:

```
go build -o /tmp/raisq ./cmd/raisq
cd /path/to/a/scratch/project && /tmp/raisq init
```

## How the code is laid out

RaisQ has three rings, and every dependency points inwards:

```
domain/      ring 1: the rules. Facts, tasks, checks, verdicts. Imports nothing from RaisQ.
usecase/     ring 2: the flows (task, attempt, queue, review, learn, convert).
slot/        ring 2: the interfaces the flows need from outside, and fakes in slot/fake.
plugin/      ring 3: what fills each slot: git, docker, the agent, plugin files, built-ins.
cmd/raisq/   ring 3: the CLI. It calls the use cases and holds no logic of its own.
```

`arch_test.go` fails the build if `domain`, `slot` or `usecase` imports anything further out. If you need something from outside in a flow, add or widen a slot, and fill it in a plugin.

The order of an attempt's steps lives in `usecase/attempt.go`. No plugin, preset or setting should be able to change it, skip a step, or decide a verdict. The verdict rules are in `domain/verdict.go`.

## Adding a tool

Most new tools need no Go at all: a plugin file in `.raisq/plugins/`, filled through the built-in `command` plugin. See [docs/reference/plugins.md](docs/reference/plugins.md). A built-in plugin file that `raisq init` writes lives in `plugin/detect/builtins/`.

Write Go only when a plugin file can't express it, such as the page crawler in `plugin/builtin/`.

## Pull requests

- Keep a pull request to one change.
- Add a test for new behaviour that could break. Tests use the fakes in `slot/fake` and a temporary git repo, not a real agent.
- If you change something a user sees, update the README, or the guide or reference in `docs/` it belongs to.
- `gofmt -l .` and `go vet ./...` must be clean. CI runs both, and the tests.

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Types:

| Type | For |
|---|---|
| `feat` | A new feature a user can see |
| `fix` | A bug fix |
| `docs` | Documentation only |
| `refactor` | A change that's neither a feature nor a fix |
| `perf` | A change that makes something faster or cheaper |
| `test` | Adding or fixing tests |
| `build` | The build, dependencies or release |
| `ci` | CI configuration |
| `chore` | Anything else that doesn't touch the code or the docs |

The scope is the package or area, such as `critic`, `docker`, `login`, `cli` or `plugins`. Write the description in the imperative and lower case, with no full stop: `fix(docker): stop the agent's whole process group on a timeout`.

A breaking change gets `!` after the type or scope, and a `BREAKING CHANGE:` footer saying what to do:

```
feat(config)!: rename critic_model to critic

BREAKING CHANGE: rename `critic_model:` to `critic:` in .raisq/local/config.yaml.
```

A breaking change to a file format in `.raisq/` always needs that footer, so the release notes tell users what to change.

## Reporting a bug

Please include:

- what you ran, and what you expected
- `raisq version` and `raisq doctor`
- the run folder from `.raisq/local/tasks/<task>/runs/`, if it's about a run. Check it first: it holds the prompt, the agent's output and the critic's answers.

Security issues go through [SECURITY.md](SECURITY.md), not a public issue.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
