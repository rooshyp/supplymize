import type {
  ForecastPoint,
  InventoryRisk,
  Kpi,
  NetworkLane,
  NetworkNode,
} from "../types";

export const kpis: Kpi[] = [
  {
    label: "Forecast accuracy",
    value: "93.8%",
    change: "+4.2%",
    trend: "up",
    detail: "MAPE · trailing 30 days",
    icon: "target",
  },
  {
    label: "Planned savings",
    value: "$128.4k",
    change: "+12.7%",
    trend: "up",
    detail: "vs. current operating plan",
    icon: "savings",
  },
  {
    label: "On-time delivery",
    value: "97.2%",
    change: "+1.8%",
    trend: "up",
    detail: "1,284 active shipments",
    icon: "truck",
  },
  {
    label: "Avoided emissions",
    value: "18.7 t",
    change: "-8.4%",
    trend: "down",
    detail: "CO₂e · this planning cycle",
    icon: "leaf",
  },
];

export const forecast: ForecastPoint[] = [
  { label: "Jul 14", actual: 118 },
  { label: "Jul 17", actual: 126 },
  { label: "Jul 20", actual: 121 },
  { label: "Jul 23", actual: 139 },
  { label: "Jul 26", actual: 147 },
  { label: "Jul 29", actual: 143 },
  { label: "Aug 01", actual: 158, forecast: 158, lower: 151, upper: 165 },
  { label: "Aug 04", forecast: 166, lower: 157, upper: 176 },
  { label: "Aug 07", forecast: 173, lower: 162, upper: 185 },
  { label: "Aug 10", forecast: 181, lower: 168, upper: 195 },
  { label: "Aug 13", forecast: 176, lower: 161, upper: 191 },
];

export const inventoryRisks: InventoryRisk[] = [
  {
    sku: "SM-1042",
    name: "Orbit running shoe",
    location: "Oakland DC",
    daysCover: 2.8,
    demandDelta: 31,
    risk: "Critical",
  },
  {
    sku: "SM-2388",
    name: "Terra hydration pack",
    location: "Reno DC",
    daysCover: 5.4,
    demandDelta: 18,
    risk: "Watch",
  },
  {
    sku: "SM-5180",
    name: "Alpine shell jacket",
    location: "Seattle DC",
    daysCover: 13.6,
    demandDelta: -4,
    risk: "Healthy",
  },
];

export const networkNodes: NetworkNode[] = [
  { id: "hcm", label: "Hồ Chí Minh", kind: "supplier", x: 12, y: 27, utilization: 74 },
  { id: "la", label: "Los Angeles", kind: "warehouse", x: 42, y: 62, utilization: 88 },
  { id: "reno", label: "Reno", kind: "warehouse", x: 61, y: 31, utilization: 96 },
  { id: "oak", label: "Oakland", kind: "warehouse", x: 72, y: 68, utilization: 63 },
  { id: "sea", label: "Seattle", kind: "customer", x: 89, y: 22, utilization: 81 },
  { id: "den", label: "Denver", kind: "customer", x: 91, y: 75, utilization: 69 },
];

export const networkLanes: NetworkLane[] = [
  { from: "hcm", to: "la", status: "flowing" },
  { from: "la", to: "reno", status: "watch" },
  { from: "la", to: "oak", status: "flowing" },
  { from: "reno", to: "sea", status: "flowing" },
  { from: "reno", to: "den", status: "watch" },
  { from: "oak", to: "den", status: "flowing" },
];
