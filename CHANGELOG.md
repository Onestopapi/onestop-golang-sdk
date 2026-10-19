# Changelog

## 0.2.1 (2026-10-19)

### Added

- Imperva (utmvc): `session.ImpervaUtmvc(scriptURL, script, cookies)` returns `*onestop.ImpervaResponse, error`; its payload is the `___utmvc` cookie value. Example: `examples/<client>/imperva_utmvc/main.go`.

## 0.2.0 (2026-10-20)

### Added

- Imperva (reese84): `session.Imperva(scriptURL, script)` returns `*onestop.ImpervaResponse, error`. Example: `examples/<client>/imperva/main.go`.

### Changed

- Akamai sensor: the pass is `_abck` containing `~0~` or the exact 17-byte `{"success": true}` reply (docs and examples).
- Kasada: each protected request sends the `x-kpsdk-h` from `mfc` and a fresh `x-kpsdk-cd` (docs and examples).
- TLS: use the newest Chrome profile the library has, and keep the User-Agent and `sec-ch-ua` on the latest Chrome (examples use Chrome 155).

## 0.1.0 (2026-10-14)

Initial release.
