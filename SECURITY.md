# Security Policy

## Supported versions

We provide security fixes for the latest minor release on the `v1.x` line.
Older versions may receive fixes at maintainer discretion.

| Version | Supported          |
| ------- | ------------------ |
| 1.x     | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Use one of:

- **Preferred:** GitHub's [private vulnerability reporting] on this repo.
  (Security tab → Report a vulnerability.)
- **Email:** security@elbits.no

Please include:

- A description of the issue and its impact.
- Steps to reproduce, or a proof-of-concept.
- Affected versions, if known.
- Your name and how you'd like to be credited (or "anonymous").

## What to expect

- **Acknowledgement** within 3 working days.
- **Initial assessment** within 10 working days.
- **Fix and disclosure timeline** agreed with you. We aim for fixes within
  90 days of report; complex issues may take longer, and we'll keep you
  informed.
- **Credit** in release notes and a published advisory, unless you prefer
  anonymity.

## Scope

In scope: the code in this repository, official container images published to
`ghcr.io/3lbits/vigil-public`, and the documented deployment configuration.

Out of scope: third-party dependencies (please report upstream), self-hosted
deployments of Vigil run by other organisations, social engineering, physical
attacks.

## Safe harbour

We will not pursue legal action against researchers who:

- Make a good-faith effort to avoid privacy violations and service disruption.
- Report vulnerabilities promptly via the channels above.
- Do not exploit a vulnerability beyond what is necessary to demonstrate it.

[private vulnerability reporting]: https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability
