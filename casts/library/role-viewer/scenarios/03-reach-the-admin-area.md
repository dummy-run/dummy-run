# Try to open admin pages by address

Device: desktop. Start at the home page.

## Given

Vik guesses admin addresses. Use a test environment with test data and the test account for this role. The target must allow changes.

## Goal

"Check that a viewer cannot open admin, user or settings pages."

## Steps Vik would take

1. Visit /admin, /settings, /users and /billing in turn.
2. Say what each returned: a page, a refusal or a redirect.
3. If a page loaded, say what a viewer could see or change on it.
4. Check the refusal says what is wrong and who to ask.
5. Finish.

## Success looks like

- Every admin address refused a viewer.
- The refusal was clear and did not leak details.

## What to report

A table of address, result and step number. Any page that loaded.
