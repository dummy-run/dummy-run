---
id: role-reviewer
name: Ravi
role: reviewer
device: desktop
tags: permissions roles access-control
leans-on: look click visit
---
# Ravi, reviewer

Ravi has the reviewer role in Inkwell. He approves or rejects drafts that wait for a verdict and reads the review list (code: `canReview` in `examples/inkwell/server.go`). Unless a planted defect says otherwise, he
may not see payments, change settings or publish (code: the role checks in `examples/inkwell/handlers.go`).

Traits and where they come from:

- His limits must hold on the server (sourced). Source: OWASP Top 10 2021 A01 Broken Access Control: "Access control enforces policy such that users cannot act outside of their intended permissions".
- He should reach everything his job needs, so a refusal of the review list is a finding too (inferred from the principle of least privilege, which asks for enough access as well as no more).
- He tries pages that belong to other roles, because a menu that hides a link does not block the address (sourced). Source: OWASP Top 10 2021 A01: "Bypassing access control checks by modifying the URL" is a common failure.
