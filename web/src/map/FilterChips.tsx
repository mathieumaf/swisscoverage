import { useEffect, useId, useRef, useState } from "react";
import type { Locale } from "../i18n/locales";
import type { MessageKey } from "../i18n/messages/en";
import { translator } from "../i18n/translate";
import {
  allFilters,
  type Filters,
  kinds,
  operators,
  powerClasses,
  technologies,
} from "./filters";
import { operatorColors, operatorDot } from "./stations";

interface Props {
  locale: Locale;
  filters: Filters;
  onFilters: (filters: Filters) => void;
}

type Group = keyof Filters;

// The width of the open list, `w-72`, in pixels.
const listWidth = 288;

// The filters, one chip per vocabulary over the map, each opening the list
// of its values. A chip that hides some values is dark.
export function FilterChips({ locale, filters, onFilters }: Props) {
  const t = translator(locale);
  const [open, setOpen] = useState<Group | null>(null);
  // Where the open list stands, under its chip.
  const [left, setLeft] = useState(0);
  // How tall it may grow: down to the bottom of the screen.
  const [maxHeight, setMaxHeight] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const listId = useId();

  // A click elsewhere, Escape or a change of the window's size closes the
  // list.
  useEffect(() => {
    if (open === null) {
      return;
    }
    const onClick = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) {
        setOpen(null);
      }
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopImmediatePropagation();
        setOpen(null);
        root.current
          ?.querySelector<HTMLButtonElement>("[aria-expanded=true]")
          ?.focus();
      }
    };
    // Its place and height were measured when it opened.
    const onResize = () => setOpen(null);
    document.addEventListener("click", onClick);
    window.addEventListener("keydown", onKey, { capture: true });
    window.addEventListener("resize", onResize);
    return () => {
      document.removeEventListener("click", onClick);
      window.removeEventListener("keydown", onKey, { capture: true });
      window.removeEventListener("resize", onResize);
    };
  }, [open]);

  const groups: {
    key: Group;
    title: MessageKey;
    values: readonly string[];
    label: (value: string) => string;
  }[] = [
    {
      key: "operators",
      title: "filters.operator",
      values: operators,
      label: (value) => t(`operator.${value as (typeof operators)[number]}`),
    },
    {
      key: "technologies",
      title: "filters.technology",
      values: technologies,
      label: (value) => value,
    },
    {
      key: "kinds",
      title: "filters.kind",
      values: kinds,
      label: (value) => t(`kind.${value as (typeof kinds)[number]}`),
    },
    {
      key: "powerClasses",
      title: "filters.powerClass",
      values: powerClasses,
      label: (value) => t(`power.${value as (typeof powerClasses)[number]}`),
    },
  ];

  function kept(group: Group): Set<string> {
    return filters[group] as Set<string>;
  }

  function toggle(group: Group, value: string) {
    const next = new Set(kept(group));
    if (next.has(value)) {
      next.delete(value);
    } else {
      next.add(value);
    }
    onFilters({ ...filters, [group]: next });
  }

  function keepAll(group: Group) {
    onFilters({ ...filters, [group]: allFilters()[group] });
  }

  const current = groups.find((group) => group.key === open);

  return (
    <div
      ref={root}
      className="pointer-events-none absolute top-4 right-[15rem] left-[26.25rem] z-[4] max-[64rem]:top-[4.25rem] max-[64rem]:right-[4.5rem] phone:right-0 phone:left-0"
    >
      <ul className="pointer-events-auto flex w-fit max-w-full flex-wrap gap-2 p-1 phone:flex-nowrap phone:overflow-x-auto phone:px-3 [scrollbar-width:none]">
        {groups.map((group) => {
          const values = kept(group.key);
          const full = values.size === group.values.length;
          const only =
            values.size === 1 ? group.label([...values][0] ?? "") : undefined;
          return (
            <li key={group.key} className="flex-none">
              <button
                type="button"
                className={`flex h-10 cursor-pointer items-center gap-1.5 rounded-full px-4 text-[0.875rem] font-bold whitespace-nowrap shadow-float ${
                  full
                    ? "bg-surface text-foreground"
                    : "bg-foreground text-surface"
                }`}
                aria-expanded={open === group.key}
                aria-controls={open === group.key ? listId : undefined}
                onClick={(event) => {
                  if (open === group.key) {
                    setOpen(null);
                    return;
                  }
                  const chip = event.currentTarget;
                  const box = root.current?.getBoundingClientRect();
                  const rect = chip.getBoundingClientRect();
                  // Under its chip, but within the screen, with a margin.
                  const margin = 12;
                  setLeft(
                    box
                      ? Math.max(
                          margin - box.left,
                          Math.min(
                            rect.left - box.left,
                            window.innerWidth - margin - box.left - listWidth,
                          ),
                        )
                      : 0,
                  );
                  setMaxHeight(
                    Math.max(
                      120,
                      window.innerHeight - (box?.bottom ?? 0) - 2 * margin,
                    ),
                  );
                  setOpen(group.key);
                }}
              >
                {t(group.title)}
                {!full && (
                  <span className="font-semibold opacity-80">
                    {only ?? `${values.size}/${group.values.length}`}
                  </span>
                )}
                <svg
                  viewBox="0 0 24 24"
                  width="14"
                  height="14"
                  aria-hidden="true"
                >
                  <path
                    d="M6 9l6 6 6-6"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2.4"
                  />
                </svg>
              </button>
            </li>
          );
        })}
      </ul>
      {current && (
        <fieldset
          id={listId}
          className="pointer-events-auto absolute top-full mt-1 w-72 max-w-[calc(100vw-1.5rem)] overflow-y-auto rounded-3xl bg-surface p-2 shadow-card phone:right-3 phone:left-3! phone:w-auto"
          style={{ left, maxHeight }}
        >
          <legend className="sr-only">{t(current.title)}</legend>
          <ul>
            {current.values.map((value) => {
              const pressed = kept(current.key).has(value);
              return (
                <li key={value}>
                  <button
                    type="button"
                    aria-pressed={pressed}
                    className="flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-2xl px-3 text-left text-[0.9375rem] hover:bg-field"
                    onClick={() => toggle(current.key, value)}
                  >
                    <span
                      className={`grid size-5 flex-none place-items-center rounded-md border-2 ${
                        pressed
                          ? "border-accent bg-accent text-accent-foreground"
                          : "border-border"
                      }`}
                    >
                      {pressed && (
                        <svg
                          viewBox="0 0 24 24"
                          width="12"
                          height="12"
                          aria-hidden="true"
                        >
                          <path
                            d="M5 12l5 5 9-10"
                            fill="none"
                            stroke="currentColor"
                            strokeWidth="3"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                          />
                        </svg>
                      )}
                    </span>
                    {current.key === "operators" && (
                      <span
                        className={operatorDot}
                        style={{
                          background:
                            operatorColors[
                              value as keyof typeof operatorColors
                            ],
                        }}
                      />
                    )}
                    <span className="font-semibold">
                      {current.label(value)}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
          {kept(current.key).size < current.values.length && (
            <button
              type="button"
              className="mt-1 flex min-h-11 w-full cursor-pointer items-center justify-center rounded-2xl bg-field text-[0.875rem] font-bold text-accent"
              onClick={() => keepAll(current.key)}
            >
              {t("filters.everything")}
            </button>
          )}
        </fieldset>
      )}
    </div>
  );
}
