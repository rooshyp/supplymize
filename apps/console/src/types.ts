export type Trend = "up" | "down" | "steady";

export interface Kpi {
  label: string;
  value: string;
  change: string;
  trend: Trend;
  detail: string;
  icon: "target" | "savings" | "truck" | "leaf";
}

export interface ForecastPoint {
  label: string;
  actual?: number;
  forecast?: number;
  lower?: number;
  upper?: number;
}

export type RiskLevel = "Critical" | "Watch" | "Healthy";

export interface InventoryRisk {
  sku: string;
  name: string;
  location: string;
  daysCover: number;
  demandDelta: number;
  risk: RiskLevel;
}

export interface NetworkNode {
  id: string;
  label: string;
  kind: "supplier" | "warehouse" | "customer";
  x: number;
  y: number;
  utilization: number;
}

export interface NetworkLane {
  from: string;
  to: string;
  status: "flowing" | "watch";
}
