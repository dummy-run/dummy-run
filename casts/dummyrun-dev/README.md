# The dummyrun.dev cast

Four fictional visitors to https://dummyrun.dev. Each only browses and reads. None signs up, fills in a form or
types into a chat widget. The cast exists to dogfood the engine against the project's own site.

| Persona | Who | Device |
|---|---|---|
| `priya-qa-lead` | A QA lead deciding whether the tool replaces her scripted regression suite | desktop |
| `tom-developer` | A developer who wants the quickstart and nothing else | desktop |
| `mariam-compliance` | A compliance officer checking what data leaves her company's network | desktop |
| `dan-executive` | An executive deciding whether his team should try it | phone |

Each persona is a folder with `profile.md` (front matter plus the profile), `prompt.md` (the system prompt) and
`scenarios/NN-<id>.md`. The format is described in [docs/ENGINE.md](../../docs/ENGINE.md).

Run one scenario with:

```sh
dummyrun run --cast casts/dummyrun-dev --persona tom-developer --scenario find-the-quickstart \
  --target targets/dummyrun-dev.json --out ~/.local/share/dummyrun/runs/dummyrun-dev \
  --browser firefox --provider azure --model <deployment> --region uksouth
```
