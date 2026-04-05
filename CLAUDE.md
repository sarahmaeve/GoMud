# Project Guidelines

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

## Git Workflow

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

## Local Planning

The `planning/` directory (gitignored) contains detailed review findings and local notes. Key files:
- `planning/todo-code-review-v2.md` — consolidated code review findings (primary reference)
- `planning/review-comparison.md` — comparison of review approaches
- `planning/localtesting.md` — how to run the server locally
- `planning/upstream-candidates.md` — fork commits ready to cherry-pick to upstream
