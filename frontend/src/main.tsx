import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import "./index.css";
import { App } from "./App";
import { ToastProvider } from "./components/Toast";
import { ThemeProvider } from "./components/ThemeProvider";

const queryClient = new QueryClient({
  defaultOptions: {
    // staleTime keeps data fresh across page navigations so revisiting a page
    // serves instantly from cache instead of refetching. gcTime holds the
    // cached data long enough to survive a round-trip away and back.
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 30_000, gcTime: 300_000 },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <ToastProvider>
            <App />
          </ToastProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);