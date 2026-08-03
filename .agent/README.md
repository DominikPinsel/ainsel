# .agent — AI Agent Skills

Reusable skill definitions for AI agents working in this repository.
Skills are step-by-step playbooks that agents should follow for common tasks.

## Structure

```
.agent/
├── README.md                        ← This file
└── skills/
    ├── pr-checklist.md              ← Full PR lifecycle (all components)
    ├── bug-fix.md                   ← How to approach and fix a bug
    └── component-reference.md       ← Verification commands per component
```

## Usage

When an agent is asked to perform a task that matches a skill, it should:

1. Read the relevant skill file(s)
2. Follow the checklist steps in order
3. Verify each step before moving to the next
4. Report which steps were completed

## Adding a New Skill

1. Create a new `.md` file in `skills/`
2. Use kebab-case naming: `my-new-skill.md`
3. Include: purpose, when to use, step-by-step checklist, common pitfalls
4. Update this README's structure section
