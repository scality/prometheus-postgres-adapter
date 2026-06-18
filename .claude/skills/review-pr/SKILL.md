---
name: review-pr
description: Review a PR on prometheus-postgres-adapter (a Prometheus remote-write/read adapter that stores time series in PostgreSQL for long-term retention and serves them to Prometheus remote-read and to Thanos via a native gRPC StoreAPI)
argument-hint: <pr-number-or-url>
disable-model-invocation: true
allowed-tools: Read, Bash(gh repo view *), Bash(gh pr view *), Bash(gh pr diff *), Bash(gh pr comment *), Bash(gh api *), Bash(git diff *), Bash(git log *), Bash(git show *)
---

# Review GitHub PR

You are an expert code reviewer. Review this PR: $ARGUMENTS

## Determine PR target

Parse `$ARGUMENTS` to extract the repo and PR number:

- If arguments contain `REPO:` and `PR_NUMBER:` (CI mode), use those values directly.
- If the argument is a GitHub URL (starts with `https://github.com/`), extract `owner/repo` and the PR number from it.
- If the argument is just a number, use the current repo from `gh repo view --json nameWithOwner -q .nameWithOwner`.

## Output mode

- **CI mode** (arguments contain `REPO:` and `PR_NUMBER:`): post inline comments and summary to GitHub.
- **Local mode** (all other cases): output the review as text directly. Do NOT post anything to GitHub.

## Steps

1. **Fetch PR details:**

```bash
gh pr view <number> --repo <owner/repo> --json title,body,headRefOid,author,files
gh pr diff <number> --repo <owner/repo>
```

2. **Read changed files** to understand the full context around each change (not just the diff hunks).

3. **Analyze the changes** against these criteria:

| Area | What to check |
|------|---------------|
| Error handling | Uses `github.com/scality/go-errors` (imported unaliased as `errors`). New failure categories are package-level `ErrXxx` sentinels; failure sites wrap with `errors.Wrap(ErrXxx, errors.WithProperty(...), errors.CausedBy(rawErr))` so `errors.Is` keeps matching the category through the chain. No `fmt.Errorf`/`%w`. |
| Logging | `log/slog` only. Use the `*Context` methods (`InfoContext`, `ErrorContext`, …) wherever a `context.Context` is in scope; structured key/value attrs (`slog.String`, `slog.Any`, …), never `fmt`-style logging. |
| Context propagation | `context.Context` threaded through call chains and cancellation respected — the ingestion goroutines and the gRPC/HTTP handlers must exit on `ctx` cancel rather than blocking forever. |
| Goroutine lifecycle | The ingestion goroutines (parser × `METRIC_PARSER_COUNT`, saver × `METRIC_WRITER_COUNT`) keep clear exit conditions; `ErrorChan` ownership stays with the saver, which closes it on context cancellation. No leaked goroutines or double-close. |
| Concurrency & single-replica invariant | metric-id allocation stays guarded (a `sync.Map` plus a mutex-protected, double-checked counter). The shared `labelRows`/`valuesRows` buffers have no lock of their own, so the safe defaults are one parser and one writer — flag anything that raises parser/writer concurrency or shares more mutable state without synchronization, and any assumption that more than one replica can run. |
| Layered architecture | Inward-only dependency rule `cmd → infrastructure → presentation → usecase → domain`. `domain` imports no other layer; a use case depends only on the small port interfaces it declares itself, never on a concrete adapter. Flag imports that point outward or skip a layer. |
| SQL safety (querybuilder) | `BuildSQLQuery` composes SQL by string interpolation, so it must only ever receive trusted matcher input from the remote-read / StoreAPI decoders. Verify values stay escaped, that no untrusted/user-controlled path reaches it, and that matcher translation is intact (equality → JSONB containment `@>`, inequality/regex → `metric_labels->>'k'` with anchored `~`/`!~`). |
| Thanos StoreAPI correctness | Series are sorted by label set; labels are built by ranging over the set, NOT via `labelpb.ZLabelsFromPromLabels` (its `unsafe` cast corrupts labels under the default `stringlabels` build); XOR chunks batched at 120 samples; `Info` time-range semantics preserved (empty DB advertises `MaxInt64` min so Thanos skips the store; max is `MaxInt64`); external labels applied to every series with precedence on name collision. |
| Configuration & breaking changes | New/renamed env vars are wired through `cmd/config` and documented; defaults preserved; changes to the `/write` and `/read` wire contracts, the StoreAPI, or the `metric_labels`/`metric_values` schema are backward compatible or explicitly called out. |
| Tests | testify (`assert` / `require`), table-driven with `t.Run` subtests; fakes and mocks are hand-written (no codegen) and live next to the code they exercise. |
| Docs in sync | A behavior/config/deployment change updates `README.md`; an architecture or design-decision change updates `DESIGN.md`; a conventions/workflow change updates `CONTRIBUTING.md`. A change that contradicts the docs must fix them in the same PR. |
| Security | The SQL trust boundary above; no credentials, connection strings, or secrets in code or logs; PostgreSQL connection and SSL-mode handling unchanged unless that is the intent. |

4. **Deliver your review:**

### If CI mode: post to GitHub

#### Part A: Inline file comments

For each issue, post a comment on the exact file and line. Keep comments short (1-3 sentences), end with `— Claude Code`. Use line numbers from the **new version** of the file.

**Without suggestion block** — single-line command, `<br>` for line breaks:
```bash
gh api -X POST -H "Accept: application/vnd.github+json" "repos/<owner/repo>/pulls/<number>/comments" -f body="Issue description.<br><br>— Claude Code" -f path="file" -F line=42 -f side="RIGHT" -f commit_id="<headRefOid>"
```

**With suggestion block** — use a heredoc (`-F body=@-`) so code renders correctly:
```bash
gh api -X POST -H "Accept: application/vnd.github+json" "repos/<owner/repo>/pulls/<number>/comments" -F body=@- -f path="file" -F line=42 -f side="RIGHT" -f commit_id="<headRefOid>" <<'COMMENT_BODY'
Issue description.

```suggestion
first line of suggested code
second line of suggested code
```

— Claude Code
COMMENT_BODY
```

Only suggest when you can show the exact replacement. For architectural or design issues, just describe the problem.

#### Part B: Summary comment

Single-line command, `<br>` for line breaks. No markdown headings — they render as giant bold text. Flat bullet list only:

```bash
gh pr comment <number> --repo <owner/repo> --body "- file:line — issue<br>- file:line — issue<br><br>Review by Claude Code"
```

If no issues: just say "LGTM". End with: `Review by Claude Code`

### If local mode: output the review as text

Do NOT post anything to GitHub. Instead, output the review directly as text.

For each issue found, output:

```
**<file_path>:<line_number>** — <what's wrong and how to fix it>
```

When the fix is a concrete line change, include a fenced code block showing the suggested replacement.

At the end, output a summary section listing all issues. If no issues: just say "LGTM".

End with: `Review by Claude Code`

## What NOT to do

- Do not comment on markdown formatting preferences
- Do not suggest refactors unrelated to the PR's purpose
- Do not praise code — only flag problems or stay silent
- If no issues are found, post only a summary saying "LGTM"
- Do not flag style issues already covered by the project's linter (eslint, biome, pylint, golangci-lint)
