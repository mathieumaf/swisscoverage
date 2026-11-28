import { formatNumber, type Locale } from "../i18n/locales";
import { translate } from "../i18n/translate";
import { Sheet } from "./Sheet";
import {
  lv95Of,
  type MapStation,
  mutedText,
  operatorColors,
  operatorDot,
} from "./stations";

interface Props {
  stations: MapStation[];
  locale: Locale;
  onChoose: (station: MapStation) => void;
  onClose: () => void;
}

// Several stations under one click, often several operators on one mast:
// the visitor picks the one whose sheet to open.
export function StationChoice({ stations, locale, onChoose, onClose }: Props) {
  return (
    <Sheet
      title={translate(locale, "choice.title", {
        count: formatNumber(locale, stations.length),
      })}
      closeLabel={translate(locale, "sheet.close")}
      onClose={onClose}
    >
      <ul className="mt-4 flex flex-col gap-2">
        {stations.map((station) => (
          <li key={station.id}>
            <button
              type="button"
              className="flex w-full cursor-pointer items-center gap-3 rounded-2xl bg-field px-4 py-3 text-left text-foreground hover:bg-border focus-visible:bg-border"
              onClick={() => onChoose(station)}
            >
              <span
                className={operatorDot}
                style={{ background: operatorColors[station.operator] }}
              />
              <span className="min-w-0">
                <strong className="wrap-anywhere">{station.name}</strong>
                <br />
                <span className={mutedText}>
                  {translate(locale, `operator.${station.operator}`)} ·{" "}
                  {lv95Of(station.id)}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    </Sheet>
  );
}
