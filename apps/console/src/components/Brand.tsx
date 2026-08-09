export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className={`brand ${compact ? "brand--compact" : ""}`}>
      <span className="brand__mark" aria-hidden="true">
        <svg viewBox="0 0 44 44">
          <rect x="2" y="2" width="40" height="40" rx="12" fill="currentColor" />
          <path d="M11 28.5 18.2 21l5.2 5.1L33 15.5" fill="none" stroke="#071c2c" strokeWidth="3.7" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M27 15.5h6v6" fill="none" stroke="#071c2c" strokeWidth="3.7" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </span>
      {!compact && <span className="brand__name">SupplyMize</span>}
    </div>
  );
}
