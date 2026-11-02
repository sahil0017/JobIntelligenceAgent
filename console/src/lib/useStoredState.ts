import { useEffect, useState } from "react";
import { Job } from "../types";

export function useStoredState<T>(key: string, initialValue: T) {
  const [value, setValue] = useState<T>(() => {
    try {
      const stored = localStorage.getItem(key);
      if (!stored) return initialValue;
      const parsed = JSON.parse(stored) as T;
      if (key === "careerpilot.jobs" && Array.isArray(parsed)) {
        return parsed.filter((job: Job) => !/^job-[1-6]$/.test(job.id) || !["Wellfound", "Careers page", "Instahyre"].includes(job.source)) as T;
      }
      return parsed;
    } catch (error) {
      console.error(`Unable to load ${key} from local storage.`, error);
      return initialValue;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch (error) {
      console.error(`Unable to save ${key} to local storage.`, error);
    }
  }, [key, value]);
  return [value, setValue] as const;
}
