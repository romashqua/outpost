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

function AdminRoute({ children }: { children: React.ReactNode }) {
  const role = useAuthStore((s) => s.user?.role)
  if (role !== 'admin') return <Navigate to="/" replace />
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
                <Route path="/users" element={<AdminRoute><UsersPage /></AdminRoute>} />
                <Route path="/groups" element={<AdminRoute><GroupsPage /></AdminRoute>} />
                <Route path="/networks" element={<AdminRoute><NetworksPage /></AdminRoute>} />
                <Route path="/devices" element={<DevicesPage />} />
                <Route path="/gateways" element={<AdminRoute><GatewaysPage /></AdminRoute>} />
                <Route path="/s2s" element={<AdminRoute><S2SPage /></AdminRoute>} />
                <Route path="/smart-routes" element={<AdminRoute><SmartRoutesPage /></AdminRoute>} />
                <Route path="/analytics" element={<AdminRoute><AnalyticsPage /></AdminRoute>} />
                <Route path="/compliance" element={<AdminRoute><CompliancePage /></AdminRoute>} />
                <Route path="/ztna" element={<AdminRoute><ZTNAPage /></AdminRoute>} />
                <Route path="/tenants" element={<AdminRoute><TenantsPage /></AdminRoute>} />
                <Route path="/docs" element={<DocsPage />} />
                <Route path="/settings" element={<AdminRoute><SettingsPage /></AdminRoute>} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
    </>
  )
}
