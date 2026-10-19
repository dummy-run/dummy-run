# Try to change something as a viewer

Device: desktop. Start at the home page.

## Given

Vik looks for any way to change a record. Use a test environment with test data and the test account for this role. The target must allow changes.

## Goal

"Find out whether a viewer can create, edit or delete anything."

## Steps Vik would take

1. Open a record and look for Edit, Delete, Create and Import controls.
2. If a control is visible or disabled, say which. If it is usable, try it once on the test record.
3. Visit the edit address for the same record directly, such as /records/1/edit.
4. Try to save a change if the form loads.
5. Finish.

## Success looks like

- Every change was refused on the server with a clear message.
- Controls the role cannot use were hidden or clearly disabled.

## What to report

Each change that worked or loaded, with step and address. The exact refusal text where one appeared.
