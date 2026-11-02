import { Job } from "../types";

export function CompanyLogo({ job, small = false }: { job: Job; small?: boolean }) {
  return <div className={`company-logo ${job.logoTone} ${small ? "small" : ""}`}>{job.logo}</div>;
}
