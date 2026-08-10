import { describe, expect, it } from "vitest";
import type { Invoice } from "./types";
import { filterInvoices } from "./invoices";

const invoices = [
  { id: "INV-1001", customerName: "Rina Kartika", status: "paid" },
  { id: "INV-1002", customerName: "Budi Santoso", status: "pending" },
] as Invoice[];

describe("filterInvoices", () => {
  it("matches invoice id or customer name case-insensitively", () => {
    expect(filterInvoices(invoices, "rina", "all")).toEqual([invoices[0]]);
    expect(filterInvoices(invoices, "1002", "all")).toEqual([invoices[1]]);
  });

  it("filters by status", () => {
    expect(filterInvoices(invoices, "", "paid")).toEqual([invoices[0]]);
  });
});
