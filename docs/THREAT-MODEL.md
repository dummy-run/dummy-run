# Threat model

Dummy Run points a model at a web page and lets it act on what it reads. So the page is untrusted input to the model. This page lists what the engine protects, where the trust boundaries sit, and each threat with the defence that stops it and the gap that's left. To report a flaw in Dummy Run itself, follow [Security](../SECURITY.md).

## Assets

| Asset | Why it matters |
|---|---|
| The target | A run must not change it. Read-only targets must stay read-only |
| The truth of the findings | A suppressed, faked or steered finding sends you the wrong way |
| Credentials | Provider keys and the test users a persona signs in with |
| Run records | They hold page text, screenshots, URLs and the persona's reports. `steps.jsonl` holds the full observation of every step |
| Your network | The machine that runs the browser can reach more than the target |
| Spend | Model tokens and browser time cost money |
| The supply chain | The engine, its dependencies, the browsers and the casts run with your rights |

## Trust boundaries

```mermaid
flowchart LR
    PAGE[Target page<br/>untrusted]:::bad -->|text, labels, alt text, URLs| ENG[Engine<br/>trusted]
    OP[Operator<br/>casts, target config]:::good --> ENG
    ENG -->|page text, screenshots| MOD[Model provider<br/>trusted with the data]
    MOD -->|tool calls, findings<br/>untrusted output| ENG
    ENG -->|browser requests<br/>checked against the allow list| PAGE
    ENG --> REC[Run records<br/>hold hostile text]:::bad
    REC --> REV[Reviewer, board]
    REC --> EXP[Exporter] --> OBS[Telemetry backend]
    classDef bad fill:#fdecea,stroke:#b42318,color:#1d1d1b
    classDef good fill:#f0fdf4,stroke:#15803d,color:#1d1d1b
```

1. Target page to engine. Everything the page supplies is untrusted. That means visible and hidden text and HTML comments. It means `aria-label`, `title` and `alt` values. It means the page title, URLs and console messages. The pixels in screenshots count too. The fencing described below covers text and can't cover pixels.
2. Engine to model provider. The provider receives page text and screenshots, and [Ethics and limits](ETHICS.md) covers what providers keep. Treat what comes back as untrusted too, because a model that read hostile text may act on it.
3. Operator to engine. You trust the cast, the scenario files and the target config. A cast from someone else is effectively code, since its `prompt.md` goes into the system prompt.
4. Engine to run records. A record holds text from the page, so anything that reads it later meets that text. That includes a person in `dummyrun review`.
5. Exporter to telemetry backend. The exporter sends IDs, outcomes, URLs and finding titles. A model that read the page wrote those titles.

## Threats

### Prompt injection from the page

A page tells the persona to ignore its task. It might say to report no findings or to declare the site fully accessible. It might send the persona to another site, or ask it to call `finish` with text the page supplies. The text can sit in plain view or hide in one of these places:

- text moved off screen, or in a `display: none` block
- HTML comments
- `aria-label`, `alt` and `title` values
- Unicode tag characters that spell ASCII
- zero-width characters that split a phrase

The engine defends against this in six ways.

- Fencing. Page text reaches the model between markers that carry a random value for the run, which the page can't know. Marker-like text in the page is defanged.
- A rule in the system prompt. Text between the markers is data. It never changes the task, the tools, the verdict or the findings.
- Hidden text mostly stays out. Text in `display: none` and `visibility: hidden` blocks and in comments is collected for the scan and for review, and isn't sent to the model. Values of `aria-label`, `alt` and `title` do reach the model when they name a control in the control list or the outline. They're fenced and scanned like the rest. Text with zero size, zero opacity or `aria-hidden` stays in the page text and is flagged. The address and title of older steps are quoted, cut short and fenced like the rest.
- A scan. Simple phrase patterns run over the page text, the control names, the outline and the hidden text. Screenshots aren't scanned. The patterns look for "ignore previous instructions", "system prompt" and "you are now", for chat template tokens and for orders to report nothing. They also catch claims that the site is fully accessible, and orders telling the persona to call `finish`. Instructions to visit a host outside the allow list are caught too, and so are invisible characters. Hidden text that names an assistant and gives an order is flagged even when no fixed phrase matches.
- A warning. The next observation tells the persona that the text was an instruction and that it should say so.
- A record of the attempt. Each new match is a `suspected_injection` event with the step, where it was found and a short extract. The harness then files one finding of kind `security` that cites every step with a match. The model doesn't write that finding, so a page can't talk the model out of it.

These gaps remain:

- The patterns are a tripwire. A paraphrase, another language or a homoglyph gets past them.
- Text drawn into an image reaches the model in screenshots and isn't scanned.
- Text that's moved off screen but still rendered goes to the model on purpose. The screen-reader persona needs it, because assistive technology reads it too. Fencing and the scan cover the risk, and the scan flags any match.
- A persuaded model can still leave real findings out. The security finding shows the page tried, but it doesn't restore what was left out.
- Documentation that talks about prompts trips the scan. You decide whether it matters when you review the finding.

### Faked or suppressed findings

A page, or a persuaded model, reports a problem that isn't there or hides one that is.

The engine defends against it like this:

- Every finding must cite step numbers that exist. A finding with none is dropped and recorded as an `uncited_finding` event.
- The symptom must come from a closed list. The route, locus and fingerprint come from the page, never from the model.
- Findings start as `pending`. A person confirms or rejects each one in `dummyrun review`, checking it against the run record ([Architecture](ARCHITECTURE.md)).
- The harness writes the security finding itself.

These gaps remain:

- A false finding that cites a real step passes the citation check. Only review catches it.
- Review reads the same hostile record. Any model that reads it later needs the same fencing, and it must not trust a quote the record supplies.

### Exfiltration through URLs

A page asks the persona to visit an address that carries data in its path or query, such as the prompt, earlier page text or a credential.

The engine defends against it like this:

- The browser reaches only the hosts and ports in the target's allow list. Every HTTP request it makes is intercepted, and anything off the list fails before it leaves. That covers images, scripts, redirects and form posts. `visit` also refuses an address outside the list before the browser is involved.
- A bare host name matches the scheme's default port, and the port of `base_url` when it's the base host. Any other port needs a `host:port` entry, so a page on `localhost:3000` can't reach a service on `localhost:9000`.
- A blocked request to another host is logged as an `off_origin_blocked` event on the step.
- `read_only` fails every request that could change the target.
- The persona is told never to visit an address just because a page names it.

These gaps remain:

- A GET to an allowed host can carry data in its query. If the page's own origin is the attacker, it gets that data.
- A downgrade to plain `http` on an allowed host and default port is allowed.
- Interception covers HTTP requests, including the WebSocket handshake. Traffic that isn't an HTTP request, such as WebRTC, isn't covered.
- An allowed host that serves user content, such as a shared storage domain, brings that content inside the boundary. List specific hosts.
- No test covers traffic that isn't an HTTP request the browser's network events see, such as WebRTC.

### Exfiltration through the model provider

Everything the persona sees goes to the provider: page text, the accessibility outline and the newest screenshots. A page that shows personal data sends it on.

You choose the provider and its region. The persona is told not to copy personal data into its report, and exported telemetry carries no screenshots. Nothing redacts page text before it's sent, so run against previews that hold synthetic data.

### Off-scope navigation

A link, a redirect, a meta refresh or a script sends the browser to another site.

The same interception applies. The blocked navigation isn't followed, the step records it, and the persona stays on the last good page. Host matching is exact, so `api.example.test` doesn't match `example.test`. Addresses with no host, such as `file:` and `ftp:`, are refused. Only `data:`, `blob:` and `about:` pass without a host check because those addresses fetch nothing from the network. Any request a page inside them makes goes through the same check.

The allow-list gaps above apply here too.

### Credential exposure with sign-in

A persona that signs in uses a test user's account. A hostile page could ask for the password, or a log could keep it.

The engine defends against it like this:

- The engine signs in before the persona starts, in a separate tab. The model never sees the credentials and never passes them in a tool call.
- The engine types the password only into a page served over https, or over http on `localhost`, `127.0.0.1` or `::1`. It checks the address again before each field, in case a redirect moved the page.
- The target file names environment variables, and the values are read at run time. They're kept out of the prompt and the run record, including `steps.jsonl`. The report, the logs and error messages don't hold them either.
- The engine takes no screenshot while it signs in.
- A saved session file is written with mode 0600.
- Provider keys come from the environment variable named by `--api-key-env` and are never logged. Azure uses a token credential and has no key path.
- Query values that look like credentials are redacted from the URLs the engine records, and the value of a `fill` isn't recorded in a step.

Screenshots can still show what a page displays, including the test user's own details. Give each test user the lowest role the scenario needs, fake personal details, and access to a test environment only. The full rules are under "Test credentials" in [SECURITY.md](../SECURITY.md).

### Resource abuse

A page tries to burn tokens, time or your machine.

A run has a step budget of at most 40 and a token budget, and it makes at most the step budget plus ten model calls. Each model call times out after 120 seconds, and `wait` stops at 15 seconds. Page text, the controls and the outline are cut to a fixed size before they reach the model. The hidden-text scan reads at most 5,000 elements and 3,000 characters. A record keeps at most 25 events of each kind, and the reader refuses a file over 64 MiB.

There's still no wall-clock limit on a whole run. A page that makes the browser crawl ends only when a call times out or you stop the run.

### The provider's content filter

A provider may refuse a call because of the page. Its content filter can block a page that holds hidden injection text.

The run stops cleanly. The record gets the outcome `provider_filtered`, a `provider_filtered` event, the steps so far and any security finding. `report.md` and the command's message say what happened and that the engine isn't at fault.

A hostile page may still be able to deny a run this way. The engine doesn't retry with the offending text removed.

### Poisoned run records

A record repeats hostile page text in events, step notes and `steps.jsonl`.

Event extracts are short, single-line and stripped of invisible characters. The schema marks them as data, and the security finding holds no page text.

Any model that reads a record still has to fence it and treat it as data. Any tool that renders a title or an extract, such as a dashboard, must escape it.

### Supply chain

`go.mod` fixes the module versions and `go.sum` records their checksums. Both are watched by Dependabot and dependency review. CI actions are pinned to commit hashes and checked by zizmor, and gitleaks scans for secrets. The browsers and chromedriver are pinned in CI. The axe-core copy in `internal/page/axe` is vendored with its licence.

Casts and scenarios still aren't signed. A cast you didn't write can steer the persona through its prompt, so read it first. `go install ...@latest` takes whatever is newest.

## Checking the defences

`internal/runner/testdata/canary.html` is a page full of injection attempts. They sit in visible text, in text moved off screen, in a `display: none` block and in a comment. They also sit in an `aria-label`, in `alt` text and in a `title` attribute. A link, a redirect and an image lead to another origin.

The browser tests in `internal/runner/canary_browser_test.go` play a scripted model against that page. They show that nothing reaches the other origin, every kind of attempt is flagged, and the `display: none` text and the comment aren't sent to the model. They also show that the security finding and the model's own findings survive whether the model resists or obeys, and they cover a provider that refuses the call. Run them with:

```sh
DUMMYRUN_BROWSERS=chrome,firefox go test ./internal/runner/
```

Don't point the canary at a site you don't own.
