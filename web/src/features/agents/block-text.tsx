import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { useTranslation } from "react-i18next";
import { Textarea } from "@/components/ui/textarea";
import { CiteTool } from "@/features/agents/cite-tool";
import { InstructionProse } from "@/features/agents/instruction-prose";
import { labelOf, type Block } from "@/features/agents/instruction-blocks";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * The prose of one block, written or read.
 *
 * Written in a textarea and read as prose, swapped on focus. The alternative
 * is a contenteditable surface with chips inside it, which owns the caret, the
 * paste and the undo stack — three things a browser already does correctly and
 * a hand-written editor gets wrong the first time somebody pastes from a
 * document. The chips are a rendering, so nothing is lost by rendering them
 * when nobody is typing.
 */
export function BlockText({
  block,
  writing,
  onWriting,
  tools,
  typed,
  pendingCaret,
  onCaretApplied,
  cite,
}: {
  block: Block;
  writing: boolean;
  onWriting: (writing: boolean) => void;
  tools: { catalogue: Tool[]; policies: Policy[]; enabled?: string[] };
  typed: (text: string, cursor?: number) => void;
  pendingCaret: CaretTarget | null;
  onCaretApplied: () => void;
  cite: { open: boolean; onPick: (tool: string) => void; onClose: () => void };
}) {
  const { t, i18n } = useTranslation();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const mirrorRef = useRef<HTMLDivElement>(null);
  const markerRef = useRef<HTMLSpanElement>(null);
  const anchorRef = useRef<HTMLSpanElement>(null);
  const focusedWriting = useRef(false);
  // The controlled block text arrives one render after the keystroke; the
  // mirror needs the immediate text to place the popover at the typed marker.
  const [localText, setLocalText] = useState(block.text);
  const [citeMarker, setCiteMarker] = useState<number | null>(null);
  const label = labelOf(block.kind, i18n.language) || t("agents.blockProse");
  const citable =
    block.kind === "howToAct"
      ? citableTools(tools.catalogue, tools.enabled)
      : tools.catalogue;

  useEffect(() => {
    if (!writing) {
      focusedWriting.current = false;
      return;
    }
    if (cite.open) return;
    const textarea = textareaRef.current;
    if (!textarea) return;
    if (pendingCaret !== null) {
      if (block.text !== pendingCaret.text) return;
      const handle = window.setTimeout(() => {
        const current = textareaRef.current;
        if (!current || current.value !== pendingCaret.text) return;
        const at = Math.min(pendingCaret.at, current.value.length);
        current.focus();
        current.setSelectionRange(at, at);
        focusedWriting.current = true;
        onCaretApplied();
      }, 0);
      return () => window.clearTimeout(handle);
    }
    textarea.focus();
    if (!focusedWriting.current) {
      const end = textarea.value.length;
      textarea.setSelectionRange(end, end);
      focusedWriting.current = true;
    }
  }, [writing, cite.open, pendingCaret, block.text, onCaretApplied]);

  // The textarea is sized to its content, never scrolling inside itself:
  // the mirror cannot scroll, and a textarea that scrolls while the mirror
  // does not puts the anchor rows below the caret — which is exactly the
  // popover arriving "at the end of the form". rows counts only newlines
  // and is blind to soft wrapping, so the height comes from scrollHeight.
  useLayoutEffect(() => {
    const textarea = textareaRef.current;
    if (!writing || !textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [writing, block.text, localText]);

  useLayoutEffect(() => {
    const anchor = anchorRef.current;
    if (!anchor) return;
    if (!writing || !cite.open || citeMarker === null) {
      placeAnchor(anchor, 0, 0);
      return;
    }
    const mirror = mirrorRef.current;
    const marker = markerRef.current;
    if (!mirror || !marker) return;
    // The mirror wraps exactly like the textarea or the anchor lies. Class
    // strings drift — the ui component's own text size once beat this
    // file's and put the anchor two hundred pixels below the caret — so
    // the typography is copied from the computed styles, which cannot.
    syncMirrorTypography(textareaRef.current, mirror);

    const mirrorRect = mirror.getBoundingClientRect();
    const markerRect = marker.getBoundingClientRect();
    const lineHeight = lineHeightOf(mirror);
    // Belt beside the sizing suspenders: if the textarea still scrolled —
    // a zoom rounding, a future style — the anchor follows the visual
    // caret, not the text position.
    const scrolled = textareaRef.current?.scrollTop ?? 0;

    placeAnchor(
      anchor,
      markerRect.left - mirrorRect.left,
      markerRect.top - mirrorRect.top + lineHeight - scrolled,
    );
  }, [writing, cite.open, citeMarker, localText]);

  if (!writing) {
    return (
      <div
        role="textbox"
        tabIndex={0}
        aria-label={label}
        onFocus={() => onWriting(true)}
        onClick={() => onWriting(true)}
        className="min-w-0 cursor-text rounded-sm focus-visible:outline-2 focus-visible:outline-ring"
      >
        {block.text.trim() === "" ? (
          <p className="text-base/[1.65] text-muted-foreground">
            {t("agents.blockPlaceholder")}
          </p>
        ) : (
          <InstructionProse
            text={block.text}
            catalogue={tools.catalogue}
            policies={tools.policies}
          />
        )}
      </div>
    );
  }

  return (
    <CiteTool
      open={cite.open}
      catalogue={citable}
      anchor={
        <span
          ref={anchorRef}
          aria-hidden="true"
          data-cite-anchor="tool"
          className="pointer-events-none"
          style={fallbackAnchorStyle}
        />
      }
      onPick={cite.onPick}
      onClose={cite.onClose}
    >
      <>
        <Textarea
          ref={textareaRef}
          value={block.text}
          onChange={(e) => {
            const next = e.target.value;
            setLocalText(next);
            setCiteMarker(markerBeforeCursor(next, e.target.selectionStart));
            typed(next, e.target.selectionStart);
          }}
          onBlur={() => !cite.open && onWriting(false)}
          placeholder={t("agents.blockPlaceholder")}
          aria-label={label}
          rows={2}
          className="min-w-0 resize-none overflow-hidden border-0 bg-transparent p-0 text-base/[1.65] break-words shadow-none focus-visible:ring-0"
        />
        <div
          aria-hidden="true"
          data-cite-mirror="tool"
          ref={mirrorRef}
          className="pointer-events-none absolute inset-x-0 top-0 -z-10 whitespace-pre-wrap break-words text-base/[1.65] opacity-0"
        >
          {localText.slice(0, citeMarker ?? 0)}
          <span ref={markerRef}>
            {citeMarker === null ? "\u200b" : localText[citeMarker]}
          </span>
          {citeMarker === null ? "" : localText.slice(citeMarker + 1)}
        </div>
      </>
    </CiteTool>
  );
}

function citableTools(catalogue: Tool[], enabled?: string[]): Tool[] {
  if (!enabled) return catalogue;
  const pack = new Set(enabled);
  return catalogue.filter((tool) => pack.has(tool.toolId));
}

function markerBeforeCursor(text: string, cursor?: number): number | null {
  const at = cursor === undefined ? text.length - 1 : cursor - 1;
  return at >= 0 && text[at] === "@" ? at : null;
}

export const MIRRORED_STYLES = [
  "fontFamily",
  "fontSize",
  "fontWeight",
  "lineHeight",
  "letterSpacing",
  "textTransform",
  "textIndent",
  "wordBreak",
  "overflowWrap",
  "whiteSpace",
  "textWrap",
  "padding",
] as const;

export function syncMirrorTypography(
  textarea: HTMLTextAreaElement | null,
  mirror: HTMLElement,
) {
  if (!textarea) return;
  const computed = getComputedStyle(textarea);
  for (const property of MIRRORED_STYLES) {
    mirror.style[property as "fontSize"] = computed[property as "fontSize"];
  }
}

function lineHeightOf(element: Element): number {
  const parsed = Number.parseFloat(getComputedStyle(element).lineHeight);
  return Number.isFinite(parsed) ? parsed : 24;
}

function placeAnchor(anchor: HTMLElement, left: number, top: number) {
  anchor.style.position = "absolute";
  anchor.style.left = `${left}px`;
  anchor.style.top = `${top}px`;
  anchor.style.width = "1px";
  anchor.style.height = "1px";
}

const fallbackAnchorStyle: CSSProperties = {
  position: "absolute",
  left: 0,
  top: 0,
  width: 1,
  height: 1,
};

interface CaretTarget {
  at: number;
  text: string;
}
