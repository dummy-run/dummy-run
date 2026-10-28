// Runs the Siteimprove Alfa rules on the loaded page and returns the failed outcomes as plain data:
// the rule's id, the WCAG criteria it maps to and an XPath-like path to each failing element.
import { Audit } from "@siteimprove/alfa-act";
import { Native } from "@siteimprove/alfa-dom/native";
import { Node } from "@siteimprove/alfa-dom";
import { Device } from "@siteimprove/alfa-device";
import { Request, Response } from "@siteimprove/alfa-http";
import { Page } from "@siteimprove/alfa-web";
import rules from "@siteimprove/alfa-rules";

globalThis.dummyrunAlfa = async function () {
  const device = Device.standard();
  const doc = Node.from(await Native.fromNode(document), device);
  const page = Page.of(Request.empty(), Response.empty(), doc, device);
  const out = [];
  for (const o of await Audit.of(page, rules).evaluate()) {
    if (o.outcome !== "failed") continue;
    const sc = [];
    for (const r of o.rule.requirements) if (r.chapter) sc.push(r.chapter);
    const targets = typeof o.target.path === "function" ? [o.target] : [...o.target].filter((t) => typeof t.path === "function");
    for (const t of targets) out.push({ rule: o.rule.uri.split("/").pop(), sc, path: t.path() });
  }
  return out;
};
