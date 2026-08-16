# Claude Code operator hooks

`.claude/settings.json` wires two `type: "http"` hooks to the insight
injection endpoints. The response is `hookSpecificOutput` carrying the
top-3 research links as `additionalContext`. An empty 200 body means
nothing relevant was found.

| Event            | Matcher                               | URL                                                 |
| ---------------- | ------------------------------------- | --------------------------------------------------- |
| UserPromptSubmit | (all prompts)                         | `http://localhost:8765/hooks/v1/user-prompt-submit` |
| PreToolUse       | `Read\|Edit\|Write\|Bash\|Grep\|Glob` | `http://localhost:8765/hooks/v1/pre-tool-use`       |

## PreToolUse matcher (assumption A1)

Per `docs/architecture.md` A1, the matcher covers file-touching tools only;
browser/TodoWrite inputs cannot match research, so they are excluded.

To widen it, edit the one `matcher` line in `.claude/settings.json`
(PreToolUse group) and add tool names, e.g. `"Read|Edit|Write|Bash|Grep|Glob|mcp__browser__.*"`.

## Running the service

```sh
make run/insight   # go run ./cmd/insight, listens on 127.0.0.1:8765
```

Embedding server (OpenAI-compatible) env vars, see `cmd/insight/internal/config`:

- `EMBED_BASE_URL` — default `http://localhost:8080/v1`
- `EMBED_MODEL` — embedding model name
- `EMBED_API_KEY` — empty for local serving

If the embedding server is unreachable, ranking degrades and hooks return an
empty 200 body. If the insight service itself is down, the http hooks fail
harmlessly (no injection); the session is unaffected.

## Activation

Settings are read at session start: hooks registered now fire from the next
Claude Code session onward.
