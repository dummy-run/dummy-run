# The persona library

Ten fictional users that most websites have. Copy one into your own cast and adapt it. How to do that is in
[docs/PERSONAS.md](../../docs/PERSONAS.md).

| Persona | Who | Device | Tags |
|---|---|---|---|
| `screen-reader-user` | Noor, blind, works from headings, landmarks and labels | desktop | accessibility, screen-reader |
| `keyboard-only-user` | Kit, no mouse, looks for tab order, focus marks and traps | desktop | accessibility, keyboard |
| `phone-slow-connection` | Eli, one thumb, one bar of signal, little patience | phone | mobile, performance |
| `low-vision-zoom` | Hana, text enlarged to 200 percent, needs reflow | tablet | accessibility, low-vision |
| `first-time-visitor` | Alex, no context, asks what it is, what it costs and how to start | desktop | general, comprehension |
| `non-native-reader` | Zofia, reads English as a second language, stops at jargon and idioms | desktop | language, content |
| `older-adult` | Margaret, careful and wary of scams, wants trust signals and kind errors | desktop | trust, error-recovery |
| `role-viewer` | Vik, may read only, probes what a viewer can still do | desktop | permissions |
| `role-editor` | Edie, may edit her own work, probes the gap to admin | desktop | permissions |
| `role-admin` | Ada, manages her organisation, probes what even an admin should not do | desktop | permissions |

Each persona has `profile.md`, `prompt.md` and four scenarios in `scenarios/`, the same layout as
[casts/dummyrun-dev](../dummyrun-dev/README.md). The first seven are visitors and read only. They find a fact,
start a sign-up or contact journey and stop before the submit button, move around the site and recover from an
error. The last three need a sign-in and a target that allows changes, so run them on a test environment with
test data.

The front matter carries three extra keys the engine ignores. `tags` groups personas and `leans-on` names the
engine tools the persona uses most. Both are space-separated lists.

Every trait is tagged `sourced` or `inferred` as [docs/STYLE.md](../../docs/STYLE.md) describes. A sourced
trait names the page and quotes it. The sources are the WCAG 2.2 Understanding documents and the W3C WAI page
"How People with Disabilities Use the Web". They also include the GOV.UK Service Manual and Nielsen Norman Group
articles. The permission trio cites the OWASP Top 10.

Run one scenario with:

```sh
dummyrun run --cast casts/library --persona keyboard-only-user --scenario find-information \
  --target examples/targets/dummyrun-dev.json --out ~/.local/share/dummyrun/runs/library \
  --browser firefox --provider azure --model <deployment> --region uksouth
```
