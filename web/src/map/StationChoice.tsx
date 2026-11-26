import { formatNumber, type Locale } from "../i18n/locales";
import { translate } from "../i18n/translate";
import { Sheet } from "./Sheet";
import { lv95Of, type MapStation, operatorColors } from "./stations";

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
      <ul className="station-choice">
        {stations.map((station) => (
          <li key={station.id}>
            <button type="button" onClick={() => onChoose(station)}>
              <span
                className="station-sheet__dot"
                style={{ background: operatorColors[station.operator] }}
              />
              <span>
                <strong>{station.name}</strong>
                <br />
                <span className="station-sheet__muted">
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
