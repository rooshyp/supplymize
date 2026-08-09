import { inventoryRisks } from "../data/demo";
import { riskTone, signedPercent } from "../lib/format";
import { Icon } from "./Icon";

export function RiskTable() {
  return (
    <article className="panel risk-panel">
      <div className="panel__header">
        <div>
          <p className="eyebrow">Exception management</p>
          <h2>Inventory at risk</h2>
        </div>
        <button type="button" className="text-link">View all <Icon name="arrow" size={14} /></button>
      </div>
      <div className="risk-table" role="table" aria-label="Inventory at risk">
        <div className="risk-table__row risk-table__head" role="row">
          <span role="columnheader">Product</span>
          <span role="columnheader">Cover</span>
          <span role="columnheader">Demand</span>
          <span role="columnheader">Status</span>
          <span />
        </div>
        {inventoryRisks.map((item) => (
          <div className="risk-table__row" role="row" key={item.sku}>
            <span className="product-cell" role="cell"><i>{item.name.slice(0, 1)}</i><span><strong>{item.name}</strong><small>{item.sku} · {item.location}</small></span></span>
            <span role="cell"><strong>{item.daysCover}</strong><small>days</small></span>
            <span className={item.demandDelta > 0 ? "demand-up" : "demand-down"} role="cell">{signedPercent(item.demandDelta)}</span>
            <span role="cell"><span className={`status-pill status-pill--${riskTone(item.risk)}`}>{item.risk}</span></span>
            <button type="button" className="row-action" aria-label={`View ${item.name}`}><Icon name="chevron" size={15} /></button>
          </div>
        ))}
      </div>
    </article>
  );
}
