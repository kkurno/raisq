// Package command reads plugin files and fills slots from them, so a new tool
// needs a text file, not new code.
package command

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kkurno/raisq/domain"
	"github.com/kkurno/raisq/plugin/login"
	"github.com/kkurno/raisq/plugin/proc"
	"github.com/kkurno/raisq/slot"
)

// File is one plugin file in .raisq/plugins/.
type File struct {
	Name string `yaml:"name"`
	Slot string `yaml:"slot"`
	Run  string `yaml:"run"`
	// Description says in one line what it does, for a person or an agent
	// choosing plugins.
	Description string `yaml:"description,omitempty"`
	// Dir is the folder, relative to the project, a check runs in, such as
	// one package of a monorepo. Its tests and locks globs are relative to it.
	Dir      string   `yaml:"dir,omitempty"`
	RunOne   string   `yaml:"run_one,omitempty"`
	Priority int      `yaml:"priority,omitempty"`
	Needs    []string `yaml:"needs,omitempty"`
	Locks    []string `yaml:"locks,omitempty"`
	Tests    []string `yaml:"tests,omitempty"`
	Result   *struct {
		From  string  `yaml:"from"`
		Path  string  `yaml:"path"`
		Scale float64 `yaml:"scale"`
	} `yaml:"result,omitempty"`
	PassAt *float64 `yaml:"pass_at,omitempty"`
	Median int      `yaml:"median,omitempty"`
	// provides
	For   string `yaml:"for,omitempty"`
	From  string `yaml:"from,omitempty"`
	Ready string `yaml:"ready,omitempty"`
	Port  int    `yaml:"port,omitempty"`
	// Env is a check's environment, or a service container's for a provider
	// with image:. It's for settings, such as where the database is; a
	// secret comes from a provider, never a committed file.
	Env map[string]string `yaml:"env,omitempty"`
	// builder and model
	Login    []login.Source `yaml:"login,omitempty"`
	LoginEnv []string       `yaml:"login_env,omitempty"`
	Image    string         `yaml:"image,omitempty"`
	Model    string         `yaml:"model,omitempty"`
	Vendor   string         `yaml:"vendor,omitempty"`
	Install  string         `yaml:"install,omitempty"`
	// Skills is where the agent reads a project's skills, when that isn't the
	// Agent Skills standard's .agents/skills.
	Skills string `yaml:"skills,omitempty"`
	// spec source
	Matches string `yaml:"matches,omitempty"`
	// documentation and tool metadata
	Tool *struct {
		Find        string `yaml:"find"`
		Supported   string `yaml:"supported,omitempty"`
		InstallHint string `yaml:"install_hint,omitempty"`
	} `yaml:"tool,omitempty"`
	ForAI *struct {
		When  string `yaml:"when"`
		How   string `yaml:"how"`
		Never string `yaml:"never,omitempty"`
	} `yaml:"for_ai,omitempty"`
	Lesson string `yaml:"lesson,omitempty"`
	Path   string `yaml:"-"`
}

func Load(dirs ...string) ([]File, error) {
	var out []File
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			var f File
			if err := yaml.Unmarshal(data, &f); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			if f.Name == "" || f.Slot == "" {
				return nil, fmt.Errorf("%s: a plugin file needs name and slot", p)
			}
			f.Path = p
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f File) RenderForAI() string {
	if f.ForAI == nil {
		return ""
	}
	s := fmt.Sprintf("### %s\nWhen: %s\nHow: %s\n", f.Name, f.ForAI.When, f.ForAI.How)
	if f.ForAI.Never != "" {
		s += "Never: " + f.ForAI.Never + "\n"
	}
	return s
}

// --- checks ---

type Check struct{ F File }

func (c Check) Name() string    { return c.F.Name }
func (c Check) Priority() int   { return c.F.Priority }
func (c Check) Needs() []string { return c.F.Needs }
func (c Check) Locks() []string { return c.inDir(c.F.Locks) }
func (c Check) Tests() []string { return c.inDir(c.F.Tests) }

// inDir makes globs written relative to dir: relative to the project, which
// is how RaisQ restores locks and matches new tests to their check.
func (c Check) inDir(globs []string) []string {
	if c.F.Dir == "" {
		return globs
	}
	out := make([]string, len(globs))
	for i, g := range globs {
		out[i] = path.Join(c.dir(), g)
	}
	return out
}

func (c Check) dir() string { return path.Clean(strings.Trim(c.F.Dir, "/")) }

// command runs line in the check's dir.
func (c Check) command(line string) string {
	if c.F.Dir == "" {
		return line
	}
	return "cd " + shellQuote(c.dir()) + " && " + line
}

func (c Check) Run(ctx context.Context, s slot.Session, env map[string]string) domain.CheckResult {
	n := c.F.Median
	if n <= 1 {
		return c.once(ctx, s, env, c.command(c.F.Run))
	}
	// A score that varies runs n times and the median counts.
	var scores []float64
	var last domain.CheckResult
	for i := 0; i < n; i++ {
		last = c.once(ctx, s, env, c.command(c.F.Run))
		if last.Status == domain.CheckBlocked {
			return last
		}
		if last.Score != nil {
			scores = append(scores, *last.Score)
		}
	}
	if len(scores) == 0 {
		return last
	}
	sort.Float64s(scores)
	med := scores[len(scores)/2]
	last.Score = &med
	last.Status = c.judge(med)
	return last
}

func (c Check) RunOne(ctx context.Context, s slot.Session, env map[string]string, file string) (domain.CheckResult, bool) {
	if c.F.RunOne == "" {
		return domain.CheckResult{}, false
	}
	// file is relative to the project; the check runs in its dir.
	if c.F.Dir != "" {
		file = strings.TrimPrefix(file, c.dir()+"/")
	}
	return c.once(ctx, s, env, c.command(strings.ReplaceAll(c.F.RunOne, "{file}", shellQuote(file)))), true
}

func (c Check) once(ctx context.Context, s slot.Session, env map[string]string, cmd string) domain.CheckResult {
	if len(c.F.Env) > 0 {
		merged := map[string]string{}
		for k, v := range c.F.Env {
			merged[k] = v
		}
		// What a provider injects, such as a test secret, wins.
		for k, v := range env {
			merged[k] = v
		}
		env = merged
	}
	r, err := s.Exec(ctx, cmd, env)
	res := domain.CheckResult{Name: c.F.Name, Output: r.Output}
	switch {
	case err != nil:
		res.Status, res.Output = domain.CheckBlocked, err.Error()
	case r.Exit == 127 || strings.Contains(r.Output, "command not found"):
		res.Status = domain.CheckBlocked
	case c.F.Result != nil:
		score, err := readScore(r.Output, c.F.Result.Path)
		if err != nil {
			res.Status, res.Output = domain.CheckFail, "couldn't read the score: "+err.Error()+"\n"+r.Output
			return res
		}
		if c.F.Result.Scale != 0 {
			score *= c.F.Result.Scale
		}
		res.Score = &score
		res.Status = c.judge(score)
	case r.Exit != 0:
		res.Status = domain.CheckFail
	default:
		res.Status = domain.CheckPass
	}
	return res
}

func (c Check) judge(score float64) domain.CheckStatus {
	if c.F.PassAt == nil || score >= *c.F.PassAt {
		return domain.CheckPass
	}
	return domain.CheckFail
}

func readScore(out, path string) (float64, error) {
	start := strings.IndexAny(out, "{[")
	if start < 0 {
		return 0, fmt.Errorf("no JSON in the output")
	}
	var v any
	if err := json.Unmarshal([]byte(out[start:]), &v); err != nil {
		return 0, err
	}
	for _, key := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return 0, fmt.Errorf("%s: not an object", key)
		}
		v = m[key]
	}
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) {
		return 0, fmt.Errorf("%s is not a number", path)
	}
	return f, nil
}

// --- providers ---

type Provider struct {
	F       File
	Project string // the user's project, for host: paths
}

func (p Provider) Name() string { return p.F.Name }
func (p Provider) Port() int    { return p.F.Port }

func (p Provider) For() slot.ProviderFor {
	if p.F.For == "builder" {
		return slot.ForBuilder
	}
	return slot.ForChecks
}

func (p Provider) hostPath() (string, bool) {
	if !strings.HasPrefix(p.F.From, "host:") {
		return "", false
	}
	path := strings.TrimSuffix(strings.TrimPrefix(p.F.From, "host:"), ":ro")
	if !filepath.IsAbs(path) {
		path = filepath.Join(p.Project, path)
	}
	return path, true
}

func (p Provider) Mount() (slot.Mount, bool) {
	if p.For() != slot.ForBuilder {
		return slot.Mount{}, false
	}
	path, ok := p.hostPath()
	if !ok {
		return slot.Mount{}, false
	}
	return slot.Mount{Host: path, Guest: "/raisq/provided/" + p.F.Name, ReadOnly: true}, true
}

// Start runs after the builder has exited. A host: file is read on the host
// and injected as environment; a run: command starts inside the session.
func (p Provider) Start(ctx context.Context, s slot.Session) (map[string]string, error) {
	env := map[string]string{}
	if path, ok := p.hostPath(); ok && p.For() == slot.ForChecks {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if ok {
				env[strings.TrimPrefix(strings.TrimSpace(k), "export ")] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	if p.F.Image != "" {
		sv, ok := s.(slot.Servicer)
		if !ok {
			return nil, fmt.Errorf("%s needs a service container (image: %s), and this isolation can't start one: use docker or podman", p.F.Name, p.F.Image)
		}
		if err := sv.Service(ctx, p.F.Name, p.F.Image, p.F.Env); err != nil {
			return nil, err
		}
	}
	if p.F.Run != "" {
		if err := s.Background(ctx, p.F.Run, env); err != nil {
			return nil, err
		}
	}
	if p.F.Ready != "" {
		if err := waitReady(ctx, s, p.F.Ready); err != nil {
			return nil, err
		}
	}
	return env, nil
}

// waitReady waits until ready answers: a URL that responds, or a command that
// exits 0, such as a database's own readiness probe.
func waitReady(ctx context.Context, s slot.Session, ready string) error {
	probe := ready
	if strings.HasPrefix(ready, "http://") || strings.HasPrefix(ready, "https://") {
		probe = "curl -fsS -o /dev/null " + shellQuote(ready) + " || wget -q -O /dev/null " + shellQuote(ready)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		r, err := s.Exec(ctx, probe, nil)
		if err == nil && r.Exit == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("%s never became ready", ready)
}

// --- extractors ---

type Extractor struct{ F File }

func (e Extractor) Name() string    { return e.F.Name }
func (e Extractor) Needs() []string { return e.F.Needs }

func (e Extractor) Extract(ctx context.Context, r slot.ExtractRequest) (string, error) {
	cmd := strings.NewReplacer("{base}", r.Base, "{head}", r.Head).Replace(e.F.Run)
	res, err := r.Session.Exec(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	if res.Exit != 0 {
		return "", fmt.Errorf("exit %d: %s", res.Exit, domain.TailLines(res.Output, 10))
	}
	return res.Output, nil
}

// --- reports ---

type Report struct {
	F       File
	Project string
}

func (r Report) Name() string { return r.F.Name }

func (r Report) Deliver(ctx context.Context, it slot.ReportItem) error {
	cmd := proc.Shell(ctx, r.F.Run)
	cmd.Dir = r.Project
	cmd.Stdin = strings.NewReader(it.Markdown)
	cmd.Env = append(os.Environ(), "RAISQ_TASK="+it.Task, "RAISQ_STATE="+string(it.State), "RAISQ_BRANCH="+it.Branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, domain.TailLines(string(out), 5))
	}
	return nil
}

// --- spec sources ---

type Source struct {
	F       File
	Project string
}

func (s Source) Name() string { return s.F.Name }

func (s Source) Matches(id string) bool {
	if s.F.Matches == "" {
		return false
	}
	ok, _ := matchRegexp(s.F.Matches, id)
	return ok
}

func (s Source) Fetch(ctx context.Context, id string) (string, error) {
	cmd := proc.Shell(ctx, strings.ReplaceAll(s.F.Run, "{id}", shellQuote(id)))
	cmd.Dir = s.Project
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", s.F.Name, err)
	}
	return string(out), nil
}

// --- model ---

// Model runs an agent CLI in no-tools mode, or any command that reads a
// prompt on stdin and answers on stdout. It runs on the host: it has no tools
// and runs no project code.
type Model struct {
	F       File
	Timeout time.Duration
}

func (m Model) Name() string { return m.F.Name }

func (m Model) Ask(ctx context.Context, prompt string) (string, error) {
	if m.Timeout == 0 {
		m.Timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	cmd := proc.Shell(ctx, strings.ReplaceAll(m.F.Run, "{model}", m.F.Model))
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = os.TempDir()
	var errb strings.Builder
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		if proc.UsageLimited(errb.String() + string(out)) {
			return "", fmt.Errorf("%s: %w", m.F.Name, slot.ErrUsageLimit)
		}
		return "", fmt.Errorf("%s: %v: %s", m.F.Name, err, domain.TailLines(errb.String(), 5))
	}
	return string(out), nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
