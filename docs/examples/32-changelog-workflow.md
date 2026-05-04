# Changelog Workflow

Gather everything you need for a commit message or changelog in a single call.

## Config

```yaml
# .factorly/tools/changelog.yaml

# Individual tools
git.log:
  type: cli
  command: git
  args: ["log", "--oneline", "{{base}}..HEAD"]
  parameters:
    - name: base
      default: "HEAD~15"

git.staged:
  type: cli
  command: git
  args: ["diff", "--cached", "--stat"]

git.staged_diff:
  type: cli
  command: git
  args: ["diff", "--cached"]

git.diff_stat:
  type: cli
  command: git
  args: ["diff", "--stat", "{{base}}..HEAD"]
  parameters:
    - name: base

git.shortlog:
  type: cli
  command: git
  args: ["shortlog", "-sn", "{{base}}..HEAD"]
  parameters:
    - name: base

# Workflows

git.commit_prep:
  type: workflow
  description: Gather staged changes and recent history for writing a commit message
  steps:
    - tool: git.staged
      store: staged_stat

    - tool: git.staged_diff
      store: staged_diff
      require: "staged_stat != ''"

    - tool: git.log
      params: { base: "HEAD~5" }
      store: recent

git.changes:
  type: workflow
  description: Gather git history for a changelog
  parameters:
    - name: since
      description: Base commit or tag
      default: "HEAD~15"
  steps:
    - tool: git.log
      params: { base: "{{since}}" }
      store: commits

    - tool: git.diff_stat
      params: { base: "{{since}}" }
      store: stats

    - tool: git.shortlog
      params: { base: "{{since}}" }
      store: authors
```

## Usage

```bash
# Prepare context for a commit message (staged changes + recent history)
factorly call git.commit_prep

# Gather changelog since a tag
factorly call git.changes --since v0.6.4

# Verbose — see each step
factorly call git.commit_prep -v
```

## `git.commit_prep`

One call gives you everything needed to write a commit message:

1. **`git.staged`** — file stat summary of what's staged
2. **`git.staged_diff`** — full diff (with `require:` — stops if nothing is staged)
3. **`git.log`** — last 5 commits for style context

```
[workflow] git.commit_prep started (run: a1b2c3d4)
[workflow]   1/3 git.staged              completed  4ms
[workflow]   2/3 git.staged_diff         completed  5ms
[workflow]   3/3 git.log                 completed  3ms
[workflow] git.commit_prep completed (3 steps, 12ms)
```

If nothing is staged, the workflow stops at step 2 (`require: "staged_stat != ''"`) — no diff, no log. Clean exit.

## `git.changes`

One call gives you a changelog between any two points:

1. **`git.log`** — commit list
2. **`git.diff_stat`** — files changed with line counts
3. **`git.shortlog`** — contributor summary

Useful for release notes, PR descriptions, or feeding to an LLM for summarization.

## What happens

- Each step runs through the proxy with full oversight and logging
- `store:` captures each step's output as a variable
- The workflow result is the last step's output
- Full state (including all stored variables) is persisted to `.factorly/workflows/<run-id>.json`
- `require:` stops the workflow gracefully if nothing is staged

---

[← Back to Examples](README.md)
