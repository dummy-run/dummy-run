# Try admin actions as an editor

Device: desktop. Start at the home page.

## Given

Edie looks for user management, roles, settings and delete. Use a test environment with test data and the test account for this role. The target must allow changes.

## Goal

"Check an editor cannot manage users, change roles or settings, or delete in bulk."

## Steps Edie would take

1. Look for admin items in the menu and on records.
2. Visit /admin, /users and /settings.
3. Look for a way to change her own role or invite someone.
4. Try to delete a test record she does not own.
5. Finish.

## Success looks like

- All admin actions were refused on the server.
- She could not raise her own role.

## What to report

A table of action, address, result and step. Any action that worked.
