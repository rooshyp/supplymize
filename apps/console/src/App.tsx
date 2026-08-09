import { ForecastChart } from "./components/ForecastChart";
import { Icon } from "./components/Icon";
import { KpiCard } from "./components/KpiCard";
import { NetworkMap } from "./components/NetworkMap";
import { Recommendation } from "./components/Recommendation";
import { RiskTable } from "./components/RiskTable";
import { Shell } from "./components/Shell";
import { kpis } from "./data/demo";

export default function App() {
  return (
    <Shell>
      <section className="dashboard">
        <div className="page-heading">
          <div>
            <p className="eyebrow">Saturday, August 8</p>
            <h1>Good morning, Aarush.</h1>
            <p>Your network is stable. SupplyMize found <strong>3 opportunities</strong> worth reviewing.</p>
          </div>
          <div className="page-heading__actions">
            <button type="button" className="secondary-button"><Icon name="calendar" size={16} /> Aug 8 – Sep 4</button>
            <button type="button" className="primary-button"><Icon name="sparkles" size={16} /> Run scenario</button>
          </div>
        </div>
        <div className="kpi-grid">
          {kpis.map((kpi) => <KpiCard kpi={kpi} key={kpi.label} />)}
        </div>
        <div className="dashboard-grid dashboard-grid--top">
          <ForecastChart />
          <NetworkMap />
        </div>
        <div className="dashboard-grid dashboard-grid--bottom">
          <Recommendation />
          <RiskTable />
        </div>
        <footer className="dashboard-footer">
          <span>SupplyMize planning cycle #4821</span>
          <span>Data synced 12 min ago · 8 sources healthy</span>
        </footer>
      </section>
    </Shell>
  );
}
