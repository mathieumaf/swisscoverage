import { type ReactNode, useEffect, useState } from "react";
import { browserTimeoutMs, createApiClient } from "../api/client";
import { formatDate, type Locale } from "../i18n/locales";
import { translator } from "../i18n/translate";
import { describePeriod, periodFields, positionText } from "./describe";
import { Sheet } from "./Sheet";
import {
  mutedText,
  operatorColors,
  operatorDot,
  operatorLine,
  type StationHistory,
  stationPath,
} from "./stations";

interface Props {
  id: string;
  locale: Locale;
  apiUrl: string;
  onClose: () => void;
  // When the sheet was opened from the list of stations at one place.
  back?: { label: string; onBack: () => void } | undefined;
}

type State =
  | { status: "loading" }
  | { status: "error" }
  | { status: "ready"; station: StationHistory };

// The sheet of one station, read from GET /v1/stations/{id}: what OFCOM
// publishes of it today, or last published. Its page has the history.
export function StationSheet({ id, locale, apiUrl, onClose, back }: Props) {
  const [state, setState] = useState<State>({ status: "loading" });
  const t = translator(locale);

  useEffect(() => {
    let cancelled = false;
    const api = createApiClient(apiUrl, browserTimeoutMs);
    api
      .GET("/v1/stations/{id}", {
        params: { path: { id }, query: { limit: 1 } },
      })
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
  const text = current && describePeriod(locale, current);

  return (
    <Sheet
      title={station?.name ?? "…"}
      eyebrow={
        station && (
          <p className={operatorLine}>
            <span
              className={operatorDot}
              style={{ background: operatorColors[station.operator] }}
            />
            {t(`operator.${station.operator}`)}
          </p>
        )
      }
      back={back}
      closeLabel={t("sheet.close")}
      onClose={onClose}
    >
      {state.status === "loading" && (
        <p role="status" className="my-[1em]">
          {t("sheet.loading")}
        </p>
      )}
      {state.status === "error" && (
        <p role="alert" className="my-[1em]">
          {t("sheet.error")}
        </p>
      )}

      {station && current && text && (
        <>
          {!station.published && (
            <p className="my-[1em] rounded-lg bg-border px-3 py-2">
              {t("sheet.notPublished")}
            </p>
          )}
          <dl className="grid grid-cols-[9rem_1fr] gap-x-4 gap-y-2 [&>dt]:text-muted">
            {periodFields.map(({ key, label }) => (
              <Field key={key} label={t(label)}>
                {key === "technologies" && current.technologies.length > 0 ? (
                  <ul className="flex flex-wrap gap-1">
                    {current.technologies.map((technology) => (
                      <li
                        key={technology}
                        className="rounded-md bg-border px-2 font-semibold"
                      >
                        {technology}
                      </li>
                    ))}
                  </ul>
                ) : (
                  text[key]
                )}
              </Field>
            ))}
            <Field label={t("sheet.position")}>
              {positionText(station).wgs84}
              <br />
              <span className={mutedText}>{positionText(station).lv95}</span>
            </Field>
            <Field label={t("sheet.firstSeen")}>
              {formatDate(locale, new Date(station.firstSeen))}
            </Field>
          </dl>

          <p className="my-[1em]">
            <a href={stationPath(locale, station.id)}>{t("sheet.page")}</a>
          </p>
          <p className={`my-[1em] ${mutedText}`}>{t("footer.source")}</p>
        </>
      )}
    </Sheet>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt>{label}</dt>
      <dd>{children}</dd>
    </>
  );
}
