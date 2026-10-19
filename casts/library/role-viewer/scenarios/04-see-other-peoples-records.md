# Look at records that belong to others

Device: desktop. Start at the home page.

## Given

Vik changes the number in a record address to see another person's record. Use a test environment with test data and the test account for this role. The target must allow changes.

## Goal

"Check that a viewer can only open records the role is meant to see."

## Steps Vik would take

1. Open a record that belongs to him and note its address.
2. Change the id in the address to a neighbouring one.
3. Say whether it loaded and whose it was.
4. Check the lists and search show only what the role may see.
5. Finish.

## Success looks like

- Records outside his scope were refused.
- Lists and search did not show them either.

## What to report

Any record he should not have seen, with the address and step. Anything that looked like real personal data.
