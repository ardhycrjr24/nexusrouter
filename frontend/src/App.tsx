import { lazy, Suspense } from "react";
import { Routes, Route } from "react-router-dom";
import { AuthGate } from "./components/AuthGate";
import { Layout } from "./components/Layout";
import { AdminBrandingProvider, PortalBrandingProvider } from "./contexts/BrandingContext";

// Routes are code-split: each page is a separate chunk loaded on demand. Heavy
// deps (recharts, @xyflow/react) ride along only with the pages that use them,
// so the first paint (Overview) no longer ships the whole app.
const named = <T extends string>(p: Promise<Record<T, React.ComponentType<unknown>>>, key: T) =>
  p.then((m) => ({ default: m[key] }));

const OverviewPage = lazy(() => named(import("./pages/Overview"), "OverviewPage"));
const ProvidersPage = lazy(() => named(import("./pages/Providers"), "ProvidersPage"));
const ProviderDetailPage = lazy(() => named(import("./pages/ProviderDetail"), "ProviderDetailPage"));
const ChainsPage = lazy(() => named(import("./pages/Chains"), "ChainsPage"));
const KeysPage = lazy(() => named(import("./pages/Keys"), "KeysPage"));
const PlansPage = lazy(() => named(import("./pages/Plans"), "PlansPage"));
const SettingsPage = lazy(() => named(import("./pages/Settings"), "SettingsPage"));
const EndpointsPage = lazy(() => named(import("./pages/Endpoints"), "EndpointsPage"));
const UsagePage = lazy(() => named(import("./pages/Usage"), "UsagePage"));
const QuotaPage = lazy(() => named(import("./pages/Quota"), "QuotaPage"));
const CLIToolsPage = lazy(() => named(import("./pages/CLITools"), "CLIToolsPage"));
const CLIToolDetailPage = lazy(() => named(import("./pages/CLIToolDetail"), "CLIToolDetailPage"));
const MediaProvidersPage = lazy(() => named(import("./pages/MediaProviders"), "MediaProvidersPage"));
const MediaProviderDetailPage = lazy(() => named(import("./pages/MediaProviderDetail"), "MediaProviderDetailPage"));
const ProxyPoolsPage = lazy(() => named(import("./pages/ProxyPools"), "ProxyPoolsPage"));
const SkillsPage = lazy(() => named(import("./pages/Skills"), "SkillsPage"));
const AgentsPage = lazy(() => named(import("./pages/Agents"), "AgentsPage"));
const ConsoleLogPage = lazy(() => named(import("./pages/ConsoleLog"), "ConsoleLogPage"));
const SystemPage = lazy(() => named(import("./pages/System"), "SystemPage"));
const OAuthCallbackPage = lazy(() => named(import("./pages/OAuthCallback"), "OAuthCallbackPage"));
const KeyPortalPage = lazy(() => named(import("./pages/KeyPortal"), "KeyPortalPage"));
const KeyDetailPage = lazy(() => named(import("./pages/KeyDetail"), "KeyDetailPage"));
const GuardrailsPage = lazy(() => named(import("./pages/Guardrails"), "GuardrailsPage"));

function PageFallback() {
  return (
    <div className="flex items-center justify-center h-full w-full py-24">
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-current border-t-transparent opacity-40" />
    </div>
  );
}

export function App() {
  return (
    <Suspense fallback={<PageFallback />}>
      <Routes>
        <Route path="portal" element={
          <PortalBrandingProvider>
            <KeyPortalPage />
          </PortalBrandingProvider>
        } />
        <Route path="*" element={
          <AuthGate>
            <AdminBrandingProvider>
            <Routes>
              {/* OAuth callback — standalone page, no sidebar layout */}
              <Route path="oauth/callback" element={<OAuthCallbackPage />} />
              <Route element={<Layout />}>
                <Route index element={<OverviewPage />} />
                <Route path="providers" element={<ProvidersPage />} />
                <Route path="providers/:id" element={<ProviderDetailPage />} />
                <Route path="endpoints" element={<EndpointsPage />} />
                <Route path="chains" element={<ChainsPage />} />
                <Route path="usage" element={<UsagePage />} />
                <Route path="quota" element={<QuotaPage />} />
                <Route path="cli-tools" element={<CLIToolsPage />} />
                <Route path="cli-tools/:toolId" element={<CLIToolDetailPage />} />
                <Route path="media" element={<MediaProvidersPage />} />
                <Route path="media/:kind" element={<MediaProvidersPage />} />
                <Route path="media/:kind/:id" element={<MediaProviderDetailPage />} />
                <Route path="proxy-pools" element={<ProxyPoolsPage />} />
                <Route path="skills" element={<SkillsPage />} />
                <Route path="agents" element={<AgentsPage />} />
                <Route path="console" element={<ConsoleLogPage />} />
                <Route path="keys" element={<KeysPage />} />
                <Route path="keys/:id" element={<KeyDetailPage />} />
                <Route path="guardrails" element={<GuardrailsPage />} />
                <Route path="plans" element={<PlansPage />} />
                <Route path="budgets" element={<PlansPage />} />
                <Route path="system" element={<SystemPage />} />
                <Route path="settings" element={<SettingsPage />} />
              </Route>
            </Routes>
            </AdminBrandingProvider>
          </AuthGate>
        } />
      </Routes>
    </Suspense>
  );
}
