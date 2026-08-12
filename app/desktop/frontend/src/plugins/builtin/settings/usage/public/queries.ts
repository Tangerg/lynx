// Query identity is the only Usage detail other bounded contexts need: Runtime
// invalidations can refresh the aggregate without importing its gateway or UI.
export { USAGE_SUMMARY_KEY } from "../application/usageConfig";
