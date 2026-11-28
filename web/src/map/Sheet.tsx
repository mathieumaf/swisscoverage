import { type ReactNode, useEffect, useRef } from "react";

interface Props {
  title: string;
  // A line above the title, such as the operator.
  eyebrow?: ReactNode;
  // A way back to the view that opened this one.
  back?: { label: string; onBack: () => void } | undefined;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}

// The panel that opens over the map: on the right of a large screen, from the
// bottom of a phone. Escape and the close button close it.
export function Sheet({
  title,
  eyebrow,
  back,
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
    <aside
      className="absolute top-3 right-3 bottom-10 z-[2] w-[min(24rem,calc(100%-1.5rem))] overflow-y-auto rounded-xl border border-border bg-background px-5 py-4 text-[0.9375rem] text-foreground shadow-[0_8px_24px_rgb(0_0_0/0.2)] phone:top-auto phone:right-0 phone:left-0 phone:max-h-[60%] phone:w-auto phone:rounded-b-none"
      aria-labelledby="station-sheet-title"
    >
      <header className="flex items-start justify-between gap-4">
        <div>
          {back && (
            <button
              type="button"
              className="mb-2 cursor-pointer text-[0.875rem] text-accent underline"
              onClick={back.onBack}
            >
              ← {back.label}
            </button>
          )}
          {eyebrow}
          <h2
            id="station-sheet-title"
            className="mb-3 text-[1.25rem] leading-[1.3] font-bold wrap-anywhere"
          >
            {title}
          </h2>
        </div>
        <button
          ref={closeButton}
          type="button"
          className="size-10 flex-none cursor-pointer rounded-full border border-border text-[1.5rem] leading-none text-foreground"
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
