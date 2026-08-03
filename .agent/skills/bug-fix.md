# Skill: Bug Fix

**When to use:** When asked to fix a bug reported in an issue.

---

## Process

### 1. Understand the Bug

- [ ] Read the issue title, body, labels, and comments
- [ ] Identify the **affected component** (frontend, service, operator, chart)
- [ ] Identify the **symptom** (what the user sees) vs. the **root cause** (why it happens)
- [ ] If unclear, check related code: `grep -rn "<keyword>" --include="*.ts*" --include="*.go"`

### 2. Reproduce (if possible)

- [ ] Find the relevant code path
- [ ] Trace the logic from trigger → symptom
- [ ] Identify the exact line(s) causing the issue
- [ ] Check if there's an existing test that should have caught this

### 3. Fix

- [ ] Make the **minimal change** that addresses the root cause
- [ ] Don't refactor surrounding code (separate PR if needed)
- [ ] If the fix changes behavior, **update existing tests** to match
- [ ] **Add a regression test** that fails without the fix and passes with it
- [ ] Verify: lint + test + build pass locally (see `component-reference.md`)

### 4. Commit & PR

- [ ] Commit message: `fix: <short description of what was broken>`
- [ ] Commit body: explain root cause and why this fix is correct
- [ ] PR body:
  ```
  ## Problem
  <what the user experienced>

  ## Root Cause
  <technical explanation of why it happened>

  ## Fix
  <what you changed and why>

  Closes #<issue>
  ```

### 5. Verify CI

- [ ] Push → wait for CI
- [ ] If CI fails: read logs, fix, amend, force-push
- [ ] CI green → request review or wait for auto-review

---

## Common Root Causes in This Repo

| Pattern | Symptom | Fix |
|---------|---------|-----|
| React StrictMode double-effect | Action happens twice on mount | `useRef` guard for one-time side effects |
| `window.open` after `await` | Popup silently blocked | Call `window.open` synchronously in gesture handler |
| Missing error handling in Go | Silent failure / 500 | Check `err`, return proper HTTP status |
| Stale closure in React hook | Handler uses old state | Use functional updates or refs |
| Missing RBAC in operator | Operator can't reconcile | Update `role.yaml` / `clusterrole.yaml` |
| Helm template nil pointer | `helm template` fails | Add `if` guards / `default` values |
| NATS subject mismatch | Events not routed | Check `shared/api` constants match publisher/subscriber |

---

## Anti-Patterns (Don't Do This)

- ❌ Fixing the symptom without understanding the root cause
- ❌ Adding workarounds instead of fixing the actual bug
- ❌ Bundling the fix with unrelated improvements
- ❌ Changing public API behavior without updating docs/tests
- ❌ Suppressing errors (`catch {}`, `_ = err`) instead of handling them
