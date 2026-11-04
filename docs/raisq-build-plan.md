# RaisQ build plan

Status: v0.1 built, 2026-11-04. Spikes S1 and S2 still need a logged-in agent CLI and a real model, and `raisq update` waits for a published release (see the README).

---

## Summary

RaisQ lets AI agents do project work while nobody is watching. It proves each result against a written spec before a person reviews it.

- **For the user:** set up with one command, add tasks, run them now or in a batch, then review.
- **Underneath:** every attempt is a fresh AI session in a disposable copy of the project. Plain code runs the checks and decides pass or fail. A separate critic AI must quote its evidence for every fact. Every mistake found later becomes a new check.
- **What RaisQ is:** one executable that orchestrates tools the user already has. It depends on nothing but git.
- **Proven in this order:** two one-day spikes, then the verifier working on its own, then the unattended builder around it (12).

**Read first:** the design concepts (section 1), what using RaisQ looks like (section 3), and the decisions in [Appendix B](#appendix-b-decisions).

---

## 1. Design concepts

Every part of RaisQ is judged against these. When two conflict, the earlier one wins.

| # | Concept | What it means in practice |
|---|---|---|
| C1 | **Safe by construction** | Safety comes from how RaisQ works, not from settings someone must get right. No AI decides its own pass or fail. The builder can't change what judges it. Isolation protects the user's machine. |
| C2 | **Easy to use** | One command sets a project up. The main path is five commands. Most people never open a config file. |
| C3 | **Easy to understand** | A person can explain the whole flow in a minute: add a task, run it, review it. Every result says why, with evidence. Names say what things are. |
| C4 | **Complexity encapsulated** | The hard parts (sessions, task checkouts, isolation, the quote check, verdict rules, tool versions) live inside RaisQ and its plugins. The user sees the result, not the machinery. |
| C5 | **Configurable, with good defaults** | Everything has a default that suits most projects. A setting exists only when many projects really need a different value. People configure exceptions, not the normal case. |
| C6 | **Plug-in approach, the clean architecture way** | RaisQ's rules sit at the centre and depend on nothing outside them. Everything outside is a plugin in a slot the core defines: agents, models, checks, extractors, spec tools, isolation, reports, storage. Plugins depend on the core, never the other way round. Any of them can be swapped without touching the rules ([6](#6-architecture)), and users can add their own, by hand or with AI ([9.3](#93-adding-your-own-plugin)). |
| C7 | **RaisQ only orchestrates** | RaisQ never holds, downloads or installs tools. It starts what the user has installed, and tells the AI how to use it. |
| C8 | **Maintainable** | The dependency rule keeps each change in one place: a new tool is a new plugin, a new rule is a change in the core, and neither touches the other. Small modules with clear contracts. Plugins and presets are text, so most changes need no code. Tool versions are pinned with metadata from official docs, and tested against recorded output, so a tool change fails loudly instead of breaking quietly. |

### What this rules out

- **Lists nobody can keep up to date.** RaisQ doesn't make the user predict every command, host or folder a task might need (C1, C5).
- **Settings for problems RaisQ can solve itself.** If a sensible default exists, it's the default, not a question (C2, C5).
- **Features that only work with one AI tool** (C6).
- **Code that has to change when a tool releases a new version,** where a plugin file could change instead (C8).

---

## 2. Goals and scope

### Goals

1. **Works while you're away.** Queue tasks, walk away, and come back to a short review.
2. **Any project.** New, existing or already running. Old failures don't block new work.
3. **Its own spec, or yours.** RaisQ has its own spec format. Spec Kit, OpenSpec or any other spec tool can plug in.
4. **Any AI.** Any agent CLI can build, any model can judge, and no AI API is required.
5. **Gets stricter over time.** Every mistake found later becomes a check for every future task.

### Prerequisites

| Need | Why |
|---|---|
| **git** | Task checkouts, branches and diffs |
| **An agent CLI, logged in** | It builds. In no-tools mode, with a different model picked, it can also be the critic, so no AI API is needed. |
| **An isolation tool**, such as Docker or Podman | Every attempt runs inside it. Without one, RaisQ runs only in manual mode, or with `isolation: none` on a machine that's disposable anyway. |
| **The project's own tools** | Whatever its build and tests already need |

**Not needed:** Node.js, Python or any other runtime for RaisQ itself, an AI API key, a particular AI vendor, Spec Kit or OpenSpec, a server or a database.

### Not in scope

- **Replacing spec tools, CI or deploys.** RaisQ reads specs and stops at review. Shipping happens through the project's own process.
- **Its own coding agent.** RaisQ starts existing agents.
- **A graphical interface.** Results are files, plus PR comments or messages through plugins.

---

## 3. Using RaisQ

This is everything a user needs to know.

```
raisq init                          set up the project, once
raisq task "Add a cake order form"  describe a piece of work; raisq asks about what's unclear
raisq run [task]                    build and check the next queued task, or the one you name
raisq batch                         build and check every queued task, while you're away
raisq review                        read what needs a person, then approve or send back
```

### A first session

```
$ raisq init
  Found: Next.js app · npm · vitest · Playwright · Docker · two agent CLIs
  Preset: web          Builder: codex-cli       Critic: claude-code
  Checks: build, lint, unit tests, e2e tests       Old failures saved: 3
  Agent login inside isolation: ~/.codex/auth.json only
  4 questions about the project (business name, phone, …)   ✓ answered
  Ready.

$ raisq task "Add a cake order form"
  2 questions: same-day orders? minimum order?     ✓ answered
  Queued: add-cake-orders (5 acceptance criteria)

$ raisq run
  Running add-cake-orders: "Add a cake order form"
  Spec: 5 acceptance criteria · 9 project facts, 2 of them guesses · saved with the run
  attempt 1 · built · checks: e2e failed (form posts to a placeholder URL)
  attempt 2 · built · checks passed · critic: 12 facts checked, 1 unsure
  Ready for review.

$ raisq review
  ❓ "Delivery within 3 km" rests on a guessed fact (from the old website)
  📝 2 choices the spec didn't cover
  [mark as done]  [send back]  [change the spec]
```

### What the user decides, and what RaisQ handles

| The user decides | RaisQ handles |
|---|---|
| What to build: answering questions about the project and each task | Turning answers into a spec, keeping it current, noticing contradictions |
| Approving the checks and the setup `init` suggests | Finding the tools, writing plugin files, setting up isolation |
| Each review: mark as done, send back, or change the spec | Fresh sessions, retries, checks, the critic, evidence, the review pack |
| Something a task needs that isolation keeps out, such as a test database | Everything else the agent does inside isolation |
| Which lessons to keep | Spotting mistakes, proposing lessons, running them on every later task |

### Other commands

They're only needed occasionally, and each one says when it's useful:

| Command | When |
|---|---|
| `raisq status` | To see each task's state: the queue in order, what's running and against which spec, what's ready, and what's parked and why |
| `raisq fact` | When a project fact changes with no task involved, such as a new phone number. Also to read them: `raisq fact list --kind rule`, `--tag inferred`, `--said-by Mai`, `--due`, or a topic name. The files group by topic; the fields answer every other question. |
| `raisq learn` | Now and then, to spot-check shipped work |
| `raisq doctor` | When something isn't working. Also runs automatically inside `init`, `run` and `batch`. |
| `raisq bench critic` | Before using a new model as the critic. `init` runs it for the model it suggests. |
| `raisq update` | To update RaisQ itself |
| `raisq plugin add`, `plugin new`, `plugin test`, `slots` | To add a plugin of your own, by hand or with AI ([9.3](#93-adding-your-own-plugin)) |

`init` sets up plugins for the tools it recognises, so most projects never need the plugin commands.

---

## 4. How it works

```
raisq task ─► task spec ─► queue
                               │
raisq run / raisq batch        ▼
   for each task:   ┌──────────────────── the loop · up to N attempts ────────────────────┐
                    │ 1 fresh agent session, isolated, builds the change                   │
                    │ 2 raisq runs the checks itself, with the tests from before the task  │
                    │ 3 a separate critic compares the output with the spec, with quotes   │
                    │ fail → the exact failures go to a new fresh session                  │
                    └──────────────────────────────────────────────────────────────────────┘
                               │ passed                     │ out of attempts
                               ▼                            ▼
                         review pack                  parked, with every failure
                               │
raisq review   mark as done · send back (a lesson) · change the spec
                               │
raisq learn    later, on a sample of shipped work ─► new checks for every future task
```

**The one-minute version:** you describe the work. An AI builds it, isolated from the rest of your machine. RaisQ checks it with your tests and with a second AI that must show its evidence. You read only what needs a person.

**Two questions, for every task:**

| Question | Answered by | Applies to |
|---|---|---|
| **Did it do what was asked?** | The checks, and the critic's coverage of each acceptance criterion | Every task |
| **Is what it says true?** | The critic's provenance of each claim against the project facts | Tasks whose output carries facts a user reads: a site, docs, an email template. A refactor or a library change has no claims, and skips this. |

The first is what any CI does, made safe for unattended agents. The second is what RaisQ adds, and it needs project facts to check against, so the presets that produce user-facing text (`web`, `docs`) set those up. A library project may have none.

### 4.1 What a run is for

A run is one task, judged against one frozen spec.

- **Which task.** `raisq run` takes the first task in the queue, which is the order they were added. `raisq run <task>` picks one by name. Parked tasks aren't in the queue; `raisq review` puts one back. The run says which task it's on before it starts, and `raisq status` shows the queue in order. A task is always in exactly one of five states: queued, running, ready, parked or done ([Appendix A](#appendix-a-vocabulary)).
- **Which spec.** Facts and task specs are never overwritten (8.2): every change makes a new revision. When the run starts, RaisQ records the revision it runs against, such as `facts@42`, and copies nothing. Every attempt and the critic read that revision, not the live one, so the builder, the checks and the critic are all judged against the same thing, and the review pack quotes from it. The revision covers every fact, so the critic's "not in spec" means what it says: nothing was filtered out in advance.
- **How the builder reads it.** The pinned revision is mounted into the container read-only, with an index: one line per fact, grouped by topic. The prompt carries the task spec and the index, and says: the facts are at `/raisq/spec/facts/`, read the topic before writing any name, number, price or condition. The builder opens what it needs when it needs it, the way an agent opens a skill file. Nobody picks facts for it in advance, because the one who knows which fact matters is the one writing the footer, at the moment it writes it. The mount is a view of one revision, not `.raisq/`: nothing in it judges the builder, and a change to it fails the audit.
- **Which facts the run relied on** isn't chosen in advance. Choosing at task time would be a guess about what the builder will write, and a wrong guess hides a true fact from the builder and fails it at the critic. Instead the list comes from evidence: every fact the critic cites in provenance (4.3) is recorded in the run with its revision, such as `business/phone@42`.
- **If the spec changes meanwhile.** A change makes a new revision, so a run that has started never sees it. If the task spec changes, or `raisq fact` changes a fact the run relied on, the run finishes as it was, then the task goes back to the queue with a handoff saying what changed, before anyone reviews it. The same applies to a task already ready for review. A change to a fact no output cited touches nothing. Nothing is re-judged halfway.

### 4.2 One attempt

1. **RaisQ writes the prompt** from the run's frozen spec: the task spec, the facts index and where the facts are mounted, the plugins' `for_ai` instructions, and on a retry the handoff. It contains no chat history and no secrets, and it's saved with the run. The prompt and the read-only mount of the frozen spec are the builder's whole interface with RaisQ, one way: it never sees `.raisq/`, never runs `raisq`, and reports back only through `result.json`.
2. **A fresh agent session builds,** isolated, then exits. It writes `result.json` (done, question, needs-something or failed) and a choice log of decisions the spec didn't cover. No `result.json` counts as failed.
3. **RaisQ audits the checkout:** the locked files, the checkout's own `.git` (hooks and config) and anything brought in read-only must be unchanged. Then RaisQ commits the change itself. Nothing else the agent touched survives, because the container is thrown away.
4. **RaisQ runs the checks.** Each is a plugin in the checks slot (7.4), usually a command whose exit code decides; what it prints goes to the handoff and the review pack. Tests the task started with run as they were before the task, so an edit to one changes nothing. A test the builder added counts only if RaisQ can prove it's real: it must **fail on the code from before the task and pass after it.** One that passes on both tests nothing and is reported; one that fails after is the builder's own test catching its own bug, and fails the attempt. Old failures are subtracted; only new failures decide.
5. **The critic judges** (4.3), even when a check failed, so one attempt reports everything wrong with it. Only an attempt with nothing to judge, such as no commit, skips the critic.
6. **Pass** goes to the review pack. **Fail** writes a handoff for the next fresh session, which continues from the committed branch: every failure with only the lines that show it, and the files changed so far. Full logs stay in the run for review. When the attempts run out, the task is parked with every failure attached.

The session and the container are new every attempt. The branch is not: each attempt's commit stays, so the next one fixes forward instead of starting over.

**The order of these steps is code, not configuration.** It lives in the attempt use case (6.1) and nothing outside RaisQ can reorder or skip a step: no plugin, no preset, no setting. That's deliberate, because the order is where the safety is. Audit after build, checks after audit, critic after checks, verdict rules after critic. A plugin fills a slot; it never sees the sequence that calls it.

**The order is fixed; what each step does is not.** Tasks differ, and the steps differ with them:

| Always, for every task | Depends on the task |
|---|---|
| The order, and that no step is skipped | What each extractor finds, and which rubrics are not applicable to this output |
| Every check runs. The audit, the commit, coverage, the verdict rules. | Provenance, only when the output carries claims (4) |
| | Proving new tests, a split into parts, a site or database started first |

Each difference is decided by whoever actually knows, and never by the builder or by a file pattern. RaisQ can't tell from a path what a change affects: an edit to a shared header changes every page. So **every check runs on every task**, and a check that wants to be faster narrows itself with its own tool's knowledge, such as `vitest --changed`, inside its plugin. **Each extractor** gets the output and returns nothing when there's nothing for it, such as `pdf-text` with no PDF. **The critic** answers *n/a* to a rubric that has nothing to look at, saying what it looked for. **Provenance** has nothing to do when the claims step finds none. And a person can add more: a criterion in the task spec ("a test reproduces the bug before the fix"), where coverage enforces it, or a plugin for one task in `tasks/<name>/plugins/`.

**A task can add things to do inside a step, never a step between steps.** Everything a task might need extra has a place in the spine already:

| A task needs | Where it goes |
|---|---|
| A database up, test data seeded, the site built | A check declares it: `needs: [site, database]`. A plugin in the provides slot answers each name, started before that check, or the task is blocked. |
| A migration run, code generated, a package installed | The builder does it. It's part of making the change, and the builder is a whole agent session. |
| Pages rendered, screenshots taken, a PDF exported | An extractor. `page-text` already starts the site and crawls it. A new kind of output is a new extractor plugin. |
| A check that only matters for this task | A plugin file in `tasks/<name>/plugins/`, read in addition to the project's for that task's attempts |
| A preview deploy, a check against production | Not a step. RaisQ stops at review; shipping is the project's own process (2). |

The step list is closed. The slots inside it are open. The moment a task could insert "run my migration" as a step, it could insert "run my test instead of yours".

### 4.3 The critic and the verdict

The critic is a separate model in its own fresh session. Its two constraints are that it **can only read** and that it **must quote** what it read, so RaisQ can check every answer. The simplest way to get that, and what v0 does, is a model with no tools that sees only the spec and the output as text from the extractor. A critic that can open the page or grep the spec, read-only in its own isolation, keeps both constraints and is the fallback if no-tools models don't reach the bar (14). The output is marked as data to judge, never as instructions. The critic works in **two calls per attempt**, so the output, the largest text, is read once (4.7):

| Call | Sees | Answers |
|---|---|---|
| **1. Read the output** | the extractors' output, the task spec, the rubrics | **Claims:** every fact a user would see, each quoted. **Coverage:** for each acceptance criterion, a quote or *missing*. **Rubrics:** *yes* · *no* · *unsure* with a quote, or *n/a* saying what it looked for. |
| **2. Provenance** | the claims as quoted, the facts index, then the facts it points at | For each claim, the fact id that backs it, or none, then *agrees* · *disagrees* quoting both. None means not in spec. |

When the facts fit comfortably, call 2 gets them whole with the index. When they don't, it points at ids from the index first and RaisQ hands over those items. A project with no facts skips call 2.

Claims come first because the critic can only check facts someone has named. A claim it misses is a miss the loop lets through, which is what spot-checks and lessons are for (11).

"Not in spec" is literal, because the index is the whole set: if the critic can't find a line for the claim, there isn't one. Same index, two directions: the builder asks "what do I need to know to write this?"; the critic asks "who vouches for this?". The ids the critic cites are recorded as the facts the run relied on (4.1), which is how a later fact change finds exactly the tasks it affects.

Then RaisQ, not the AI, decides:

| The critic's answer | Verdict |
|---|---|
| A quote that isn't really in the spec or the output | asked again once, then unsure |
| A claim not in the spec | fail (no fabrication) |
| Disagrees, or a criterion missing | fail |
| Agrees with a guessed or expired fact | unsure |
| A rubric answered n/a | not judged. Listed in the review pack with what the critic looked for. |
| Anything else | pass |

A claim is a fact a user would read, such as "delivery within 5 km". A design choice the spec doesn't mention, such as a field order or a button label, isn't a claim: it goes to the choice log for the review pack, never to a verdict. The claims step is where the critic draws that line, and the review pack shows both lists so a person can move an item across.

### 4.4 The review pack

One per task, in the order a person needs it:

1. The task's state, ready or parked, and attempts used
2. **Unsure items,** each with its quote and the spec item it rests on. These are read first.
3. **Choices the spec didn't cover**
4. Tests the builder changed or added, and new dependencies
5. Anything out of date, flaky, or brought in
6. What earlier attempts got wrong, from the handoffs
7. Everything that passed, with links to the evidence
8. The decision: mark as done, send back, or change the spec. Done means the branch is ready for the project's normal merge process. RaisQ doesn't merge it.

### 4.5 Batches

A batch runs the loop for every queued task, one after another. It adds only what running for hours needs:

- **Checks every tool and isolation before the first task,** so it doesn't fail at task 7.
- **Never waits.** A question or a request for something isolation keeps out parks the task, and the next one starts. Answering it at review puts the task back in the queue.
- **Stops cleanly** at its time limit or the agent's usage limit, and resumes from where it stopped.
- **Stops entirely** if the audit finds a breach, because that means the setup isn't safe.
- **Ends with a batch report:** what's ready and what's parked, with what each parked task needs.

### 4.6 Running from an agent's chat

Many people start RaisQ from inside an agent's chat. The chat only relays: it runs the CLI and shows the results. The builder is always a separate fresh session, because the chat holds context that isn't in the spec. Anything said in the chat counts only once RaisQ saves it to the spec.

This is the one place an AI uses RaisQ's CLI, and it's acting as your hands. Every AI that RaisQ starts itself gets a prompt, not a tool:

| Who | Touches RaisQ how | Sees | Never sees |
|---|---|---|---|
| You, or your agent chat acting for you | the CLI | everything | |
| A model writing a task spec (8.1) | text in, JSON out | the request, the facts index, queued task titles; then your answers and the facts the draft cited | code, plugins, rubrics |
| The builder | a prompt and a read-only mount of the frozen spec in, `result.json` out | the task spec, the facts index with the facts mounted to open, every plugin's `for_ai`, the handoff on a retry | `.raisq/`, rubrics, check plugins |
| The critic | text in, JSON out, two calls (4.3) | first the extractors' output, the task spec and the rubrics; then the claims, the facts index and the facts it points at | code beyond the output, the builder's transcript, plugins |

An AI chooses only inside its own answer: which fact backs a claim, which facts a draft rests on. What goes into each prompt is assembled by RaisQ according to the role, and nobody picks it. If `for_ai` ever grows long, each plugin's `when:` line serves as its index entry, the way a skill's description does.

### 4.7 What it costs

Almost all of a task's tokens go to the builder. One attempt is a whole agent session reading code, running commands and reading their output: tens to hundreds of thousands of tokens. Next to that the rest is small. The critic reads one diff or a few changed pages, thousands to tens of thousands. The facts index is about 25 tokens a fact, so fifty facts are under 2,000. Writing a task spec is two short calls. So the order of what to save is: **attempts first, then the critic, then everything else.**

**Fewer attempts.**
- **One attempt reports everything wrong with it.** A failed check doesn't end the attempt: the critic still runs on whatever the extractors produced, so the handoff carries every failure at once instead of one per attempt. Only when there's nothing to judge, such as no commit, does the attempt stop at the failure.
- **Questions are asked before the run, not found during it.** The spec writer asks about everything the facts don't answer (8.1), because a question found by the builder costs a parked task and another attempt.
- **The stuck rule** parks a task after two identical handoffs (10).

**Cheaper retries.**
- **The handoff is short and exact:** each failure with only the lines that show it, not whole logs, which stay in the run for review; and the files changed so far, which RaisQ reads from the branch rather than asking the builder. The next session starts oriented without inheriting the last one's reasoning (D9).
- **Prompts put what changes least first:** the `for_ai` blocks, then the facts index, then the task spec, then the handoff. Where the vendor caches prompt prefixes, most of the prompt is reused across attempts and across tasks pinned to the same revision. There's no setting; it's the order RaisQ writes in.

**The critic reads the output once.**
- **Two calls per attempt** (4.3). The first reads the output with the task spec and the rubrics. The second judges provenance from the claims' quotes and the facts index, without the output. The largest text goes in once.
- **Extractors return what the task changed,** measured on the output, never guessed from paths. The diff extractor does this already. `page-text` renders the site before and after the task and returns the pages whose text changed, whole, with the changed lines marked. Claims come from the marked lines: text that was there before the task isn't this task's to answer for.
- **A verdict already reached is reused.** On a retry, a claim with the same quote against the same fact revision keeps its provenance verdict, and only new or changed claims are asked. Coverage and rubrics are asked again, because the output changed.

**Time.**
- **Baselines are per base commit.** Old failures and the "before" render are measured once for the commit a task starts from, and reused by every task that starts from the same one. A batch of ten pays once.
- **A batch overlaps independent work:** while one task's checks and critic run, the next task's builder can start. Tasks are on separate branches and share nothing that changes.

---

## 5. Safety

Safety comes from how RaisQ works (C1). None of it needs the user to configure anything.

### 5.1 The rules

| Rule | Why |
|---|---|
| **AI never decides pass or fail.** RaisQ's own code runs the checks and applies fixed verdict rules. | The AI being checked can't be the one checking |
| **The builder can't change what judges it.** A lock is not a permission: the builder can edit any file, but RaisQ ignores what it did to the ones that judge it. `.raisq/` is never read back from the checkout, existing tests and check config run as they were before the task, and a new test counts only once it has failed on the old code and passed on the new. A changed locked file fails the attempt. | A stuck builder can't edit a test until it passes, and can't add one that proves nothing |
| **The critic is independent.** It's a separate AI in its own fresh session, and must be a different model from the builder, from a different vendor when there is one. It gets only the spec and the output, has no tools, and must quote both for every answer. RaisQ checks that the quotes exist. | It doesn't share the builder's blind spots, and can't make things up unnoticed |
| **No fabrication.** Every fact in the output needs a source in the spec. Unknowns stay out. | The original reason for RaisQ |
| **The agent works in isolation** (5.2) | Agents' own safeguards can't be relied on when nobody is there to approve them |
| **RaisQ never ships.** No run merges, deploys or releases. Report plugins may post drafts and notes, such as a draft PR or an issue comment. | A person reviews first, every time |
| **Fail loudly.** A missing tool, an unreadable spec or an invalid file stops the run, never skips. | A skipped check would be a pass nobody earned |

### 5.2 Isolation

Every builder attempt is headless, whether watched or not. Nobody can approve the agent's actions as they happen, and some agents have no safeguards at all. So RaisQ doesn't try to predict which actions are allowed. It limits **what can be lost**:

| Inside isolation | Kept out |
|---|---|
| A copy of the project, on the task's branch, with no remotes | The user's other files, and the main repo's `.git` |
| Normal work: any command, installing packages, the network | SSH keys, git credentials, tokens, cloud logins |
| The agent's own login, if it needs one (below) | Anything that could push, deploy or publish |
| Read-only references a provider mounts for the builder (5.3) | Test secrets, until the builder has exited (5.3) |

- **Nothing the agent does lasts** unless it comes out through RaisQ, as a commit that's audited and then reviewed.
- **The checkout is a clone with no remote,** never a worktree, which would carry the main repo's `.git` inside. So there is nothing to push to, even with a token. RaisQ brings the commit out itself, and pushes later, outside isolation, only through a report plugin and only as a draft.
- **The audit after each attempt** checks the locked files and the checkout's own `.git`. It doesn't need to check the user's machine: isolation never gave the agent a path to it. A breach fails the attempt. In a batch, it stops the batch.
- **The running site, anything else a check needs, and the checks themselves** run isolated too, because they run the builder's code.
- **The critic and other model jobs** need no isolation. They have no tools and run no project code.
- **Report and spec source plugins run in RaisQ itself,** outside isolation. Their tokens, such as for GitHub or Jira, are never inside isolation, so the builder can't use them.
- **Isolation comes from a plugin in the isolation slot** ([6.2](#62-slots)), such as Docker. RaisQ doesn't install it (C7).

**The agent's login.** Most agent CLIs need a credential to run at all. That credential is the one secret that must go inside isolation, so it's the one exception the design has to be honest about. The agent plugin says what the login is (a file or a variable) and RaisQ brings in only that, and nothing else from the user's home. With an OAuth login, what the builder can do with it is call the same AI it already is, so the loss if it leaks is spend, not access. An API key is the same, with a hard cap set at the vendor. Either way, the login must never also be a key that can push code or reach the user's cloud, and `doctor` warns when it recognises one.

### 5.3 Bringing things in

Most tasks need nothing beyond what's already inside isolation. When something is needed, it comes in one way: **a plugin in the provides slot, asked for by name.** There is no setting that opens isolation.

**What a check needs** is named on the check, and a provider answers it after the builder has exited and the audit has passed:

```yaml
# .raisq/plugins/playwright.yaml
needs: [site, env.test]

# .raisq/plugins/env-test.yaml · written by init when .env.test is gitignored
name: env.test
slot: provides
from: host:.env.test           # read on the host, injected into the check's environment only
```

A test database URL or a test payment key is in the container exactly as long as the checks run. The builder, which could have sent it anywhere, never had it. If the builder needs to run tests while working, it gets `.env.test.example`, which most projects commit, and a line in the prompt saying real values arrive at check time.

**What the builder needs** is the same mechanism, with one difference: it's present during the build, so it's read-only and may hold nothing that can be lost. A shared design system in a sibling repo, a brand guide, a reference implementation:

```yaml
# .raisq/plugins/design-system.yaml
name: design-system
slot: provides
for: builder                   # mounted during the build, read-only, audited after
from: host:../design-system:ro
```

The builder's prompt says what's provided and where. A changed read-only mount fails the audit. `doctor` refuses a `for: builder` provider that looks like a secret, and warns on a path under the home directory, because plugin files are committed and a teammate's machine won't have it. If the reference is part of this project, it belongs in the repo, where the clone already has it.

**When the builder asks for something it doesn't have,** it reports that in `result.json`. The task stops until the implementor decides: **allow once**, **write a provider**, or **deny**. In a watched run you're asked right away. In a batch the task is parked and the next one starts. Nothing is ever allowed automatically.

None of this is a fact (8). A fact is content the critic checks claims against. A provider is access.

### 5.4 Without isolation

| Situation | What works |
|---|---|
| An isolation tool is available | Everything |
| None, and `isolation: none` isn't set | Manual mode: you make the change yourself, in a chat or by hand, and `raisq run --check` does the rest. Nothing of RaisQ's runs the builder, so nothing needs isolating. |
| `isolation: none` | Everything, with the agent's own safeguards and the audit as the only protection. Meant for a machine that's disposable anyway, such as a CI runner. It's recorded at the top of every review pack. |

Manual mode isn't a fallback. It's the verifier on its own, and it's how RaisQ works with an agent that has only a chat window.

---

## 6. Architecture

### 6.1 The dependency rule

RaisQ follows clean architecture. Code is in rings, and **every dependency points inwards**. The inner rings know nothing about the outer ones: no Docker, no agent CLI, no file paths, no YAML.

A slot is extracted when its second implementation arrives, not scaffolded empty at the start (12).

```
┌───────────────────────────────────────────────────────────────┐
│ 3  Plugins     agent CLIs · models · Docker · git · checks ·  │
│                extractors · spec tools · reports · YAML · CLI │
│   ┌───────────────────────────────────────────────────────┐   │
│   │ 2  Use cases   init · add task · run attempt ·        │   │
│   │                run batch · review · learn             │   │
│   │     Slots      the interfaces the use cases need      │   │
│   │                from outside (6.2)                     │   │
│   │   ┌───────────────────────────────────────────────┐   │   │
│   │   │ 1  Domain  spec items and tags · tasks ·      │   │   │
│   │   │            checks · verdicts and rules ·      │   │   │
│   │   │            lessons · attempt limit            │   │   │
│   │   └───────────────────────────────────────────────┘   │   │
│   └───────────────────────────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────┘
                 dependencies point inwards only
```

| Ring | Holds | Depends on | Changes when |
|---|---|---|---|
| **1 Domain** | The rules: what a spec item is, how tags work, the verdict rules, the attempt limit, when a lesson retires | Nothing | RaisQ's rules change |
| **2 Use cases and slots** | The flows: add a task, run one attempt, run a batch, review, learn. Each slot is an interface a use case needs from outside, with its contract. | The domain | A flow changes, or a use case needs something new from outside |
| **3 Plugins** | What fills each slot: built-in plugins, and plugins for the tools the user installed | The slots | A tool changes, or a new one is added |

Slots sit in ring 2, next to the use cases that need them, not in a ring of their own. If they sat further out, the use cases would depend outwards, which is the one thing the rule forbids. The inner ring owns the interface, the outer ring fills it.

**Why this shape:**
- **Swap anything outside without touching the rules.** A new agent, isolation tool, spec tool or report target is a new plugin. The verdict rules don't change (C6).
- **Safety lives where nothing can bypass it.** The verdict rules, the locks and the quote check are in rings 1 and 2. A plugin only fills a slot, so it can't skip them (C1).
- **Test the core with no tools at all.** Every slot has a fake plugin for tests. The whole flow, from task to verdict, runs in a unit test with no agent, no Docker and no git (C8).
- **Even the CLI is outside the core.** It's one front end on the use cases. An MCP server or an agent's slash commands are others. None of them holds logic.

### 6.2 Slots

A slot is an interface the core defines. A plugin fills it. Some slots have a built-in plugin, and some need a tool the user installed.

| Slot | Job | Filled by default with | Other plugins, for example |
|---|---|---|---|
| **Builder** | Make the change | an installed agent CLI | any agent CLI |
| **Model** | The critic, and every other AI job with no tools. Set by `critic:` in `raisq.yaml`. | an installed agent CLI in no-tools mode | an AI API, a local model |
| **Isolation** | Keep each attempt away from the user's machine (5.2) | an installed tool such as Docker | Podman, bubblewrap |
| **Checks** | Given a checkout, return pass, fail or a number, plus text (7.4) | the project's own commands, one plugin file each, found by `init` | Playwright, Lighthouse, a `median` wrapper, per-test parsers |
| **Provides** | Start something a check or extractor `needs:`, inside isolation, and say when it's ready | `site`, written by `init` from the project's start script | a database, a mock API, a browser |
| **Extractor** | Turn the output into text for the critic | git diff (built in) | page text, PDF text |
| **Spec source** | Where spec items come from | RaisQ's own writer (built in) | Spec Kit, OpenSpec, Jira, GitHub issues |
| **Report** | Deliver review packs and batch reports | markdown files (built in) | a draft PR on GitHub, a Jira comment, Slack |
| **Checkout** | A copy of the project for each task | git (built in) | a fake, for tests |
| **Store** | Where `.raisq/` data is kept | files (built in) | a fake, for tests |

Checkout and Store have one real plugin each. They're slots only so the core can be tested with fakes. There's no slot for *where* runs happen: CI or a schedule is just a place that invokes the CLI, and an example workflow file covers it.

**How a plugin gets picked.** Every plugin file says which slot it fills (`slot: checks`). What happens next depends on whether the slot takes one plugin or many:

| Slot takes | Slots | Picked by |
|---|---|---|
| **One** | Builder, Model, Isolation, Checkout, Store | A line in `raisq.yaml`, such as `builder: codex-cli`, because a choice has to be made. `init` writes it. |
| **All of them** | Checks, Extractor, Report | Every plugin file with that `slot:` is used, on every task. A check you never want is a file you delete. Checks run in `priority:` order, alphabetically when equal. An extractor with nothing to extract returns nothing. |
| **The one that matches** | Spec source | `raisq task --from SHOP-123` asks each spec source plugin, and the one whose `matches:` id pattern fits it, such as `"[A-Z]+-[0-9]+"` for Jira, runs. RaisQ's own spec writer is used when there's no `--from`. |
| **By name, on request** | Provides | A check or extractor says `needs: [site]`; the file with `slot: provides` and `name: site` is started. Nothing starts unless something asked for it. |

So `raisq.yaml` names a plugin only where a choice exists. For checks, the files being in `plugins/` is the whole configuration. `init` writes them from what it recognises in the project, the preset adds its own, and `raisq learn` adds one when a lesson becomes a check.

`init` is the one action that runs before `plugins/` exists. It uses the built-in `detect` plugin, which reads the project (`package.json`, a Makefile, test configs, what's on `PATH`) and proposes the preset, the one-plugin choices for `raisq.yaml`, and a check file per command it found. You approve the list, and from then on every action uses the rule above.

The core never sees any of this. A use case asks the slot registry for "the builder", "all checks" or "the extractors for this output", and the registry applies the rule. File names, vendors and Docker exist only in ring 3.

A plugin can be written three ways: a file describing an installed tool, a small program in any language, or Go code built into RaisQ ([9.3](#93-adding-your-own-plugin)). They all fill a slot the same way, and the core can't tell them apart. The built-in `command` plugin is what lets a file describe any CLI, so a new tool needs a text file, not new code (C8).

### 6.3 Inside RaisQ

| Ring | Packages |
|---|---|
| 1 Domain | `domain`: one package with the types and rules from [Appendix A](#appendix-a-vocabulary). Imports nothing from RaisQ. |
| 2 Use cases and slots | `usecase`: one file per flow (setup, task, attempt, batch, review, learn); `slot`: one Go interface per slot, with its contract and a fake plugin for tests |
| 3 Plugins | `plugin/git`, `plugin/store`, `plugin/command`, `plugin/api`, `plugin/isolation`, `plugin/extract`, `plugin/report`, `plugin/detect`; `cmd/raisq`, the CLI |

`domain` stays one package until it's too big to read, then it splits along the vocabulary groups. The import rule is three lines: `domain` imports nothing from RaisQ; `slot` and `usecase` import only `domain`; everything else may import inward.

**Rules that keep it this way:**
- A package may import only from rings further in. A lint rule in CI fails the build on any import pointing outwards.
- Every slot has a JSON Schema for anything that crosses it as a file, and a fake plugin for tests.
- Every behaviour that matters has a test that runs offline.

### 6.4 What ships

| Layer | What | Who changes it |
|---|---|---|
| **RaisQ** | One executable: all three rings, prompts, built-in plugins and presets | A RaisQ release |
| **Tools** | What the user installed: agent CLIs, Docker, Node.js, test runners | The user. RaisQ never installs them (C7). |
| **The project** | `.raisq/`: one settings file, plus files RaisQ writes and keeps up to date | RaisQ writes it. The user approves it, and rarely edits it. |

### 6.5 Technology

| Choice | Decision |
|---|---|
| Language | Go: one ready-to-run file per platform, with no runtime for users |
| Release | GitHub Releases only at first, with an install script and `raisq update`. Unsigned at first. |
| Formats | YAML for anything a person might read, JSON for machine records. A JSON Schema for every file. |
| Tests | Go's own test tool, plus fixture projects with planted mistakes |

---

## 7. Configuration

### 7.1 What the user sees

After `raisq init`, a typical project has one settings file:

```yaml
# .raisq/raisq.yaml · written by raisq init
preset: web
builder: codex-cli
critic: claude-code      # a different model from the builder, in no-tools mode
isolation: docker
```

Four lines, and one rule with no exceptions: **a line in `raisq.yaml` names a plugin for a slot that takes one.** `builder`, `critic`, `isolation`, and `preset`, which picks the starting files. A default from 7.2 can be overridden here too, such as `attempts: 5`, but most projects never do. `critic` fills the model slot for every AI job with no tools, including writing specs and drafting plugins.

**There are no project commands in it.** Two that would seem to belong don't:

- **No setup step.** The builder is a whole agent session; it sees `package.json` and runs `npm ci` when it needs to, as every agent does. Caching dependencies between attempts is the isolation plugin's business, invisible to the core.
- **No start command.** A check or an extractor that needs the site running says `needs: [site]`, and a plugin in the **provides** slot answers it ([7.4](#74-checks-are-plugins)). The command to start a Next.js site is knowledge about Next.js, so it lives in a plugin file `init` writes from `package.json`, not in settings.

**What else isn't in it, on purpose:** the list of checks, reports or extractors (every matching plugin file runs, 6.2); anything about a specific tool (its plugin file); rubrics and facts (content, not settings); allowed commands or hosts (there is no allowlist, 5.2); and prompts, verdict rules or the step order (not configurable at all, 4.2). The builder never reads this file: `.raisq/` is locked and never read back from the checkout.

There is no setting that opens isolation. Anything that enters the container, for a check or for the builder, is a plugin in the provides slot, asked for by name, and approved like every other plugin (5.3).

### 7.2 Defaults

| Setting | Default | Change it when |
|---|---|---|
| `attempts` | 3 per task | Tasks are usually bigger or smaller than that |
| `batch` | stop after 8 hours, or at the agent's usage limit | Runs need a different window |
| `split_over` | 6 acceptance criteria | Tasks are usually bigger or smaller than that |
| `spot_check` | 20% of shipped tasks | You want more or fewer spot-checks |
| `keep_artifacts` | 30 days, longer for parked or sampled tasks | Disk space or audit needs differ |
| `recheck` | from the preset, by kind of fact | Facts go out of date faster or slower |

A setting is added only when many projects really need a different value (C5).

### 7.3 Files RaisQ writes

The user doesn't have to open these. They're plain text, committed, and reviewed like code.

| File | What | Edited by hand? |
|---|---|---|
| `facts/` | What's true about the project, one file per topic: facts and rules that outlive any task. Rules include how to work here: commit style, what not to touch, house conventions. | Rarely. `raisq task` and `raisq fact` write it. `init` seeds rules from an existing `CONTRIBUTING.md` or `AGENTS.md`, tagged inferred until confirmed. |
| `tasks/<name>/` | Everything about one task: its `spec.yaml`, its `state.yaml`, and `runs/`, each run with the spec revision it ran against and the facts it relied on, every attempt's prompt, result, verdicts and handoff, and the review pack. Optionally `plugins/`, for a check that matters only to this task. | Rarely. `raisq task` creates it, runs fill it. |
| `plugins/` | One file per plugin, checks included. Every tool RaisQ runs is described here and nowhere else. | Rarely. Written from built-in plugins or by `init`. |
| `rubrics/` | The critic's standing questions, asked on every task: what good looks like for this kind of project, which no task's criteria would say. Provenance and coverage need no file; they're derived from the facts and the task spec. | Rarely. Written by presets and `raisq learn`; add one by hand if you like. |

There's no `lessons/` folder. A lesson is a check or a rubric that came from a mistake, so it lives where checks and rubrics live, with a `from:` line saying which mistake (11).

A rubric file is a list of questions, each with the answer that fails it:

```yaml
# .raisq/rubrics/web.yaml · copied in by the web preset
- id: web/no-placeholder
  ask: Does any text a user would see look like placeholder or sample text?
  fail_if: yes
- id: web/form-states
  ask: Does every form show what happened after submitting, success or error?
  fail_if: no
```

Every rubric is asked on every task. When the output has nothing a rubric could look at, such as no form at all, the critic answers *n/a* and says what it looked for. That isn't a fail, and the review pack lists it, so a wrong *n/a* is visible.

A question goes here, not in a check, when a command can't answer it. "Lorem ipsum" is greppable; "Your text here" and "TODO: real copy" aren't, and the critic reads them the way a person would.

A task spec lives with its task, not with the facts, because the facts outlive every task and a task's spec dies with it. One folder per task also means `raisq status` is a read of `tasks/*/state.yaml`, and deleting a folder removes the task, runs and all.

There is no separate checks file. A check is a plugin in the checks slot ([7.4](#74-checks-are-plugins)), so the project's test command, Playwright and Lighthouse each get a file in `plugins/`, the same kind of file as the agent and the isolation tool.

### 7.4 Checks are plugins

A check is anything that fills the checks slot. The slot's contract is small on purpose: **given a checkout, return pass, fail or a number, plus text for the handoff.** The domain knows nothing more than that. Everything a particular tool needs lives in its own plugin file, where a new tool can't force a change to RaisQ (C6, C8).

```yaml
# .raisq/plugins/unit-tests.yaml · written by init from package.json
name: unit-tests
slot: checks
run: npm test                      # exit 0 passes; output goes to the handoff
```

```yaml
# .raisq/plugins/lighthouse.yaml · copied from the built-in by the web preset
name: lighthouse
slot: checks
run: npx lighthouse http://localhost:3000 --output=json
needs: [site]                      # what it needs; raisq provides it before running
result: { from: json, path: categories.performance.score, scale: 100 }
pass_at: 90
tool: { find: npx lighthouse --version, supported: ">=12 <13" }
docs: { url: <official docs>, verified_version: 12.x, verified_on: YYYY-MM-DD }
```

- **Every file with `slot: checks` runs, on every task.** There's no list of checks to maintain: RaisQ loads `plugins/`, keeps the ones in the checks slot, and runs them in `priority` order, alphabetically when equal (6.2). RaisQ never skips a check because of which files changed, since it can't know what a change affects. A check that's slow can narrow itself inside its own plugin, with its tool's own knowledge of what changed, such as `vitest --changed`.
- **`init` writes one plugin file per project command** it finds: build, lint, test. Three small files instead of one list, but the same kind of file as every other tool, so there's one place to look and one format to learn (D13).
- **What a check needs is the plugin's business.** `needs: [site]` names it, and a plugin in the **provides** slot with `name: site` answers it, started inside isolation before the check runs and waited for until `ready:` responds. No provider, the task is blocked. `init` writes `site.yaml` from the project's start script; a `database` provider is written the same way when a check asks for one. The core never knows what a site or a database is; it knows a check asked for something by name and a plugin offered it.

```yaml
# .raisq/plugins/site.yaml · written by init from package.json scripts
name: site
slot: provides
run: npm run build && npm run start
ready: http://localhost:3000
```
- **How a number passes is the plugin's business.** `pass_at` is read by the `command` plugin, not the domain. The domain sees pass or fail.
- **Running a flaky check three times is a plugin too.** The built-in `median` plugin wraps another check. A preset uses it for Lighthouse. The core has no idea.
- **A lesson that becomes a check is a plugin file** written by `raisq learn` after approval, with `from: task/<name>` naming the mistake it came from. A check has nowhere else to live (11).
- **Checks still run inside isolation,** from the committed plugin files and never from the checkout, with existing tests restored from before the task, new tests proven, old failures subtracted, and a missing tool blocking rather than passing (4.2). The safety is in how the slot is called, which no plugin can change.

---

## 8. Facts and specs

### 8.1 Two kinds

- **Facts:** what's true about the project across tasks, one file per topic. In `facts/`. For a bakery site, `init` might write `business.yaml` (name, contact, hours), `orders.yaml` (delivery, minimums, and the rule that every order needs a phone) and `conventions.yaml` (how to work here: commit style, what not to touch). RaisQ reads every `.yaml` under it, however it's arranged: a project that wants `facts/api/` and `facts/site/` just makes the folders. An item's `id` is a name, not a path; moving the file changes nothing.

  Files group by **topic**, never by kind or by source. Those are fields on the item (`kind: rule`, `said_by`), and a folder that repeats a field can disagree with it. A rule about orders sits next to the delivery radius, because that's its topic; the house rules get their own file because "how we work" is a topic too.
- **Task spec:** one piece of work and its acceptance criteria. In `tasks/<name>/spec.yaml`.

`raisq task` writes both. It sorts every answer into the right one, and asks before saving. `raisq fact` writes only the first, for when something changes with no task involved.

**What the model sees while writing a task spec.** Two no-tools calls, each starting empty, with a person in between:

| Call | The model sees | It answers |
|---|---|---|
| **1. Draft** | The request, the facts index, the titles of queued tasks | Draft acceptance criteria; the fact ids it rests on; a queued task it duplicates, if any; questions about what the facts don't answer, and nothing the index already covers |
| You answer | No model | |
| **2. Settle** | The request, the draft, your answers, and the facts the draft cited plus any your answers touch | For each answer: task criterion or fact. For each new fact: which existing fact it contradicts, if any. A contradiction is asked about, never settled. |

Everything about a task is decided before it's queued, because a question found by the builder costs an attempt and a parked task (4.7).

**The spec holds only what's specific to this task.** Steps, checks, rubrics and `for_ai` apply to every task, and RaisQ assembles them per role when the run starts (4.6). So the spec never lists them and the spec writer never reads them. A spec that chose its own checks could leave one out. What a person wants beyond that goes in the spec as a criterion ("a test reproduces the bug before the fix"), where coverage enforces it.

**No code goes in.** A spec says what must be true when the task is done, and the builder decides how. Read from code, a spec drifts into instructions ("edit `OrderForm.tsx`") and costs far more context. The facts come through the same index the builder and the critic use (4.1, 4.3), so a question the facts already answer is never asked. When the index itself is long, the model sees the topic names first and opens a topic's lines, the way an agent opens a skill.

**What's not a fact.** A fact is something the critic can check the output against. Three things look like facts and aren't:

| It says | It is | Lives in |
|---|---|---|
| "Use Playwright for anything a user sees in the browser" | How to use one tool. Nobody can check it from the output. | That plugin's `for_ai` block (9.1). Remove the plugin, the instruction goes with it. |
| "Run lint before finishing" | Something RaisQ should verify itself, without the builder's cooperation | A check (7.4) |
| "The delivery radius is in the spec" | A pointer, not a fact | Nothing. Facts are content, not references. |

A **rule** is a fact: something true about this project that the output must respect. "All copy in British English." "Use the design system's components, never raw HTML." And the house rules too: "small commits", "don't refactor what the task didn't ask for", "test first". Those are as true of this project as its phone number is, and they go in `facts/` with `kind: rule`. The critic judges the ones it can see in the output, which is most of them: an unasked-for refactor is in the diff, test-first shows as a proven test, style is what a critic reads. The rest are still told to the builder, and the checks and rubrics catch what it ignores. `init` seeds rules from an existing `CONTRIBUTING.md` or `AGENTS.md`, tagged inferred until you confirm them.

`for_ai` is the one thing that looks like a rule and isn't: it's a tool's instructions for itself, shipped with the plugin, gone when the plugin is removed. A fact may point at another fact (`see: business/phone`) so nothing is written twice, and at nothing else. The moment a fact names a plugin, the critic would have to know what a plugin is.

**Where every kind of rule goes,** by who reads it and whether it can be verified:

| The question | Where | Who reads it | Verified by |
|---|---|---|---|
| What to build | `tasks/<name>/spec.yaml` | builder, critic | the critic's coverage |
| What's true, and what the output must respect, including how to work here | `facts/` | builder, critic | the critic's provenance, where it can see it |
| What good looks like for this kind of project | `rubrics/` | critic | the critic's answers |
| What must hold, that a command can check | `plugins/`, `slot: checks` | RaisQ | the check itself |
| How to use one tool | that plugin's `for_ai` | builder | nobody; it's the tool's own guidance |

### 8.2 Every item says how we know it

```yaml
# .raisq/facts/orders.yaml
- id: orders/delivery-radius
  text: Delivery is offered within 5 km
  kind: contact-terms             # the preset sets a recheck period per kind
  tag: confirmed                  # confirmed · inferred · gap
  said_by: Mai (stakeholder)
  said_on: 2026-11-03

- id: orders/hours
  text: Tue–Sun 8:00–18:00
  kind: hours
  tag: inferred                   # read off the old site by init; nobody has confirmed it
  needs: stakeholder              # the implementor can't confirm this one; Mai has to

- id: orders/minimum
  tag: gap                        # we know we don't know; the builder is told to leave it out

- id: orders/phone-required
  text: Every order must have a phone number
  kind: rule                      # a constraint, not a world fact; never expires
  tag: confirmed
  said_by: implementor
  said_on: 2026-11-03
```

| Field | What it does |
|---|---|
| `text` | The fact. What the builder is told, and what the critic quotes. |
| `kind` | What sort of fact, from the preset's list: `contact`, `hours`, `price`, `terms`, `rule`. The preset sets a recheck period per kind, so nobody sets one per item. An item may override it with `recheck:`. |
| `tag` | How we know it. `confirmed`: a person said so. `inferred`: an AI's guess, or a draft nobody has checked. `gap`: known to be unknown. |
| `needs: stakeholder` | The implementor can't confirm this one. It stays inferred until the stakeholder has seen it, and the review pack says so. |
| `said_by`, `said_on` | Who and when. The audit trail. |

**What the tag does to a verdict** (4.3): a claim backed by a confirmed fact passes. Backed by an inferred or expired fact, unsure, at the top of the review pack. Disagreeing with any fact, fail. Backed by a gap or by nothing, fail: the builder invented it.

- **Only a person confirms.** Anything the AI drafts starts as inferred. An answer typed at `raisq task` is confirmed by the implementor; a fact that needs the stakeholder is marked `needs: stakeholder` and stays inferred until they've seen it.
- **Gaps stay out of the output.** The builder is told what's unknown, and must leave it out.
- **Nothing is overwritten.** A change makes a new revision and keeps the old one in history. A run reads only the revision it started on, and no AI reads history.
- **Confirmations expire,** by kind. The `web` preset might say contact details every 180 days, prices every 30. A rule never expires. An expired fact counts as inferred again until someone reconfirms it, and `raisq status` lists what's due.
- **Contradictions are asked about,** never settled silently.
- **A project may have no facts at all.** A library has no phone number. Then the critic skips provenance and checks coverage and rubrics only (4). The `web` and `docs` presets are the ones that ask for facts at `init`, because their output carries claims a user reads.

### 8.3 Other spec tools

A project that uses Spec Kit, OpenSpec or another tool keeps using it, and its files stay the source of truth. The spec converter reads them in any format, a model splits them into items, and RaisQ checks that every item quotes a line of the original and that every line of the original landed in some item. If anything is missing, the run stops. Items made this way are inferred, not confirmed: the converter knows what the file says, not who agreed to it. Each tool's plugin says which of its fields, if any, count as a person's confirmation.

Issue trackers work the same way, through spec source plugins. `raisq task --from SHOP-123` turns a Jira or GitHub issue into a task spec. What comes in this way is inferred until a person confirms it, and it's marked as text from outside, because it could contain instructions aimed at the builder.

---

## 9. Plugins and presets

### 9.1 A plugin file

```yaml
# .raisq/plugins/playwright.yaml
name: playwright
tool:
  find: npx playwright --version
  supported: ">=X.Y <Z"            # from the official docs
  install_hint: npm i -D @playwright/test
docs: { url: <official docs>, verified_version: X.Y.Z, verified_on: YYYY-MM-DD }
slot: checks                       # how raisq runs it
run: npx playwright test --reporter=json
for_ai:                            # how the AI should use it
  when: Changing anything a user sees in the browser
  how: Add or update a test for every page you change.
  never: Never update screenshots.
```

- **One file covers both RaisQ and the AI.** Either part is optional.
- **`slot:` is how RaisQ knows what the file is for,** and the only required line besides `name:`. Checks read `priority:` for their order, and spec sources read `matches:` for which ids they take (6.2).
- **RaisQ never runs `install_hint`.** It's text for a person.
- **Built-in plugins** for common tools are text inside RaisQ, copied into `.raisq/plugins/` by `init`.
- **Metadata from the official docs is required** (C8): supported versions, the docs URL, and the version and date the docs were read. A recorded output sample for each supported version is used by RaisQ's tests.
- **`supported` is a warning, not a lock.** Outside the range, `doctor` says so and the run continues; a version whose output no longer parses stops the run. Blocking on every new release would make RaisQ the thing that falls behind (C7). The output schema is what's pinned, and tests catch the day it breaks.

### 9.2 Agents and models

| Kind | What it is | Contract |
|---|---|---|
| **Agent** | An AI that works with tools. Fills the builder slot. | In: an isolated task checkout, a prompt file. Out: `result.json` (done, question, needs-something or failed), changed files, a choice log. |
| **Model** | An AI that answers one question with no tools. Fills the model slot. | In: a prompt and an answer schema. Out: JSON on stdout, checked against the schema by RaisQ. Anything that doesn't validate is asked again once, then counts as unsure. |

- **Every call is a new process with an empty context.**
- **The builder's words aren't the result.** RaisQ reads the files it wrote and the changed task checkout, never its transcript. A model, by contrast, has nothing but its answer, which is why that answer is validated.
- **One agent CLI can fill both slots** when it has a no-tools mode, but the critic must be a different model from the builder ([5.1](#51-the-rules)). `init` prefers a different vendor when it finds two. With one, it picks a different model from the same vendor and says so on every review pack. There is no setting that turns the critic off.
- **Unknown agents** are described in a plugin file, then tested with `raisq plugin test` before use. Agents also get extra trials: does it run without prompts, report a denied request, and log in inside isolation?
- **An agent with only a chat window** works in manual mode.

### 9.3 Adding your own plugin

Anyone can add a plugin for any slot, by hand or by asking an AI to write it. There are three ways, and each needs less from RaisQ than the one after it:

| Way | What you write | Language | When |
|---|---|---|---|
| **1. A plugin file** | A YAML file in `.raisq/plugins/` describing an installed tool | none | The tool has a CLI, and its output fits the slot with a little mapping. Most cases. |
| **2. A small program** | Any program that fills the slot over a small JSON protocol: RaisQ sends a request on stdin, the program answers on stdout | any language | The tool needs logic a plugin file can't express, such as calling an API, parsing an unusual format, or combining several commands |
| **3. Built into RaisQ** | Go code inside RaisQ that fills the slot | Go | Contributing to RaisQ itself, for something many projects need |

Ways 1 and 2 need no change to RaisQ, and no rebuild. A program plugin is just a plugin file whose `run:` points at the program:

```yaml
# .raisq/plugins/jira-source.yaml
name: jira-source
slot: spec-source                 # which slot it fills
protocol: raisq-plugin/v1         # JSON in on stdin, JSON out on stdout
run: ./tools/jira-source          # any program, in any language
tool:
  find: ./tools/jira-source --version
```

#### Every slot is documented for plugin authors

Each slot ships with what anyone, a person or an AI, needs to write a plugin for it:

| Part | What it gives |
|---|---|
| **The contract** | What goes in and what must come out, as a JSON Schema |
| **Examples** | Recorded requests and expected responses |
| **A plugin test** | `raisq plugin test <name>` runs the plugin against the slot's examples and edge cases: invalid input, timeouts, missing tool, oversized output |
| **A template** | `raisq plugin new <slot>` writes a starting plugin file, or a starting program |

`raisq slots` lists every slot, what it's for, and its contract version.

#### Letting AI write the plugin

The same files that document a slot are what an AI needs to write a plugin for it. `raisq plugin add <tool> --slot <slot>`:

1. **Asks first,** then reads the tool's `--help` and the project's own usage of it, such as `package.json` scripts.
2. **Gives a model the slot's contract, examples and template,** and asks it to draft the plugin. It writes a plugin file if it can, and a program only if the slot needs it.
3. **Runs `raisq plugin test`** on the draft. If the draft fails, the failures go back to the model, up to a few times. This is the same loop as building, applied to the plugin.
4. **Shows the result to the implementor:** the file, what it can do, and the test results. It's used only after they approve it (C1).

A plugin written this way follows the same rules as any other:

- **It's a project file.** It's committed, reviewed and locked during runs.
- **It runs isolated** when it runs the builder's code or the project's code.
- **It only fills its slot.** It can't reach the domain or the use cases, so it can't skip a check or change a verdict (6.1).
- **Its AI instructions (`for_ai`) reach the builder only after approval.**

#### Sharing plugins

A plugin is a file, or a file and a program, so it can be shared through a gist, a repo or a preset. RaisQ never fetches one (C7). People copy it in, and `raisq plugin test` checks it on their machine before they approve it.

### 9.4 Presets

A preset is a starting point for one kind of project. It's a bundle of plugin files, rubrics and defaults, and nothing else: `init` copies it into `.raisq/`, and from then on it's the project's own files. A preset has no code of its own, so it can't do anything a project couldn't do by hand with the same files.

| Preset | Adds |
|---|---|
| `web` | Starts the site, finds its pages, and checks links, consistency of contact details, Lighthouse and accessibility. The page text goes to the critic. |
| `docs` | Reads Markdown directly, checks links, and checks that the docs cover every feature |
| `library-go`, `library-ts`, … | A check that the public API didn't change, per language |

- **A project can use several.** `preset: [web, library-ts]` for a site that also publishes a package. `init` copies each one's files in turn. Because a preset is only files, the collisions are small: the same plugin file from two presets is copied once if identical and asked about if not; the same default set twice, the last in the list wins; two rubrics asking the same thing both run, and a person can delete one. `detect` proposes the list when it sees more than one kind of project.
- **A check that needs logic is a built-in plugin, not preset code.** The contact-details check or the page crawler are Go plugins in the checks and extractor slots, used by the `web` preset like any other plugin. A project that doesn't want them deletes a file.
- **Every built-in plugin is fully specified** before it's written: tool, versions, exact command, the output it reads, inputs, noise handling, from the official docs.
- **Plugins say what they need, not which tool,** for example "a browser". Any plugin that provides it works.

---

## 10. Edge cases

Most people won't need this section.

| Situation | What happens |
|---|---|
| The agent has a question | Watched run: asked right away. Batch: the task is parked, and the next one starts. |
| The agent needs something isolation keeps out | Same as above, with allow once, write a provider, or deny (5.3) |
| A check's tool is missing | The task is blocked, never passed. A batch checks every tool before it starts. |
| A check gives different results on the same output, such as a Lighthouse score | The preset wraps it in the built-in `median` plugin, which runs it 3 times. A pass-or-fail check that flips is reported as flaky, not sent back to the builder. |
| A big task, over `split_over` criteria | `raisq task` proposes a split along the acceptance criteria and asks. Each part runs the loop on the same branch, in order, then the whole task is checked together. |
| A fact or the task spec changed while a task was running or waiting for review | If the run relied on it (the critic cited it), the run finishes against its frozen spec, then the task goes back to the queue with a handoff saying what changed. Nobody reviews the stale result. A change to a fact the output never cited touches nothing (4.1). |
| A bug fix | Same rule as any new test (4.2): the builder's test must fail on the code from before the task and pass after. The spec for a bug fix says there must be one. |
| A task that fails the same way every attempt | After two identical handoffs, the task is parked early with a note, instead of spending the last attempt on the same wall. |
| An unknown agent | `raisq plugin add <agent> --slot builder` drafts and tests a plugin ([9.3](#93-adding-your-own-plugin)) |
| An unknown tool, or a whole new kind of plugin | The same, for any slot. Or write it by hand from `raisq plugin new <slot>`. |
| A project without git | `init` offers to run `git init` |
| A report plugin can't reach its service | The local record is complete either way. RaisQ retries on the next run, and `raisq status` says so. |
| A project's checks can't run in a container, such as an iOS app | `isolation: none` on a machine set aside for it, recorded on every run |

---

## 11. Learning

A lesson is a check or a rubric that exists because a mistake got through: a wrong result at review, a wrong check, a failed deploy, or a false pass found by a spot-check. `raisq learn` asks what would have caught it, and there are only two possible answers, because RaisQ has only two kinds of verification:

| The mistake could be caught by | The lesson becomes | Written to |
|---|---|---|
| A command, deterministically | A check | `plugins/<name>.yaml` with `slot: checks`, because that's where every check lives |
| Judgment, reading the output | A rubric | `rubrics/<name>.yaml` |

Either file carries `from: task/<name>`, so the reason it exists stays visible. A lesson isn't a third kind of file; it's one of the two existing kinds with a `from:` line.

**Where the command comes from,** when a lesson becomes a check:

- **Usually a built-in check plugin already does it.** Contact details not matching the facts is the most common mistake on a small site, so `raisq-check contacts` is built in. The lesson writes a file pointing at it with the right arguments.
- **Otherwise a model drafts a script,** the same way `raisq plugin add` drafts a plugin (9.3). It's tested the way a new test is: it must fail on the commit that had the mistake and pass on the fixed one. If it can't, the lesson becomes a rubric instead.
- **Nothing runs until you approve it.** The AI proposes; the implementor approves.

**Rules:**

- **A lesson says how to check, never what's true.** "Every page shows the same phone number" is a lesson. "The phone number is 02-123-4567" is a fact and goes in `facts/`.
- **A check where possible, a rubric otherwise.** A grep beats an AI's opinion when a grep will do.
- **Lessons that keep getting overruled retire.** Every time a reviewer marks one of its fails as wrong, the count goes up. Past a threshold, `raisq learn` proposes retiring it. A retired lesson keeps its file with `retired: true`, so the history isn't lost and it isn't re-learned.

---

## 12. Building it

Everything in this plan gets built together. This section is only about the order to **prove** things in, so a wrong assumption is found after a day, not after the whole thing is written.

### Two spikes first

| # | Spike | Shows |
|---|---|---|
| S1 | **The critic.** Hand a candidate model a diff with five planted false facts, the spec, and the two calls in 4.3. Count what it catches and whether its quotes are real. Run it again with provenance in the same call as the output, and compare. Try two models. Record the tokens of each call. | Whether a no-tools model can do the job at all. If it catches fewer than 4 of 5, the critic gets read-only tools (4.3) before anything is built on it. Whether provenance from quotes alone loses anything; if it does, call 2 also gets the lines around each quote. The real numbers behind 4.7. |
| S2 | **The headless agent.** Run each candidate agent CLI inside a container with only its login brought in, on a toy task. Record what it needed. | Which agents can be unattended builders, and exactly what each one needs inside isolation (5.2). Answers Q1. |

Each is a day. Both change the design if they fail, so they come before the code that depends on them.

### The order to get things working

Not phases, just the sequence in which each piece can be seen working on a fixture, so the next one builds on something proven:

1. **The verifier alone,** with a person making the change: `raisq init`, `raisq task`, freeze the spec, restore tests from before the task, prove new tests, run checks, the critic, the verdict rules, the review pack, `raisq review`. This is manual mode, and it's the core. Once it works, every other piece is plumbing around it.
2. **The unattended builder:** the task checkout as a clone, isolation, the agent's login brought in alone, the audit, fresh sessions, prompts, handoffs, the attempt limit, `result.json`, the choice log.
3. **Hours unattended:** `raisq batch`, parking, resume, the batch report, spec history and expiry, the split, `raisq learn`.
4. **Open it up:** plugin files, the `command` plugin, the program protocol, slot contracts and `plugin test`, `raisq plugin add`, presets, the spec converter, spec source and report plugins.

The slots in 6.2 are extracted as the second implementation of each arrives, not scaffolded empty at the start.

### Done when

- Every row of the verdict table and every edge case in section 10 has a planted mistake in a fixture, and gets the expected verdict.
- The critic hits the bar in 13 on two models.
- The safety tests in 13 pass: a builder can't edit a test to pass, add one that proves nothing, reach credentials, plant a hook or push.
- A new user with Docker runs a first task unattended in under 30 minutes.
- Ten fixture tasks run overnight and the batch report matches what happened.
- The bakery fixture runs end to end on the `web` preset, and a plugin file for a tool RaisQ has never seen fills its slot with no change to RaisQ.

---

## 13. Testing

| Level | Covers | AI needed? |
|---|---|---|
| **Unit** | Every core module | No. Runs offline in CI. |
| **Contract** | Every built-in plugin against recorded tool output | No |
| **End to end** | Full runs on fixture projects | Yes, nightly and before releases, with a cost limit |

**Fixtures:** a website, a library, a docs site, a Spec Kit app, and a running project with existing failures. Each has planted mistakes and the expected verdicts. The planted mistakes cover every row of the verdict table in 4.3, every edge case in section 10, and a builder that tries to steer the critic.

**The critic is measured, not assumed:** catch rate at least 90%, false fails at most 10%, quote accuracy at least 95%. Every miss found in real use becomes a new planted mistake.

**Safety tests** cover every rule in section 5. For example: a builder can't edit a test to pass, can't reach credentials, can't plant a git hook, can't push, and a missing tool never counts as a pass.

---

## 14. Risks

| Risk | What we do |
|---|---|
| **The critic doesn't work well enough to be worth the loop.** Everything rests on a model catching fabricated facts from a diff. | Spike S1 measures it before anything is built on it. `bench critic` keeps measuring. If no-tools models don't reach the bar, the critic gets read-only tools in its own isolation, with the same quote rule (4.3). |
| **Tests from before the task can't gate new features,** so a new form is checked only by the critic | A new test counts once it has failed on the old code and passed on the new (4.2). That's the one rule that makes a builder's own tests trustworthy. |
| **The agent CLI can't run headless in a container.** Login flows, telemetry prompts and first-run wizards all assume a person. | Spike S2 tries it first. The verifier doesn't depend on it, and manual mode keeps working whatever the answer. |
| **The plugin file format fits the tools imagined rather than the ones used** | Slots are extracted from real implementations, and the file format is designed from those, not up front (12) |
| The critic misses soft claims | Spot-checks, and every miss becomes a fixture and a lesson |
| The builder writes text that steers the critic, such as a comment saying "this matches the spec" | The output is passed to the critic as data, never as instructions. The quote check means a claim still needs a real spec line. Fixtures plant this. |
| Too many false fails | Behaviour the spec doesn't mention goes to the choice log, not a fail. Lessons that keep getting overruled are retired. |
| An agent CLI changes | Agents are plugin files, fixable without a RaisQ release. `raisq plugin test` runs against recorded output in CI. |
| A tool changes its output | Pinned versions, metadata from the official docs, and recorded samples. An unknown format fails loudly. |
| Isolation is heavy on macOS and Windows | Any isolation tool can be a plugin. `doctor` says exactly what to install. |
| Some agents can't log in inside isolation | Tested per agent. Manual mode still works. |
| A user-written plugin misbehaves | It only fills its slot, runs isolated when it runs code, passes the plugin test, and needs approval. It can't change a verdict. |
| The core slowly starts depending on a tool | The import lint rule fails the build on any outward dependency |
| Thin specs make reviews long | The review pack counts the choices and suggests adding the important ones to the spec |

---

## 15. Open questions

| # | Question | Default until decided |
|---|---|---|
| Q1 | Which two agent CLIs get plugins first? | Decided by spike S2: whichever two run headless in a container |
| Q2 | Which isolation tool gets the first plugin? | A container tool, because it works on all three systems |
| Q3 | Is requiring an isolation tool for a first `raisq run` too high a bar? | Keep it. Manual mode and `isolation: none` are the ways around it. |
| Q4 | Should `raisq spec` be renamed, since both kinds of spec are specs? | Closed: it's `raisq fact`, and the folder is `facts/`. Only a task's acceptance criteria are called a spec. |
| Q5 | The default values in 7.2 | As listed, to be tuned with the first users |
| Q6 | May report plugins post gap questions to the stakeholder, for example on a Jira issue? | Off by default, and switched on per plugin |
| Q7 | May report plugins open draft PRs during a batch? | Yes. A draft isn't shipping. |
| Q8 | Should the agent's login go inside isolation at all, or should a proxy on the host hold it? | Inside (5.2). A host-side proxy is the cleaner answer, and can come later without changing anything else. |
| Q9 | Is `raisq run --check` the right name for the manual-mode step, or should it be its own command such as `raisq check`? | `--check` for now. Decide from how it feels to type. |

---

## Appendix A. Vocabulary

Every term RaisQ uses, by domain. One word per concept (D13): if two words here mean the same thing, that's a bug in the plan. Terms in *italics* are the values a field can take.

### People

| Term | Meaning |
|---|---|
| Stakeholder | Whoever the work is for. Confirms business facts. |
| Implementor | The person running RaisQ. Answers questions, approves, reviews. |
| User | Either of the above, when it doesn't matter which |

### Work

| Term | Meaning |
|---|---|
| Task | One piece of work with its own spec, state and runs, in one folder, on its own branch |
| Task state | Where a task is. Always exactly one of: *queued* (waiting its turn) · *running* (in a run now) · *ready* (passed, waiting for review) · *parked* (stopped, waiting for a person) · *done* (marked done at review) |
| Queue | The tasks in state *queued*, in the order they were added |
| Run | One task through the loop, against one frozen spec. `raisq run` is one run; a batch is many. |
| Frozen spec | The revision of the facts and task spec a run is judged against, pinned when the run starts. Nothing is copied: the run records the revision. The run's requirement. |
| Revision | A numbered version of the facts or a task spec. Every change makes a new one; old ones stay in history. |
| Attempt | One pass of build, check and critic, in a fresh session. A task gets a limited number. |
| Attempt limit | How many attempts a task gets before it's parked. A default, per project. |
| Session | One start-to-exit of an agent or model process, with an empty context. Every attempt and every critic call is a new one. |
| Handoff | What the next attempt is told: the exact failures, and what was tried |
| Parked reason | Why a task is parked. One of: *out of attempts* · *question* (the builder asked one) · *needs something* (isolation keeps it out) · *stuck* (two identical handoffs). A spec change doesn't park: the task goes straight back to the queue. |
| Batch | Running every queued task, one after another, with nobody watching |
| Watched run | A run with a person present, so questions are asked right away instead of parking |
| Manual mode | You make the change yourself, in a chat or by hand, and RaisQ verifies it. The verifier alone, and the first thing that ships. |
| Part | One piece of a split task, run on the same branch as the others, in order |
| Integration check | The loop run once more over a whole split task, after its parts |

### Specs

| Term | Meaning |
|---|---|
| Spec | Everything written down about what's correct: the facts plus the task's spec. What a run is judged against. |
| Facts | What's true about the project across tasks, one file per topic, in `facts/`. Both facts and rules live here. |
| Task spec | One task's acceptance criteria, in `tasks/<name>/spec.yaml`. Dies with its task. |
| Item | One entry in `facts/` or a task spec, with an id, text, a kind, a tag, who said it and when |
| Kind | What sort of fact an item is, from the preset's list: contact, hours, price, terms, rule. Sets its recheck period. |
| Acceptance criterion | A task spec item: one thing the output must show |
| Fact | An item in `facts/` that's true about the world: a price, a phone number, a delivery radius. Expires. |
| Rule | An item in `facts/` that's a constraint, not a world fact: "orders need a phone number", "small commits", "don't refactor what the task didn't ask for". Never expires. |
| Tag | How we know an item. One of: *confirmed* (a person said so) · *inferred* (an AI's guess, or a person's unchecked draft) · *gap* (known to be unknown) |
| Confirmation | A person marking an item *confirmed*. Only a person can. |
| Recheck date | When a confirmed fact needs confirming again. Comes from its kind, via the preset; an item may override it. |
| Expired | A confirmed fact past its recheck date. Counts as inferred until reconfirmed. |
| History | Earlier revisions of changed items. Kept so a run can read its pinned revision and a review pack can quote it. No AI reads history to learn from it. |
| Contradiction | Two items that can't both be true. Always asked about, never settled by RaisQ. |
| Spec source | The slot that supplies items: RaisQ's own writer, a spec tool, or an issue tracker |
| Spec converter | The step that turns a spec tool's files into items, checking every quote both ways |
| Outside text | Text that came from an issue tracker or a spec tool, marked so because it may contain instructions aimed at the builder |

### Building

| Term | Meaning |
|---|---|
| Builder | The agent that makes the change. Fills the builder slot. |
| Prompt | What the builder is given: the task spec, the facts index and where the frozen facts are mounted, the `for_ai` instructions, and on a retry the handoff. Saved with the run. |
| Facts index | One line per fact, grouped by topic, written into the frozen spec. The builder reads it in the prompt and opens the topic it needs, the way an agent opens a skill. |
| Result | What the builder reports in `result.json`. One of: *done* · *question* · *needs something* · *failed*. No file means *failed*. |
| Output | What the builder produced: the committed diff, and whatever the extractor reads from it, such as page text |
| Choice log | The builder's decisions the spec didn't cover |
| Choice | One entry in the choice log. Goes to the review pack, never to a verdict. |

### Checking

| Term | Meaning |
|---|---|
| Check | A plugin in the checks slot. Gives a result without an AI. Most are a command whose exit code decides. |
| Check result | What a check plugin returns: *pass* · *fail* · a number, plus text for the handoff. How a number becomes pass or fail is the plugin's job. |
| Old failures | Checks that already failed when RaisQ was set up. Only new failures count. |
| New failure | A check that passed before the task and fails after it. The only kind that fails an attempt. |
| Proven test | A test the builder added that fails on the code from before the task and passes after it. Only proven tests count. One that passes on both is reported as proving nothing. |
| Flaky | A check that gives different results on the same output. Reported, not sent back to the builder. |
| Blocked | A check whose tool is missing. Blocks the task, never passes it. |
| Fixture | A test project with planted mistakes and the verdicts they should get. RaisQ's own tests run on these. |
| Planted mistake | A deliberate error in a fixture that the loop must catch |

### Judging

| Term | Meaning |
|---|---|
| Critic | The separate AI that judges the output. A different model from the builder, read-only, and it must quote. In v0 it has no tools at all. Fills the model slot. |
| Extractor | The slot that turns the output into text the critic can read: a diff, page text, PDF text |
| Claim | One fact in the output that a user would see. The critic lists these first. |
| Provenance | For each claim: which spec item says this? *agrees* · *disagrees* · *not in spec* |
| Coverage | For each acceptance criterion: where does the output show it? A quote, or *missing* |
| Rubric | One standing question the critic asks on every task, with the answer that fails it. In `rubrics/`, from the preset or a lesson. Answers *yes* · *no* · *unsure*, with a quote, or *n/a* when the output has nothing it could look at. |
| Quote | The exact text the critic points at, in the output or the spec. Required with every answer. |
| Quote check | RaisQ confirming a quote really exists where the critic says. A quote that doesn't is asked again once, then counts as unsure. |
| Verdict | The result for one claim, criterion or rubric. One of: *pass* · *fail* · *unsure* (a person must settle it; read first in the review pack). Decided by RaisQ's rules from the critic's quoted answers. |
| Verdict rules | The fixed table that turns the critic's answers into verdicts ([4.3](#43-the-critic-and-the-verdict)). In the domain, so no plugin can change it. |
| Fabrication | A claim with no source in the spec. Always a fail. |

### Reviewing

| Term | Meaning |
|---|---|
| Review pack | What the implementor reads for one task: unsure items, choices, changes, evidence, and the decision |
| Evidence | The quotes and check results a verdict rests on, linked from the review pack |
| Decision | What the implementor does at review. One of: *mark as done* · *send back* (with a lesson) · *change the spec* |
| Batch report | What a batch ends with: what's ready, what's parked and why |
| Report | The slot that delivers review packs and batch reports: files, a draft PR, a comment, a message |
| Draft | What a report plugin may create outside: a draft PR, a comment, a note. Never a merge, deploy or release. |

### Learning

| Term | Meaning |
|---|---|
| Lesson | A check or rubric that exists because a mistake got through. Not a separate kind of file: a plugin file or a rubric with a `from:` line. Says how to check, never what's true. |
| Spot-check | A person re-reading a sample of shipped work, to find what the loop missed |
| Overruled | A lesson's fail that a person marked wrong at review. Lessons that keep being overruled are flagged to retire. |
| Retired | A lesson no longer run. Its file stays, marked `retired: true`, so it isn't re-learned. |

### Safety

| Term | Meaning |
|---|---|
| Isolation | What keeps each attempt away from the user's files and credentials. Filled by a plugin such as Docker. |
| Task checkout | The copy of the project each task works in, on the task's branch, with no remotes |
| Agent login | The one secret an agent CLI needs to run, named by its plugin and brought in alone |
| Locked file | A file RaisQ judges with: specs, checks, rubrics, plugins, tests and check config. The builder can touch it, but RaisQ ignores the change and fails the attempt. |
| Audit | RaisQ's check after each attempt that no locked file, the checkout's `.git`, or anything read-only changed |
| Breach | An audit failure. Fails the attempt; in a batch, stops the batch. |
| Before the task | The state of the branch when the run started. Tests and check config are restored from it before checks run. |

### Parts · the architecture

| Term | Meaning |
|---|---|
| Ring | One layer of RaisQ's code: *domain* · *use cases and slots* · *plugins*. Dependencies point inwards only. |
| Core | Rings 1 and 2 together. Depends on nothing outside itself. |
| Domain | Ring 1: the rules. The types in this vocabulary, and what RaisQ decides from them. |
| Use case | One flow in ring 2: init, add task, run attempt, run batch, review, learn. The steps, in order, that no plugin can change. |
| Slot | An interface the core defines for something outside it. A plugin fills it. |
| Contract | What a slot takes and must return, as a JSON Schema plus recorded examples |
| Plugin | What fills a slot. Three ways: a plugin file, a program, or Go code built in. |
| Provider | A plugin in the provides slot: supplies something by name when asked. Starts the site or a database, injects a test secret for the checks after the builder has exited, or mounts a read-only reference for the builder. The only way anything enters isolation. |
| Plugin file | A YAML file in `.raisq/plugins/` describing an installed tool |
| Program plugin | A plugin file whose `run:` points at a program that speaks the plugin protocol |
| Plugin protocol | JSON in on stdin, JSON out on stdout, versioned as `raisq-plugin/v1` |
| Built-in plugin | A plugin shipped inside RaisQ, as text or Go code |
| Fake | A plugin for a slot that does nothing real, so the core can be tested with no tools |
| Tool | A program the user installed. RaisQ starts it, never installs it. |
| Agent | An AI that works with tools. Fills the builder slot. In no-tools mode, where it can only answer and not act, it can fill the model slot too. |
| Model | An AI that answers one question with no tools. Fills the model slot. |
| Preset | A bundle of plugin files, rubrics and defaults for one kind of project. Files only, no code, so a project can use several. |
| `for_ai` | The part of a plugin file that tells the builder how to use the tool: when, how, never |
| Dependency rule | Code depends only on rings further in: plugins on slots, slots and use cases on the domain, never the other way |

---

## Appendix B. Decisions

| # | Decision | Why |
|---|---|---|
| D1 | Design concepts C1 to C8 decide every trade-off, in that order | So the design stays consistent as it grows |
| D2 | Five main commands. Everything else is occasional or automatic. | Easy to use (C2) |
| D3 | One settings file. Everything else is written by RaisQ or a default. | Easy to use, encapsulated (C2, C4) |
| D4 | Safety by construction: isolation, locks, tests from before the task, the independent critic | Safe without configuration (C1) |
| D5 | Isolation limits what can be lost, instead of listing what's allowed | Nobody can predict every need a task will have (C1, C5) |
| D6 | Clean architecture: domain, use cases with their slots, then plugins, with dependencies pointing inwards only | Any outside part can be swapped without touching the rules, the rules can't be bypassed, and the core is testable with no tools (C1, C6, C8) |
| D6a | Slots live in the use-case ring, not a ring of their own | A use case must own the interface it calls, or it would depend outwards |
| D6b | Most plugins are files describing installed tools, read by the built-in `command` plugin | A new tool needs a text file, not new code (C7, C8) |
| D6c | Users can add a plugin for any slot, as a file, a program in any language, or Go code. AI can draft one from the slot's contract, test it, and ask for approval. | Plug-in approach that doesn't wait for RaisQ releases (C6) |
| D7 | No AI tool, vendor or model built in, and no AI API required | Plug-in approach (C6) |
| D8 | Go, one file, GitHub Releases at first | Nothing to install for RaisQ itself (C2, C7) |
| D9 | A fresh session for every attempt, with a handoff file | Context doesn't grow, and failed approaches don't carry over |
| D10 | Tool metadata from official docs, with recorded output samples | Maintainable (C8) |
| D11 | Behaviour the spec doesn't mention goes to the choice log, not a fail | One workflow fits both thin and detailed specs |
| D12 | "Task checkout", not "workspace". "Spec converter", not "adapter". | Workspace means monorepo packages in many projects. The converter is one step for every format. |
| D13 | One word per concept: slot, plugin, tool, preset. No "port", "adapter" or "room". | Easy to understand (C3). Clean architecture doesn't need its own words to apply its idea. |
| D14 | External tools such as GitHub, Jira and Slack are plugins in the Report and Spec source slots, not a separate feature | The core already has those slots. Local files stay the record, and an unreachable service never blocks a run. |
| D15 | "RaisQ never ships" means no merge, deploy or release. Drafts and notes from report plugins are allowed. | A draft PR or a comment is a report, and a person still decides |
| D16 | A lock is enforced by ignoring and failing, not by preventing | Preventing needs a sandbox inside the sandbox. Restoring the judged files from before the task gives the same guarantee with none of the machinery (C1, C4). |
| D17 | Presets are bundles of files, never code. Logic lives in built-in plugins. | A preset stays something a project can read, edit and delete file by file (C3, C8) |
| D18 | The critic must be a different model from the builder, and there is no way to turn it off | Same model means shared blind spots; off means a pass nobody earned (C1) |
| D19 | The agent's login is the one secret brought into isolation, and it's named by the agent plugin, never the whole home folder | Agents need it to run at all. Naming it keeps everything else out (C1). |
| D20 | Two spikes come before any code: critic catch rate, and a headless agent in a container | Both are assumptions the whole design rests on, and either can be tested in a day |
| D21 | A run pins the revision of the facts and task spec at the start, and every attempt and the critic read that revision. Nothing is copied. | "What is this run for?" must have one answer, and the builder, checks and critic must be judged against the same one (C3). Facts are never overwritten anyway, so a revision number does what a copy would, without a copy per run. |
| D22 | The verifier ships first, in manual mode, before the automated builder | It's the part that's new. Orchestrating an agent is plumbing, and a verifier that's wrong makes the plumbing worthless. |
| D23 | A new test counts only once it has failed on the code from before the task and passed on the new | Tests from before the task can't gate a new feature, and a builder's test that passes on both proves nothing (C1) |
| D24 | Two questions, named apart: did it do what was asked, and is what it says true. Project facts serve only the second. | A refactor has no claims, and shouldn't carry the tag and expiry machinery (C3, C5) |
| D25 | The critic's principle is read-only and must quote. No-tools is how v0 gets that, not the rule. | A read-only critic with tools keeps both constraints and is the fallback if no-tools models fall short |
| D26 | Slots are extracted from real implementations, and the plugin file format is designed from those | A file format designed from imagined tools fits imagined tools (C8) |
| D27 | A check is a plugin in the checks slot, not a line in a checks file. The slot returns pass, fail or a number. Everything a tool needs lives in its plugin. | A checks file with fields for scores, thresholds and "needs a site" is the core learning each tool's shape, which is what C6 and C8 forbid |
| D28 | `raisq.yaml` holds one-of slot choices and nothing else. No setup step, no start command, no `bring_in`. | A project command in settings is the core learning the project's shape. The builder already knows how to install; a start command is a fact about the framework (C4, C6). A setting that opens isolation is a setting someone must get right (C1). |
| D29 | Anything that enters isolation is a provider: a plugin in the provides slot, asked for by name. Test secrets enter for the checks only, after the builder has exited. Read-only references for the builder are mounted, audited, and may hold nothing that can be lost. | One mechanism, one slot, one approval path. The checks need the secret; the builder could have sent it anywhere (C1, C6). |
| D30 | Every proposed setting, file or step is checked against C1 to C8 **before** it's written, not after someone objects | `checks.yaml`, `setup:`, `start:` and `bring_in` all came from habit (CI configs, Compose files) and each failed the concepts on inspection. The concepts are the thing to reach for first. |
| D31 | Cost is saved where it's spent: attempts first, then the critic. One attempt reports every failure; questions are settled before the run; handoffs carry only the failing lines; prompts put the stable parts first; the critic reads the output once, in two calls; extractors return only what the task changed, measured on the output; settled provenance is reused on a retry; baselines are measured once per base commit. | A builder attempt costs ten to a hundred times a critic call, so a saved attempt beats any prompt trim. None of these is a setting (C5) and none skips a check or a question (C1). |
| D32 | Found while building: a revision is a content hash of `facts/` (or the task spec) at run start, kept under `history/`, so a hand edit gets a new revision whether or not it was committed. Manual mode is one pass: nothing changes between attempts, so a fail goes back to the person. Each attempt gets a fresh checkout path, because a container tool can hold on to a deleted one. The critic's data block uses a fresh random tag per call, so the builder's text can't close it. | Each came from running the code end to end, and each keeps a rule the plan already had (D21, D22, 5.2, 4.3). |
| D33 | Found while finishing the build: the agent's login is copied into the container as one read-only folder and placed in the user's home there, never mounted read-write, so a hook the builder writes into it can't reach the host. Every container command runs as an unprivileged user with the host's uid, because agent CLIs refuse to skip their prompts as root. The agent plugin says where its login lives (a file, or a command such as the macOS Keychain's) and which of its fields go in; nothing else in the login store does, so other tools' tokens and the refresh token stay on the host. A login about to expire is renewed first. A check or extractor that compares with the base commit (`page-text`, `go-api`) snapshots it in the baseline, once per base. A watched question is settled into the spec the same way an answer at `raisq task` is, and doesn't use up an attempt. A task may break a library's public API only if its spec says `changes_api: true`. | Each kept a rule the plan already had (5.2, 4.5, 4.7, 8.1) once the code met a real container, a real agent CLI and a real site. |

Detail that doesn't change the plan, such as every field of every file, each slot's full contract and the threat model, goes into design docs next to the code, checked against the concepts in section 1.
