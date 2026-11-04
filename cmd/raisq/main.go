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
	"strings"
)

var version = "0.1.0"

const usage = `raisq: AI-built work isn't trusted until it can prove what it says.

  raisq init                          set up the project, once
  raisq task "Add a cake order form"  describe a piece of work; raisq asks about what's unclear
  raisq run [task]                    build and check the next queued task, or the one you name
  raisq batch                         build and check every queued task, while you're away
  raisq review [task]                 read what needs a person, then decide

Now and then:
  raisq status                        each task's state
  raisq fact add|list|confirm         project facts
  raisq learn <task> "what was wrong" turn a mistake into a check or a rubric
  raisq doctor                        when something isn't working
  raisq bench critic                  measure the critic before trusting it
  raisq slots | plugin list|new|add|test   plugins
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
	case "doctor":
		return cmdDoctor(ctx, rest)
	case "bench":
		return cmdBench(ctx, rest)
	case "slots":
		return cmdSlots()
	case "plugin":
		return cmdPlugin(ctx, rest)
	case "update":
		// GitHub Releases is the only channel, and it needs a published
		// repo first; until then, update is a rebuild.
		fmt.Println("  raisq " + version + ". No release channel is published yet: rebuild from the source with `go build -o ~/bin/raisq ./cmd/raisq`.")
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
