import { useState, useMemo, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { clsx } from 'clsx'
import {
  BookOpen,
  Search,
  ChevronDown,
  ChevronRight,
  Rocket,
  Users,
  Network,
  Laptop2,
  KeyRound,
  Cable,
  Route,
  Shield,
  ShieldCheck,
  BarChart3,
  Settings,
  Container,
  Terminal,
  Wrench,
  Copy,
  Check,
  ExternalLink,
} from 'lucide-react'
import Card from '@/components/ui/Card'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface DocSection {
  id: string
  titleKey: string
  icon: React.ElementType
  subsections: DocSubsection[]
}

interface DocSubsection {
  id: string
  titleKey: string
  contentKey: string
}

/* ------------------------------------------------------------------ */
/*  Section definitions                                                */
/* ------------------------------------------------------------------ */

const docSections: DocSection[] = [
  {
    id: 'getting-started',
    titleKey: 'docs.gettingStarted.title',
    icon: Rocket,
    subsections: [
      { id: 'quick-start', titleKey: 'docs.gettingStarted.quickStart.title', contentKey: 'docs.gettingStarted.quickStart.content' },
      { id: 'architecture', titleKey: 'docs.gettingStarted.architecture.title', contentKey: 'docs.gettingStarted.architecture.content' },
      { id: 'first-login', titleKey: 'docs.gettingStarted.firstLogin.title', contentKey: 'docs.gettingStarted.firstLogin.content' },
    ],
  },
  {
    id: 'user-management',
    titleKey: 'docs.userManagement.title',
    icon: Users,
    subsections: [
      { id: 'creating-users', titleKey: 'docs.userManagement.creatingUsers.title', contentKey: 'docs.userManagement.creatingUsers.content' },
      { id: 'roles', titleKey: 'docs.userManagement.roles.title', contentKey: 'docs.userManagement.roles.content' },
      { id: 'groups-acls', titleKey: 'docs.userManagement.groupsAcls.title', contentKey: 'docs.userManagement.groupsAcls.content' },
      { id: 'ldap-sync', titleKey: 'docs.userManagement.ldapSync.title', contentKey: 'docs.userManagement.ldapSync.content' },
      { id: 'scim', titleKey: 'docs.userManagement.scim.title', contentKey: 'docs.userManagement.scim.content' },
    ],
  },
  {
    id: 'network-config',
    titleKey: 'docs.networkConfig.title',
    icon: Network,
    subsections: [
      { id: 'creating-networks', titleKey: 'docs.networkConfig.creatingNetworks.title', contentKey: 'docs.networkConfig.creatingNetworks.content' },
      { id: 'gateway-setup', titleKey: 'docs.networkConfig.gatewaySetup.title', contentKey: 'docs.networkConfig.gatewaySetup.content' },
      { id: 'wireguard-settings', titleKey: 'docs.networkConfig.wireguardSettings.title', contentKey: 'docs.networkConfig.wireguardSettings.content' },
    ],
  },
  {
    id: 'device-management',
    titleKey: 'docs.deviceManagement.title',
    icon: Laptop2,
    subsections: [
      { id: 'adding-devices', titleKey: 'docs.deviceManagement.addingDevices.title', contentKey: 'docs.deviceManagement.addingDevices.content' },
      { id: 'approve-revoke', titleKey: 'docs.deviceManagement.approveRevoke.title', contentKey: 'docs.deviceManagement.approveRevoke.content' },
      { id: 'download-config', titleKey: 'docs.deviceManagement.downloadConfig.title', contentKey: 'docs.deviceManagement.downloadConfig.content' },
      { id: 'key-rotation', titleKey: 'docs.deviceManagement.keyRotation.title', contentKey: 'docs.deviceManagement.keyRotation.content' },
    ],
  },
  {
    id: 'auth-sso',
    titleKey: 'docs.authSso.title',
    icon: KeyRound,
    subsections: [
      { id: 'built-in-oidc', titleKey: 'docs.authSso.builtInOidc.title', contentKey: 'docs.authSso.builtInOidc.content' },
      { id: 'external-oidc', titleKey: 'docs.authSso.externalOidc.title', contentKey: 'docs.authSso.externalOidc.content' },
      { id: 'saml', titleKey: 'docs.authSso.saml.title', contentKey: 'docs.authSso.saml.content' },
      { id: 'ldap-ad', titleKey: 'docs.authSso.ldapAd.title', contentKey: 'docs.authSso.ldapAd.content' },
      { id: 'mfa', titleKey: 'docs.authSso.mfa.title', contentKey: 'docs.authSso.mfa.content' },
      { id: 'wg-mfa', titleKey: 'docs.authSso.wgMfa.title', contentKey: 'docs.authSso.wgMfa.content' },
    ],
  },
  {
    id: 's2s-tunnels',
    titleKey: 'docs.s2sTunnels.title',
    icon: Cable,
    subsections: [
      { id: 'mesh-vs-hub', titleKey: 'docs.s2sTunnels.meshVsHub.title', contentKey: 'docs.s2sTunnels.meshVsHub.content' },
      { id: 'creating-tunnels', titleKey: 'docs.s2sTunnels.creatingTunnels.title', contentKey: 'docs.s2sTunnels.creatingTunnels.content' },
      { id: 'adding-members', titleKey: 'docs.s2sTunnels.addingMembers.title', contentKey: 'docs.s2sTunnels.addingMembers.content' },
      { id: 'route-mgmt', titleKey: 'docs.s2sTunnels.routeManagement.title', contentKey: 'docs.s2sTunnels.routeManagement.content' },
      { id: 'allowed-domains', titleKey: 'docs.s2sTunnels.allowedDomains.title', contentKey: 'docs.s2sTunnels.allowedDomains.content' },
      { id: 'config-generation', titleKey: 'docs.s2sTunnels.configGeneration.title', contentKey: 'docs.s2sTunnels.configGeneration.content' },
      { id: 's2s-client', titleKey: 'docs.s2sTunnels.s2sClient.title', contentKey: 'docs.s2sTunnels.s2sClient.content' },
      { id: 'systemd-setup', titleKey: 'docs.s2sTunnels.systemdSetup.title', contentKey: 'docs.s2sTunnels.systemdSetup.content' },
    ],
  },
  {
    id: 'smart-routes',
    titleKey: 'docs.smartRoutes.title',
    icon: Route,
    subsections: [
      { id: 'route-groups', titleKey: 'docs.smartRoutes.routeGroups.title', contentKey: 'docs.smartRoutes.routeGroups.content' },
      { id: 'actions', titleKey: 'docs.smartRoutes.actions.title', contentKey: 'docs.smartRoutes.actions.content' },
      { id: 'proxy-setup', titleKey: 'docs.smartRoutes.proxySetup.title', contentKey: 'docs.smartRoutes.proxySetup.content' },
    ],
  },
  {
    id: 'ztna',
    titleKey: 'docs.ztna.title',
    icon: Shield,
    subsections: [
      { id: 'trust-scores', titleKey: 'docs.ztna.trustScores.title', contentKey: 'docs.ztna.trustScores.content' },
      { id: 'trust-weights', titleKey: 'docs.ztna.trustWeights.title', contentKey: 'docs.ztna.trustWeights.content' },
      { id: 'conditional-access', titleKey: 'docs.ztna.conditionalAccess.title', contentKey: 'docs.ztna.conditionalAccess.content' },
      { id: 'split-dns', titleKey: 'docs.ztna.splitDns.title', contentKey: 'docs.ztna.splitDns.content' },
    ],
  },
  {
    id: 'compliance',
    titleKey: 'docs.compliance.title',
    icon: ShieldCheck,
    subsections: [
      { id: 'soc2', titleKey: 'docs.compliance.soc2.title', contentKey: 'docs.compliance.soc2.content' },
      { id: 'iso27001', titleKey: 'docs.compliance.iso27001.title', contentKey: 'docs.compliance.iso27001.content' },
      { id: 'gdpr', titleKey: 'docs.compliance.gdpr.title', contentKey: 'docs.compliance.gdpr.content' },
      { id: 'remediation', titleKey: 'docs.compliance.remediation.title', contentKey: 'docs.compliance.remediation.content' },
    ],
  },
  {
    id: 'analytics',
    titleKey: 'docs.analytics.title',
    icon: BarChart3,
    subsections: [
      { id: 'bandwidth', titleKey: 'docs.analytics.bandwidth.title', contentKey: 'docs.analytics.bandwidth.content' },
      { id: 'top-users', titleKey: 'docs.analytics.topUsers.title', contentKey: 'docs.analytics.topUsers.content' },
      { id: 'heatmap', titleKey: 'docs.analytics.heatmap.title', contentKey: 'docs.analytics.heatmap.content' },
      { id: 'prometheus', titleKey: 'docs.analytics.prometheus.title', contentKey: 'docs.analytics.prometheus.content' },
    ],
  },
  {
    id: 'settings',
    titleKey: 'docs.settings.title',
    icon: Settings,
    subsections: [
      { id: 'general-settings', titleKey: 'docs.settings.general.title', contentKey: 'docs.settings.general.content' },
      { id: 'wg-settings', titleKey: 'docs.settings.wireguard.title', contentKey: 'docs.settings.wireguard.content' },
      { id: 'smtp-settings', titleKey: 'docs.settings.smtp.title', contentKey: 'docs.settings.smtp.content' },
      { id: 'webhooks', titleKey: 'docs.settings.webhooks.title', contentKey: 'docs.settings.webhooks.content' },
      { id: 'security-settings', titleKey: 'docs.settings.security.title', contentKey: 'docs.settings.security.content' },
    ],
  },
  {
    id: 'deployment',
    titleKey: 'docs.deployment.title',
    icon: Container,
    subsections: [
      { id: 'docker-compose', titleKey: 'docs.deployment.dockerCompose.title', contentKey: 'docs.deployment.dockerCompose.content' },
      { id: 'systemd-units', titleKey: 'docs.deployment.systemdUnits.title', contentKey: 'docs.deployment.systemdUnits.content' },
      { id: 'helm', titleKey: 'docs.deployment.helm.title', contentKey: 'docs.deployment.helm.content' },
      { id: 'env-vars', titleKey: 'docs.deployment.envVars.title', contentKey: 'docs.deployment.envVars.content' },
    ],
  },
  {
    id: 'api-reference',
    titleKey: 'docs.apiReference.title',
    icon: Terminal,
    subsections: [
      { id: 'auth-jwt', titleKey: 'docs.apiReference.authJwt.title', contentKey: 'docs.apiReference.authJwt.content' },
      { id: 'openapi', titleKey: 'docs.apiReference.openapi.title', contentKey: 'docs.apiReference.openapi.content' },
      { id: 'curl-examples', titleKey: 'docs.apiReference.curlExamples.title', contentKey: 'docs.apiReference.curlExamples.content' },
    ],
  },
  {
    id: 'troubleshooting',
    titleKey: 'docs.troubleshooting.title',
    icon: Wrench,
    subsections: [
      { id: 'common-errors', titleKey: 'docs.troubleshooting.commonErrors.title', contentKey: 'docs.troubleshooting.commonErrors.content' },
      { id: 'wg-handshake', titleKey: 'docs.troubleshooting.wgHandshake.title', contentKey: 'docs.troubleshooting.wgHandshake.content' },
      { id: 'db-connection', titleKey: 'docs.troubleshooting.dbConnection.title', contentKey: 'docs.troubleshooting.dbConnection.content' },
      { id: 'gateway-connectivity', titleKey: 'docs.troubleshooting.gatewayConnectivity.title', contentKey: 'docs.troubleshooting.gatewayConnectivity.content' },
    ],
  },
  {
    id: 'vpn-client',
    icon: Laptop2,
    titleKey: 'docs.vpnClient.title',
    subsections: [
      { id: 'outpost-client', titleKey: 'docs.vpnClient.outpostClient.title', contentKey: 'docs.vpnClient.outpostClient.content' },
      { id: 'wireguard-native', titleKey: 'docs.vpnClient.wireguardNative.title', contentKey: 'docs.vpnClient.wireguardNative.content' },
      { id: 'mfa-2fa-clients', titleKey: 'docs.vpnClient.mfa2fa.title', contentKey: 'docs.vpnClient.mfa2fa.content' },
    ],
  },
]

/* ------------------------------------------------------------------ */
/*  Code block component                                               */
/* ------------------------------------------------------------------ */

function CodeBlock({ code, lang }: { code: string; lang?: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="group relative my-3 rounded-md border border-[var(--border)] bg-[#0d0d14] overflow-hidden">
      {lang && (
        <div className="flex items-center justify-between border-b border-[var(--border)] px-4 py-1.5 text-[10px] font-mono uppercase tracking-widest text-[var(--text-muted)]">
          <span>{lang}</span>
          <button
            onClick={handleCopy}
            className="flex items-center gap-1 text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors cursor-pointer"
          >
            {copied ? <Check size={12} /> : <Copy size={12} />}
            <span>{copied ? 'Copied' : 'Copy'}</span>
          </button>
        </div>
      )}
      <pre className="overflow-x-auto p-4 text-xs leading-relaxed font-mono text-[var(--text-secondary)]">
        <code>{code}</code>
      </pre>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Content renderer — parses content string into rich elements         */
/* ------------------------------------------------------------------ */

function DocContent({ content }: { content: string }) {
  const parts = content.split(/(\[code:[\w-]*\][\s\S]*?\[\/code\])/g)

  return (
    <div className="docs-content space-y-3 text-sm leading-relaxed text-[var(--text-secondary)]">
      {parts.map((part, i) => {
        const codeMatch = part.match(/\[code:([\w-]*)\]([\s\S]*?)\[\/code\]/)
        if (codeMatch) {
          return <CodeBlock key={i} lang={codeMatch[1] || undefined} code={codeMatch[2].trim()} />
        }
        // Split by paragraphs and render with inline formatting
        return part.split('\n\n').map((paragraph, pi) => {
          if (!paragraph.trim()) return null

          // Heading lines (### prefix)
          if (paragraph.trim().startsWith('### ')) {
            return (
              <h4 key={`${i}-${pi}`} className="text-sm font-semibold text-[var(--text-primary)] mt-4 mb-1">
                {paragraph.trim().replace(/^###\s*/, '')}
              </h4>
            )
          }

          // Bullet list
          if (paragraph.trim().match(/^[-*] /m)) {
            const items = paragraph.trim().split(/\n/).filter(Boolean)
            return (
              <ul key={`${i}-${pi}`} className="list-none space-y-1.5 pl-0">
                {items.map((item, ii) => (
                  <li key={ii} className="flex items-start gap-2">
                    <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--accent)] opacity-60" />
                    <span dangerouslySetInnerHTML={{ __html: formatInline(item.replace(/^[-*]\s*/, '')) }} />
                  </li>
                ))}
              </ul>
            )
          }

          // Regular paragraph
          return (
            <p key={`${i}-${pi}`} dangerouslySetInnerHTML={{ __html: formatInline(paragraph.trim()) }} />
          )
        })
      })}
    </div>
  )
}

/** Simple inline formatting: **bold**, `code`, [link](url) */
function formatInline(text: string): string {
  return text
    .replace(/\*\*(.*?)\*\*/g, '<strong class="text-[var(--text-primary)] font-medium">$1</strong>')
    .replace(/`(.*?)`/g, '<code class="rounded bg-[var(--bg-tertiary)] border border-[var(--border)] px-1.5 py-0.5 text-xs font-mono text-[var(--accent)]">$1</code>')
    .replace(
      /\[([^\]]+)\]\(([^)]+)\)/g,
      '<a href="$2" target="_blank" rel="noopener noreferrer" class="text-[var(--accent)] hover:underline">$1</a>',
    )
}

/* ------------------------------------------------------------------ */
/*  Main page                                                          */
/* ------------------------------------------------------------------ */

export default function DocsPage() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['getting-started']))
  const [activeId, setActiveId] = useState('quick-start')
  const contentRef = useRef<HTMLDivElement>(null)
  const sectionRefs = useRef<Record<string, HTMLDivElement | null>>({})

  // Filter sections based on search
  const filteredSections = useMemo(() => {
    if (!search.trim()) return docSections
    const q = search.toLowerCase()
    return docSections
      .map((section) => ({
        ...section,
        subsections: section.subsections.filter(
          (sub) =>
            t(sub.titleKey).toLowerCase().includes(q) ||
            t(sub.contentKey).toLowerCase().includes(q) ||
            t(section.titleKey).toLowerCase().includes(q),
        ),
      }))
      .filter((section) => section.subsections.length > 0)
  }, [search, t])

  // Expand all when searching
  useEffect(() => {
    if (search.trim()) {
      setExpandedSections(new Set(filteredSections.map((s) => s.id)))
    }
  }, [search, filteredSections])

  const toggleSection = (id: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const scrollToSubsection = (sectionId: string, subId: string) => {
    setActiveId(subId)
    if (!expandedSections.has(sectionId)) {
      setExpandedSections((prev) => new Set(prev).add(sectionId))
    }
    setTimeout(() => {
      sectionRefs.current[subId]?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 50)
  }

  return (
    <div className="flex h-full gap-0 -m-6 overflow-hidden">
      {/* ---- Sidebar TOC ---- */}
      <aside className="flex w-72 shrink-0 flex-col border-r border-[var(--border)] bg-[var(--bg-secondary)]">
        {/* Header */}
        <div className="flex items-center gap-2 border-b border-[var(--border)] px-4 py-3">
          <BookOpen size={18} className="text-[var(--accent)]" />
          <h2 className="text-sm font-bold font-mono text-[var(--text-primary)]">
            {t('docs.title')}
          </h2>
        </div>

        {/* Search */}
        <div className="border-b border-[var(--border)] px-3 py-2">
          <div className="relative">
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t('docs.search')}
              className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-primary)] py-1.5 pl-8 pr-3 text-xs font-mono text-[var(--text-primary)] placeholder:text-[var(--text-muted)] glow-focus transition-all duration-150"
            />
          </div>
        </div>

        {/* Navigation tree */}
        <nav className="flex-1 overflow-y-auto py-2 px-2 custom-scrollbar">
          {filteredSections.map((section) => {
            const Icon = section.icon
            const isExpanded = expandedSections.has(section.id)
            return (
              <div key={section.id} className="mb-0.5">
                <button
                  onClick={() => toggleSection(section.id)}
                  className={clsx(
                    'flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-xs font-medium transition-all duration-150 cursor-pointer',
                    isExpanded
                      ? 'text-[var(--accent)] bg-[rgba(0,255,136,0.05)]'
                      : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]',
                  )}
                >
                  {isExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                  <Icon size={13} />
                  <span className="truncate">{t(section.titleKey)}</span>
                </button>
                {isExpanded && (
                  <div className="ml-5 mt-0.5 space-y-px border-l border-[var(--border)] pl-3">
                    {section.subsections.map((sub) => (
                      <button
                        key={sub.id}
                        onClick={() => scrollToSubsection(section.id, sub.id)}
                        className={clsx(
                          'block w-full truncate rounded px-2 py-1 text-left text-[11px] transition-all duration-150 cursor-pointer',
                          activeId === sub.id
                            ? 'text-[var(--accent)] bg-[rgba(0,255,136,0.08)]'
                            : 'text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]',
                        )}
                      >
                        {t(sub.titleKey)}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )
          })}
          {filteredSections.length === 0 && (
            <div className="px-4 py-8 text-center text-xs text-[var(--text-muted)]">
              {t('docs.noResults')}
            </div>
          )}
        </nav>
      </aside>

      {/* ---- Main content ---- */}
      <div ref={contentRef} className="flex-1 overflow-y-auto p-8 custom-scrollbar">
        {/* Hero */}
        <div className="mb-8">
          <h1
            className="text-2xl font-bold font-mono tracking-wider mb-2"
            style={{ color: 'var(--accent)', textShadow: '0 0 30px var(--accent-glow)' }}
          >
            {t('docs.heroTitle')}
          </h1>
          <p className="text-sm text-[var(--text-secondary)] max-w-2xl leading-relaxed">
            {t('docs.heroDescription')}
          </p>
        </div>

        {/* Sections */}
        {filteredSections.map((section) => {
          const Icon = section.icon
          return (
            <div key={section.id} className="mb-10">
              {/* Section heading */}
              <div className="flex items-center gap-2.5 mb-4 pb-2 border-b border-[var(--border)]">
                <div className="flex h-7 w-7 items-center justify-center rounded-md bg-[rgba(0,255,136,0.1)] text-[var(--accent)]">
                  <Icon size={15} />
                </div>
                <h2 className="text-base font-bold font-mono text-[var(--text-primary)]">
                  {t(section.titleKey)}
                </h2>
              </div>

              {/* Subsection cards */}
              <div className="space-y-4">
                {section.subsections.map((sub) => (
                  <Card
                    key={sub.id}
                    className="scroll-mt-6"
                  >
                    <div
                      ref={(el) => { sectionRefs.current[sub.id] = el }}
                      className="scroll-mt-6"
                    >
                      <h3 className="text-sm font-bold font-mono text-[var(--text-primary)] mb-3 flex items-center gap-2">
                        <span
                          className="inline-block h-1 w-1 rounded-full bg-[var(--accent)]"
                          style={{ boxShadow: '0 0 6px var(--accent)' }}
                        />
                        {t(sub.titleKey)}
                      </h3>
                      <DocContent content={t(sub.contentKey)} />
                    </div>
                  </Card>
                ))}
              </div>
            </div>
          )
        })}

        {/* Footer */}
        <div className="mt-12 mb-4 border-t border-[var(--border)] pt-6 text-center">
          <p className="text-xs text-[var(--text-muted)] font-mono">
            {t('docs.footer')}
          </p>
          <a
            href="https://github.com/romashqua/outpost"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 mt-2 text-xs text-[var(--accent)] hover:underline font-mono"
          >
            GitHub <ExternalLink size={11} />
          </a>
        </div>
      </div>
    </div>
  )
}
