import { networkLanes, networkNodes } from "../data/demo";
import { Icon } from "./Icon";

export function NetworkMap() {
  const byId = Object.fromEntries(networkNodes.map((node) => [node.id, node]));
  return (
    <article className="panel network-panel">
      <div className="panel__header panel__header--dark">
        <div>
          <p className="eyebrow">Control tower</p>
          <h2>Network health</h2>
        </div>
        <button type="button" className="dark-link">Open network <Icon name="arrow" size={15} /></button>
      </div>
      <div className="network-canvas">
        <div className="map-grid" />
        <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
          {networkLanes.map((lane) => {
            const start = byId[lane.from];
            const end = byId[lane.to];
            return (
              <line
                key={`${lane.from}-${lane.to}`}
                x1={start.x}
                y1={start.y}
                x2={end.x}
                y2={end.y}
                className={`network-lane network-lane--${lane.status}`}
              />
            );
          })}
        </svg>
        {networkNodes.map((node) => (
          <button
            type="button"
            className={`network-node network-node--${node.kind} ${node.utilization > 90 ? "network-node--alert" : ""}`}
            style={{ left: `${node.x}%`, top: `${node.y}%` }}
            key={node.id}
            aria-label={`${node.label}, ${node.utilization}% utilized`}
          >
            <span className="network-node__point" />
            <span className="network-node__label"><strong>{node.label}</strong><small>{node.utilization}% utilized</small></span>
          </button>
        ))}
        <div className="network-alert"><Icon name="warning" size={15} /><span><strong>2 constraints</strong> need attention</span></div>
      </div>
      <div className="network-footer">
        <span><i className="node-key node-key--supplier" /> Supplier</span>
        <span><i className="node-key node-key--warehouse" /> Distribution center</span>
        <span><i className="node-key node-key--customer" /> Demand region</span>
      </div>
    </article>
  );
}
