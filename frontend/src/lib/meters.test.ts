import { describe, expect, it } from "vitest";
import { meterBarClassName } from "./meters";

describe("meterBarClassName", () => {
  it("clamps negative values to the minimum width class", () => {
    expect(meterBarClassName(-50)).toBe("meter-bar w-[6%]");
  });

  it("clamps values above 100 to the maximum width class", () => {
    expect(meterBarClassName(150)).toBe("meter-bar w-full");
    expect(meterBarClassName(101)).toBe("meter-bar w-full");
  });

  it("uses w-full at exactly 100", () => {
    expect(meterBarClassName(100)).toBe("meter-bar w-full");
  });

  it("uses w-[90%] for values in [90, 100)", () => {
    expect(meterBarClassName(90)).toBe("meter-bar w-[90%]");
    expect(meterBarClassName(95)).toBe("meter-bar w-[90%]");
    expect(meterBarClassName(99.9)).toBe("meter-bar w-[90%]");
  });

  it("uses w-4/5 for values in [80, 90)", () => {
    expect(meterBarClassName(80)).toBe("meter-bar w-4/5");
    expect(meterBarClassName(85)).toBe("meter-bar w-4/5");
    expect(meterBarClassName(89.9)).toBe("meter-bar w-4/5");
  });

  it("uses w-3/4 for values in [75, 80)", () => {
    expect(meterBarClassName(75)).toBe("meter-bar w-3/4");
    expect(meterBarClassName(78)).toBe("meter-bar w-3/4");
  });

  it("uses w-2/3 for values in [66, 75)", () => {
    expect(meterBarClassName(66)).toBe("meter-bar w-2/3");
    expect(meterBarClassName(70)).toBe("meter-bar w-2/3");
  });

  it("uses w-3/5 for values in [60, 66)", () => {
    expect(meterBarClassName(60)).toBe("meter-bar w-3/5");
    expect(meterBarClassName(64)).toBe("meter-bar w-3/5");
  });

  it("uses w-1/2 for values in [50, 60)", () => {
    expect(meterBarClassName(50)).toBe("meter-bar w-1/2");
    expect(meterBarClassName(55)).toBe("meter-bar w-1/2");
  });

  it("uses w-2/5 for values in [40, 50)", () => {
    expect(meterBarClassName(40)).toBe("meter-bar w-2/5");
    expect(meterBarClassName(45)).toBe("meter-bar w-2/5");
  });

  it("uses w-1/3 for values in [33, 40)", () => {
    expect(meterBarClassName(33)).toBe("meter-bar w-1/3");
    expect(meterBarClassName(37)).toBe("meter-bar w-1/3");
  });

  it("uses w-1/4 for values in [25, 33)", () => {
    expect(meterBarClassName(25)).toBe("meter-bar w-1/4");
    expect(meterBarClassName(30)).toBe("meter-bar w-1/4");
  });

  it("uses w-1/5 for values in [20, 25)", () => {
    expect(meterBarClassName(20)).toBe("meter-bar w-1/5");
    expect(meterBarClassName(24)).toBe("meter-bar w-1/5");
  });

  it("uses w-[12%] for values in [10, 20)", () => {
    expect(meterBarClassName(10)).toBe("meter-bar w-[12%]");
    expect(meterBarClassName(15)).toBe("meter-bar w-[12%]");
  });

  it("uses w-[6%] for values below 10", () => {
    expect(meterBarClassName(0)).toBe("meter-bar w-[6%]");
    expect(meterBarClassName(9.9)).toBe("meter-bar w-[6%]");
  });
});
