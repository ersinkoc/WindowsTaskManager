import { describe, expect, it } from "vitest";
import { formatBytes, formatPercent, formatRate } from "./format";

describe("formatBytes", () => {
  it("returns '0 B' for non-finite numbers", () => {
    expect(formatBytes(Number.NaN)).toBe("0 B");
    expect(formatBytes(Number.POSITIVE_INFINITY)).toBe("0 B");
    expect(formatBytes(Number.NEGATIVE_INFINITY)).toBe("0 B");
  });

  it("returns '0 B' for zero and negative numbers", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(-1)).toBe("0 B");
    expect(formatBytes(-1024)).toBe("0 B");
  });

  it("formats byte values (< 1024) with one decimal for small numbers, no decimal for large", () => {
    expect(formatBytes(1)).toBe("1.0 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1023)).toBe("1023 B");
  });

  it("formats KB values with single decimal when under 10", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(1024 * 9 + 512)).toBe("9.5 KB");
  });

  it("formats KB values with no decimals when 10 or higher", () => {
    expect(formatBytes(1024 * 10)).toBe("10 KB");
    expect(formatBytes(1024 * 12.34)).toBe("12 KB");
  });

  it("formats MB values", () => {
    expect(formatBytes(1024 ** 2)).toBe("1.0 MB");
    expect(formatBytes(1024 ** 2 * 250)).toBe("250 MB");
  });

  it("formats GB values", () => {
    expect(formatBytes(1024 ** 3)).toBe("1.0 GB");
    expect(formatBytes(1024 ** 3 * 16)).toBe("16 GB");
  });

  it("formats TB values", () => {
    expect(formatBytes(1024 ** 4)).toBe("1.0 TB");
    expect(formatBytes(1024 ** 4 * 8)).toBe("8.0 TB");
  });

  it("caps the unit index at the last unit for huge values", () => {
    // value way beyond TB — index should clamp to TB
    const huge = 1024 ** 6;
    expect(formatBytes(huge)).toMatch(/TB$/);
  });
});

describe("formatRate", () => {
  it("appends /s to a formatted byte value", () => {
    expect(formatRate(0)).toBe("0 B/s");
    expect(formatRate(1024)).toBe("1.0 KB/s");
    expect(formatRate(1024 ** 3 * 16)).toBe("16 GB/s");
  });
});

describe("formatPercent", () => {
  it("formats a number with one decimal and a percent sign", () => {
    expect(formatPercent(0)).toBe("0.0%");
    expect(formatPercent(7)).toBe("7.0%");
    expect(formatPercent(12.345)).toBe("12.3%");
    expect(formatPercent(100)).toBe("100.0%");
  });
});
