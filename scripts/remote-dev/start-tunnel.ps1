[CmdletBinding()]
param([string]$ConfigPath)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot 'RemoteDev.psm1') -Force
$resolvedConfigPath = Resolve-RemoteDevConfigPath -Path $ConfigPath
Import-RemoteDevEnvironment -Path $resolvedConfigPath

$sshHost = Get-RequiredRemoteDevEnvironmentValue -Name 'SSH_HOST'
if ($sshHost -notmatch '^[A-Za-z0-9._@-]+$') {
    throw 'SSH_HOST has an invalid format'
}
$localPort = ConvertTo-RemoteDevPort -Name 'SSH_LOCAL_PORT'
$remoteHost = Get-RequiredRemoteDevEnvironmentValue -Name 'SSH_REMOTE_HOST'
if ($remoteHost -ne '127.0.0.1') {
    throw 'SSH_REMOTE_HOST must be 127.0.0.1'
}
$remotePort = ConvertTo-RemoteDevPort -Name 'SSH_REMOTE_PORT'

if (-not (Test-RemoteDevLocalPortAvailable -Port $localPort)) {
    throw 'SSH local port is already in use'
}

$runtimeDirectory = Get-RemoteDevRuntimeDirectory -Ensure
$pidPath = Join-Path $runtimeDirectory 'ssh-tunnel.pid'
if (Test-Path -LiteralPath $pidPath) {
    throw 'SSH tunnel PID file already exists; run stop-tunnel.ps1 first'
}

$sshCommand = Get-Command 'ssh.exe' -CommandType Application -ErrorAction Stop
$forward = '127.0.0.1:{0}:{1}:{2}' -f $localPort, $remoteHost, $remotePort
$sshArguments = @(
    '-T',
    '-N',
    '-o', 'ExitOnForwardFailure=yes',
    '-o', 'ServerAliveInterval=30',
    '-o', 'ServerAliveCountMax=3',
    '-L', $forward,
    $sshHost
)

# The SSH child does not need database identity or credential variables.
foreach ($name in @('DB_DSN', 'DB_EXPECTED_DATABASE', 'DB_EXPECTED_SERVER_UUID', 'DB_EXPECTED_USER')) {
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
}

$process = $null
try {
    $process = Start-Process -FilePath $sshCommand.Source `
        -ArgumentList $sshArguments `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput (Join-Path $runtimeDirectory 'ssh-tunnel.stdout.log') `
        -RedirectStandardError (Join-Path $runtimeDirectory 'ssh-tunnel.stderr.log')

    Set-Content -LiteralPath $pidPath -Value ([string]$process.Id) -Encoding ASCII -NoNewline
    if (-not (Wait-RemoteDevLocalPort -Port $localPort -TimeoutSeconds 10)) {
        throw 'SSH local forwarding port did not become available'
    }
    $process.Refresh()
    if ($process.HasExited) {
        throw 'SSH tunnel process exited during startup'
    }
}
catch {
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item -LiteralPath $pidPath -Force -ErrorAction SilentlyContinue
    throw 'SSH tunnel failed to start'
}

Write-Output 'SSH_TUNNEL PASS'
