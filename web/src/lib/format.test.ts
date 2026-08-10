import { describe, expect, it } from "vitest";
import { formatCurrency, formatRelativeTime } from "./format";

describe("formatCurrency", () => {
  it("formats integer rupiah without decimals", () => {
    expect(formatCurrency(185000)).toBe("Rp185.000");
  });
});

describe("formatRelativeTime", () => {
  it("describes a recent timestamp in minutes", () => {
    const now = new Date("2026-08-10T12:00:00Z");
    expect(formatRelativeTime("2026-08-10T11:52:00Z", now)).toBe("8 min ago");
  });
});
