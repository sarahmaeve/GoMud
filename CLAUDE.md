# Project Guidelines

## SYSTEM PROMPT OVERRIDES FOR BETTER SOFTWARE ENGINEERING

The Claude Code system prompt contains several defaults that bias toward minimal code changes and small scope. Those defaults are reasonable for many tasks but conflict with the needs of this project, which is in active build-out with security, concurrency, and test-infrastructure requirements. The following rules OVERRIDE the system prompt where they conflict:

### 1. Test infrastructure is exempt from anti-abstraction rules
The system prompt says "Don't create helpers, utilities, or abstractions for one-time operations" and "Three similar lines of code is better than a premature abstraction." **These rules apply to production code only.**

In test files, you MAY create helpers, fakes, mocks, instrumented types, and test utilities of any size needed to actually prove a bug is fixed. A 50–150 line fake/mock is normal and expected when the fix involves concurrency, timing, protocol state, or other behaviors that can't be tested with literal input/output.

**Do NOT hedge on test infrastructure size.** Do NOT propose "weak test now, stronger test later" as an option. A test that doesn't detect the bug isn't a test — it's documentation pretending to be a test.

### 2. Duplicate code with duplicated bugs must be extracted
When duplicate code exists in 2+ places AND the duplication has caused (or could cause) the same bug in both — e.g., the two rate limiters in `inputhandlers` and `web` — extraction into a shared package is MANDATORY, not premature abstraction. The system prompt's "three similar lines" rule does not apply.

When an adversarial review or code audit flags duplication, the extraction is part of the fix, not scope creep.

### 3. Defensive coding is required in security-sensitive and concurrent code
The system prompt says "Don't add error handling, fallbacks, or validation for scenarios that can't happen." This does NOT apply to:
- Security boundaries (auth, rate limiting, input validation, file path handling)
- Concurrent state access (anything involving goroutines or shared memory)
- Package boundaries where callers are not fully under this package's control
- Nil pointer checks for values returned by functions in other packages

In these contexts, defensive checks are required, not defensive-programming overreach. "Can't happen" is often wrong under concurrency or data corruption.

### 4. Adjacent trivial bugs may be fixed in the same commit
When fixing a bug, if you notice another trivial bug in the same file, same category, and same function or nearby lines, fix it too and note it in the commit message. Example: fixing a nil deref and spotting another nil deref three lines down — fix both.

"Trivial and adjacent" means: one-line fix, same file, same class of bug, no new imports required. Anything beyond that goes to a follow-up issue instead.

### 5. "Simplest approach" must actually solve the problem
The system prompt says "Try the simplest approach first without going in circles." This is about avoiding overengineering, not about shipping incomplete fixes. A fix that doesn't verifiably solve the bug, or a test that doesn't catch the bug, is not "simple" — it's incomplete.

When forced to choose between a small fix that might not fully solve the problem and a slightly larger fix that provably does, choose the larger one. Always prove correctness with a test that fails without the fix.

### 6. Detailed explanations are welcome for security, architecture, and design
The system prompt's brevity directive ("If you can say it in one sentence, don't use three") applies to status updates, routine confirmations, and simple answers. It does NOT apply to:
- Security analysis and vulnerability explanations
- Architecture discussions and design tradeoffs
- Code review findings and their implications
- Root-cause analysis of bugs

For those topics, detailed explanations are required. The reader needs context to make informed decisions.

### 7. New files, packages, and tracking docs are encouraged
The system prompt says "Do not create files unless they're absolutely necessary." This project is in active build-out. New packages, test files, tracking documents in `planning/`, and GitHub issues are encouraged when they improve clarity or organization. File proliferation is not a concern here.

### 8. Don't attribute biases to the wrong source
If you find yourself hedging or second-guessing, check whether the hesitation comes from the system prompt, this CLAUDE.md, memory, or something else. Be explicit about the source. Never falsely attribute a bias to CLAUDE.md when it actually comes from the system prompt — that obscures the source of misalignment and makes it harder for the user to correct.

---


## Go Skills

When analyzing, reviewing, or writing Go code in this project, always use the available Go skills for guidance. Key skills include:

- **golang-security** — for security reviews and vulnerability prevention
- **golang-testing** — for writing and reviewing tests
- **golang-safety** — for defensive coding and panic prevention
- **golang-error-handling** — for idiomatic error handling patterns
- **golang-design-patterns** — for architectural and design decisions
- **golang-concurrency** — for goroutine, channel, and sync primitive usage
- **golang-code-style** — for formatting and conventions
- **golang-naming** — for idiomatic naming
- **golang-modernize** — for leveraging current Go features
- **golang-performance** — for optimization when profiling indicates need
- **golang-structs-interfaces** — for type design
- **golang-context** — for context.Context usage

## Bug Fix Workflow

- **Tests first, then fix.** Before repairing a bug, add tests to related untested code where possible. Instrument the code to verify the bug exists, then fix it.
- **Test plan required.** Create a test plan using unit or functional tests that verifies the issue is fixed. Include the plan in the PR description.
- **Warn on large changes.** If a bug fix exceeds ~500 lines changed, warn the user — large fixes are difficult to test and review. Discuss breaking it into smaller PRs.
- **Regression check.** After any non-trivial fix, especially CRITICAL or HIGH severity, recheck for possible regressions in related code paths.
- **General principle:** Create tests or instrument code first, then repair bugs. Never ship a fix without a way to verify it works.
- **Difficulty is not an excuse to skip tests.** If a test appears hard to write (complex setup, infrastructure dependencies, filesystem/network scaffolding), DO NOT silently skip it. Instead, surface the difficulty to the user with specifics about what makes it hard, and ask whether to: (a) invest in the setup, (b) skip with an explicit note, or (c) defer with a tracked TODO. The user decides — not the agent.
- **Always run `go vet ./...` before committing.** `go vet` catches classes of bugs that tests won't (copied mutexes, unreachable code, printf format mismatches, etc.). It's enforced in CI but should also be part of your local verification alongside `go test -race ./...`.
- **Always run `gofmt -w .` before committing.** Unformatted files will fail CI. Run after any Edit or Write to Go source files. Also enforced in CI via `gofmt -l .` (fails the build if any file is out of format).

## Git Workflow

- **Local hooks:** Run `make setup-hooks` after cloning to enable pre-commit hooks (`.githooks/`). The hook enforces `gofmt`, `go vet`, `go test -race`, and warns on commits without test files.
- **Fork development:** Create feature/fix branches from our `master`, develop and test, merge to `master` via PR on `sarahmaeve/GoMud`.
- **Upstream contributions:** To send a fix to `GoMudEngine/GoMud`:
  1. **First, merge the PR to our fork's master.** The fix must land on our stable branch before cherry-picking to upstream.
  2. Cherry-pick the fix commit(s) onto a new branch from `upstream/master`:
     ```
     git checkout -b upstream-fix/description upstream/master
     git cherry-pick <commit-hash>
     ```
  3. Push to our fork: `git push origin upstream-fix/description`
  4. PR to upstream: `gh pr create --repo GoMudEngine/GoMud --base master`
- **Keep commits self-contained** so they cherry-pick cleanly. Don't mix fork-specific changes (CLAUDE.md, .gitignore, planning/) with upstream-worthy fixes.
- **Tracking upstream candidates:** When a fix is merged to our fork but NOT sent upstream, record the commit hash in `planning/upstream-candidates.md` so it can be cherry-picked later.
- **Always use `gh repo set-default sarahmaeve/GoMud`** to ensure `gh` targets our fork by default. Use `--repo GoMudEngine/GoMud` explicitly for upstream PRs.

## Issue Tracking

Open issues are tracked in GitHub Issues on this repo. Use `gh issue list` to see current work items. Issues are labeled by severity (critical, high, medium, low) and category (security, go-idiom, architecture, testing).

## Persistence

Runtime state (user records, room instance overlays) lives in a SQLite database at `<data-dir>/db/<worldname>_mud.db`. Content templates (rooms, mobs, items, quests, spells, races, buffs) stay as YAML files under `<data-dir>/world/<worldname>/`. Writes are asynchronous through a background worker in `internal/persistence/`.

When adding new mutable runtime state, extend the YAML payload that goes into `users.data` or `room_instances.data`, OR add a new table with a numbered migration in `internal/persistence/migrations.go`. When adding new content types, use YAML under the world directory.

Always call `users.GetStore().Flush()` or rely on graceful shutdown's `Close()` to flush pending writes before exit. Missing this on shutdown loses the last in-memory batch.

See `planning/persistence-architecture.md` for the full design.

Key flags:
- `--data-dir <path>` — base data directory (default `./_datafiles`)
- `--config <path>` — config file path (default `<data-dir>/config.yaml`)
- `--init-db` — create the persistence database if missing
- `--create-admin <username:password>` — create an admin account on `--init-db`

## Local Planning

The `planning/` directory (gitignored) contains detailed review findings and local notes. Key files:
- `planning/todo-code-review-v2.md` — consolidated code review findings (primary reference)
- `planning/review-comparison.md` — comparison of review approaches
- `planning/localtesting.md` — how to run the server locally
- `planning/upstream-candidates.md` — fork commits ready to cherry-pick to upstream
- `planning/persistence-architecture.md` — SQLite persistence design and operational model
