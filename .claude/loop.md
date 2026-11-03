# Conduit work loop: one iteration

You are improving Conduit, a self-directed project. Do exactly ONE focused unit of work per iteration, then stop.

## 1. Orient (keep it short)
- Run `git switch loop/auto` if you are not already on it. Never work on `main`.
- Read `.claude/loop-log.md` (most recent entries first) so you don't repeat or undo earlier work.
- Run `go build ./... && go vet ./... && go test -race ./...` to see the current state.

## 2. Pick the single most valuable next task
Priority order:
1. **Broken build, vet, or tests.** Fix these first. Generated proto code (`proto/*.pb.go`) is committed. If you edit a `.proto` file, run `make proto` and commit the regenerated files in the same commit.
2. **Correctness and security bugs**, especially at the mTLS, schema-validation, and rate-limiting boundaries described in `README.md` and `docs/security.md`.
3. **Missing tests** for existing logic in `internal/*`, `sdk/go`, and the `cmd/*` handlers.
4. **Gaps between the docs and the code**: features in `README.md` and `docs/*.md` that are missing or stubbed.
5. Small refactors that make the next task easier.

Keep each task small enough to finish in one iteration (roughly one coherent commit). If a task is bigger, do the first slice and record the rest in the log.

## 3. Do it
- Match the existing Go style. Add or extend tests for any behavior you change.
- Don't add heavy dependencies. The project's pitch is "no supply chain surprises," so prefer the standard library and justify any new module in the log.
- Don't touch `certs/` private keys, secrets, or `deployments/` credentials.
- Don't run `docker compose` or anything long-running or networked beyond `go` module downloads.

## 4. Verify, then commit
- Required: `go build ./... && go vet ./... && go test -race ./...` must pass. If the build was already broken before you started and you didn't fully fix it, you may commit only if you made it strictly less broken and didn't break any package that was passing.
- Commit on `loop/auto` with a clear message (`area: what and why`). One commit per iteration.
- Then push the branch: `git push -u origin loop/auto`. If the push fails, for example because the remote has commits you don't have, do not force it. Run `git pull --rebase origin loop/auto`, re-run the checks, and push again. If it still fails, log the error under "Blockers / needs human".
- **Only ever push `loop/auto`. Never push `main` or any other branch. Never force-push. Never rewrite pushed history. Never merge into main or open PRs.**
- If you can't get to a passing state, run `git stash` or `git restore` to drop the work, and log what blocked you. Never push a failing state.

## 5. Log
Prepend an entry to `.claude/loop-log.md` and include it in the commit:
```
## <ISO date/time> — <short title>
- Did: …
- Verified: build/vet/test status
- Next candidates: 1–3 ideas
- Blockers / needs human: … (if any)
```

## Stop conditions
If something needs a human decision (product direction, a new heavy dependency, anything destructive, or credentials), log it under "Blockers / needs human", skip to the next independent task, and mention it in your end-of-iteration summary. If the last three log entries were all blocked, stop the loop.
