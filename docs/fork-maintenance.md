# Jai's Google Chat fork

Source: [jai/gogcli](https://github.com/jai/gogcli). Upstream: [openclaw/gogcli](https://github.com/openclaw/gogcli).

`main` remains pinned to upstream until a validated sync PR. Its active fork ruleset requires signed commits, a PR with resolved threads and the strict checks listed below, in addition to deletion/non-fast-forward protection. No human approvals are required. Development and dogfooding use `jai/chat-rich-messages`, with a personal fork PR kept open. No upstream PR is authorized yet; propose it after dogfooding.

## Current evidence

Use the personal fork PR and its exact-head check runs for current validation evidence. The local provenance manifest records the active binary; the parent thread and Brain project page record activation and daily automation state. This document defines the workflow and does not certify that a particular head has passed its gates.

## Patch contract

Existing compact JSON and plain output stay compatible. Opt-in `--full` exposes the fields recognized by the pinned Google SDK, including formatted hyperlinks, attachment/image metadata and CardsV2. It is not raw or lossless for unknown future API fields. Native message get returns full JSON; message update/delete follow Google permissions and the CLI's existing mutation safeguards.

Images in this patch are metadata and references, not downloaded bytes or rendered cards. Do not execute card actions or treat a thumbnail URL as a media-download API.

## Gates before installation

Require independent AI review, a clean committed and pushed dogfood branch, local `make ci`, and green hosted checks at the exact branch head. Run the local gate with `env -u GOG_ACCOUNT mise exec go@1.27.1 -- make ci`; inherited account defaults affect existing no-account tests. Inherit upstream CI YAML unchanged: Linux `test`, `minimum-go`, `worker`, `windows`, `darwin-cgo-build`, and Docker `image`. These are strict required checks on fork `main` under ruleset 25208668. The fork's open PR must trigger these checks; confirm their actual names and head SHA in GitHub before installation. A newly created fork may not register workflows until its first PR event; an empty workflow registry is not CI success.

Keep redacted Workspace evidence for hyperlinks, image metadata and CardsV2 reads. Write tests use synthetic messages only: create, get, update, verify, delete, then verify a deleted tombstone or not-found response. An ambiguous provider 403 requires a successful scoped listing that proves the run-owned target is absent. Never include tokens, customer messages or private card content in evidence.

CI and Docker workflows are enabled in the fork. Upstream release publishing, Pages deployment, ClawSweeper dispatch and Crabbox hydration are disabled in the fork's Actions settings; their source files remain unchanged. Independent Codex review supplies the review evidence while the upstream reviewer app is absent.

## Install a validated head

From the clean dogfood checkout, run:

```sh
mise exec go@1.27.1 -- scripts/install-fork-build.sh
```

This uses the repository Makefile and stamps the version with `jai-chat` and the source SHA. It validates `--version` before atomically switching `~/.local/bin/gog` to `~/.local/share/gogcli-fork/builds/<fullsha>/gog`. A text provenance manifest accompanies each build. Existing builds are reused only after provenance and executable validation. Build/validation failures leave the active binary unchanged.

Homebrew's original `/opt/homebrew/Cellar/gogcli/0.43.0/bin/gog` is never overwritten. Unknown local executables or symlink targets are preserved and reported as an error; inspect them before proceeding. The installer does not run review or CI itself. Verify `command -v gog`, `gog --version` and the provenance manifest after installation; `~/.local/bin` must precede Homebrew in PATH.

## Daily upstream maintenance

The thread heartbeat fetches upstream and merges upstream append-only into `jai/chat-rich-messages`; do not rebase or force-push dogfood history, and do not push `main` directly. Advance the `main` mirror only through a separately validated sync PR. Resolve conflicts within the patch scope, rerun independent review and `make ci`, push, wait for the exact-head hosted gates, and install only a green head. If validation fails, keep the last installed build active and report the failing command/check with evidence. No change means no replacement build is needed.

Keep the personal PR open while dogfooding. Preserve the same capability and upstream defaults as releases evolve. Record actual validation, live proof and installation evidence in the parent task; this file must not turn intended checks into completion claims.
