# Ethics and limits

Dummy Run plays fictional users so you can find likely problems early, while they're still cheap to fix. It's useful, and it has real limits. This page sets out both, so you know what a run can tell you and what it can't.

## What a persona is

A persona is a model playing a part. It simulates a barrier, such as having no mouse or a small screen, and reports where that barrier stopped it. That makes it a quick, cheap way to catch problems before release. It doesn't replace testing with disabled people, and it doesn't replace a conformance audit. If a run finds nothing, that doesn't show your site works for people with that need.

The personas are composites. Each one describes a need, and none of them speaks for a group of people. We'd welcome review of the library by disabled-led organisations and by disabled testers. Open an issue or a discussion on [GitHub](https://github.com/dummy-run/dummy-run) to start that conversation.

## What a report can say

Each run ends with a verdict for the persona. A pass means the persona managed its job in that run, and nothing more. It doesn't mean the page meets a standard. The accessibility sweep lists the rules a page failed and the checks that need a person to decide. Nothing in Dummy Run calls a page compliant or certifies it against a standard.

## The screen-reader persona

The engine doesn't run a real screen reader. The screen-reader persona works from the page as text, with the accessibility outline and the results of the accessibility sweep, and it's never sent screenshots. That's enough to catch controls with no name or a reading order that jumps about. It can't tell you what NVDA, JAWS or VoiceOver would actually say, or how someone who uses one every day would get on. The outline is worked out from the page, so it only approximates what a screen reader announces.

## What goes to the model provider

The persona is a model reading what the browser shows, so the provider you choose receives page text and, for every persona except the screen-reader one, screenshots. You choose the provider with `--provider`, and [Providers](PROVIDERS.md) lists what each one needs.

Some providers keep prompts and responses for a while to monitor abuse. Azure OpenAI, for example, can keep them for up to 30 days unless you have an approved opt-out, as Microsoft's [data, privacy and security page for Azure OpenAI](https://learn.microsoft.com/legal/cognitive-services/openai/data-privacy) explains. Check your provider's terms before you point a run at anything sensitive, and run against previews that hold test data rather than real customers. A page can also try to give the model instructions, and the [threat model](THREAT-MODEL.md) explains how the engine handles that. Exported telemetry carries no screenshots, and [Telemetry](TELEMETRY.md) lists what it does carry.

## Read-only by policy

A target with `"read_only": true` blocks every request to another host, and every request that could change data apart from the sign-in form's own request while the engine signs in. Use it for any site you don't own, and add `--identify` and `--respect-robots` so the site's owner can see who you are and keep you out if they want to. The [engine guide](ENGINE.md) describes the policy in full. Run the permission personas only on a test environment with test accounts.

## Findings about apps you don't own

Don't sign in to a site you have no right to test, and don't run the permission personas against one. A read-only look at public pages, which is all the free scan does, loads only what any visitor could load. [Security](../SECURITY.md) explains how to report a problem you find in someone else's site. If Dummy Run visited your site and you'd like it to stop, see [Dummy Run visited my site](https://dummyrun.dev/scanner/).
