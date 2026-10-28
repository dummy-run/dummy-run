# Personas

A persona is a fictional user that the engine plays through a scenario. The library in
[casts/library](../casts/library/README.md) has eleven generic ones, for the kinds of user every site has.
Copy the ones you need and adapt them to your product. The [persona library page](https://dummyrun.dev/personas/)
shows them as cards, each with the command to run it.

What a persona can and can't tell you is in [ETHICS.md](ETHICS.md).

## Copy a persona into your cast

1. Copy the folder, for example `cp -r casts/library/keyboard-only-user casts/<your-cast>/`.
2. Edit `profile.md`. You can change the name, but keep `id` the same as the folder name because the loader
   checks it. `device` must be `desktop`, `tablet` or `phone`.
3. Fill in the placeholders. In the scenarios, `[the product]` is your product's name and `[the fact]` is
   something people look for on your site. `[the organisation]` and `[the app]` are what they sound like.
   Replace every placeholder before a run because the model reads them as written and guesses what they mean.
4. Rename the roles in the permission trio. `role-viewer`, `role-editor` and `role-admin` use generic names.
   Change `role` in the front matter, the folder names and the text to match the roles in your app. Swap the
   example addresses, such as `/admin` and `/records/1/edit`, for real ones.
5. Add your product's own words. If your users talk about a "workspace" or a "claim", put that word in the
   scenario so the persona looks for it.
6. Add scenarios for the risks you care about. Each file is `scenarios/NN-<id>.md`, with a heading, a goal and
   steps.

Then run it with `dummyrun run`, as [ENGINE.md](ENGINE.md) describes. For the seven visitor personas, start
with a read-only target.

If you'd like to contribute a persona to the library, [CONTRIBUTING.md](../CONTRIBUTING.md) explains what it
needs.

## Keep personas fictional

Every persona in the library is invented. Each is a composite of common patterns, and none is based on a
real person or a real customer's data. Keep it that way when you adapt one. Don't paste interview notes,
support tickets or analytics about named people into a profile.

A persona describes a need. It doesn't stand for a whole group. Margaret is careful online, while plenty of
older adults are confident. Noor uses a screen reader, and no one person speaks for all blind users.

## The permission trio

`role-viewer`, `role-editor` and `role-admin` test what each role can see and do beyond its rights. Each has
four scenarios that try the obvious things a curious user would, such as opening an edit address as a viewer
or an admin page as an editor. Run them only on a test environment, with test data and test accounts. A
finding that a role can do something it shouldn't is often the first one to act on.

These three sign in as the role in their `role` key. Give your target file a `signin` block with credentials
for those roles, or for the names you renamed them to, and the engine signs each persona in before it starts.
The persona never sees the password. The block and the two sign-in methods are in the Sign in section of
[ENGINE.md](ENGINE.md).

Use a test account with no multi-factor step, or a saved session file. A read-only target still works
because the engine's own sign-in request is the one write it allows.

## Settings a persona can use

The engine reads these keys from `profile.md`, and `Device:`, `Zoom:` and `Text scale:` lines from a
scenario. [ENGINE.md](ENGINE.md) has the full rules.

| Setting | How the library uses it |
|---|---|
| `zoom: 200` | `low-vision-zoom` runs at 200 percent. A desktop window of 1400 pixels then gives the page 700 CSS pixels |
| `text_scale: 200` | `large-text-phone` raises the browser's default font size to 200 percent and leaves the window alone, as a larger system text size does. Text set in `rem` or `em` grows, and text set in `px` doesn't. [ENGINE.md](ENGINE.md) compares it with zoom |
| `signin: editor` | The credentials role the engine signs in as before the run. `none` turns sign-in off. Without the key, the persona's `role` is used when the target has credentials under that name |
| `tools-denied: click` | `keyboard-only-user` can't click. The engine refuses the call and records a `blocked:` note |
| Tool limits as a modality | A persona that can't click records `keyboard-only` in the run record's `persona.dimensions.at`. One that can't use `screenshot` records `screen-reader-view`, and the rest record `none` |
| `tools-denied: screenshot` | `screen-reader-user` gets no screenshots. They're still saved for the reviewer. Instead, each look carries what a screen reader says reading the page from the top, one phrase per line, such as `heading, Opening hours, level 2` and `button, Menu, not expanded`. guidepup's virtual screen reader produces it from the page's accessibility semantics as they are at that look, so it changes when the page does. It isn't NVDA, JAWS or VoiceOver, which each have their own habits. The reader is the bundled one, never one the page supplies. A reading stops after eight seconds or 400 phrases, and the persona is sent the first 150. A phrase longer than 300 characters is cut to 297 and ends in three dots. When the reading didn't reach the end of the page, the look says so. A persona that also can't click hears the page too |
| `Text scale:` in a scenario | Replaces the persona's text size for that scenario. `Text scale: 100` sets it back to normal |
| `Device:` in a scenario | Runs a persona on another device without a second copy of it |
| `Sign in as:` in a scenario | Runs a persona as another role, or signed out with `Sign in as: none` |
| `Site:` in a scenario | Says what kind of site this is and who it's for, such as `Site: personal site - people who want to know who the author is`. The self-check judges each finding against it. Up to 300 characters |

## Limits

The engine ignores front matter keys it doesn't know, and front matter is flat `key: value` pairs. A few
personas in the library ask for things the engine can't do, so their prompts work around them.

| Persona | What to expect |
|---|---|
| `phone-slow-connection` | The engine doesn't throttle the network. The prompt has the persona give up early, but pages load at full speed |
| `screen-reader-user` | No real screen reader runs. The persona works from the text view, the accessibility outline, guidepup's virtual screen reader and `a11y` results. The virtual reader and the outline are computed from the page's accessibility semantics, so they approximate what NVDA, JAWS or VoiceOver would say |
