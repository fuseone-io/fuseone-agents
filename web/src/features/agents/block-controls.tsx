import { useTranslation } from "react-i18next";
import { GripVertical, SplitSquareVertical, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * What can be done to a block, reached on hover and on focus.
 *
 * On focus as well, so none of it is a control that exists only for a mouse.
 */
export function BlockControls({
  splittable,
  onGrab,
  onDrop,
  onSplit,
  onRemove,
}: {
  /** Whether there is more than one paragraph to break apart. */
  splittable: boolean;
  onGrab: () => void;
  onDrop: () => void;
  onSplit: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
      {/* The handle rather than the row: a paragraph you cannot select with
          the pointer is worse than one you have to grab by its grip. */}
      <span
        draggable
        onDragStart={onGrab}
        onDragEnd={onDrop}
        aria-label={t("agents.moveBlock")}
        className="cursor-grab text-text-disabled"
      >
        <GripVertical className="size-4" aria-hidden />
      </span>

      {splittable && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7"
          onClick={onSplit}
          aria-label={t("agents.splitBlock")}
        >
          <SplitSquareVertical className="size-3.5" aria-hidden />
        </Button>
      )}

      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7"
        onClick={onRemove}
        aria-label={t("agents.removeBlock")}
      >
        <Trash2 className="size-3.5" aria-hidden />
      </Button>
    </div>
  );
}
