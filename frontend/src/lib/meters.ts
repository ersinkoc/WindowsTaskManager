export function meterBarClassName(value: number): string {
  const clamped = Math.max(0, Math.min(100, value));
  if (clamped >= 100) return "meter-bar w-full";
  if (clamped >= 90)  return "meter-bar w-[90%]";
  if (clamped >= 80)  return "meter-bar w-4/5";
  if (clamped >= 75)  return "meter-bar w-3/4";
  if (clamped >= 66)  return "meter-bar w-2/3";
  if (clamped >= 60)  return "meter-bar w-3/5";
  if (clamped >= 50)  return "meter-bar w-1/2";
  if (clamped >= 40)  return "meter-bar w-2/5";
  if (clamped >= 33)  return "meter-bar w-1/3";
  if (clamped >= 25)  return "meter-bar w-1/4";
  if (clamped >= 20)  return "meter-bar w-1/5";
  if (clamped >= 10)  return "meter-bar w-[12%]";
  return "meter-bar w-[6%]";
}