import { describe, expect, it } from "vitest";
import {
  MIRRORED_STYLES,
  syncMirrorTypography,
} from "@/features/agents/block-text";

/*
The cite popover's anchor is positioned by a mirror of the textarea, and the
mirror wraps exactly like the textarea or the anchor lies: the ui
component's own text size once beat this file's classes and put the anchor
two hundred pixels below the caret — the popover "at the end of the form".
Class strings drift; computed styles cannot, so the mirror copies them.
*/
describe("the cite mirror", () => {
  it("copies the textarea's computed typography, property by property", () => {
    const textarea = document.createElement("textarea");
    textarea.style.fontSize = "13px";
    textarea.style.lineHeight = "19px";
    textarea.style.fontFamily = "serif";
    textarea.style.letterSpacing = "1px";
    textarea.style.padding = "4px";
    document.body.appendChild(textarea);
    const mirror = document.createElement("div");

    syncMirrorTypography(textarea, mirror);

    expect(mirror.style.fontSize).toBe("13px");
    expect(mirror.style.lineHeight).toBe("19px");
    expect(mirror.style.fontFamily).toBe("serif");
    expect(mirror.style.letterSpacing).toBe("1px");
    expect(mirror.style.padding).toBe("4px");
    textarea.remove();
  });

  // The list is the contract: the font box is what decides wrapping, and a
  // property dropped from it reopens the two-hundred-pixel lie.
  it("mirrors every property wrapping depends on", () => {
    for (const must of [
      "fontSize",
      "fontFamily",
      "lineHeight",
      "whiteSpace",
      "overflowWrap",
      "padding",
    ]) {
      expect(MIRRORED_STYLES).toContain(must);
    }
  });
});
