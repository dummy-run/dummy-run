// Exposes guidepup's virtual screen reader. The engine rewrites this one assignment to a variable local to its own
// call (see localReader in ../../screenreader.go), so the page never supplies the reader.
import { virtual } from "@guidepup/virtual-screen-reader/browser.js";

globalThis.dummyrunVSR = virtual;
