import { useEffect, useState } from "react";
import { browserTimeoutMs, createApiClient } from "../api/client";
import { formatDate, formatNumber, type Locale } from "../i18n/locales";
import type { MessageKey } from "../i18n/messages/en";
import { translate } from "../i18n/translate";
import { Sheet } from "./Sheet";
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
// publishes of it today, or last published.
export function StationSheet({ id, locale, apiUrl, onClose }: Props) {
  const [state, setState] = useState<State>({ status: "loading" });
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

  const station = state.status === "ready" ? state.station : undefined;
  const current = station?.periods[0];

  return (
    <Sheet
      title={station?.name ?? "…"}
      eyebrow={
        station && (
          <p className="station-sheet__operator">
            <span
              className="station-sheet__dot"
              style={{ background: operatorColors[station.operator] }}
            />
            {t(`operator.${station.operator}`)}
          </p>
        )
      }
      closeLabel={t("sheet.close")}
      onClose={onClose}
    >
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

          <p className="station-sheet__source">{t("footer.source")}</p>
        </>
      )}
    </Sheet>
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
