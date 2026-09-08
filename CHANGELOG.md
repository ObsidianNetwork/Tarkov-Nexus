# Changelog

All notable public changes to Tarkov Nexus are documented here.

This changelog is maintained with [Changie](https://changie.dev/) from reviewed,
public-safe change fragments in the private development repository. Internal
implementation notes, private issue references, experiments, credentials, and
other non-public context are intentionally excluded.

The entries through v3.3.4 were reconstructed from the surviving release
metadata and the private release/tag history. They are written as user-facing
summaries rather than copied private commit messages.

## [v3.3.4] - 2026-09-05

### Changed

- Promoted the v3.3.4 beta line to the stable channel.
- Aligned packaged, development, and displayed version metadata so the updater and UI report the same release.
- Includes all changes shipped in v3.3.4-beta.1 and v3.3.4-beta.2 below.

## [v3.3.4-beta.2] - 2026-09-05

### Fixed

- Fixed the compact map window leaving unused space after the tarkov.dev cookie-consent banner was dismissed.
- Persisted cookie-consent state for the map window and prevented duplicate map-window processes from being opened.
- Hardened the local consent endpoint and single-window launch path.
- Improved version/channel badges so local development builds and installed releases are identified correctly.

### Changed

- Increased the contrast of the floating pin, minimise, and close controls without adding a conventional title bar.
- Restyled update-channel controls and removed misleading hover behaviour from static information cards.

## [v3.3.4-beta.1] - 2026-09-02

### Added

- Added Stable and Beta update channels, including prerelease discovery, explicit beta selection, download progress, and confirmation before downgrades.
- Added live status notices that can surface service incidents or release information without requiring a new application release.
- Added generated unique party display names for new users and enforced a 16-character display-name limit.

### Changed

- Reworked tarkov.dev communication to use only supported protocol messages and to identify Tarkov Nexus traffic explicitly.
- Added fail-closed map-name validation and corrected map aliases before remote commands are sent.
- Changed the local map proxy to preserve upstream security headers and stop modifying consent, branding, and saved map preferences beyond what is required to embed the map.
- Changed updater downloads to install an exact release tag and require the matching SHA-256 checksum before installation.
- Added one-way, allowlisted source publication so the public repository receives sanitized snapshots rather than private Git history.

### Fixed

- Fixed saved settings being discarded when a Remote ID was missing.
- Fixed a concurrent WebSocket-write crash during party map use.
- Fixed first-launch integration starting before the setup wizard was complete.
- Kept local party/overlay position delivery working when the remote tarkov.dev connection fails.
- Improved manual map override normalization and map selection reliability.

### Security

- Restricted which parent origins can frame the local map proxy.
- Reduced unnecessary CI/package permissions and strengthened secret-scan failure handling in the public snapshot publisher.

## [v3.3.3] - 2026-03-14

### Changed

- Redesigned the party map into a compact 500x500 frameless window.
- Added floating pin, minimise, and close controls, including an always-on-top toggle.
- Made the map window transparent/translucent and resizable down to a smaller minimum size.

## [v3.3.2] - 2026-03-14

### Fixed

- Fixed party display-name changes not being saved correctly from the Party tab.

## [v3.3.1] - 2026-03-14

### Added

- Added a first-run setup wizard with automatic path detection and map verification.
- Added Party Central configuration with a required display name and sensible party defaults.
- Added safer background update-check initialization and runtime control of automatic checks.

### Changed

- Added automatic settings persistence and refreshed the Dashboard and Settings layouts.
- Clarified Remote ID guidance and first-run behaviour.

## [v3.2.2] - 2026-01-28

### Fixed

- Improved automatic detection of the Escape from Tarkov logs directory on Windows by checking launcher logs, common install locations, and drive roots.

## [v3.2.1] - 2025-12-05

### Historical note

- No version-specific end-user change was clearly documented for this release, so none is inferred here.

## [v3.2.0] - 2025-12-04

### Added

- Added acceptance-based friend requests instead of immediately adding users as friends.
- Added incoming and outgoing friend-request views, mutual-request auto-accept, and pending-request cancellation.
- Added floating notifications with sound effects for friend requests, party invites, party membership changes, and friends coming online.
- Added direct accept/decline actions for friend requests and party invites from notifications.

### Fixed

- Fixed the party join button sometimes requiring two clicks.
- Fixed invite notifications remaining after accept/decline and declined invites reappearing.
- Added invite-spam protection and removed duplicate log messages.

## [v3.1.0] - 2025-12-04

### Added

- Added the initial Party System.
- Added the ability to join a party with friends and view party-member positions on the map.

## [v3.0.4] - 2025-11-21

### Changed

- Improved the updater so installations continue to update correctly when the executable has been renamed.

## [v3.0.3] - 2025-11-21

### Added

- Added automatic application restart support after an update is installed.
- Added a **Restart Now** action when an installed update is ready.

## [v3.0.2] - 2025-11-21

### Historical note

- No durable version-specific end-user change was documented for this release.

## [v3.0.1] - 2025-11-21

### Added

- First public release of Tarkov Nexus.
- Real-time position synchronization with tarkov.dev maps.
- Automatic map detection from Escape from Tarkov logs.
- TarkovTracker quest synchronization.
- Automatic application updates.
- Windows desktop interface built with Wails, Go, and React.
