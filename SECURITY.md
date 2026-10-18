# Security

## Reporting a vulnerability in Dummy Run

Report it privately through GitHub. Open the Security tab of this repository and choose "Report a
vulnerability". We acknowledge reports within three working days. You get a fix or a plan within thirty.
Please keep security problems out of public issues.

The harness, the reviewer and the results board are in scope. So are the Helm chart and the adapters in
this organisation. The apps that Dummy Run tests are out of scope, and so are third-party model
providers.

## Findings about other people's apps

Dummy Run finds permission leaks and broken flows in the apps it tests. Suppose you run it against
software you do not own, and it finds something. That finding belongs to the software's owner. Report it
to them through their own disclosure route. Keep it out of this project's issues, discussions and
example data.

## What this project never stores

Run data stays in the operator's environment unless the operator publishes it. This repository holds no
run results or screenshots. It holds no credentials either, and no persona data drawn from a real
customer. CI checks the tree for known client identifiers. If you find one that it missed, report it
privately as described above.

## Supported versions

Dummy Run is in pre-release. Until the first tagged release, only `main` is supported.
