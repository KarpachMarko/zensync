import { AppLink } from "@/components/navigation";
import { Breadcrumb, BreadcrumbList, BreadcrumbItem, BreadcrumbLink, BreadcrumbPage, BreadcrumbSeparator } from "@/components/ui/breadcrumb";

export function UtilitiesBreadcrumbs({ current }: { current: string }) {
  return <Breadcrumb className="mb-[22px]">
    <BreadcrumbList className="m-0 list-none p-0 text-[13px]">
      <BreadcrumbItem><BreadcrumbLink asChild><AppLink to="/utilities" end>Utilities</AppLink></BreadcrumbLink></BreadcrumbItem>
      <BreadcrumbSeparator />
      <BreadcrumbItem><BreadcrumbPage>{current}</BreadcrumbPage></BreadcrumbItem>
    </BreadcrumbList>
  </Breadcrumb>;
}
