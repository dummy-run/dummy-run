# Try to lock the organisation out

Device: desktop. Start at the home page.

## Given

Ada tries to remove her own admin rights while she is the only admin. Use a test environment with test data and the test account for this role. The target must allow changes.

## Goal

"Check the app stops an admin removing the last admin."

## Steps Ada would take

1. Open her own user page.
2. Try to change her role to viewer.
3. Try to delete her own account.
4. Read any warning or refusal.
5. Finish.

## Success looks like

- The app refused or warned clearly.
- The organisation always kept one admin.

## What to report

Whether the guard held, with the exact words of any warning and the step.
