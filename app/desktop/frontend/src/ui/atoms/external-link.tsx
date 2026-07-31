import type { ReactNode } from "react";
import { AnchorPrimitive, type AnchorPrimitiveProps } from "@/ui/primitives";

export type ExternalLinkProps = Omit<AnchorPrimitiveProps, "children" | "rel" | "target"> & {
  children: ReactNode;
};

export function ExternalLink({ children, ...props }: ExternalLinkProps) {
  return (
    <AnchorPrimitive {...props} target="_blank" rel="noopener noreferrer">
      {children}
    </AnchorPrimitive>
  );
}
