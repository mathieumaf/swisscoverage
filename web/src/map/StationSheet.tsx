import { useEffect, useRef, useState } from "react";
import { browserTimeoutMs, createApiClient } from "../api/client";
import { formatDate, formatNumber, type Locale } from "../i18n/locales";
import type { MessageKey } from "../i18n/messages/en";
import { translate } from "../i18n/translate";
import { operatorColors, type Period, type StationHistory } from "./stations";

interface Props {
  id: string;
  locale: Locale;
  apiUrl: string;
  onClose: () => void;
}

type State =
  | { status: "loading" }
  | { status: "error" }
  | { status: "ready"; station: StationHistory };

// The sheet of one station, read from GET /v1/stations/{id}: what OFCOM
// publishes of it today and what changed since the archive began.
export function StationSheet({ id, locale, apiUrl, onClose }: Props) {
  const [state, setState] = useState<State>({ status: "loading" });
  const closeButton = useRef<HTMLButtonElement>(null);
  const t = (key: MessageKey, values?: Record<string, string>) =>
    translate(locale, key, values);
  const date = (value: string) => formatDate(locale, new Date(value));

  useEffect(() => {
    let cancelled = false;
    const api = createApiClient(apiUrl, browserTimeoutMs);
    api
      .GET("/v1/stations/{id}", { params: { path: { id } } })
      .then(({ data, response }) => {
        if (data === undefined) {
          throw new Error(`The API answered ${response.status}`);
        }
        if (!cancelled) {
          setState({ status: "ready", station: data });
        }
      })
      .catch((cause: unknown) => {
        console.error("Loading the station failed:", cause);
        if (!cancelled) {
          setState({ status: "error" });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [apiUrl, id]);

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

  const station = state.status === "ready" ? state.station : undefined;
  const current = station?.periods[0];

  return (
    <aside className="station-sheet" aria-labelledby="station-sheet-title">
      <header className="station-sheet__header">
        <div>
          {station && (
            <p className="station-sheet__operator">
              <span
                className="station-sheet__dot"
                style={{ background: operatorColors[station.operator] }}
              />
              {t(`operator.${station.operator}`)}
            </p>
          )}
          <h2 id="station-sheet-title">{station?.name ?? "…"}</h2>
        </div>
        <button
          ref={closeButton}
          type="button"
          className="station-sheet__close"
          onClick={onClose}
          aria-label={t("sheet.close")}
        >
          ×
        </button>
      </header>

      {state.status === "loading" && <p role="status">{t("sheet.loading")}</p>}
      {state.status === "error" && <p role="alert">{t("sheet.error")}</p>}

      {station && current && (
        <>
          {!station.published && (
            <p className="station-sheet__notice">{t("sheet.notPublished")}</p>
          )}
          <dl className="station-sheet__facts">
            <dt>{t("sheet.kind")}</dt>
            <dd>{t(`kind.${current.kind}`)}</dd>
            <dt>{t("sheet.powerClass")}</dt>
            <dd>{t(`power.${current.powerClass}`)}</dd>
            <dt>{t("sheet.technologies")}</dt>
            <dd>
              {current.technologies.length === 0 ? (
                t("sheet.noTechnology")
              ) : (
                <ul className="station-sheet__chips">
                  {current.technologies.map((technology) => (
                    <li key={technology}>{technology}</li>
                  ))}
                </ul>
              )}
            </dd>
            <dt>{t("sheet.adaptive")}</dt>
            <dd>{adaptive(current, t)}</dd>
            <dt>{t("sheet.limitValue")}</dt>
            <dd>
              {current.installationLimitValue === null
                ? t("sheet.unknown")
                : `${formatNumber(locale, current.installationLimitValue, {
                    minimumFractionDigits: 1,
                  })} V/m`}
            </dd>
            <dt>{t("sheet.permit")}</dt>
            <dd>{permit(current, t, date)}</dd>
            <dt>{t("sheet.position")}</dt>
            <dd>
              {station.position.lat.toFixed(5)},{" "}
              {station.position.lon.toFixed(5)}
              <br />
              <span className="station-sheet__muted">
                LV95 {station.lv95.east}, {station.lv95.north}
              </span>
            </dd>
            <dt>{t("sheet.firstSeen")}</dt>
            <dd>{date(station.firstSeen)}</dd>
          </dl>

          {station.periods.length > 1 && (
            <section>
              <h3>{t("sheet.history")}</h3>
              <ol className="station-sheet__history">
                {station.periods.map((period) => (
                  <li key={period.from}>
                    <strong>
                      {period.to === null
                        ? t("sheet.since", { date: date(period.from) })
                        : t("sheet.between", {
                            from: date(period.from),
                            to: date(period.to),
                          })}
                    </strong>
                    <br />
                    {[
                      t(`kind.${period.kind}`),
                      t(`power.${period.powerClass}`),
                      period.technologies.join(", ") || t("sheet.noTechnology"),
                    ].join(" · ")}
                  </li>
                ))}
              </ol>
            </section>
          )}

          <p className="station-sheet__source">{t("footer.source")}</p>
        </>
      )}
    </aside>
  );
}

function adaptive(period: Period, t: (key: MessageKey) => string): string {
  if (period.adaptive === null) {
    return t("sheet.unknown");
  }
  return period.adaptive ? t("adaptive.yes") : t("adaptive.no");
}

function permit(
  period: Period,
  t: (key: MessageKey, values?: Record<string, string>) => string,
  date: (value: string) => string,
): string {
  if (period.permit === null) {
    return t("sheet.unknown");
  }
  if (period.permit.type === "exempt") {
    return t("permit.exempt");
  }
  return period.permit.date === undefined
    ? t("permit.site_data_sheet")
    : t("permit.site_data_sheet_dated", { date: date(period.permit.date) });
}
