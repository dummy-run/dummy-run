# Security

## Reporting a vulnerability in Dummy Run

Use GitHub's private vulnerability reporting on this repository ("Report a vulnerability" under the
Security tab). You will get an acknowledgement within three working days and a fix or a plan within
thirty. Please do not open a public issue for security problems.

In scope: the harness, the reviewer, the results board, the Helm chart and the adapters in this
organisation. Out of scope: the applications Dummy Run is pointed at (see below), and the third-party
model providers.

## Findings about other people's applications

Dummy Run exists to find permission leaks and broken flows in the apps it tests. If you run it against
software you do not own and it finds something, that finding belongs to the owner of that software, not to
this project. Report it to them through their own disclosure route. Do not post it here, in issues, in
discussions or in example data.

## What this project never stores

No run data from any target leaves the operator's environment unless the operator publishes it. The
repository contains no run results, screenshots, credentials or persona data derived from a real customer.
CI enforces a clean tree for known client identifiers; if you find one it missed, report it privately
through the route above.

## Supported versions

Pre-release. Only `main` is supported until the first tagged release.
