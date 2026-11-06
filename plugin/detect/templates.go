package detect

import (
	"fmt"
	"strings"
)

// Template is a starting plugin file for a slot, with every field explained.
func Template(slot, name string) (string, error) {
	t, ok := templates[slot]
	if !ok {
		return "", fmt.Errorf("no slot %q: see raisq slots", slot)
	}
	return strings.ReplaceAll(t, "%s", name), nil
}

// RubricTemplate is a starting rubric file, with every field explained.
func RubricTemplate(area string) string {
	return strings.ReplaceAll(`# Rubrics are yes-or-no questions the critic answers about every task's
# result, quoting what it found. Use one only for what no command can check.
- id: %s/<short-name>
  # About the task's result, answerable from its diff or pages. One thing.
  ask: <a question answered yes or no>
  # The answer that fails the task: yes or no.
  fail_if: <yes or no>
`, "%s", area)
}

// Slots is every slot a plugin file can fill, for raisq plugin new and add.
var Slots = []string{"builder", "model", "checks", "provides", "extractor", "spec-source", "report"}

var templates = map[string]string{
	"builder": `name: %s
slot: builder
vendor: <who makes the model, so review packs can say when the critic is the same>
tool:
  find: <command> --version
  install_hint: <how a person installs it>
# Runs inside the container, unattended. {prompt} is the prompt file. It must
# write /raisq/out/result.json before it exits.
run: <command that reads {prompt} and works in the current directory>
install: <command that installs it in a fresh container, if the image lacks it>
login:
  - file: ~/.<tool>/<credential file>
    to: /home/raisq/.<tool>/<credential file>
    keep: [<the fields the agent needs>]
login_env: [<API_KEY_VARIABLE>]
image: node:{node}-bookworm
`,
	"model": `name: %s
slot: model
vendor: <who makes the model>
model: <model id>
# Runs on the host with no tools. Reads the prompt on stdin, answers on stdout.
run: <command> --model {model}
`,
	"checks": `name: %s
slot: checks
# One line on what it checks and where it comes from, such as a CI job.
description: <what it checks, from which CI step or script>
# Exit 0 passes. What it prints goes to the builder's next try and your review.
# Use a command the project already runs. Put setup first: a && b.
run: <command>
# Optional: the folder it runs in, such as apps/api in a monorepo. tests,
# locks and {file} are then relative to it.
# dir: <folder>
# Runs one test file, so a test the builder adds is proven: it must fail on
# the old code and pass on the new. Leave it out only if the tool can't.
run_one: <command> {file}
# This check's test files. Before checks run, RaisQ puts every matching file
# back to how it was before the task, so the builder can't pass by editing a
# test. Leave it empty only for a check that runs no tests, such as a build.
tests: ["<glob of its test files>"]
# Files that decide how it judges: config, setup, fixtures, snapshots.
# Restored the same way, so the builder can't loosen it.
locks: ["<glob of its config>"]
# Settings, such as where a database is. Committed: never a secret.
# env: {<NAME>: <value>}
# Providers to start first, such as a database or the site.
# needs: [<provider>]
# Lower runs first: build 10, lint and types 15, tests 20, e2e 30.
priority: 20
`,
	"provides": `name: %s
slot: provides
description: <what it gives the checks>
# Started only when a check names it in needs:, after the builder has exited.
# Keep one of these three, and delete the others.
#
# 1. A service in its own container, for the checks:
image: <image and version, such as postgres:16>
# The port it listens on inside its container.
port: <port, such as 5432>
env: {<NAME>: <value>}
# Runs inside its container, with the image's own tools, until it exits 0.
ready: <a command, such as pg_isready -U postgres, or redis-cli ping>
# A check reaches it at {%s.host}:{%s.port} in its env, filled in by RaisQ.
#
# 2. The project's own app, started in the checks' container:
# run: <command that starts it>
# port: <port>
# ready: http://localhost:<port>
#
# 3. A git-ignored file on this machine, given to the checks as environment:
# from: host:.env.test
`,
	"extractor": `name: %s
slot: extractor
# Prints text for the critic, from the task's result. {base} and {head} are
# the commits before and after. Print nothing when there's nothing for it.
run: <command>
`,
	"spec-source": `name: %s
slot: spec-source
# Which ids it takes, as a regular expression.
matches: "[A-Z]+-[0-9]+"
# Prints the issue's text. {id} is the id. Runs on the host, outside isolation.
run: <command> {id}
`,
	"report": `name: %s
slot: report
# Gets the review pack on stdin, with RAISQ_TASK, RAISQ_STATE and RAISQ_BRANCH
# set. Runs on the host. May post a draft or a note; never merges or deploys.
run: <command>
`,
}
