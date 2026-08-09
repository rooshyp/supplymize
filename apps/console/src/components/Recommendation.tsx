import { formatCurrency } from "../lib/format";
import { Icon } from "./Icon";

export function Recommendation() {
  return (
    <article className="recommendation">
      <div className="recommendation__glow" />
      <div className="recommendation__header">
        <span className="ai-chip"><Icon name="sparkles" size={15} /> SupplyMize AI</span>
        <span className="confidence">94% confidence</span>
      </div>
      <p className="eyebrow">Recommended action</p>
      <h2>Rebalance inventory before the weekend surge.</h2>
      <p className="recommendation__copy">Move <strong>160 units of SM-1042</strong> from Reno to Oakland by Thursday. This protects service levels while avoiding an expedited inbound order.</p>
      <div className="recommendation__route">
        <span><i className="route-node" /><small>From</small><strong>Reno DC</strong></span>
        <span className="route-line"><i /><Icon name="truck" size={20} /><i /></span>
        <span><i className="route-node route-node--end" /><small>To</small><strong>Oakland DC</strong></span>
      </div>
      <div className="recommendation__impact">
        <span><small>Expected savings</small><strong>{formatCurrency(12480)}</strong></span>
        <span><small>Service-level lift</small><strong>+6.3 pts</strong></span>
        <span><small>Added emissions</small><strong>0.18 t CO₂e</strong></span>
      </div>
      <div className="recommendation__actions">
        <button type="button" className="primary-button">Review plan <Icon name="arrow" size={15} /></button>
        <button type="button" className="text-button">Dismiss</button>
      </div>
    </article>
  );
}
