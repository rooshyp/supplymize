import type { Kpi } from "../types";
import { Icon } from "./Icon";

export function KpiCard({ kpi }: { kpi: Kpi }) {
  return (
    <article className="kpi-card">
      <div className="kpi-card__top">
        <span className="kpi-card__icon"><Icon name={kpi.icon} size={19} /></span>
        <span className={`trend trend--${kpi.trend}`}>{kpi.change}</span>
      </div>
      <p>{kpi.label}</p>
      <strong>{kpi.value}</strong>
      <small>{kpi.detail}</small>
    </article>
  );
}
