# OpenAI Images Passthrough Design

## Goal

Allow Codex ImageGen requests to reach OpenAI-compatible upstream image endpoints without being rewritten to `/v1/responses`, and ship the fix as ccNexus 5.3.1 for macOS arm64.

## Root cause

Codex sends image generation and editing directly to `/v1/images/generations` and `/v1/images/edits`. ccNexus currently classifies every unrecognized path as Claude format. An `openai2` endpoint then converts that request to the Responses route, so CPA rejects `gpt-image-2` on `/v1/responses`.

## Design

- Add an OpenAI Images client format covering generation and edit paths, with and without the `/v1` prefix.
- Reuse the existing OpenAI Responses passthrough transformer for `openai` and `openai2` endpoints; it already leaves JSON requests and responses unchanged.
- Preserve the original Images API path before text transformer route selection.
- Keep the existing endpoint authentication, rotation, retry, model override, statistics, and upstream error handling unchanged.
- Reject Claude and Gemini endpoints through the existing unsupported-transformer path instead of inventing an image conversion.

## Version and artifact

- Set the Wails product version and macOS bundle versions to `5.3.1`.
- Build the existing Vue frontend and a local unsigned/ad-hoc macOS arm64 Wails application.
- Deliver the `.app` and a zip archive without touching `/Applications/ccNexus.app`.

## Verification

- Add a table-driven regression test for both generation and edit paths.
- Prove RED before implementation and GREEN after implementation.
- Run focused proxy tests, `go test ./... -count=1`, the frontend production build, and the Wails macOS arm64 build.
- Inspect the built bundle architecture, `Info.plist` version fields, and code-signing status.
