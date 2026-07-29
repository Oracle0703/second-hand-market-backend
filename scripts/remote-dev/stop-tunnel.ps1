[CmdletBinding()]
param([string]$ConfigPath)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot 'RemoteDev.psm1') -Force
$resolvedConfigPath = Resolve-RemoteDevConfigPath -Path $ConfigPath
Import-RemoteDevEnvironment -Path $resolvedConfigPath

$sshHost = Get-RequiredRemoteDevEnvironmentValue -Name 'SSH_HOST'
$localPort = ConvertTo-RemoteDevPort -Name 'SSH_LOCAL_PORT'
$remoteHost = Get-RequiredRemoteDevEnvironmentValue -Name 'SSH_REMOTE_HOST'
$remotePort = ConvertTo-RemoteDevPort -Name 'SSH_REMOTE_PORT'
$forward = '127.0.0.1:{0}:{1}:{2}' -f $localPort, $remoteHost, $remotePort

$pidPath = Join-Path (Get-RemoteDevRuntimeDirectory) 'ssh-tunnel.pid'
if (-not (Test-Path -LiteralPath $pidPath -PathType Leaf)) {
    throw 'SSH tunnel PID file was not found'
}
$pidText = (Get-Content -LiteralPath $pidPath -Raw).Trim()
$tunnelProcessID = 0
if ($pidText -notmatch '^[0-9]+$' -or -not [int]::TryParse($pidText, [ref]$tunnelProcessID) -or $tunnelProcessID -le 0) {
    throw 'SSH tunnel PID file is invalid'
}

$process = Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $tunnelProcessID" -ErrorAction Stop
if ($null -eq $process) {
    throw 'SSH tunnel process was not found'
}
$executableName = [System.IO.Path]::GetFileName([string]$process.ExecutablePath)
if ($process.Name -ine 'ssh.exe' -or $executableName -ine 'ssh.exe') {
    throw 'PID does not identify ssh.exe'
}

$commandLine = [string]$process.CommandLine
$requiredTokens = @(
    '-T',
    '-N',
    'ExitOnForwardFailure=yes',
    'ServerAliveInterval=30',
    'ServerAliveCountMax=3',
    $sshHost
)
foreach ($token in $requiredTokens) {
    $tokenPattern = '(?i)(?:^|\s)"?' + [regex]::Escape($token) + '"?(?=\s|$)'
    if ($commandLine -notmatch $tokenPattern) {
        throw 'ssh.exe command line did not match the managed tunnel'
    }
}
$forwardPattern = '(?i)(?:^|\s)-L\s+"?' + [regex]::Escape($forward) + '"?(?=\s|$)'
if ($commandLine -notmatch $forwardPattern) {
    throw 'ssh.exe forwarding argument did not match the managed tunnel'
}

Stop-Process -Id $tunnelProcessID -Force -ErrorAction Stop
Wait-Process -Id $tunnelProcessID -Timeout 5 -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $pidPath -Force
Write-Output 'SSH_TUNNEL STOPPED'
