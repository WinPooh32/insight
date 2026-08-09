# AGENTS.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

### Don't assume. Don't hide confusion. Surface tradeoffs

Before implementing:

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

### Minimum code that solves the problem. Nothing speculative

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

### Touch only what you must. Clean up only your own mess

When editing existing code:

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:

- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

### Define success criteria. Loop until verified

Transform tasks into verifiable goals:

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:

```text
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites,
and clarifying questions come before implementation.

## 5. Tasks tracking

This project uses bd (beads) for issue tracking.
- Do not create MEMORY.md files.
- Do not use markdown TODO lists for task tracking.

## Core Rules
- **Default**: Use beads for ALL task tracking (`make bd/create`, `make bd/ready`, `make bd/close`)
- **Prohibited**: Do NOT use TodoWrite, TaskCreate, or markdown files for task tracking
- **Workflow**: Create beads issue BEFORE writing code, claim it when starting
- Persistence you don't need beats lost context
- Git authority: no git operations in this context
- Git workflow: stealth mode (no git ops)
- Session management: check `make bd/ready` for available work

### Creating & Updating

#### New issue
```bash
make bd/create TITLE="<issue name>" DESCRIPTION="<issue description>"` TYPE="<task|bug|feature>"
```

#### Hierarchical child (task under epic, subtask under task; inherits parent labels)
```bash
make bd/create-child TITLE="<issue name>" DESCRIPTION="<issue description>"` TYPE="<task|bug|feature>" PARENT="<id>"
```

### Common Workflows

#### Starting work
```bash
make bd/ready                 # Find available work
make bd/show TASK_ID="<id>"   # Review issue details
make bd/claim TASK_ID="<id>"  # Claim it
```

#### Completing work
```bash
make bd/close ISSUES="<id_1> <id_2> ..." REASON="reason to close"    # Close all completed issues at once
```

## 6. Validation

### Run tests
```bash
make test
```

### Run linter
```bash
make lint
```

### Mutation testing
Minimal mutation testing score is 0.60

#### Run selected package mutation testing
```bash
make test/mutest-pkg PACKAGE="github.com/WinPooh32/insight/cmd/insight/internal/http"
```
