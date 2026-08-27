import { PRODUCT_SLUG } from "@/product";

/** Identifies this client to the runtime in request metadata. */
export const DESKTOP_CLIENT_INFO = {
  name: `${PRODUCT_SLUG}-desktop`,
  version: "0.0.0",
} as const;
