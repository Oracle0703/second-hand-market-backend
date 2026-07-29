import { BlockList, isIP } from 'node:net'

const ProductionAPIBaseURL = 'https://market.meaningful.ink/api/v1'
const DevelopmentAPIBaseURL = 'http://127.0.0.1:8080/api/v1'

type IPFamily = 'ipv4' | 'ipv6'
type Subnet = readonly [network: string, prefix: number, family: IPFamily]

function createBlockList(subnets: readonly Subnet[]): BlockList {
  const blockList = new BlockList()
  for (const [network, prefix, family] of subnets) {
    blockList.addSubnet(network, prefix, family)
  }
  return blockList
}

const ControlledDevelopmentAddresses = createBlockList([
  ['10.0.0.0', 8, 'ipv4'],
  ['127.0.0.0', 8, 'ipv4'],
  ['172.16.0.0', 12, 'ipv4'],
  ['192.168.0.0', 16, 'ipv4'],
  ['::1', 128, 'ipv6'],
  ['fc00::', 7, 'ipv6']
])

const NonPublicIPv4Addresses = createBlockList([
  ['0.0.0.0', 8, 'ipv4'],
  ['10.0.0.0', 8, 'ipv4'],
  ['100.64.0.0', 10, 'ipv4'],
  ['127.0.0.0', 8, 'ipv4'],
  ['169.254.0.0', 16, 'ipv4'],
  ['172.16.0.0', 12, 'ipv4'],
  ['192.0.0.0', 24, 'ipv4'],
  ['192.0.2.0', 24, 'ipv4'],
  ['192.88.99.0', 24, 'ipv4'],
  ['192.168.0.0', 16, 'ipv4'],
  ['198.18.0.0', 15, 'ipv4'],
  ['198.51.100.0', 24, 'ipv4'],
  ['203.0.113.0', 24, 'ipv4'],
  ['224.0.0.0', 4, 'ipv4'],
  ['240.0.0.0', 4, 'ipv4']
])

const NonPublicIPv6Addresses = createBlockList([
  ['::', 96, 'ipv6'],
  ['::ffff:0:0', 96, 'ipv6'],
  ['::ffff:0:0:0', 96, 'ipv6'],
  ['64:ff9b::', 96, 'ipv6'],
  ['64:ff9b:1::', 48, 'ipv6'],
  ['100::', 64, 'ipv6'],
  ['2001::', 32, 'ipv6'],
  ['2001:2::', 48, 'ipv6'],
  ['2001:10::', 28, 'ipv6'],
  ['2001:20::', 28, 'ipv6'],
  ['2001:db8::', 32, 'ipv6'],
  ['2002::', 16, 'ipv6'],
  ['fc00::', 7, 'ipv6'],
  ['fe80::', 10, 'ipv6'],
  ['fec0::', 10, 'ipv6'],
  ['ff00::', 8, 'ipv6']
])

function rejectAPIBaseURL(reason: string): never {
  throw new Error(`TARO_APP_API_BASE_URL ${reason}`)
}

function normalizeAddressHostname(hostname: string): string {
  return hostname
    .replace(/^\[/, '')
    .replace(/\]$/, '')
    .toLowerCase()
}

function normalizeDevelopmentHostname(hostname: string): string {
  return normalizeAddressHostname(hostname).replace(/\.+$/, '')
}

function normalizeProductionHostname(hostname: string): string {
  const normalized = normalizeAddressHostname(hostname)
  if (ipFamily(normalized)) {
    return normalized
  }

  const dnsHostname = stripOptionalRootDot(normalized)
  if (!dnsHostname || !isValidDNSHostname(dnsHostname)) {
    return rejectAPIBaseURL('must use a valid public DNS hostname in production')
  }

  return dnsHostname
}

function stripOptionalRootDot(hostname: string): string | undefined {
  if (hostname.endsWith('..')) {
    return undefined
  }
  return hostname.endsWith('.') ? hostname.slice(0, -1) : hostname
}

function isValidDNSHostname(hostname: string): boolean {
  if (hostname.length < 1 || hostname.length > 253) {
    return false
  }

  const labels = hostname.split('.')
  const labelPattern = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/
  return labels.every((label) => labelPattern.test(label))
}

function ipFamily(hostname: string): IPFamily | undefined {
  const version = isIP(hostname)
  return version === 4 ? 'ipv4' : version === 6 ? 'ipv6' : undefined
}

function isControlledDevelopmentHost(hostname: string): boolean {
  if (hostname === 'localhost' || hostname.endsWith('.localhost')) {
    return true
  }

  const family = ipFamily(hostname)
  return Boolean(family && ControlledDevelopmentAddresses.check(hostname, family))
}

function isNonPublicHost(hostname: string): boolean {
  const family = ipFamily(hostname)
  if (family === 'ipv4') {
    return NonPublicIPv4Addresses.check(hostname, family)
  }
  if (family === 'ipv6') {
    return NonPublicIPv6Addresses.check(hostname, family)
  }

  return (
    hostname === 'local' ||
    hostname.endsWith('.local') ||
    hostname === 'localhost' ||
    hostname.endsWith('.localhost') ||
    hostname === 'home.arpa' ||
    hostname.endsWith('.home.arpa') ||
    !hostname.includes('.')
  )
}

function validateAPIBaseURL(value: string, isProduction: boolean): string {
  if (/[\u0000-\u001f\u007f\\]/.test(value)) {
    return rejectAPIBaseURL('must not contain control characters or backslashes')
  }

  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    return rejectAPIBaseURL('must be an absolute HTTP(S) URL')
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return rejectAPIBaseURL('must use HTTP or HTTPS')
  }
  if (parsed.username || parsed.password) {
    return rejectAPIBaseURL('must not contain credentials')
  }
  if (parsed.search || parsed.hash) {
    return rejectAPIBaseURL('must not contain a query string or fragment')
  }
  if (parsed.port === '0') {
    return rejectAPIBaseURL('must not use port 0')
  }

  const hostname = isProduction
    ? normalizeProductionHostname(parsed.hostname)
    : normalizeDevelopmentHostname(parsed.hostname)
  if (!hostname) {
    return rejectAPIBaseURL('must include a hostname')
  }

  if (isProduction) {
    if (parsed.protocol !== 'https:') {
      return rejectAPIBaseURL('must use HTTPS in production')
    }
    if (isNonPublicHost(hostname)) {
      return rejectAPIBaseURL('must use a public hostname or address in production')
    }
  } else {
    const isControlledLocalHost = isControlledDevelopmentHost(hostname)
    if (parsed.protocol === 'http:' && !isControlledLocalHost) {
      return rejectAPIBaseURL('may use HTTP only with a loopback or private LAN address in development')
    }
    if (parsed.protocol === 'https:' && isNonPublicHost(hostname) && !isControlledLocalHost) {
      return rejectAPIBaseURL('must not use a non-public address outside the controlled development ranges')
    }
  }

  return value
}

export function resolveAPIBaseURL(options: {
  taroEnv: string
  nodeEnv: string
  envBaseURL?: string
}): string {
  const nodeEnv = options.nodeEnv.trim().toLowerCase()
  if (nodeEnv !== 'development' && nodeEnv !== 'production') {
    throw new Error('NODE_ENV must be either development or production for miniapp builds')
  }

  const isProduction = nodeEnv === 'production'
  const override = options.envBaseURL?.trim()
  const selectedURL = override || (isProduction ? ProductionAPIBaseURL : DevelopmentAPIBaseURL)

  return validateAPIBaseURL(selectedURL, isProduction)
}
