<div align="center">

<img src="Branding/ON-512-logo-transparent.png" alt="Tarkov Nexus" width="160" />

# Tarkov Nexus

**See your position on [tarkov.dev](https://tarkov.dev) maps in real time.**

[![Website](https://img.shields.io/badge/website-tarkov.nexus-5865F2?style=flat-square)](https://tarkov.nexus)
[![Latest Release](https://img.shields.io/github/v/release/ObsidianNetwork/Tarkov-Nexus?style=flat-square)](https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/ObsidianNetwork/Tarkov-Nexus/total?style=flat-square)](https://github.com/ObsidianNetwork/Tarkov-Nexus/releases)
[![Built with Wails](https://img.shields.io/badge/built%20with-Wails-DF0000?style=flat-square)](https://wails.io)

[Download](https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/latest) · [Website](https://tarkov.nexus) · [Changelog](CHANGELOG.md) · [Report a Bug](https://github.com/ObsidianNetwork/Tarkov-Nexus/issues)

</div>

---

## About

Tarkov Nexus is a lightweight Windows companion app for **Escape from Tarkov**
that monitors your game and automatically syncs your in-raid coordinates to the
interactive maps on [tarkov.dev](https://tarkov.dev). Take a screenshot in-game
and your marker appears on the map — no memory injection and no game
modification.

This repository contains the sanitized public source snapshot for the desktop
application, the party service, the landing page at
[tarkov.nexus](https://tarkov.nexus), and the release binaries users download.

Development happens in a separate private repository. Public source is published
here as reviewed snapshot commits so private Git history and private development
material never become part of this repository's ancestry.

## Features

- **Real-Time Position** — Your in-game coordinates sync to tarkov.dev as you move through raids
- **Auto Map Detection** — Reads the game log and detects which map you're on
- **Screenshot Monitoring** — Watches the EFT screenshot folder and parses coordinates from filenames
- **Party System** — Share positions with your squad
- **Compact Map Window** — Keep a small frameless map pinned above the game
- **Stable/Beta Updates** — Choose the release channel that suits you
- **Privacy First** — Screenshots and game logs stay local
- **Quest Tracking** — Optional [TarkovTracker](https://tarkovtracker.io) integration

## How It Works

When you take a screenshot in Tarkov, the game saves your coordinates in the
screenshot filename. Tarkov Nexus watches the screenshot folder, extracts those
coordinates, and sends the position to the configured map integration.

1. **Setup** — Download Tarkov Nexus and follow the setup wizard.
2. **Connect** — Configure the map integration and optional party/quest services.
3. **Play** — Load into a raid and use your screenshot key when position data is needed.
4. **See Your Position** — Tarkov Nexus updates the map from the detected coordinates.

## Requirements

- Windows 10 or 11 (64-bit)
- Escape from Tarkov
- A modern web browser
- Internet access for online integrations

## Installation

1. Download the latest `TarkovNexus-vX.Y.Z-Windows-x64.zip` from the [Releases page](https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/latest).
2. Extract the archive anywhere on your PC.
3. Run `TarkovNexus.exe`.
4. Follow the in-app setup wizard.

Each release also includes SHA-256 checksum files for download verification.

> Tarkov Nexus does not read or modify game memory. It watches local files used
> by its supported integrations and communicates with external services through
> their network interfaces.

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for the reconstructed historical release notes
and all releases going forward. New release entries are authored as reviewed
public-safe change fragments before a release is tagged.

---

## Repository Structure

```text
.
├── internal/              # Go application packages
├── ui-wails/              # Wails desktop application and frontend
├── party-server/          # Party/squad service
├── website/               # Astro landing page deployed to GitHub Pages
├── Branding/              # Public brand assets
├── CHANGELOG.md           # Public release history
└── .github/workflows/     # Public-repository CI/Pages workflows
```

The source tree is published from the private development repository through an
allowlisted, secret-scanned snapshot process. Files and workflows owned by this
public repository are maintained here rather than copied from private CI.

## Developing the Website

The landing page is an [Astro](https://astro.build) static site under `website/`.

### Prerequisites

- Node.js 22 or newer
- npm

### Local Development

```bash
cd website
npm install
npm run dev
```

For a production build:

```bash
npm run build
npm run preview
```

### Deployment

The website is built and deployed to GitHub Pages by the workflow in
`.github/workflows/deploy.yml`. Public GitHub workflows are intentionally owned
by this repository and are not synchronized from the private development repo.

## Contributing

Issues and pull requests are welcome. If you find a bug or have an idea for a
feature, please [open an issue](https://github.com/ObsidianNetwork/Tarkov-Nexus/issues).

## Disclaimer

Tarkov Nexus is a community-made tool and is **not affiliated with Battlestate
Games** or the official Escape from Tarkov product. All trademarks belong to
their respective owners.
