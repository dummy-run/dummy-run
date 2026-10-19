// Accessibility checks against the built site, served locally. Run twice: THEME=light and THEME=dark.
// Every page in the sitemap is tested at 1280 and 375 pixels wide.
const fs = require("node:fs");

const base = process.env.SITE_URL || "http://127.0.0.1:1313";
const theme = process.env.THEME === "dark" ? "dark" : "light";
const sitemap = fs.readFileSync("public/sitemap.xml", "utf8");
const paths = [...sitemap.matchAll(/<loc>https:\/\/dummyrun\.dev(\/[^<]*)<\/loc>/g)].map((m) => m[1]);
paths.push("/404.html");

const urls = [];
for (const path of paths) {
  for (const width of [1280, 375]) {
    urls.push({ url: base + path, viewport: { width, height: width === 375 ? 740 : 900 } });
  }
}

module.exports = {
  defaults: {
    standard: "WCAG2AA",
    // axe marks a contrast check "needs review" when a table cell is cut by the edge of its scroll
    // container at 375px. Those are reported as warnings and reviewed by hand; real failures stay errors.
    levelCapWhenNeedsReview: "warning",
    runners: ["axe", "htmlcs"],
    timeout: 60000,
    wait: 300,
    chromeLaunchConfig: {
      executablePath: process.env.CHROME_PATH || undefined,
      // Blink's preferredColorScheme is 0 for dark and 1 for light. It overrides the operating
      // system's appearance, so the light run is light on a Mac set to dark.
      args: ["--no-sandbox", "--disable-dev-shm-usage"].concat(
        theme === "dark" ? ["--blink-settings=preferredColorScheme=0", "--force-dark-mode"] : ["--blink-settings=preferredColorScheme=1"]
      ),
    },
  },
  urls,
};
