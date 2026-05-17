import { useEffect } from "react";
import type { Metric } from "web-vitals";

export function WebVitals() {
  useEffect(() => {
    const trackMetric = async (
      name: "LCP" | "CLS" | "FCP" | "INP",
      handler: (metric: Metric) => void,
    ) => {
      try {
        const webVitals = await import("web-vitals");
        const fn = webVitals[`on${name}`] as ((handler: (metric: Metric) => void) => void) | undefined;
        if (fn) {
          fn(handler);
        }
      } catch {
        // web-vitals not available in this environment
      }
    };

    const logMetric = (name: string) => (metric: Metric) => {
      console.log(`[WebVitals] ${name}: ${metric.value.toFixed(2)}ms (${metric.rating})`);
      if (metric.rating === "poor") {
        console.warn(`[WebVitals] ${name} needs improvement`);
      }
    };

    trackMetric("LCP", logMetric("LCP"));
    trackMetric("CLS", logMetric("CLS"));
    trackMetric("FCP", logMetric("FCP"));
    trackMetric("INP", logMetric("INP"));
  }, []);

  return null;
}