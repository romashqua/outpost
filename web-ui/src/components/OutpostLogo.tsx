interface OutpostLogoProps {
  size?: number
  className?: string
}

export default function OutpostLogo({ size = 32, className }: OutpostLogoProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 512 512"
      width={size}
      height={size}
      className={className}
    >
      <defs>
        <linearGradient id="outpost-glow" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor="#00ff88" stopOpacity={0.8} />
          <stop offset="100%" stopColor="#00cc66" stopOpacity={1} />
        </linearGradient>
        <filter id="outpost-shadow">
          <feDropShadow dx="0" dy="0" stdDeviation="8" floodColor="#00ff88" floodOpacity={0.4} />
        </filter>
      </defs>
      {/* Shield outline */}
      <path
        d="M256 56 L416 136 L416 280 C416 368 344 440 256 464 C168 440 96 368 96 280 L96 136 Z"
        fill="none"
        stroke="url(#outpost-glow)"
        strokeWidth="6"
        filter="url(#outpost-shadow)"
      />
      {/* Inner shield fill */}
      <path
        d="M256 76 L400 148 L400 276 C400 356 334 422 256 444 C178 422 112 356 112 276 L112 148 Z"
        fill="#0a0a0f"
        fillOpacity={0.9}
      />
      {/* WireGuard tunnel lines */}
      <line x1="176" y1="200" x2="336" y2="200" stroke="#00ff88" strokeWidth="3" opacity={0.3} />
      <line x1="176" y1="240" x2="336" y2="240" stroke="#00ff88" strokeWidth="3" opacity={0.5} />
      <line x1="176" y1="280" x2="336" y2="280" stroke="#00ff88" strokeWidth="3" opacity={0.3} />
      {/* Lock circle */}
      <circle cx="256" cy="228" r="28" fill="none" stroke="#00ff88" strokeWidth="5" />
      {/* Lock body */}
      <rect x="236" y="248" width="40" height="44" rx="4" fill="#00ff88" opacity={0.9} />
      {/* Keyhole */}
      <circle cx="256" cy="264" r="6" fill="#0a0a0f" />
      <rect x="253" y="264" width="6" height="14" rx="2" fill="#0a0a0f" />
    </svg>
  )
}
