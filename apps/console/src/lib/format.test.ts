import { describe, expect, it } from "vitest";
import { formatCurrency, riskTone, signedPercent } from "./format";

describe("dashboard formatting", () => {
  it("formats rounded US dollars", () => {
    expect(formatCurrency(12480)).toBe("$12,480");
  });

  it("adds a sign only for positive percentages", () => {
    expect(signedPercent(31)).toBe("+31%");
    expect(signedPercent(-4)).toBe("-4%");
  });

  it("maps risk levels to semantic tones", () => {
    expect(riskTone("Critical")).toBe("danger");
    expect(riskTone("Watch")).toBe("warning");
    expect(riskTone("Healthy")).toBe("success");
  });
});
