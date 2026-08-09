import type { RiskLevel } from "../types";

export function formatCurrency(value: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(value);
}

export function signedPercent(value: number): string {
  return `${value > 0 ? "+" : ""}${value}%`;
}

export function riskTone(risk: RiskLevel): "danger" | "warning" | "success" {
  if (risk === "Critical") return "danger";
  if (risk === "Watch") return "warning";
  return "success";
}
