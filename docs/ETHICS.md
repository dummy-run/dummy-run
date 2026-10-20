# Ethics and limits

Dummy Run plays fictional users to find likely problems early. This page says what that can and cannot tell you.

## What a persona is

A persona is a model. It simulates a barrier, such as no mouse or a small screen, and reports where that barrier stops it. That finds problems early and cheaply. It doesn't replace testing with disabled people, and it doesn't replace a conformance audit. A run that finds nothing tells you nothing about whether your site works for a person with that need.

The personas are composites. Each one describes a need, and none speaks for a group. We'd welcome a review of the library by disabled-led organisations and disabled testers. To start one, open an issue or a discussion on [GitHub](https://github.com/dummy-run/dummy-run).

## What a report can say

A run record holds a verdict for the persona. A pass means the persona did its job in that run. It doesn't mean the page meets a standard. The accessibility sweep lists failing rules and the checks a person needs to decide. Nothing in Dummy Run calls a page compliant or certifies it against a standard.

## The screen-reader persona

The engine doesn't run a screen reader. The screen-reader persona reads the page as text, along with the accessibility outline and the results of the accessibility sweep, and it gets no screenshots. That catches missing names and a broken reading order. It can't tell you what NVDA, JAWS or VoiceOver says, or how someone who uses one every day would get on. The outline is computed from the page, so it only approximates what a screen reader announces.

## What goes to the model provider

The persona is a model that reads what the browser shows, so the provider you pick with `--provider` receives page text and screenshots. [Providers](PROVIDERS.md) lists what each one needs.

Some providers keep prompts and responses for abuse monitoring. Azure OpenAI, for example, can keep them for up to 30 days unless you have an approved opt-out. Check your provider's terms before you point a run at anything sensitive, and run against previews that hold synthetic data.

A page can also try to instruct the model. [Threat model](THREAT-MODEL.md) explains how the engine handles that. Exported telemetry carries no screenshots, and [Telemetry](TELEMETRY.md) lists what it does carry.

## Read-only by policy

A target with `"read_only": true` blocks every request that could change data, and every request to another host. Use it for any site you don't own, and add `--identify` and `--respect-robots` when you test someone else's site. [Engine](ENGINE.md) describes the policy. Run the permission personas only on a test environment with test accounts.

## Findings about apps you do not own

Don't sign in to a site you have no right to test, and don't run the permission personas on one. A read-only look at public pages, like the free scan, loads only what any visitor can load. [Security](../SECURITY.md) explains how to report a problem in a site you don't own. If Dummy Run visited your site and you want it to stop, see [Dummy Run visited my site](https://dummyrun.dev/scanner/).
