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
  const position = station && positionText(station);

  return (
    <Sheet
      title={station?.name ?? "…"}
      eyebrow={
        station &&
        text && (
          <p className={operatorLine}>
            <span
              className={operatorDot}
              style={{ background: operatorColors[station.operator] }}
            />
            {t(`operator.${station.operator}`)} · {text.kind}
          </p>
        )
      }
      meta={
        current &&
        current.technologies.length > 0 && (
          <ul
            className="mt-2 flex flex-wrap gap-1.5 text-[0.8125rem] font-bold"
            aria-label={t("sheet.technologies")}
          >
            {current.technologies.map((technology) => (
              <li key={technology} className="rounded-lg bg-field px-2 py-0.5">
                {technology}
              </li>
            ))}
          </ul>
        )
      }
      back={back}
      closeLabel={t("sheet.close")}
      onClose={onClose}
    >
      {state.status === "loading" && (
        <p role="status" className="mt-4 text-muted">
          {t("sheet.loading")}
        </p>
      )}
      {state.status === "error" && (
        <p role="alert" className="mt-4">
          {t("sheet.error")}
        </p>
      )}

      {station && current && text && position && (
        <>
          <Actions
            locale={locale}
            pagePath={stationPath(locale, station.id)}
            title={station.name}
          />
          {!station.published && (
            <p className="mt-4 rounded-2xl bg-notice px-4 py-3 text-[0.875rem] font-semibold text-notice-foreground">
              {t("sheet.notPublished")}
            </p>
          )}
          <dl className="mt-4 rounded-2xl bg-field px-4 text-[0.875rem]">
            {periodFields
              // The technologies show as chips under the title, or here when
              // none is recognised.
              .filter(
                ({ key }) =>
                  key !== "technologies" || current.technologies.length === 0,
              )
              .map(({ key, label }) => (
                <Field key={key} label={t(label)}>
                  {text[key]}
                </Field>
              ))}
            <Field label={t("sheet.position")}>
              {position.wgs84}
              <br />
              <span className={mutedText}>{position.lv95}</span>
            </Field>
            <Field label={t("sheet.firstSeen")}>
              {formatDate(locale, new Date(station.firstSeen))}
            </Field>
          </dl>
          <p className={`mt-4 ${mutedText}`}>{t("footer.source")}</p>
        </>
      )}
    </Sheet>
  );
}

// What can be done with a station: open its page, share it.
function Actions({
  locale,
  pagePath,
  title,
}: {
  locale: Locale;
  pagePath: string;
  title: string;
}) {
  const t = translator(locale);
  const [copied, setCopied] = useState(false);
  const action =
    "flex min-h-16 cursor-pointer flex-col items-center justify-center gap-1 rounded-2xl text-[0.75rem] font-bold no-underline";

  async function share() {
    const url = new URL(pagePath, window.location.origin).href;
    try {
      if (navigator.share) {
        await navigator.share({ title, url });
      } else {
        await navigator.clipboard.writeText(url);
        setCopied(true);
      }
    } catch {
      // The visitor cancelled, or the browser refused: nothing to say.
    }
  }

  return (
    <>
      <div className="mt-4 grid grid-cols-2 gap-2">
        <a
          className={`${action} bg-accent text-accent-foreground`}
          href={pagePath}
          title={t("sheet.page")}
        >
          <Icon d="M7 4h10a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2ZM9 9h6M9 13h6M9 17h3" />
          {t("sheet.actionPage")}
        </a>
        <button
          type="button"
          className={`${action} bg-field text-accent`}
          onClick={share}
        >
          <Icon d="M12 15V4M8 8l4-4 4 4M5 13v6h14v-6" />
          {t("sheet.share")}
        </button>
      </div>
      <p className="sr-only" role="status">
        {copied ? t("sheet.copied") : ""}
      </p>
      {copied && (
        <p className={`mt-2 text-center ${mutedText}`} aria-hidden="true">
          {t("sheet.copied")}
        </p>
      )}
    </>
  );
}

function Icon({ d }: { d: string }) {
  return (
    <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true">
      <path
        d={d}
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-border py-2.5 last:border-b-0">
      <dt className="text-muted">{label}</dt>
      <dd className="text-right font-semibold">{children}</dd>
    </div>
  );
}
