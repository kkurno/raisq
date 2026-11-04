package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"raisq/domain"
	"raisq/plugin/command"
	"raisq/plugin/detect"
	"raisq/plugin/login"
	"raisq/plugin/proc"
	"raisq/plugin/registry"
	"raisq/slot"
	"raisq/usecase"
)

func slotItem(md string) slot.ReportItem {
	return slot.ReportItem{Title: "Batch report", Markdown: md, Batch: true}
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func cmdFact(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageErr{"raisq fact add|list|confirm"}
	}
	proj, err := open()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		fs := flags("fact list")
		kind := fs.String("kind", "", "only this kind, such as rule")
		tag := fs.String("tag", "", "only this tag: confirmed, inferred, gap")
		by := fs.String("said-by", "", "only facts said by someone whose name contains this")
		due := fs.Bool("due", false, "only confirmed facts past their recheck date")
		pos, err := parse(fs, args[1:])
		if err != nil {
			return err
		}
		facts, err := proj.Store.LoadFacts()
		if err != nil {
			return err
		}
		now := time.Now()
		for _, f := range facts.Items {
			eff := f.EffectiveTag(now, proj.Store.Periods())
			switch {
			case *kind != "" && f.Kind != *kind,
				*tag != "" && string(f.Tag) != *tag,
				*by != "" && !strings.Contains(strings.ToLower(f.SaidBy), strings.ToLower(*by)),
				*due && !(f.Tag == domain.Confirmed && eff != domain.Confirmed),
				len(pos) > 0 && f.Topic != pos[0]:
				continue
			}
			text := f.Text
			if f.Tag == domain.Gap {
				text = "(unknown)"
			}
			fmt.Printf("  %-28s %-9s %s\n", f.ID, f.Tag, text)
		}
		return nil
	case "add", "confirm":
		fs := flags("fact " + args[0])
		kind := fs.String("kind", "", "contact, hours, price, terms or rule")
		topic := fs.String("topic", "", "file it goes in; default: the part of the id before /")
		by := fs.String("by", "you", "who said it")
		stakeholder := fs.Bool("needs-stakeholder", false, "only the stakeholder can confirm this; keep it inferred")
		pos, err := parse(fs, args[1:])
		if err != nil {
			return err
		}
		facts, _ := proj.Store.LoadFacts()
		if len(pos) < 1 {
			return usageErr{`raisq fact add <id> "text"  ·  raisq fact confirm <id>`}
		}
		f, exists := facts.Get(pos[0])
		if args[0] == "confirm" {
			if !exists {
				return fmt.Errorf("no fact %s", pos[0])
			}
		} else {
			if len(pos) < 2 {
				return usageErr{`raisq fact add <id> "text"`}
			}
			f = domain.Fact{ID: pos[0], Text: pos[1], Kind: f.Kind}
			if *kind != "" {
				f.Kind = *kind
			}
		}
		f.Tag, f.SaidBy, f.SaidOn, f.Needs = domain.Confirmed, *by, time.Now().Format("2006-01-02"), ""
		if *stakeholder {
			f.Tag, f.Needs = domain.Inferred, "stakeholder"
		}
		t := *topic
		if t == "" && exists {
			t = f.Topic
		}
		if err := proj.Store.SaveFact(f, t); err != nil {
			return err
		}
		logf("Saved %s (%s).", f.ID, f.Tag)
		d, err := proj.Deps("", true)
		if err != nil {
			return err
		}
		back, err := usecase.FactChanged(d, f.ID)
		if err != nil {
			return err
		}
		for _, b := range back {
			logf("Sent %s back to the queue: its result relied on %s.", b, f.ID)
		}
		return nil
	}
	return usageErr{"raisq fact add|list|confirm"}
}

// cmdLearn turns a mistake into a check or a rubric, proposes retiring
// lessons people keep overruling, and picks a sample of done tasks to re-read.
func cmdLearn(ctx context.Context, args []string) error {
	fs := flags("learn")
	check := fs.String("check", "", "a command that catches it: exits 0 when the mistake is absent")
	bad := fs.String("bad", "", "a commit that had the mistake, to prove --check on")
	good := fs.String("good", "", "a commit without it, to prove --check on")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	proj, err := open()
	if err != nil {
		return err
	}
	d, err := proj.Deps("", true)
	if err != nil {
		return err
	}
	t := newTTY()
	if len(pos) == 0 {
		// Housekeeping: lessons to retire, then a spot-check sample.
		rs, err := usecase.ToRetire(d)
		if err != nil {
			return err
		}
		for _, r := range rs {
			fmt.Printf("  %s was overruled %d times: %s\n", r.ID, r.Overruled, r.Ask)
			if ok, err := t.Confirm("Retire it? It stays on file, marked retired."); err != nil {
				return err
			} else if ok {
				if err := usecase.Retire(d, r.ID); err != nil {
					return err
				}
			}
		}
		share := proj.Config.SpotCheck
		if share == 0 {
			share = 0.2
		}
		pick, err := usecase.SpotCheck(d, share, rand.New(rand.NewSource(time.Now().UnixNano())))
		if err != nil {
			return err
		}
		if len(pick) == 0 {
			logf("No done tasks to spot-check yet.")
			return nil
		}
		logf("Re-read these shipped tasks. Anything wrong: raisq learn <task> \"what was wrong\"")
		for _, n := range pick {
			logf("  %s  (raisq review %s)", n, n)
		}
		return nil
	}
	if len(pos) < 2 {
		return usageErr{`raisq learn <task> "what was wrong" [--check "command" --bad <commit> --good <commit>]`}
	}
	l, err := usecase.ProposeLesson(ctx, d, pos[0], pos[1], *check, *bad, *good)
	if err != nil {
		return err
	}
	if l.Check != "" {
		if !l.Proven {
			return fmt.Errorf("not kept: %s", l.Why)
		}
		fmt.Printf("  Proposed check: %s (%s)\n", l.Check, l.Why)
	} else {
		fmt.Printf("  Proposed rubric: %s (fails if %s)\n", l.Rubric.Ask, l.Rubric.FailIf)
	}
	ok, err := t.Confirm("Keep it? It runs on every task from now on.")
	if err != nil || !ok {
		return err
	}
	p, err := usecase.KeepLesson(d, l)
	if err != nil {
		return err
	}
	logf("Saved to %s.", p)
	return nil
}

func cmdDoctor(ctx context.Context, args []string) error {
	proj, err := open()
	if err != nil {
		return err
	}
	problems := doctor(ctx, proj)
	if len(problems) == 0 {
		logf("✓ everything RaisQ needs is here")
		return nil
	}
	for _, p := range problems {
		logf("✗ %s", p)
	}
	return fmt.Errorf("%d problem(s)", len(problems))
}

func doctor(ctx context.Context, proj *registry.Project) []string {
	var problems []string
	if _, err := proj.Store.LoadFacts(); err != nil {
		problems = append(problems, "facts: "+err.Error())
	}
	if _, err := proj.Store.LoadRubrics(); err != nil {
		problems = append(problems, "rubrics: "+err.Error())
	}
	d, err := proj.Deps("", false)
	if err != nil {
		return append(problems, err.Error())
	}
	if err := d.Isolation.Available(ctx); err != nil {
		problems = append(problems, err.Error())
	}
	for _, f := range proj.Files {
		if f.Tool == nil || f.Tool.Find == "" || f.Slot == "checks" || f.Slot == "builder" {
			// Checks and builders run inside isolation; their tools are found there.
			continue
		}
		if f.Slot == "model" && f.Name != proj.Config.Critic {
			continue
		}
		bin := strings.Fields(f.Tool.Find)[0]
		if _, err := exec.LookPath(bin); err != nil {
			hint := ""
			if f.Tool.InstallHint != "" {
				hint = ": " + f.Tool.InstallHint
			}
			problems = append(problems, fmt.Sprintf("%s needs %s%s", f.Name, bin, hint))
		}
	}
	for _, f := range proj.Files {
		if f.Slot != "provides" {
			continue
		}
		if f.For == "builder" && looksSecret(f.From) {
			problems = append(problems, f.Name+": a builder provider must not hold a secret")
		}
		if strings.Contains(f.From, "~") {
			problems = append(problems, f.Name+": a path under the home directory won't exist on a teammate's machine")
		}
	}
	for _, f := range proj.Files {
		if f.Slot != "builder" || f.Name != proj.Config.Builder || len(f.Login)+len(f.LoginEnv) == 0 {
			continue
		}
		found := false
		var where []string
		for _, l := range f.Login {
			where = append(where, l.Describe())
			if l.IsDir() {
				found = true
				continue
			}
			if _, err := l.Read(ctx); err == nil {
				found = true
			} else if !errors.Is(err, login.ErrNotFound) {
				problems = append(problems, f.Name+": "+err.Error())
				found = true
			}
		}
		for _, k := range f.LoginEnv {
			where = append(where, "$"+k)
			if os.Getenv(k) != "" {
				found = true
			}
		}
		if !found {
			tool := f.Name
			if f.Tool != nil && f.Tool.Find != "" {
				tool = strings.Fields(f.Tool.Find)[0]
			}
			problems = append(problems, fmt.Sprintf("%s: not logged in on this machine. Log in with %s once; RaisQ brings that login into each container. Looked in: %s",
				f.Name, tool, strings.Join(where, ", ")))
		}
	}
	return problems
}

func looksSecret(p string) bool {
	l := strings.ToLower(p)
	for _, s := range []string{".env", "secret", "token", "id_rsa", "id_ed25519", ".aws", ".ssh", "credentials", ".npmrc"} {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

func cmdSlots() error {
	for _, s := range [][2]string{
		{"builder", "one · makes the change · an agent CLI"},
		{"model", "one · the critic and every AI job with no tools"},
		{"isolation", "one · keeps each attempt off your machine"},
		{"checks", "all · pass, fail or a number, from a command"},
		{"provides", "by name · a site, a database, a test secret, a read-only reference"},
		{"extractor", "all · turns the output into text for the critic"},
		{"spec-source", "the one that matches · turns an issue id into a task"},
		{"report", "all · delivers review packs and batch reports"},
		{"checkout", "built in · git"},
		{"store", "built in · files in .raisq/"},
	} {
		fmt.Printf("  %-12s %s\n", s[0], s[1])
	}
	return nil
}

func cmdPlugin(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageErr{"raisq plugin list|new|add|test"}
	}
	proj, err := open()
	if err != nil {
		return err
	}
	switch args[0] {
	case "new":
		if len(args) < 3 {
			return usageErr{"raisq plugin new <slot> <name>  (slots: " + strings.Join(detect.Slots, ", ") + ")"}
		}
		body, err := detect.Template(args[1], args[2])
		if err != nil {
			return err
		}
		p := filepath.Join(proj.Raisq, "plugins", args[2]+".yaml")
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s exists", p)
		}
		if err := write(p, body); err != nil {
			return err
		}
		logf("Wrote %s. Fill in the <...> parts, then: raisq plugin test %s", p, args[2])
		return nil
	case "add":
		return pluginAdd(ctx, proj, args[1:])
	case "list":
		for _, f := range proj.Files {
			fmt.Printf("  %-12s %-24s %s\n", f.Slot, f.Name, filepath.Base(f.Path))
		}
		return nil
	case "test":
		if len(args) < 2 {
			return usageErr{"raisq plugin test <name>"}
		}
		for _, f := range proj.Files {
			if f.Name != args[1] {
				continue
			}
			body, err := os.ReadFile(f.Path)
			if err != nil {
				return err
			}
			if bad := command.Contract(ctx, string(body), proj.Dir); len(bad) > 0 {
				for _, b := range bad {
					logf("✗ %s", b)
				}
				return fmt.Errorf("%s fails its slot's contract", f.Name)
			}
			logf("✓ %s fills the %s slot", f.Name, f.Slot)
			return nil
		}
		return fmt.Errorf("no plugin %s", args[1])
	}
	return usageErr{"raisq plugin list|new|add|test"}
}

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 32 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

// pluginAdd has a model draft a plugin for a tool RaisQ hasn't seen, tests it
// against the slot's contract, and saves it only once a person approves.
func pluginAdd(ctx context.Context, proj *registry.Project, args []string) error {
	fs := flags("plugin add")
	slotName := fs.String("slot", "", "the slot it fills: "+strings.Join(detect.Slots, ", "))
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 || *slotName == "" {
		return usageErr{"raisq plugin add <tool> --slot <slot>"}
	}
	tool := pos[0]
	template, err := detect.Template(*slotName, tool)
	if err != nil {
		return err
	}
	t := newTTY()
	if ok, err := t.Confirm(fmt.Sprintf("Run `%s --help` and send it to %s to draft a plugin?", tool, proj.Config.Critic)); err != nil || !ok {
		return err
	}
	help, _ := proc.Shell(ctx, tool+" --help 2>&1 | head -200").Output()
	if len(strings.TrimSpace(string(help))) == 0 {
		return fmt.Errorf("%s --help printed nothing: is it installed?", tool)
	}
	usage := ""
	if data, err := os.ReadFile(filepath.Join(proj.Dir, "package.json")); err == nil && strings.Contains(string(data), tool) {
		for _, l := range strings.Split(string(data), "\n") {
			if strings.Contains(l, tool) {
				usage += strings.TrimSpace(l) + "\n"
			}
		}
	}
	d, err := proj.Deps("", true)
	if err != nil {
		return err
	}
	test := func(ctx context.Context, body string) []string { return command.Contract(ctx, body, proj.Dir) }
	dr, err := usecase.DraftPlugin(ctx, d, tool, *slotName, template, string(help), usage, test, 3)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s\n", dr.Body)
	logf("Passed the plugin test after %d draft(s).", dr.Tries)
	if ok, err := t.Confirm("Save it to .raisq/plugins/? It's used on every task from then on."); err != nil || !ok {
		return err
	}
	p := filepath.Join(proj.Raisq, "plugins", tool+".yaml")
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("%s exists: not overwritten", p)
	}
	if err := write(p, dr.Body); err != nil {
		return err
	}
	logf("Saved %s.", p)
	return nil
}
