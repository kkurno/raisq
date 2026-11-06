// Command raisq is the CLI: one front end on the use cases, holding no logic.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"

	"golang.org/x/term"

	"github.com/kkurno/raisq/usecase"
)

// version is set by the release build. A `go install` build gets the
// version Go recorded instead, in init below.
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		version = strings.TrimPrefix(bi.Main.Version, "v")
	}
}

const usage = `raisq: AI-built work isn't trusted until it can prove what it says.

  raisq init                          set up RaisQ here; then /raisq set up this project, in your agent's chat
  raisq task "Add a cake order form"  describe a piece of work; raisq asks about what's unclear
  raisq run [task]                    build and check the next queued task, or the one you name
  raisq batch                         build and check every queued task, while you're away
  raisq review [task]                 read what needs a person, then decide

Now and then:
  raisq status                        each task's state
  raisq fact add|list|confirm         project facts
  raisq learn <task> "what was wrong" turn a mistake into a check or a rubric
  raisq config [key [value]]          see or change a setting, such as raisq config builder cursor
  raisq plugin add <file|name>...     add checks and providers, each tried on the project first
  raisq rubric add <file or set>      add standing questions for the critic
  raisq skill [agent]                 use RaisQ from your agent's chat: /raisq ...
  raisq doctor                        when something isn't working
  raisq bench critic                  measure the critic before trusting it
  raisq slots | plugin list [--built-in]|new|remove|test   plugins
  raisq version | update
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		var u usageErr
		if errors.As(err, &u) {
			fmt.Fprint(os.Stderr, usage)
		}
		fmt.Fprintln(os.Stderr, "raisq:", err)
		os.Exit(1)
	}
}

type usageErr struct{ msg string }

func (u usageErr) Error() string { return u.msg }

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "init":
		return cmdInit(ctx, rest)
	case "task":
		return cmdTask(ctx, rest)
	case "run":
		return cmdRun(ctx, rest)
	case "batch":
		return cmdBatch(ctx, rest)
	case "review":
		return cmdReview(ctx, rest)
	case "status":
		return cmdStatus(ctx, rest)
	case "fact":
		return cmdFact(ctx, rest)
	case "learn":
		return cmdLearn(ctx, rest)
	case "config":
		return cmdConfig(rest)
	case "rubric":
		return cmdRubric(rest)
	case "skill":
		return cmdSkill(rest)
	case "doctor":
		return cmdDoctor(ctx, rest)
	case "bench":
		return cmdBench(ctx, rest)
	case "slots":
		return cmdSlots()
	case "plugin":
		return cmdPlugin(ctx, rest)
	case "update":
		// RaisQ never fetches or installs itself; it says how.
		fmt.Println("  raisq " + version + ". To update, run the installer again:")
		fmt.Println("    curl -fsSL https://raw.githubusercontent.com/kkurno/raisq/main/install.sh | sh")
		fmt.Println("  or, if you installed with Go: go install github.com/kkurno/raisq/cmd/raisq@latest")
		fmt.Println("  Then, in each project: raisq init, to refresh the chat skill. It changes nothing you set.")
		return nil
	case "version", "--version":
		fmt.Println("raisq", version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}
	return usageErr{"unknown command " + cmd}
}

func flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// parse lets flags come after positional arguments, as people type them.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func logf(format string, a ...any) { fmt.Printf("  "+format+"\n", a...) }

// tty asks a person at the terminal.
type tty struct{ r *bufio.Reader }

func newTTY() *tty { return &tty{r: bufio.NewReader(os.Stdin)} }

// person is who can answer now: someone at a terminal, or nil when RaisQ runs
// from an agent's chat, a script or CI, and questions wait in the task's state.
func person() usecase.Asker {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	return newTTY()
}

func (t *tty) Ask(q string) (string, error) {
	fmt.Printf("  ? %s\n    > ", q)
	line, err := t.r.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("no answer: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func (t *tty) Confirm(q string) (bool, error) {
	a, err := t.Ask(q + " [y/N]")
	return strings.HasPrefix(strings.ToLower(a), "y"), err
}

func wd() string {
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return d
}
