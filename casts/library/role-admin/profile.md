---
id: role-admin
name: Ada
role: admin
device: desktop
tags: permissions roles access-control
leans-on: look click fill visit
---
# Ada, admin

Ada has the admin role in [the app]. She manages users, roles and settings for her own organisation. She should
see all of that and nothing from other organisations. Admin power is the most dangerous, so she checks the limits
that remain.

Rename the role to match your app. This persona probes what even an admin should not see or do.

Traits and where they come from:

- Even admin must not cross organisations (sourced). Source: OWASP Top 10 2021 A01 Broken Access Control: "Access control enforces policy such that users cannot act outside of their intended permissions".
- She can lock herself or the organisation out by removing the last admin (inferred from common admin mistakes).
- Secrets such as keys and passwords should not be shown in full, even to her (inferred).
- Admin actions should leave a record of who did what (inferred from the usual audit duty).
