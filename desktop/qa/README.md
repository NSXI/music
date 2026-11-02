# Rendering regression fixtures

Run `python generate_bidi.py`, serve `bidi/` on localhost port 43926, and open its index. It checks logical text identity, visual order, joining widths, and lyric/translation pairs using the production renderer.

Run `python generate_fullscreen.py`, serve `fullscreen/` on localhost port 43928, and open its index. This development fixture uses original artwork and four original timed lines, with a mocked desktop bridge. Check the ten media designs, progress styles, popup/Escape behavior and independent card sizing. Native window transitions and actual playback require running the desktop EXE.

Both generators copy production assets and use no provider credentials or user files. Start a fixture server with `python -m http.server PORT --bind 127.0.0.1 --directory FOLDER`.

## Background interface checks

From Source, run `npm ci --prefix qa --ignore-scripts --no-audit --no-fund`, then `node qa/ui_logic_test.cjs`, `node qa/i18n_test.cjs` and `node qa/app_boot_test.cjs`. The DOM tests open no browser or desktop window. Media and the native bridge are mocked; they verify state preservation and interface behavior, not audible output or native graphics.

## Current lyric, cache, folder and resource checks

Run the tests from the Source directory after the LinkeDOM setup above:
`node qa/lyric_clock_test.cjs`, `node qa/lyric_timing_test.cjs`, `node qa/resource_frontend_test.cjs`, and `node qa/metadata_ui_test.cjs`.
They write controlled output under qa/results and operate only on mock state.

For the optional software raster checks, install `npm install --prefix qa --no-save --package-lock=false --ignore-scripts --no-audit --no-fund @napi-rs/canvas@0.1.100`, then run `node qa/verci_art_test.cjs` and `node qa/verci_integration_test.cjs`. The tests register standard Windows fonts from C:/Windows/Fonts and create original fixture previews under qa/previews. These measure CPU software rendering and behavior, not native GPU FPS or audible playback. No desktop window is opened by these checks.

Run `go test ./...` from Source for backend checks; live/network/model tests are opt-in. Tests use temporary roots and do not import the user's library. Evidence is a snapshot of observations, not a guarantee of continuing provider availability.

Imported gallery checks: `node qa/motion_library_test.cjs` (optional canvas dependency above) and `node qa/motion_restore_test.cjs`. All supplied modes are tested using controlled timing; images are original fixtures.

Alignment v2: run `python qa/alignment_quality_test.py` and `node qa/alignment_preview_test.cjs` from Source. Optional full offline-model check: set MADAR_ALIGN=1 and MADAR_ALIGNMENT_FIXTURE to the absolute qa/alignment-fixture folder, then run `go test -run ^TestPackagedLocalAlignment$ -v`. Keep tools beside Source as distributed. This opens no desktop window and uses original test speech, not the user library.


Nawa identity checks: `node qa/nawa_ui_test.cjs` uses the existing LinkeDOM harness. Optional real CSS/headless checks require Playwright (development only). Install playwright1.62.1 outside the packaged runtime; set NAWA_CHROMIUM to an existing compatible headless executable if the default browser is absent, then `node qa/nawa_headless_check.cjs` from Source. It serves only local fixture assets and blocks external requests. No user account or audible playback is used. These are CSS/behavior checks, not native EXE GPU measurements.

The optional resource generator under qa/icon-tool uses its pinned Go module; from that directory run `go run .` to rebuild Source/resources_windows_amd64.syso using the distributed Identity PNGs. It creates Windows icons and product/version metadata; runtime does not require Go or this package. Backend build identity paths retain the earlier Madar project name for compatibility.


Latest word-reveal regression: `node qa/focus_word_render_test.cjs` with the optional Canvas dependency above. `node qa/focus_headless_check.cjs` uses the same optional Playwright setup/NAWA_CHROMIUM as nawa_headless_check. It checks all104 style previews, three reveal modes and backgrounds, the hidden left dock and keyboard playback preservation using original fixtures. Tests do not assess native GPU FPS or every song's acoustic alignment accuracy. Backend `go test .` includes word_alignment_queue_test.go.


Regional lyrics and translation motion: `node qa/translation_motion_test.cjs` uses the existing optional Canvas dependency. `node qa/regional_headless_check.cjs` uses the same Playwright/NAWA_CHROMIUM setup. Default `go test .` exercises public-source contracts against local original fixtures. Optional live-source smoke check: NAWA_REGIONAL_LIVE=1 then `go test -run ^TestLiveRegionalLyrics$ -v`; it contacts public services with public metadata, uses no user account/library, and never prints lyric bodies. Catalog changes can invalidate these particular live probes. Evidence counts scope the checks; they are not accuracy or native GPU frame-rate guarantees.


Theme entrances, logo assets and local font previews: `node qa/theme_font_headless_check.cjs` uses the existing optional Playwright setup and NAWA_CHROMIUM. It serves only local embedded images and original fixture data. It checks allnine finite/cancellable entrances, three local game marks, independent four-language font previews, audio state and Escape ordering. No desktop window, audible playback, user library or external runtime request is used. Logo source attribution and hashes are in frontend/theme-logos/sources.json and THIRD-PARTY.md. This is behavior/CSS evidence, not native GPU frame-rate measurement.
