package detect

import "fmt"

// Template is a starting plugin file for a slot (9.3).
func Template(slot, name string) (string, error) {
	t, ok := templates[slot]
	if !ok {
		return "", fmt.Errorf("no slot %q: see raisq slots", slot)
	}
	return fmt.Sprintf(t, name), nil
}

// Slots is every slot a plugin file can fill, for raisq plugin new and add.
var Slots = []string{"builder", "model", "checks", "provides", "extractor", "spec-source", "report"}

var templates = map[string]string{
	"builder": `name: %s
slot: builder
vendor: <who makes the model, so the critic can be a different one>
tool:
  find: <command> --version
  install_hint: <how a person installs it>
# Runs inside the container, unattended. {prompt} is the prompt file. It must
# write /raisq/out/result.json before it exits (4.2).
run: <command that reads {prompt} and works in the current directory>
install: <command that installs it in a fresh container, if the image lacks it>
login: ["~/.<tool>/<credential file>:/root/.<tool>/<credential file>"]
login_env: [<API_KEY_VARIABLE>]
image: node:22-bookworm
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
priority: 30
# Exit 0 passes. What it prints goes to the handoff.
run: <command>
# Optional: run one test file, so RaisQ can prove a new test is real.
run_one: <command> {file}
# Files that judge this check: restored from before the task before it runs.
locks: []
# Test files this check runs, for proving new ones.
tests: []
# Something to start first, such as site.
needs: []
`,
	"provides": `name: %s
slot: provides
# For checks (the default): started after the builder has exited.
# run: <command that starts it>
# ready: http://localhost:<port>
# port: <port>
# Or a file read on the host and given to the checks as environment:
# from: host:.env.test
# Or, for the builder, a read-only reference with nothing secret in it:
# for: builder
# from: host:../design-system:ro
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
