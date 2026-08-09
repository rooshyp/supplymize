import type { PropsWithChildren } from "react";
import { Brand } from "./Brand";
import { Icon } from "./Icon";

const navItems = [
  { label: "Overview", icon: "chart" as const, active: true },
  { label: "Network", icon: "route" as const },
  { label: "Inventory", icon: "inventory" as const, badge: "3" },
  { label: "Scenarios", icon: "sparkles" as const },
];

export function Shell({ children }: PropsWithChildren) {
  return (
    <div className="shell">
      <aside className="sidebar">
        <Brand />
        <div className="sidebar__workspace">
          <span>Workspace</span>
          <button type="button" aria-label="Change workspace">
            North America <Icon name="chevron" size={14} />
          </button>
        </div>
        <nav aria-label="Primary navigation">
          <p className="nav-label">Plan</p>
          {navItems.map((item) => (
            <button
              type="button"
              className={`nav-item ${item.active ? "is-active" : ""}`}
              aria-current={item.active ? "page" : undefined}
              key={item.label}
            >
              <Icon name={item.icon} size={19} />
              <span>{item.label}</span>
              {item.badge && <span className="nav-item__badge">{item.badge}</span>}
            </button>
          ))}
          <p className="nav-label nav-label--second">Manage</p>
          <button type="button" className="nav-item">
            <Icon name="boxes" size={19} />
            <span>Data sources</span>
          </button>
          <button type="button" className="nav-item">
            <Icon name="settings" size={19} />
            <span>Settings</span>
          </button>
        </nav>
        <div className="sidebar__signal">
          <span className="sidebar__signal-icon"><Icon name="sparkles" size={17} /></span>
          <div>
            <strong>Model refreshed</strong>
            <span>12 minutes ago</span>
          </div>
        </div>
        <div className="sidebar__profile">
          <span className="avatar">AP</span>
          <span><strong>Aarush Parekh</strong><small>Administrator</small></span>
          <Icon name="chevron" size={14} />
        </div>
      </aside>
      <main className="main">
        <header className="topbar">
          <label className="search">
            <span className="sr-only">Search SupplyMize</span>
            <Icon name="search" size={18} />
            <input placeholder="Search products, lanes, locations…" />
            <kbd>⌘ K</kbd>
          </label>
          <div className="topbar__actions">
            <span className="live-status"><i /> Live data</span>
            <button type="button" className="icon-button" aria-label="Notifications">
              <Icon name="bell" size={19} />
              <span className="notification-dot" />
            </button>
          </div>
        </header>
        {children}
      </main>
    </div>
  );
}
