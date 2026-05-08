# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

As of this writing, the repository contains no source code, build configuration, or tests. The only tracked content is `README.md`, which records the project name ("p5-gamemafia" / "P5") and nothing else. There is no package manager manifest, no language runtime declared, and no CI configuration.

The name suggests a [p5.js](https://p5js.org/) project ("p5") implementing a Mafia-style game ("gamemafia"), but this is not yet confirmed by any code in the repo. Do not assume a stack, framework, or directory layout — verify by reading the tree before acting.

## Working in this repo

- Before scaffolding anything, confirm with the user what stack to use (e.g., p5.js in the browser via `index.html` + `sketch.js`, a bundler-based setup with Vite, a Node/TypeScript project, etc.). The directory name alone is not enough to commit to a toolchain.
- Once a stack is chosen and code is added, update this file with: the actual build / lint / test commands, how to run a single test, and the high-level architecture (entry point, game state model, role/role-assignment logic, turn/phase loop, UI rendering). Those sections are intentionally omitted now because there is nothing to describe accurately.
- Keep `README.md` in sync when project structure or run instructions are introduced — right now it is a stub.

## Branching

Per the task instructions for this session, development happens on `claude/add-claude-documentation-X8Pol`. Push to that branch; do not push to `main` without explicit user approval.
