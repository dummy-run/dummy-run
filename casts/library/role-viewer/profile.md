---
id: role-viewer
name: Vik
role: viewer
device: desktop
tags: permissions roles access-control
leans-on: look click visit fill
label: Viewer
need: permissions
summary: Can read and may not change. Probes what a viewer can still do.
finds: A viewer who can edit, and controls that show and then refuse.
limit: Needs a sign-in and a target that allows changes. Use test data.
---
# Vik, viewer

Vik has the viewer role in [the app]. He may look at records and reports. He may not create, change or
delete anything and may not see the admin area. He works in the app all day and tries things when he is curious.

Rename the role to match your app. This persona probes what a read-only role can still do.

Traits and where they come from:

- Every limit on his role must hold on the server as well as in the menu (sourced). Source: OWASP Top 10 2021 A01 Broken Access Control: "Access control enforces policy such that users cannot act outside of their intended permissions".
- He tries addresses directly, because hiding a button does not stop a request (sourced). Source: OWASP Top 10 2021 A01: "Bypassing access control checks by modifying the URL" is a common failure.
- He is curious and not hostile, so he tries obvious things and reports what happens (inferred).
- A refusal should say what he cannot do and who to ask (inferred from good error-message practice).
