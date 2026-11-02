# Third-party components and protocol references

Madar Music application code is an independent implementation. It does not contain SpotiFLAC application code, accounts, cookies, provider credentials or user downloads.

| Component | Purpose | Primary source |
| --- | --- | --- |
| Wails v2.13.0 | Native Windows/WebView2 host | https://github.com/wailsapp/wails |
| FFmpeg / FFprobe | Media inspection, full decoded validation, metadata and playback decoding | https://ffmpeg.org / https://github.com/yt-dlp/FFmpeg-Builds |
| yt-dlp 2026.10.26 | SoundCloud/Bandcamp extraction and accessible transfer | https://github.com/yt-dlp/yt-dlp/releases/tag/2026.10.26 |
| OpenAI Whisper 20250625 / base model | Local speech model used for alignment | https://github.com/openai/whisper |
| stable-ts 2.19.1 | Forced word alignment | https://github.com/jianfch/stable-ts |
| PyTorch / Torchaudio 2.9.1+cpu | Local inference | https://pytorch.org |
| PyInstaller 6.22.3 | Standalone alignment worker packaging | https://pyinstaller.org |
| winres v0.3.1 | Application icon/version/manifest resources | https://github.com/tc-hib/winres |

The included model is the official Whisper `base.pt` checkpoint, verified against SHA-256 `ed3a0b6b1c0edf879ad9b11b1af5a0e6ab5db9205f891f668f8b0e6c6326e34e`. It is loaded from a local file. The worker does not upload audio or require a service account.

Provider references: [Spotify Web API](https://developer.spotify.com/documentation/web-api), [SoundCloud guide](https://developers.soundcloud.com/docs/api/guide), [Internet Archive developer portal](https://archive.org/developers/), [hifi-api protocol](https://github.com/binimum/hifi-api), [LRCLIB](https://lrclib.net/docs), [LyricsKit provider contracts](https://github.com/ddddxxx/LyricsKit), [QQ provider contract reference](https://github.com/WXRIW/Lyricify-Lyrics-Helper), [Musixmatch developer portal](https://developer.musixmatch.com/).

Third-party components retain their respective licenses. License texts and package attribution are supplied under `licenses`. FFmpeg build details and corresponding source references are recorded there. The Windows credential encryption is tied to the current Windows user; no provider account credentials are bundled.

## Offline translation

NLLB-200 distilled 600M was created by Meta / the NLLB Team. This package uses the int8 CTranslate2 conversion published by JustFrederik, preserving the original **CC-BY-NC-4.0 (non-commercial)** license. It is a machine translation model, not an accuracy-certified lyric translator. Madar does not modify the checkpoint beyond using the published quantization, and adds its own local line mapping, cache, review and display code.

- Original model: https://huggingface.co/facebook/nllb-200-distilled-600M
- Converted model: https://huggingface.co/JustFrederik/nllb-200-distilled-600M-ct2-int8
- License: https://creativecommons.org/licenses/by-nc/4.0/legalcode.en
- CTranslate2 4.8.4, MIT: https://github.com/OpenNMT/CTranslate2
- Tokenizers 0.22.2, Apache-2.0: https://github.com/huggingface/tokenizers
- langdetect 1.0.9, Apache-2.0: https://github.com/Mimino666/langdetect

The exact checkpoint revision, file sizes and SHA-256 hashes are recorded in `tools/models/nllb-int8/model-lock.json`. Model.bin SHA-256: `ed1beaf75134de7505315a5223162f56acff397eff6b50638a500d3936fe707b`. No remote Python model code is executed. Both workers load local files with network model discovery disabled.

## Additional music connectors and streaming runtime

The public API references are [Audius](https://docs.audius.co/api/), [Mixcloud](https://www.mixcloud.com/developers/), and [MediaWiki image/search APIs](https://www.mediawiki.org/wiki/API:Search). hearthis.at uses its public api-v2 endpoint. YouTube Music, Jamendo, Audiomack, ReverbNation and public show extraction use the bundled yt-dlp extractors; these are separate from the independently authored application. Indexed searches are best-effort and may return no source links. No private account, browser cookies or tokens are provided.

Node.js v24.18.0 is included only as the supported JavaScript runtime for yt-dlp. Its full license and bundled notices are in licenses/Node-LICENSE.txt. Primary source: https://nodejs.org/ and https://github.com/nodejs/node/tree/v24.18.0 . The packaged Node executable SHA-256 is included in SHA256SUMS.txt.

Search-result previews run through a token-scoped loopback HTTP stream server bound to 127.0.0.1. It forwards range requests, rejects HTML/error payloads, and supports local FFmpeg decode for incompatible public streams. Previews are not committed to the download library.

## Full-screen visual reference

Madar's independently authored full-screen renderer is visually inspired by the user's NovaTune cover/control-card and lyric-column arrangement. Its CSS media shapes, controls, background treatments and JavaScript integration were written for Madar. NovaTune/Glowify source, character assets and theme scripts are not redistributed. The user's installed NovaTune archive is a read-only design reference. Runtime notices above continue to apply to the existing dependencies.

## Verci-inspired renderer and metadata adapters

The nine additional lyric modes are independently authored Canvas2D and DOM integration code. The user-supplied Verci Windows.exe and adjacent source ZIP were inspected read-only. The ZIP exposed nine renderer modes; no Verci source, compiled shader, bundled assets, NovaTune code, or fetched third-party lyric/artwork samples are redistributed. Canvas2D geometry approximates the fisheye appearance; it is not the original WebGL/Metal renderer. Inspection of the source ZIP does not establish byte identity with compressed assets embedded in the supplied native .NET apphost.

Provider API evidence and availability limitations are recorded in Source/qa/evidence/lyrics-sources.md and artwork-sources.md. Lyrics connectors: Lyrics.ovh, BetterLyrics, compatible LrcAPI deployments, Vagalume, and AudD. Artwork connectors: Apple/iTunes, Deezer, MusicBrainz + Cover Art Archive, TheAudioDB, and Fanart.tv. No private credentials are bundled. AudD is opt-in and metered. Source URLs remain attached to artwork candidates. API availability is not a license to redistribute returned content.

Development-only background tests use LinkeDOM 0.18.14 (ISC), and optional raster tests use @napi-rs/canvas 0.1.100 (MIT, with its own transitive notices). Neither Node package is a runtime dependency of Madar or bundled as node_modules. Existing runtime notices above remain applicable.


## Lyric Motion integration · 30 October 2026

The two user-supplied HTML galleries were inspected as source data. The 60-preset drawing bodies match the saved lyric-motion-library editable source; 20 drawing modes were adapted from Lyric Motion.html. Only the drawing geometry and preset labels are integrated. Demo clocks, sample lyrics, remote fonts, UI wrappers, widget APIs and autonomous animation loops are excluded. Runtime data and timing come from Madar. Existing source notices remain applicable. Game-inspired geometry contains no extracted game assets.


## محسن توقيت الكلمات v2 · 30 أكتوبر 2026

The versioned MadarAlignV2 worker uses the same pinned local Whisper base/stable-ts dependencies and notices as the previous worker. A word-boundary refinement pass uses stable-ts refine(steps='se', word_level=True, precision=0.02, single_batch=True). The official refinement API is documented at https://github.com/jianfch/stable-ts#refinement . Search precision is not an accuracy guarantee. No phoneme/character alignment model or new remote service is bundled. The previous executable is retained for compatibility; the shared native runtime versions are unchanged.


## Nawa identity and game-inspired themes

Nawa is the new public product name. Ten original geometric/vector logos, gothic architectural lines, a glass rose-window pattern and three CSS theme systems were authored for this app. No game screenshots, characters, extracted interface layouts, game audio or proprietary game fonts are bundled. The three game logos explicitly requested for theme cards/entrances are now included as credited local image assets; see the theme-logo section below. Bloodborne, Kingdom Hearts and Persona 3 Reload names identify visual inspiration; Nawa is independently authored and unaffiliated. System fonts are referenced by family and are not redistributed.

Primary visual references: Bloodborne official product/screenshots https://www.fromsoftware.jp/ww/detail.html?csm=bloodborne and https://www.playstation.com/en-us/games/bloodborne/ ; Kingdom Hearts official site https://www.kingdomhearts.com/ ; Persona 3 Reload official site https://persona.atlus.com/p3r/ . Themes reinterpret atmosphere, geometry and palettes across the app; they are not literal reproductions of every game asset.


## Regional public lyric adapters

Primary source references inspected on30 October2026: Uta-Net public title search and lyric pages https://www.uta-net.com/search/ and https://www.uta-net.com/ ; Anghami public lyric search https://kalimat.anghami.com/ ; LyricsTranslate original-song search https://lyricstranslate.com/en/songs ; SyncLRC developer API https://synclrc.dev/ (GET api.synclrc.dev/lyrics).

Uta-Net, Anghami and LyricsTranslate adapters parse bounded public HTML/text results after matching title/artist; no sign-in, authentication bypass, private token or site script execution is included. LyricsTranslate song-body selects the original rather than community translations/transliterations. These three return plain lyrics. SyncLRC uses its public JSON API and retains supplied line synchronization. All lyrics belong to their authors/publishers; no fetched song lyric bodies or page captures are redistributed with this app. Offline caches are created at runtime for the user's selected recordings. Public catalog coverage and availability vary. golang.org/x/net/html is part of the already pinned x/net dependency and retains its existing license notices.

Translated animation reuses the selected renderer in a second viewport on the same Canvas and audio-driven frame. Its normalized progress is derived from the original speech windows, including inter-word gaps; it is not measured translated audio timing and does not change saved translation/source timestamps.


## Requested game-logo assets and finite entrances

Nawa remains an independent music application. Its ten geometric identity logos and theme motion/decorations are independently authored. Three game marks are included specifically for the user-requested theme cards and entrance overlays; they do not change the executable's Nawa icon or indicate endorsement.

- Bloodborne: frontend/theme-logos/bloodborne.svg. Unmodified SVG from https://commons.wikimedia.org/wiki/File:Bloodborne_logo_black.svg ; direct file https://upload.wikimedia.org/wikipedia/commons/6/69/Bloodborne_logo_black.svg . The file page credits various authors and Fangamer as the source and labels it PD-textlogo with trademark notice. The original black mark is displayed in white through a CSS invert filter on dark scenes. Bloodborne marks belong to their respective owners.
- KINGDOM HEARTS HD1.5+2.5 ReMIX: frontend/theme-logos/kingdom-hearts.png. Unmodified publisher PNG from https://www.jp.square-enix.com/kingdom/khhd1_2/images/pc/index/logo.png ; source page https://www.jp.square-enix.com/kingdom/khhd1_2/ . ©Disney. Developed by SQUARE ENIX. This is the collection's title logo, not the portal-site wordmark.
- Persona 3 Reload: frontend/theme-logos/persona-3-reload.svg. Original SVG bytes decoded without modification from the h1.title-logo img data URI on https://p3re.jp/ . ©ATLUS ©SEGA. Its paths contain the publisher's lettering; no proprietary game font file is redistributed.

frontend/theme-logos/sources.json records source URLs, sizes and SHA256 hashes. These image files are attributed separately from the app's own code and original decorative SVGs. No game music, sound effects, character art or extracted menu animation is included. Entrances are independent finite CSS animations, not extracted game cinematics. Local font previews reference fonts installed by the user and distribute no system font files.


## Approved game theme resources (1.3)

The three game theme designs use bundled offline font files and retain notices under `frontend/game-themes`. KHMenu and KHMenuBoldItalic are Televo's faithful recreations from [Kingdom Hearts Re:Collection](https://github.com/Televo/kingdom-hearts-recollection); author notes are retained. EB Garamond, Amiri, Noto Sans Arabic and Barlow Semi Condensed use their included SIL Open Font License notices. Adobe Garamond is used only through an installed local font if present; its commercial font file is not redistributed. Barlow is a disclosed visual alternative for Reload, not the original licensed Rodin.

Makoto's portrait is from https://p3re.jp/ ; the KHII promotional image is from https://www.jp.square-enix.com/kingdom/khhd1_2/ ; Bloodborne promotional artwork is from the official PlayStation listing. Exact URLs, byte lengths and SHA-256 are in `frontend/game-themes/sources.json`. Small vector symbols and UI motions are independent interpretations. Game images/logos/trademarks belong to their owners and imply no affiliation. Existing user album covers are preserved. The animations adapt dialogue/message/menu styles to the app's audio clock; no extracted game engine animations or independent lyric scheduler are shipped.
