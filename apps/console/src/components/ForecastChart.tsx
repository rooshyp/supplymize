import { forecast } from "../data/demo";
import { Icon } from "./Icon";

const width = 760;
const height = 250;
const pad = { top: 18, right: 18, bottom: 34, left: 44 };
const minValue = 100;
const maxValue = 210;

function xAt(index: number) {
  return pad.left + (index * (width - pad.left - pad.right)) / (forecast.length - 1);
}

function yAt(value: number) {
  return pad.top + ((maxValue - value) * (height - pad.top - pad.bottom)) / (maxValue - minValue);
}

function pathFor(key: "actual" | "forecast") {
  return forecast
    .map((point, index) => ({ value: point[key], index }))
    .filter((point): point is { value: number; index: number } => point.value !== undefined)
    .map((point, sequence) => `${sequence === 0 ? "M" : "L"}${xAt(point.index)} ${yAt(point.value)}`)
    .join(" ");
}

function confidenceBand() {
  const band = forecast
    .map((point, index) => ({ ...point, index }))
    .filter((point): point is typeof point & { lower: number; upper: number } => point.lower !== undefined && point.upper !== undefined);
  const upper = band.map((point) => `${xAt(point.index)},${yAt(point.upper)}`).join(" ");
  const lower = [...band].reverse().map((point) => `${xAt(point.index)},${yAt(point.lower)}`).join(" ");
  return `${upper} ${lower}`;
}

export function ForecastChart() {
  return (
    <article className="panel forecast-panel">
      <div className="panel__header">
        <div>
          <p className="eyebrow">Demand intelligence</p>
          <h2>14-day demand forecast</h2>
        </div>
        <div className="panel__actions">
          <div className="legend" aria-label="Chart legend">
            <span><i className="legend__actual" /> Actual</span>
            <span><i className="legend__forecast" /> Forecast</span>
          </div>
          <button type="button" className="ghost-button"><Icon name="calendar" size={15} /> 30 days</button>
        </div>
      </div>
      <div className="forecast-summary">
        <strong>181 units</strong>
        <span className="trend trend--up">↑ 15.2%</span>
        <small>predicted peak · Aug 10</small>
      </div>
      <div className="chart-wrap" role="img" aria-label="Actual demand rises from 118 to 158 units and is forecast to peak at 181 units on August 10.">
        <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
          <defs>
            <linearGradient id="actualArea" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#168bd2" stopOpacity="0.22" />
              <stop offset="100%" stopColor="#168bd2" stopOpacity="0" />
            </linearGradient>
            <linearGradient id="confidenceArea" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor="#a94b2d" stopOpacity="0.08" />
              <stop offset="100%" stopColor="#a94b2d" stopOpacity="0.2" />
            </linearGradient>
          </defs>
          {[100, 125, 150, 175, 200].map((value) => (
            <g key={value}>
              <line x1={pad.left} y1={yAt(value)} x2={width - pad.right} y2={yAt(value)} className="chart-grid" />
              <text x={pad.left - 10} y={yAt(value) + 4} textAnchor="end" className="chart-axis">{value}</text>
            </g>
          ))}
          <polygon points={confidenceBand()} fill="url(#confidenceArea)" />
          <path d={`${pathFor("actual")} L${xAt(6)} ${height - pad.bottom} L${xAt(0)} ${height - pad.bottom} Z`} fill="url(#actualArea)" />
          <path d={pathFor("actual")} className="chart-line chart-line--actual" />
          <path d={pathFor("forecast")} className="chart-line chart-line--forecast" />
          {forecast.map((point, index) => {
            const value = point.actual ?? point.forecast;
            if (value === undefined) return null;
            return <circle key={point.label} cx={xAt(index)} cy={yAt(value)} r="3.8" className={point.actual !== undefined ? "chart-dot chart-dot--actual" : "chart-dot chart-dot--forecast"} />;
          })}
          {forecast.map((point, index) => index % 2 === 0 && (
            <text key={point.label} x={xAt(index)} y={height - 10} textAnchor="middle" className="chart-axis chart-axis--x">{point.label.replace(" ", "\n")}</text>
          ))}
          <line x1={xAt(6)} y1={pad.top} x2={xAt(6)} y2={height - pad.bottom} className="chart-today" />
          <text x={xAt(6) + 7} y={pad.top + 8} className="chart-today-label">TODAY</text>
        </svg>
      </div>
      <div className="signal-row">
        <span><i className="signal-dot signal-dot--blue" /> Seasonality <strong>+8.1%</strong></span>
        <span><i className="signal-dot signal-dot--rust" /> Weather <strong>+4.7%</strong></span>
        <span><i className="signal-dot signal-dot--slate" /> Campaign <strong>+2.4%</strong></span>
        <span className="signal-row__confidence"><Icon name="check" size={14} /> 91% confidence</span>
      </div>
    </article>
  );
}
