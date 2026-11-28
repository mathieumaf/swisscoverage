import { type ReactNode, useEffect, useId, useRef } from "react";

interface Props {
  title: string;
  // A line above the title, such as the operator.
  eyebrow?: ReactNode;
  // A line below the title.
  meta?: ReactNode;
  // A way back to the view that opened this one.
  back?: { label: string; onBack: () => void } | undefined;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}

// What the panel of the map shows in place of its home: a station, or the
// stations at one place. Escape and the close button close it.
export function Sheet({
  title,
  eyebrow,
  meta,
  back,
  closeLabel,
  onClose,
  children,
}: Props) {
  const closeButton = useRef<HTMLButtonElement>(null);
  const titleId = useId();

  useEffect(() => {
    closeButton.current?.focus({ preventScroll: true });
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <section className="px-5 pt-4 pb-5" aria-labelledby={titleId}>
      {back && (
        <button
          type="button"
          className="mb-3 -ml-1 flex min-h-10 cursor-pointer items-center gap-1 rounded-full px-1 text-[0.875rem] font-bold text-accent"
          onClick={back.onBack}
        >
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <path
              d="M15 6l-6 6 6 6"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.4"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          {back.label}
        </button>
      )}
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          {eyebrow}
          <h2
            id={titleId}
            className="text-[1.625rem] leading-[1.15] font-extrabold tracking-[-0.02em] wrap-anywhere"
          >
            {title}
          </h2>
          {meta}
        </div>
        <button
          ref={closeButton}
          type="button"
          className="grid size-10 flex-none cursor-pointer place-items-center rounded-full bg-field text-muted hover:text-foreground"
          onClick={onClose}
          aria-label={closeLabel}
        >
          <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
            <path
              d="M6 6l12 12M18 6L6 18"
              stroke="currentColor"
              strokeWidth="2.6"
              strokeLinecap="round"
            />
          </svg>
        </button>
      </header>
      {children}
    </section>
  );
}
