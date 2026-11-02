import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
    mutations: { retry: false, gcTime: 0 },
  },
});
