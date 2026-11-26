import { type ReactNode, useEffect, useRef } from "react";

interface Props {
  title: string;
  // A line above the title, such as the operator.
  eyebrow?: ReactNode;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}

// The panel that opens over the map: on the right of a large screen, from the
// bottom of a phone. Escape and the close button close it.
export function Sheet({
  title,
  eyebrow,
  closeLabel,
  onClose,
  children,
}: Props) {
  const closeButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    closeButton.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <aside className="station-sheet" aria-labelledby="station-sheet-title">
      <header className="station-sheet__header">
        <div>
          {eyebrow}
          <h2 id="station-sheet-title">{title}</h2>
        </div>
        <button
          ref={closeButton}
          type="button"
          className="station-sheet__close"
          onClick={onClose}
          aria-label={closeLabel}
        >
          ×
        </button>
      </header>
      {children}
    </aside>
  );
}
