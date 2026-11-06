// Package slot defines the interfaces the use cases need from outside the
// core. Plugins fill them; the core never knows which plugin it got.
package slot

import (
	"context"
	"errors"
	"time"

	"github.com/kkurno/raisq/domain"
)

// ErrUsageLimit is a builder or model saying its vendor's quota is used up.
// The task isn't failed: a batch stops, and the task resumes later.
var ErrUsageLimit = errors.New("the agent's usage limit was reached")

// Builder makes the change. It gets a session with the checkout at Workdir,
// the frozen spec mounted read-only, and a prompt file. It reports only
// through result.json and choices.json in the out directory.
type Builder interface {
	Name() string
	Build(ctx context.Context, s Session, prompt string) error
}

// Model answers one prompt with no tools. The answer is JSON on stdout.
type Model interface {
	Name() string
	Ask(ctx context.Context, prompt string) (string, error)
}

// Isolation starts a session that keeps work away from the user's machine.
type Isolation interface {
	Name() string
	Start(ctx context.Context, req StartRequest) (Session, error)
	Available(ctx context.Context) error
}

type Mount struct {
	Host     string
	Guest    string
	ReadOnly bool
}

type StartRequest struct {
	Checkout string // host path of the task checkout
	SpecDir  string // host path of the frozen spec view, mounted read-only
	OutDir   string // host path the builder reports into
	Mounts   []Mount
	Ports    []int // guest ports a provider will listen on
	Label    string
}

// Paths are where things are as seen from inside the session.
type Paths struct {
	Workdir string
	Spec    string
	Out     string
}

type ExecResult struct {
	Exit   int
	Output string
}

type Session interface {
	Paths() Paths
	Exec(ctx context.Context, cmd string, env map[string]string) (ExecResult, error)
	Background(ctx context.Context, cmd string, env map[string]string) error
	// URL maps a guest port to an address the host can reach.
	URL(port int) (string, error)
	Stop(ctx context.Context) error
}

// Check is one plugin in the checks slot.
type Check interface {
	Name() string
	Priority() int
	Needs() []string
	// Locks are the files that judge this check: its tests and config.
	// They're restored from before the task before the check runs.
	Locks() []string
	// Tests is the glob of this check's test files, for proving new tests.
	Tests() []string
	Run(ctx context.Context, s Session, env map[string]string) domain.CheckResult
	// RunOne runs a single test file, or reports false if it can't.
	RunOne(ctx context.Context, s Session, env map[string]string, file string) (domain.CheckResult, bool)
}

type ProviderFor string

const (
	ForChecks  ProviderFor = "checks"
	ForBuilder ProviderFor = "builder"
)

// Provider supplies something by name when asked.
type Provider interface {
	Name() string
	For() ProviderFor
	// Mount is what a builder provider mounts, read-only.
	Mount() (Mount, bool)
	Port() int
	// Start runs inside the session after the builder has exited, and returns
	// any environment it injects into the checks that asked for it.
	Start(ctx context.Context, s Session) (map[string]string, error)
}

// Extractor turns the output into text for the critic. Empty means nothing
// for it in this output.
type Extractor interface {
	Name() string
	Extract(ctx context.Context, req ExtractRequest) (string, error)
}

// Servicer is a session that can start a service container beside it, such as
// a database for the checks. It returns where the session reaches it, which
// depends on the isolation, and the service stops with the session.
type Servicer interface {
	Service(ctx context.Context, req ServiceRequest) (Address, error)
}

// ServiceRequest is a service to start from an image.
type ServiceRequest struct {
	Name  string
	Image string
	Env   map[string]string
	// Port is the port the service listens on inside its container.
	Port int
}

// Address is where a session reaches a service.
type Address struct {
	Host string
	Port int
}

// Needer is an extractor that needs a provider started first, as a check does.
type Needer interface {
	Needs() []string
}

// Snapshotter is an extractor that compares against how the output looked
// before the task. Its snapshot is taken once per base commit.
type Snapshotter interface {
	Snapshot(ctx context.Context, s Session) (map[string]string, error)
}

type ExtractRequest struct {
	Checkout Checkout
	Dir      string
	Base     string
	Head     string
	Session  Session
	// Before is this extractor's snapshot at the base commit, if it takes one.
	Before map[string]string
}

// SpecSource turns an outside id, such as a Jira key, into draft text.
type SpecSource interface {
	Name() string
	Matches(id string) bool
	Fetch(ctx context.Context, id string) (string, error)
}

// Report delivers a review pack or a batch report.
type Report interface {
	Name() string
	Deliver(ctx context.Context, r ReportItem) error
}

type ReportItem struct {
	Task     string
	Title    string
	State    domain.State
	Markdown string
	Branch   string
	Batch    bool
}

// Checkout makes and reads the copy of the project each task works in.
type Checkout interface {
	Head(ctx context.Context) (string, error)
	Clean(ctx context.Context) (bool, error)
	// Prepare makes a clone with no remote and no .raisq/, on branch, from base
	// or from the branch if it already exists.
	Prepare(ctx context.Context, task, branch, base string) (dir string, err error)
	Commit(ctx context.Context, dir, message string) (sha string, changed bool, err error)
	// Bring fetches the branch from the checkout into the project.
	Bring(ctx context.Context, dir, branch string) error
	Diff(ctx context.Context, dir, base, head string) (string, error)
	Changed(ctx context.Context, dir, base, head string) ([]string, error)
	// Restore puts files matching globs back to how they were at base, and
	// returns the files the task added that match.
	Restore(ctx context.Context, dir, base string, globs []string) (restored, added []string, err error)
	// AuditGit reports changes to the checkout's own hooks and config.
	AuditGit(ctx context.Context, dir string) error
	// ToBase puts every file the task changed back to base, except keep, so a
	// new test can run against the old code with the same installed deps.
	ToBase(ctx context.Context, dir, base string, keep []string) error
	// Reset returns the working tree to the committed head.
	Reset(ctx context.Context, dir string) error
	Remove(dir string) error
}

// Store keeps .raisq/ data.
type Store interface {
	Root() string
	LoadFacts() (domain.Facts, error)
	SaveFact(f domain.Fact, topic string) error
	// Pin records the current facts as a revision and returns it.
	PinFacts() (domain.Facts, error)
	FactsAt(rev string) (domain.Facts, error)
	LoadRubrics() ([]domain.Rubric, error)
	SaveRubric(r domain.Rubric, file string) error
	Periods() map[string]time.Duration

	Tasks() ([]string, error)
	LoadSpec(task string) (domain.TaskSpec, error)
	SaveSpec(s domain.TaskSpec) error
	PinSpec(task string) (string, error)
	SpecAt(task, rev string) (domain.TaskSpec, error)
	LoadState(task string) (domain.TaskState, error)
	SaveState(task string, s domain.TaskState) error

	NewRun(task string) (string, error)
	RunDir(task, run string) string
	SaveRunFile(task, run, name string, data any) error
	LoadRunFile(task, run, name string, into any) error

	Baseline(commit string) (domain.Baseline, bool, error)
	SaveBaseline(b domain.Baseline) error
	// Snapshot is an extractor's render of a base commit, kept like a baseline.
	Snapshot(commit, extractor string) (map[string]string, bool, error)
	SaveSnapshot(commit, extractor string, pages map[string]string) error
}
