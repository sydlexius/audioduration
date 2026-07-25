# Security Policy

## Scope

This repository is a fork of [lizc2003/audioduration](https://github.com/lizc2003/audioduration),
maintained to carry fixes that upstream is not currently accepting (its issue tracker is
disabled and it has had no commits since January 2026).

The library parses untrusted binary input -- audio file headers and container structures --
so malformed or hostile files are within scope. Report anything that causes a panic, an
out-of-bounds read, unbounded allocation, or a hang on a crafted input file.

## Supported Versions

Security updates are provided for the current `main` branch and the latest released version.

## Reporting a Vulnerability

Please report suspected vulnerabilities privately through GitHub Security Advisories:

https://github.com/sydlexius/audioduration/security/advisories/new

Do not open a public issue for security-sensitive reports. Include a clear description,
reproduction steps or proof of concept when available, affected versions, and any known
mitigations. A minimal crafted input file is the most useful proof of concept for this
library; please attach it rather than describing it.

Reports are triaged as time permits. Confirmed vulnerabilities will be fixed in `main` and
released with an advisory when appropriate.

Where a report also affects upstream, it will be handled here first and disclosed upstream
once a fix is available.
