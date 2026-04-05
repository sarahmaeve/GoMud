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

## Issue Tracking

Open issues are tracked in GitHub Issues on this repo. Use `gh issue list` to see current work items. Issues are labeled by severity (critical, high, medium, low) and category (security, go-idiom, architecture, testing).

## Local Planning

The `planning/` directory (gitignored) contains detailed review findings and local notes. Key files:
- `planning/todo-code-review-v2.md` — consolidated code review findings (primary reference)
- `planning/review-comparison.md` — comparison of review approaches
- `planning/localtesting.md` — how to run the server locally
- `planning/todo-remove-goja.md` — future consideration for scripting engine replacement
