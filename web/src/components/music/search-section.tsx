import { ExpandableSection } from "@/components/expandable-section";
import React from "react";
export function SearchSection({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: React.ReactNode;
}) {
  return (
    <section aria-label={title}>
      <ExpandableSection title={title} count={count}>
        {children}
      </ExpandableSection>
    </section>
  );
}
