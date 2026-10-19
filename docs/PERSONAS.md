# Personas

A persona is a fictional user that the engine plays through a scenario. The library in
[casts/library](../casts/library/README.md) holds ten generic ones for the kinds of user that every site has.
Your team copies the ones it needs and adapts them to its product. The [persona library page](https://dummyrun.dev/personas/) shows them as cards with the command to run each.

What a persona can and cannot tell you is in [ETHICS.md](ETHICS.md).

## Fictional composites

Every persona in the library is invented. Each is a composite built from public sources and from common
patterns, and none is built from a real person or a real customer's data. [CONTRIBUTING.md](../CONTRIBUTING.md)
sets the same rule for new personas. Keep it when you adapt one. Do not paste interview notes, support tickets
or analytics about named people into a profile.

A persona describes a need and does not stand for a group. Margaret is careful online, and many older adults
are confident. Noor uses a screen reader, and no single person speaks for all blind users.

## Copy a persona into your cast

1. Copy the folder. For example, `cp -r casts/library/keyboard-only-user casts/<your-cast>/`.
2. Edit `profile.md`. Change the name if you like. Keep `id` the same as the folder name, because the loader
   checks it. Keep `device` as `desktop`, `tablet` or `phone`.
3. Replace the placeholders. In the scenarios, `[the product]` is your product's name. `[the fact]` is a fact
   people look for on your site. `[the organisation]` and `[the app]` are what they sound like. Fill in every placeholder before a run. The model reads them as written and guesses what they mean.
4. Rename the roles for the permission trio. `role-viewer`, `role-editor` and `role-admin` use generic names.
   Change `role` in the front matter, the folder names and the text to the role names in your app. Replace the
   example addresses such as `/admin` and `/records/1/edit` with real ones.
5. Add the product's own words. If your product has a term such as "workspace" or "claim", put it in the
   scenario so the persona looks for it.
6. Add scenarios that match your risks. Each file is `scenarios/NN-<id>.md` with a heading, a goal and steps.
7. Keep the source lines. If you add a trait, say where it comes from and tag it `sourced` or `inferred`
   as [STYLE.md](STYLE.md) describes. A trait with no source is removed.

Run it with `dummyrun run` as [ENGINE.md](ENGINE.md) describes. Start with a read-only target for the seven
visitor personas.

## The permission trio

`role-viewer`, `role-editor` and `role-admin` probe what each role can see and do beyond its rights. Each has
four scenarios that try obvious things a curious user would try, such as an edit address for a viewer or an
admin page for an editor. Run them only on a test environment with test data and test accounts. A finding where
a role can do something it should not is usually the most useful one a run produces.

These three sign in as the role named in their `role` key. Give the target file a `signin` block with credentials
for those roles, or for your renamed ones. The engine then signs each persona in before it starts. The persona never sees the password. [ENGINE.md](ENGINE.md#sign-in) has the block and the two methods.
Use a test account that has no multi-factor step, or a saved session file. A target that is read-only still
works, because the engine's own sign-in request is the one write it allows.

## Settings a persona can use

The engine reads these keys from `profile.md`, and `Device:` and `Zoom:` lines from a scenario. [ENGINE.md](ENGINE.md)
has the full rules.

| Setting | Use in the library |
|---|---|
| `zoom: 200` | `low-vision-zoom` runs at 200 percent. A desktop window of 1400 pixels then gives the page 700 CSS pixels |
| `signin: editor` | The credentials role the engine signs in as before the run. `none` turns it off. Without the key the persona's `role` is used when the target has credentials under that name |
| `tools-denied: click` | `keyboard-only-user` cannot click. The engine refuses the call and records a `blocked:` note |
| Tool limits as a modality | The run record's `persona.dimensions.at` is `keyboard-only`. A persona that may not use `screenshot` records `screen-reader-view`. Other personas record `none` |
| `tools-denied: screenshot` | `screen-reader-user` is not sent screenshots. They are still saved for the reviewer |
| `Device:` in a scenario | Runs one persona on another device without a second copy of the persona |
| `Sign in as:` in a scenario | Runs one persona as another role, or signed out with `Sign in as: none` |

## Format gaps

The cast format is a folder with `profile.md`, `prompt.md` and scenarios. The engine ignores front matter
keys it does not know. These are the things the library needs that the format still lacks. The library works
around each one in the prompt, and the engine code is unchanged.

| Gap | Effect in the library |
|---|---|
| No network throttling or latency | `phone-slow-connection` gives up early in the prompt. The page loads at full speed |
| No screen reader | `screen-reader-user` works from the text view, the accessibility outline and `a11y` results. The outline is computed from the page and only approximates what a screen reader says |
| Front matter is flat `key: value` | `tags` and `leans-on` are space-separated strings and nothing reads them yet. The website reads `label`, `need`, `summary`, `finds` and `limit` |
