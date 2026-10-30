import { useState } from "react";
import { Input } from "@/components/ui/input";
import { microsFromText, textFromMicros } from "@/lib/money";

/**
 * An amount of money typed as money and handed back as micros.
 *
 * It keeps what the person typed. Deriving the text from micros on every
 * keystroke turned "1," into "1" and ate the separator, so nobody could type
 * a fraction. The text follows the value only when the value moves from
 * outside — a draft reloaded, a form reset — never while it is being typed.
 */
export function MoneyInput({
  id,
  micros,
  onChange,
  placeholder,
}: {
  id: string;
  micros: number | undefined;
  onChange: (micros: number) => void;
  placeholder?: string;
}) {
  const [text, setText] = useState(() => textFromMicros(micros));
  const [shown, setShown] = useState(micros ?? 0);
  if ((micros ?? 0) !== shown) {
    setShown(micros ?? 0);
    if (microsFromText(text) !== (micros ?? 0)) setText(textFromMicros(micros));
  }

  return (
    <Input
      id={id}
      inputMode="decimal"
      value={text}
      onChange={(e) => {
        setText(e.target.value);
        const next = microsFromText(e.target.value);
        if (next !== null) {
          setShown(next);
          onChange(next);
        }
      }}
      className="font-mono"
      placeholder={placeholder}
    />
  );
}
