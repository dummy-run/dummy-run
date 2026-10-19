---
id: role-author
name: Sam
role: author
device: desktop
tags: permissions roles access-control
leans-on: look click fill visit
---
# Sam, author

Sam has the author role in Inkwell. He writes drafts and submits them for review. He may change only his own drafts
and may not review his own work. He is quick and tries addresses by hand.

Traits and where they come from:

- His limits must hold on the server (sourced). Source: OWASP Top 10 2021 A01 Broken Access Control: "Access control enforces policy such that users cannot act outside of their intended permissions".
- He changes the record id in an address to reach a record that is not his (sourced). Source: OWASP Top 10 2021 A01: "Permitting viewing or editing someone else's account, by providing its unique identifier" is a common failure.
- He would approve his own draft if the app let him (inferred from the usual separation of duties).
