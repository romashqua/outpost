import { Routes, Route, Navigate } from 'react-router-dom'
import { useAuthStore } from '@/store/auth'
import Layout from '@/components/Layout'
import ToastContainer from '@/components/ui/Toast'
import LoginPage from '@/pages/LoginPage'
import DashboardPage from '@/pages/DashboardPage'
import UsersPage from '@/pages/UsersPage'
import NetworksPage from '@/pages/NetworksPage'
import DevicesPage from '@/pages/DevicesPage'
import GatewaysPage from '@/pages/GatewaysPage'
import S2SPage from '@/pages/S2SPage'
import AnalyticsPage from '@/pages/AnalyticsPage'
import CompliancePage from '@/pages/CompliancePage'
import SettingsPage from '@/pages/SettingsPage'
import SmartRoutesPage from '@/pages/SmartRoutesPage'
import GroupsPage from '@/pages/GroupsPage'
import ZTNAPage from '@/pages/ZTNAPage'
import TenantsPage from '@/pages/TenantsPage'
import DocsPage from '@/pages/DocsPage'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <>
    <ToastContainer />
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/users" element={<UsersPage />} />
                <Route path="/groups" element={<GroupsPage />} />
                <Route path="/networks" element={<NetworksPage />} />
                <Route path="/devices" element={<DevicesPage />} />
                <Route path="/gateways" element={<GatewaysPage />} />
                <Route path="/s2s" element={<S2SPage />} />
                <Route path="/smart-routes" element={<SmartRoutesPage />} />
                <Route path="/analytics" element={<AnalyticsPage />} />
                <Route path="/compliance" element={<CompliancePage />} />
                <Route path="/ztna" element={<ZTNAPage />} />
                <Route path="/tenants" element={<TenantsPage />} />
                <Route path="/docs" element={<DocsPage />} />
                <Route path="/settings" element={<SettingsPage />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
    </>
  )
}
