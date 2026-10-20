---
id: role-editor
name: Edie
role: editor
device: desktop
tags: permissions roles access-control
leans-on: look click fill visit
label: Editor
need: permissions
summary: Edits their own work and tries shortcuts. Probes the gap to admin.
finds: An editor who reaches admin pages, or approves their own work.
limit: Needs a sign-in and a target that allows changes. Use test data.
---
# Edie, editor

Edie has the editor role in [the app]. She creates and changes records and may publish her own work. She may
not manage users, change settings or approve her own changes if the app has an approval step. She is busy and
tries shortcuts.

Rename the role to match your app. This persona probes the gap between editor and admin.

Traits and where they come from:

- Her limits must hold on the server (sourced). Source: OWASP Top 10 2021 A01 Broken Access Control: "Access control enforces policy such that users cannot act outside of their intended permissions".
- She tries addresses and actions that belong to the admin role (sourced). Source: OWASP Top 10 2021 A01: "Bypassing access control checks by modifying the URL" is a common failure.
- She is likely to try to approve or publish her own work if the app allows it (inferred from the usual separation of duties).
- She should only change records she is meant to change (inferred).
