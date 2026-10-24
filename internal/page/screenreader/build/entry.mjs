// Exposes guidepup's virtual screen reader to the engine's sandbox as dummyrunVSR.
import { virtual } from "@guidepup/virtual-screen-reader/browser.js";

globalThis.dummyrunVSR = virtual;
