# Ethics and limits

Dummy Run plays fictional users to find likely problems early. This page says what that can and cannot tell you.

## What a persona is

A persona is a model. It simulates a barrier, such as no mouse or a small screen, and reports where the barrier stops it. It finds problems early and cheaply. It does not replace testing with disabled people, and it does not replace a conformance audit. A run that finds nothing says nothing about whether your site works for a person with that need.

The personas are composites. Each one describes a need and does not speak for a group. We welcome review of the library by disabled-led organisations and by disabled testers. Open an issue or a discussion on [GitHub](https://github.com/dummy-run/dummy-run) to start.

## What a report can say

A run record holds a verdict for the persona. A verdict of pass means the persona did its job in that run. It does not mean the page meets a standard. The accessibility sweep lists failing rules and the checks that need a person to decide. Nothing in Dummy Run calls a page compliant or certifies a standard.

## The screen-reader persona

The engine does not run a screen reader. The screen-reader persona reads the page as text, with the accessibility outline and the results of the accessibility sweep. It is not sent screenshots. That catches missing names and a broken reading order. It cannot tell you what NVDA, JAWS or VoiceOver says, or how a person who uses one every day would get on. The outline is computed from the page and only approximates what a screen reader says.

## What goes to the model provider

The persona is a model that reads what the browser shows. The provider you choose receives page text and screenshots. You choose the provider with `--provider`. [Providers](PROVIDERS.md) lists what each one needs.

Some providers keep prompts and responses for abuse monitoring. Azure OpenAI can keep them for up to 30 days unless the customer has an approved opt-out (sourced: Microsoft's Azure OpenAI data, privacy and security documentation). Check the terms of your provider before you point a run at anything sensitive. Run against previews that hold synthetic data. A page can also try to instruct the model. [Threat model](THREAT-MODEL.md) says how the engine treats that. Exported telemetry carries no screenshots (code: `integrations/grafana/exporter/internal/telemetry/plan.go`), and [Telemetry](TELEMETRY.md) lists what it does carry.

## Read-only by policy

A target with `"read_only": true` blocks every request that could change data and every request to another host. Use it for any site you do not own. Add `--identify` and `--respect-robots` when you test a site that is not yours. [Engine](ENGINE.md) describes the policy. Run the permission personas only on a test environment with test accounts.

## Findings about apps you do not own

Do not sign in to a site you have no right to test, and do not run the permission personas on one. A read-only look at public pages, as the free scan does, loads only what any visitor can load. [Security](../SECURITY.md) explains how to report a problem in a site you do not own. If Dummy Run visited your site and you want it to stop, see [Dummy Run visited my site](../site/content/scanner.md).
