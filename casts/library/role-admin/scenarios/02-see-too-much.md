# Check what even an admin should not see

Device: desktop. Start at the home page.

## Given

Ada looks at the settings and user pages for secrets. Use a test environment with test data and the test account for this role. The target must allow changes.

## Goal

"Check that passwords, keys and tokens are masked and that no other organisation's data shows."

## Steps Ada would take

1. Open the user list and one user.
2. Open the settings and any key or token page.
3. Look for passwords, keys or tokens shown in full.
4. Change an id in an address to try another organisation's record.
5. Finish.

## Success looks like

- Secrets were masked.
- Other organisations' records were refused.

## What to report

Anything shown that should be masked, with step and address. Any cross-organisation record that loaded.
